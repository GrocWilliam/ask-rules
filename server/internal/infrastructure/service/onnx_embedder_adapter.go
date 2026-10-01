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
	"unicode/utf8"

	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/infrastructure/config"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	embeddingDims = 384
	// maxTokens : limite du modèle E5. Les tensors sont dimensionnés à la longueur
	// réelle du texte, une limite haute ne coûte donc rien pour les textes courts.
	maxTokens = 512
	// maxPieceRunes : longueur max (en runes) d'un token cherché par Viterbi.
	// Les tokens plus longs du vocabulaire sont ignorés (très rares).
	maxPieceRunes = 24

	// Préfixes attendus par E5 (modèle asymétrique)
	queryPrefix   = "query: "
	passagePrefix = "passage: "
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

// vocabEntry : ID + log-probabilité d'un token Unigram.
type vocabEntry struct {
	id    int32
	score float32
}

// ONNXEmbedderAdapter implémente EmbedderService avec ONNX Runtime.
type ONNXEmbedderAdapter struct {
	initMu  sync.Mutex // protège initialized + session
	ortMu   sync.Mutex // protège les appels Run()
	timerMu sync.Mutex // protège idleTimer uniquement

	// vocab : token → ID + score (pour Viterbi Unigram)
	vocab       map[string]vocabEntry
	unkID       int32
	clsID       int32
	sepID       int32
	vocabLoaded bool // le tokenizer est chargé une seule fois (léger)

	session       *ort.DynamicAdvancedSession
	hasTokenTypes bool // le modèle attend-il token_type_ids ?

	initialized bool
	persistErr  error // erreur d'init conservée pour les appels suivants

	// Timer d'inactivité : libère la session après idleTimeout sans appel Embed.
	idleTimer   *time.Timer
	idleTimeout time.Duration
}

// NewONNXEmbedder crée un nouvel adapter pour l'embedder ONNX.
// Le modèle est libéré automatiquement après 5 minutes d'inactivité.
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

// Init charge la session ONNX.
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
		e.vocab, e.clsID, e.sepID, e.unkID, loadErr = e.loadTokenizer(tokPath)
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

	// XLM-RoBERTa n'a pas toujours token_type_ids selon l'export : lire les entrées du modèle
	inputsInfo, _, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		e.persistErr = fmt.Errorf("lecture entrées du modèle : %w", err)
		return e.persistErr
	}
	e.hasTokenTypes = false
	for _, in := range inputsInfo {
		if in.Name == "token_type_ids" {
			e.hasTokenTypes = true
		}
	}
	inputNames := []string{"input_ids", "attention_mask"}
	if e.hasTokenTypes {
		inputNames = append(inputNames, "token_type_ids")
	}

	// Options de session orientées faible empreinte mémoire :
	// - pas d'arène CPU (sinon ORT garde le pic d'allocation indéfiniment)
	// - pas de memory pattern (inutile avec des shapes dynamiques)
	// - peu de threads (chaque thread a sa propre pile + buffers)
	options, err := ort.NewSessionOptions()
	if err != nil {
		e.persistErr = fmt.Errorf("options ORT : %w", err)
		return e.persistErr
	}
	defer options.Destroy()
	if err := options.SetCpuMemArena(false); err != nil {
		log.Printf("[WARN] ONNXEmbedder - SetCpuMemArena : %v", err)
	}
	if err := options.SetMemPattern(false); err != nil {
		log.Printf("[WARN] ONNXEmbedder - SetMemPattern : %v", err)
	}
	if err := options.SetIntraOpNumThreads(config.C.OnnxThreads); err != nil {
		log.Printf("[WARN] ONNXEmbedder - SetIntraOpNumThreads : %v", err)
	}
	if err := options.SetInterOpNumThreads(1); err != nil {
		log.Printf("[WARN] ONNXEmbedder - SetInterOpNumThreads : %v", err)
	}

	e.session, err = ort.NewDynamicAdvancedSession(modelPath,
		inputNames, []string{"last_hidden_state"}, options)
	if err != nil {
		e.persistErr = fmt.Errorf("création session ORT : %w", err)
		return e.persistErr
	}

	log.Printf("[INFO] ONNXEmbedder - Modèle chargé : %s", modelPath)
	e.persistErr = nil
	e.initialized = true
	return nil
}

// Release libère la session ONNX de la RAM.
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

	e.initialized = false
	log.Printf("[INFO] ONNXEmbedder - Modèle libéré de la RAM")
	runtime.GC()
}

// Embed génère le vecteur (dimension 384) d'une requête utilisateur.
func (e *ONNXEmbedderAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.embed(queryPrefix + text)
}

// EmbedPassage génère le vecteur (dimension 384) d'un passage de document à indexer.
func (e *ONNXEmbedderAdapter) EmbedPassage(ctx context.Context, text string) ([]float32, error) {
	return e.embed(passagePrefix + text)
}

