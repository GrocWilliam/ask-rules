// infrastructure/service/onnx_embedder_adapter.go — Embedder ONNX Runtime intégré
//
// Utilise intfloat/multilingual-e5-small en format ONNX (XLM-RoBERTa, Unigram SentencePiece).
// Le modèle doit être téléchargé dans le répertoire models/ : scripts/download-model.sh
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/infrastructure/config"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	embeddingDims = 384
	maxTokens     = 256 // Réduit de 512 pour économiser RAM (~50% sur tensors)
)

// ortEnvOnce garantit que InitializeEnvironment n'est appelé qu'une seule fois
// dans tout le processus (contrainte globale libbonnxruntime).
var (
	ortEnvOnce sync.Once
	ortEnvErr  error
)

func initOrtEnv(libPath string) error {
	ortEnvOnce.Do(func() {
		ort.SetSharedLibraryPath(libPath)
		ortEnvErr = ort.InitializeEnvironment()
	})
	return ortEnvErr
}

// ONNXEmbedderAdapter implémente EmbedderService avec ONNX Runtime.
type ONNXEmbedderAdapter struct {
	initMu  sync.Mutex // protège initialized + session/tensors
	ortMu   sync.Mutex // protège les appels Run()
	timerMu sync.Mutex // protège idleTimer uniquement

	// vocab : token → ID (position dans le tableau Unigram)
	vocab map[string]int32
	// vocabScores : token → log-probabilité (pour Viterbi Unigram)
	vocabScores map[string]float64
	unkID       int32
	clsID       int32
	sepID       int32
	vocabLoaded bool // le tokenizer est chargé une seule fois (léger)

	session *ort.AdvancedSession

	// Tensors pré-alloués (réutilisés à chaque inférence)
	inIDs     *ort.Tensor[int64]
	inMask    *ort.Tensor[int64]
	inTypes   *ort.Tensor[int64]
	outTensor *ort.Tensor[float32]

	initialized bool
	persistErr  error // erreur d'init conservée pour les appels suivants

	// Timer d'inactivité : libère la session/tensors après idleTimeout sans appel Embed.
	idleTimer   *time.Timer
	idleTimeout time.Duration
}

// NewONNXEmbedder crée un nouvel adapter pour l'embedder ONNX.
// Le modèle est libéré automatiquement après 1 minute d'inactivité.
func NewONNXEmbedder() service.EmbedderService {
	return &ONNXEmbedderAdapter{
		unkID:       3, // <unk> dans multilingual-e5-small
		clsID:       0, // <s>
		sepID:       2, // </s>
		idleTimeout: time.Minute * 5,
	}
}

// resetIdleTimer (re)démarre le timer d'inactivité.
// Doit être appelé AVANT d'acquérir ortMu ou initMu pour éviter
// toute inversion de verrous avec Release().
func (e *ONNXEmbedderAdapter) resetIdleTimer() {
	e.timerMu.Lock()
	defer e.timerMu.Unlock()
	if e.idleTimer != nil {
		e.idleTimer.Stop()
	}
	e.idleTimer = time.AfterFunc(e.idleTimeout, func() {
		log.Printf("[INFO] ONNXEmbedder - Inactivité %s, libération de la RAM", e.idleTimeout)
		e.Release()
	})
}