func (e *ONNXEmbedderAdapter) embed(text string) ([]float32, error) {
	// Réinitialiser le timer d'inactivité AVANT d'acquérir tout autre verrou.
	e.resetIdleTimer()

	if err := e.Init(); err != nil {
		return nil, err
	}

	ids := e.tokenize(text)
	n := int64(len(ids))
	shape := ort.NewShape(1, n)

	mask := make([]int64, n)
	for i := range mask {
		mask[i] = 1
	}

	inIDs, err := ort.NewTensor(shape, ids)
	if err != nil {
		return nil, fmt.Errorf("tensor input_ids : %w", err)
	}
	defer inIDs.Destroy()
	inMask, err := ort.NewTensor(shape, mask)
	if err != nil {
		return nil, fmt.Errorf("tensor attention_mask : %w", err)
	}
	defer inMask.Destroy()
	inputs := []ort.Value{inIDs, inMask}
	if e.hasTokenTypes {
		inTypes, err := ort.NewTensor(shape, make([]int64, n))
		if err != nil {
			return nil, fmt.Errorf("tensor token_type_ids : %w", err)
		}
		defer inTypes.Destroy()
		inputs = append(inputs, inTypes)
	}

	outTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, n, embeddingDims))
	if err != nil {
		return nil, fmt.Errorf("tensor output : %w", err)
	}
	defer outTensor.Destroy()

	e.ortMu.Lock()
	defer e.ortMu.Unlock()
	if e.session == nil {
		return nil, fmt.Errorf("session ONNX libérée pendant l'appel")
	}
	if err := e.session.Run(inputs, []ort.Value{outTensor}); err != nil {
		return nil, err
	}

	return meanPoolAndNormalize(outTensor.GetData(), int(n)), nil
}

// Dimensions retourne la dimension des vecteurs produits (384 pour E5-small).
func (e *ONNXEmbedderAdapter) Dimensions() int {
	return embeddingDims
}

// ── Tokenisation Unigram SentencePiece (Metaspace / ▁) ─────────────────────

// tokenize tokenise `text` (préfixe E5 déjà inclus) : <s> + tokens + </s>,
// tronqué à maxTokens.
func (e *ONNXEmbedderAdapter) tokenize(text string) []int64 {
	tokens := e.unigramEncode(text)
	if len(tokens) > maxTokens-2 {
		tokens = tokens[:maxTokens-2]
	}
	ids := make([]int64, 0, len(tokens)+2)
	ids = append(ids, int64(e.clsID))
	for _, t := range tokens {
		ids = append(ids, int64(t))
	}
	return append(ids, int64(e.sepID))
}

// unigramEncode encode une chaîne via Metaspace + Viterbi Unigram.
func (e *ONNXEmbedderAdapter) unigramEncode(text string) []int32 {
	// Metaspace : espaces/retours ligne compactés puis remplacés par ▁ (U+2581), préfixe ▁
	normalized := "▁" + strings.Join(strings.Fields(text), "▁")
	return e.unigramViterbi(normalized)
}

// unigramViterbi décode le texte en tokens via l'algorithme de Viterbi.
// Cherche la séquence de tokens maximisant la somme des log-probabilités.
func (e *ONNXEmbedderAdapter) unigramViterbi(text string) []int32 {
	// Positions en octets du début de chaque rune (+ fin) : les sous-chaînes
	// text[i:j] ne copient rien, la recherche dans la map n'alloue pas.
	offsets := make([]int, 0, len(text)+1)
	for i := range text {
		offsets = append(offsets, i)
	}
	offsets = append(offsets, len(text))
	n := len(offsets) - 1
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
		maxJ := i + maxPieceRunes
		if maxJ > n {
			maxJ = n
		}
		for j := i + 1; j <= maxJ; j++ {
			if entry, ok := e.vocab[text[offsets[i]:offsets[j]]]; ok {
				candidate := dp[i] + float64(entry.score)
				if candidate > dp[j] {
					dp[j] = candidate
					from[j] = back{i, entry.id}
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

// meanPoolAndNormalize moyenne les n vecteurs de tokens (aucun padding) puis normalise.
func meanPoolAndNormalize(hidden []float32, n int) []float32 {
	result := make([]float32, embeddingDims)
	if n == 0 {
		return result
	}
	for t := 0; t < n; t++ {
		row := hidden[t*embeddingDims : (t+1)*embeddingDims]
		for d, v := range row {
			result[d] += v
		}
	}
	var norm float64
	for d := 0; d < embeddingDims; d++ {
		result[d] /= float32(n)
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
func (e *ONNXEmbedderAdapter) loadTokenizer(path string) (v map[string]vocabEntry, cls, sep, unk int32, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 2, 3, err
	}

	var tj struct {
		Model struct {
			Type  string               `json:"type"`
			UnkID int32                `json:"unk_id"`
			Vocab [][2]json.RawMessage `json:"vocab"` // [[token, score], ...]
		} `json:"model"`
		AddedTokens []struct {
			ID      int32  `json:"id"`
			Content string `json:"content"`
		} `json:"added_tokens"`
	}
	if err = json.Unmarshal(data, &tj); err != nil {
		return nil, 0, 2, 3, fmt.Errorf("parse tokenizer.json : %w", err)
	}
	data = nil
	if len(tj.Model.Vocab) == 0 {
		return nil, 0, 2, 3, fmt.Errorf("vocab vide dans tokenizer.json (type=%s)", tj.Model.Type)
	}

	v = make(map[string]vocabEntry, len(tj.Model.Vocab))
	for i, pair := range tj.Model.Vocab {
		var token string
		var score float64
		if jsonErr := json.Unmarshal(pair[0], &token); jsonErr != nil {
			continue
		}
		if jsonErr := json.Unmarshal(pair[1], &score); jsonErr != nil {
			continue
		}
		if utf8.RuneCountInString(token) > maxPieceRunes {
			continue
		}
		v[token] = vocabEntry{id: int32(i), score: float32(score)}
	}

	// Tokens spéciaux depuis le vocab (defaults)
	cls = v["<s>"].id
	sep = v["</s>"].id
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

	return v, cls, sep, unk, nil
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