// Init charge la session ONNX et les tensors.
// Le tokenizer est chargé une seule fois ; la session peut être recréée après Release().
func (e *ONNXEmbedderAdapter) Init() error {
	e.initMu.Lock()
	defer e.initMu.Unlock()

	if e.initialized {
		return nil
	}

	modelDir := config.C.ModelPath

	// Charger le tokenizer seulement si nécessaire (survivrait à une Release)
	if !e.vocabLoaded {
		tokPath := filepath.Join(modelDir, "tokenizer.json")
		var loadErr error
		e.vocab, e.vocabScores, e.clsID, e.sepID, e.unkID, loadErr = e.loadTokenizer(tokPath)
		if loadErr != nil {
			e.persistErr = fmt.Errorf("chargement tokenizer depuis %s : %w", tokPath, loadErr)
			return e.persistErr
		}
		e.vocabLoaded = true
	}

	// Initialisation ORT globale (no-op si déjà faite)
	if err := initOrtEnv(e.findOrtLib()); err != nil {
		e.persistErr = fmt.Errorf("init ORT : %w", err)
		return e.persistErr
	}

	// Cherche le fichier modèle (quantized en priorité, puis onnx/ sous-répertoire)
	modelCandidates := []string{
		filepath.Join(modelDir, "model_quantized.onnx"),
		filepath.Join(modelDir, "onnx", "model_quantized.onnx"),
		filepath.Join(modelDir, "model.onnx"),
		filepath.Join(modelDir, "onnx", "model.onnx"),
	}
	modelPath := ""
	for _, c := range modelCandidates {
		if _, err := os.Stat(c); err == nil {
			modelPath = c
			break
		}
	}
	if modelPath == "" {
		e.persistErr = fmt.Errorf("fichier modèle ONNX introuvable dans %s", modelDir)
		return e.persistErr
	}

	// Création de la session ONNX
	options, err := ort.NewSessionOptions()
	if err != nil {
		e.persistErr = fmt.Errorf("options ORT : %w", err)
		return e.persistErr
	}
	defer options.Destroy()

	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	// Tensors pré-alloués — réutilisés à chaque Run()
	e.inIDs, err = ort.NewEmptyTensor[int64](ort.NewShape(1, maxTokens))
	if err != nil {
		e.persistErr = fmt.Errorf("tensor input_ids : %w", err)
		return e.persistErr
	}
	e.inMask, err = ort.NewEmptyTensor[int64](ort.NewShape(1, maxTokens))
	if err != nil {
		e.persistErr = fmt.Errorf("tensor attention_mask : %w", err)
		return e.persistErr
	}
	e.inTypes, err = ort.NewEmptyTensor[int64](ort.NewShape(1, maxTokens))
	if err != nil {
		e.persistErr = fmt.Errorf("tensor token_type_ids : %w", err)
		return e.persistErr
	}
	e.outTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, maxTokens, embeddingDims))
	if err != nil {
		e.persistErr = fmt.Errorf("tensor output : %w", err)
		return e.persistErr
	}

	e.session, err = ort.NewAdvancedSession(modelPath,
		inputNames, outputNames,
		[]ort.Value{e.inIDs, e.inMask, e.inTypes},
		[]ort.Value{e.outTensor},
		options)
	if err != nil {
		// XLM-RoBERTa n'a pas de token_type_ids : réessayer sans
		inputNames = []string{"input_ids", "attention_mask"}
		e.inTypes.Destroy()
		e.inTypes = nil
		e.session, err = ort.NewAdvancedSession(modelPath,
			inputNames, outputNames,
			[]ort.Value{e.inIDs, e.inMask},
			[]ort.Value{e.outTensor},
			options)
		if err != nil {
			e.persistErr = fmt.Errorf("création session ORT : %w", err)
			return e.persistErr
		}
	}

	e.persistErr = nil
	e.initialized = true
	return nil
}

// Release libère la session ONNX et les tensors de la RAM.
// L'environnement ORT global et le tokenizer restent en mémoire (légers).
// Le prochain appel à Embed() rechargera automatiquement le modèle.
func (e *ONNXEmbedderAdapter) Release() {
	// Stopper le timer d'inactivité en premier (avant initMu) pour
	// éviter toute inversion de verrous avec resetIdleTimer().
	e.timerMu.Lock()
	if e.idleTimer != nil {
		e.idleTimer.Stop()
		e.idleTimer = nil
	}
	e.timerMu.Unlock()

	e.initMu.Lock()
	defer e.initMu.Unlock()

	if !e.initialized {
		return
	}

	e.ortMu.Lock()
	defer e.ortMu.Unlock()

	if e.session != nil {
		_ = e.session.Destroy()
		e.session = nil
	}
	if e.inIDs != nil {
		e.inIDs.Destroy()
		e.inIDs = nil
	}
	if e.inMask != nil {
		e.inMask.Destroy()
		e.inMask = nil
	}
	if e.inTypes != nil {
		e.inTypes.Destroy()
		e.inTypes = nil
	}
	if e.outTensor != nil {
		e.outTensor.Destroy()
		e.outTensor = nil
	}

	e.initialized = false
	log.Printf("[INFO] ONNXEmbedder - Modèle libéré de la RAM")
	runtime.GC()
}

// Embed génère un vecteur de dimension 384 pour le texte donné.
func (e *ONNXEmbedderAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	// Réinitialiser le timer d'inactivité AVANT d'acquérir tout autre verrou.
	e.resetIdleTimer()

	if !e.initialized {
		if err := e.Init(); err != nil {
			return nil, err
		}
	}

	inputIDs, attentionMask, tokenTypeIDs := e.tokenize(text)

	e.ortMu.Lock()
	defer e.ortMu.Unlock()

	copy(e.inIDs.GetData(), inputIDs)
	copy(e.inMask.GetData(), attentionMask)
	if e.inTypes != nil {
		copy(e.inTypes.GetData(), tokenTypeIDs)
	}

	if err := e.session.Run(); err != nil {
		return nil, err
	}

	return e.meanPoolAndNormalize(e.outTensor.GetData(), attentionMask), nil
}

// Dimensions retourne la dimension des vecteurs produits (384 pour E5-small).
func (e *ONNXEmbedderAdapter) Dimensions() int {
	return embeddingDims
}

// ── Tokenisation Unigram SentencePiece (Metaspace / ▁) ─────────────────────

// tokenize tokenise `text` avec le tokenizer Unigram SentencePiece.
// Le modèle E5 attend un préfixe "query: " pour les requêtes.
func (e *ONNXEmbedderAdapter) tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64) {
	inputIDs = make([]int64, maxTokens)
	attentionMask = make([]int64, maxTokens)
	tokenTypeIDs = make([]int64, maxTokens)

	// Préfixe E5 pour les requêtes
	prefixed := "query: " + text

	tokens := e.unigramEncode(prefixed)
	// CLS + tokens + SEP
	all := make([]int64, 0, len(tokens)+2)
	all = append(all, int64(e.clsID))
	for _, t := range tokens {
		all = append(all, int64(t))
	}

	// Troncature
	if len(all) >= maxTokens-1 {
		all = all[:maxTokens-1]
	}
	all = append(all, int64(e.sepID))

	for i, t := range all {
		inputIDs[i] = t
		attentionMask[i] = 1
	}
	return
}

// unigramEncode encode une chaîne via Metaspace + Viterbi Unigram.
func (e *ONNXEmbedderAdapter) unigramEncode(text string) []int32 {
	// Metaspace : remplacer espaces par ▁ (U+2581), préfixer avec ▁
	normalized := "▁" + strings.ReplaceAll(text, " ", "▁")
	return e.unigramViterbi(normalized)
}

// unigramViterbi décode le texte en tokens via l'algorithme de Viterbi.
// Cherche la séquence de tokens maximisant la somme des log-probabilités.
func (e *ONNXEmbedderAdapter) unigramViterbi(text string) []int32 {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return nil
	}

	const negInf = -1e38

	dp := make([]float64, n+1)
	type back struct {
		start int
		id    int32
	}
	from := make([]back, n+1)
	for i := range dp {
		dp[i] = negInf
	}
	dp[0] = 0.0

	for i := 0; i < n; i++ {
		if dp[i] == negInf {
			continue
		}
		for j := i + 1; j <= n; j++ {
			sub := string(runes[i:j])
			if score, ok := e.vocabScores[sub]; ok {
				candidate := dp[i] + score
				if candidate > dp[j] {
					dp[j] = candidate
					from[j] = back{i, e.vocab[sub]}
				}
			}
		}
		// Caractère inconnu : avancer d'un rune avec unkID (pénalité)
		if dp[i+1] == negInf {
			dp[i+1] = dp[i] - 100.0
			from[i+1] = back{i, e.unkID}
		}
	}

	// Remontée (backtrace)
	var ids []int32
	pos := n
	for pos > 0 {
		f := from[pos]
		ids = append(ids, f.id)
		pos = f.start
	}
	// Inverser
	for l, r := 0, len(ids)-1; l < r; l, r = l+1, r-1 {
		ids[l], ids[r] = ids[r], ids[l]
	}
	return ids
}

// ── Mean pooling + normalisation L2 ─────────────────────────────────────────

func (e *ONNXEmbedderAdapter) meanPoolAndNormalize(hidden []float32, mask []int64) []float32 {
	result := make([]float32, embeddingDims)
	count := 0
	for t := 0; t < maxTokens; t++ {
		if mask[t] == 0 {
			continue
		}
		count++
		for d := 0; d < embeddingDims; d++ {
			result[d] += hidden[t*embeddingDims+d]
		}
	}
	if count == 0 {
		return result
	}
	var norm float64
	for d := 0; d < embeddingDims; d++ {
		result[d] /= float32(count)
		norm += float64(result[d]) * float64(result[d])
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for d := 0; d < embeddingDims; d++ {
			result[d] = float32(float64(result[d]) / norm)
		}
	}
	return result
}

// ── Chargement du tokenizer.json (Unigram SentencePiece HuggingFace) ─────────

// loadTokenizer charge vocab Unigram + scores depuis tokenizer.json.
// Format : model.vocab = [[token, score], ...] où position = ID.
func (e *ONNXEmbedderAdapter) loadTokenizer(path string) (v map[string]int32, scores map[string]float64, cls, sep, unk int32, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, 2, 3, err
	}

	var tj struct {
		Model struct {
			Type  string            `json:"type"`
			UnkID int32             `json:"unk_id"`
			Vocab []json.RawMessage `json:"vocab"` // [[token, score], ...]
		} `json:"model"`
		AddedTokens []struct {
			ID      int32  `json:"id"`
			Content string `json:"content"`
		} `json:"added_tokens"`
	}
	if err = json.Unmarshal(data, &tj); err != nil {
		return nil, nil, 0, 2, 3, fmt.Errorf("parse tokenizer.json : %w", err)
	}
	if len(tj.Model.Vocab) == 0 {
		return nil, nil, 0, 2, 3, fmt.Errorf("vocab vide dans tokenizer.json (type=%s)", tj.Model.Type)
	}

	v = make(map[string]int32, len(tj.Model.Vocab))
	scores = make(map[string]float64, len(tj.Model.Vocab))
	for i, raw := range tj.Model.Vocab {
		var pair [2]json.RawMessage
		if jsonErr := json.Unmarshal(raw, &pair); jsonErr != nil {
			continue
		}
		var token string
		var score float64
		if jsonErr := json.Unmarshal(pair[0], &token); jsonErr != nil {
			continue
		}
		if jsonErr := json.Unmarshal(pair[1], &score); jsonErr != nil {
			continue
		}
		v[token] = int32(i)
		scores[token] = score
	}

	// Tokens spéciaux depuis le vocab (defaults)
	cls = int32(v["<s>"])
	sep = int32(v["</s>"])
	unk = tj.Model.UnkID

	// Priorité aux added_tokens si présents
	for _, at := range tj.AddedTokens {
		switch at.Content {
		case "<s>":
			cls = at.ID
		case "</s>":
			sep = at.ID
		}
	}

	return v, scores, cls, sep, unk, nil
}

// ── Détection de la bibliothèque ONNX Runtime ─────────────────────────────

func (e *ONNXEmbedderAdapter) findOrtLib() string {
	candidates := []string{
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
		"/usr/lib/aarch64-linux-gnu/libonnxruntime.so",
		"./libonnxruntime.so",
		"libonnxruntime.so",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "libonnxruntime.so"
}
