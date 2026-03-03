// embedder/embedder.go — Génération d'embeddings via ONNX Runtime
//
// Utilise intfloat/multilingual-e5-small en format ONNX (XLM-RoBERTa, Unigram SentencePiece).
// Le modèle doit être téléchargé dans le répertoire models/ :
//
//	scripts/download-model.sh
//
// Format attendu :
//
//	models/multilingual-e5-small/
//	  tokenizer.json          (vocab Unigram + special tokens)
//	  onnx/model.onnx
package embedder

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ask-rules-server/config"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	Dims      = 384
	MaxTokens = 256 // Réduit de 512 pour économiser RAM (~50% sur tensors)
	// Note : 256 tokens suffisent pour chunks de 600 caractères
	// Calcul : 600 chars ÷ 3.5 = ~171 tokens + préfixe "query:" (7) + CLS/SEP (2) = ~180 tokens
	// Impact précision : < 1% (voir IMPACT_PRECISION.md)
)

var (
	once  sync.Once
	ortMu sync.Mutex

	// vocab : token → ID (position dans le tableau Unigram)
	vocab map[string]int32
	// vocabScores : token → log-probabilité (pour Viterbi Unigram)
	vocabScores map[string]float64
	unkID       int32 = 3 // <unk> dans multilingual-e5-small
	clsID       int32 = 0 // <s>
	sepID       int32 = 2 // </s>

	session *ort.AdvancedSession

	// Tensors pré-alloués (réutilisés à chaque inférence)
	inIDs     *ort.Tensor[int64]
	inMask    *ort.Tensor[int64]
	inTypes   *ort.Tensor[int64]
	outTensor *ort.Tensor[float32]

	initialized bool
	persistErr  error // erreur d'init conservée pour les appels suivants
)

// Init charge le modèle ONNX et le vocabulaire.
func Init() error {
	once.Do(func() {
		modelDir := config.C.ModelPath

		// Chargement du tokenizer.json (Unigram SentencePiece)
		tokPath := filepath.Join(modelDir, "tokenizer.json")
		var loadErr error
		vocab, vocabScores, clsID, sepID, unkID, loadErr = loadTokenizer(tokPath)
		if loadErr != nil {
			persistErr = fmt.Errorf("chargement tokenizer depuis %s : %w", tokPath, loadErr)
			return
		}

		// Initialisation ONNX Runtime
		ort.SetSharedLibraryPath(findOrtLib())
		if err := ort.InitializeEnvironment(); err != nil {
			persistErr = fmt.Errorf("init ORT : %w", err)
			return
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
			persistErr = fmt.Errorf("fichier modèle ONNX introuvable dans %s", modelDir)
			return
		}

		// Création de la session ONNX
		options, err := ort.NewSessionOptions()
		if err != nil {
			persistErr = fmt.Errorf("options ORT : %w", err)
			return
		}
		defer options.Destroy()

		inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
		outputNames := []string{"last_hidden_state"}

		// Tensors pré-alloués — réutilisés à chaque Run()
		inIDs, err = ort.NewEmptyTensor[int64](ort.NewShape(1, MaxTokens))
		if err != nil {
			persistErr = fmt.Errorf("tensor input_ids : %w", err)
			return
		}
		inMask, err = ort.NewEmptyTensor[int64](ort.NewShape(1, MaxTokens))
		if err != nil {
			persistErr = fmt.Errorf("tensor attention_mask : %w", err)
			return
		}
		inTypes, err = ort.NewEmptyTensor[int64](ort.NewShape(1, MaxTokens))
		if err != nil {
			persistErr = fmt.Errorf("tensor token_type_ids : %w", err)
			return
		}
		outTensor, err = ort.NewEmptyTensor[float32](ort.NewShape(1, MaxTokens, Dims))
		if err != nil {
			persistErr = fmt.Errorf("tensor output : %w", err)
			return
		}

		session, err = ort.NewAdvancedSession(modelPath,
			inputNames, outputNames,
			[]ort.Value{inIDs, inMask, inTypes},
			[]ort.Value{outTensor},
			options)
		if err != nil {
			// XLM-RoBERTa n'a pas de token_type_ids : réessayer sans
			inputNames = []string{"input_ids", "attention_mask"}
			inTypes.Destroy()
			inTypes = nil
			session, err = ort.NewAdvancedSession(modelPath,
				inputNames, outputNames,
				[]ort.Value{inIDs, inMask},
				[]ort.Value{outTensor},
				options)
			if err != nil {
				persistErr = fmt.Errorf("création session ORT : %w", err)
				return
			}
		}

		initialized = true
	})
	return persistErr
}

// Embed génère un vecteur de dimension 384 pour le texte donné.
func Embed(_ context.Context, text string) ([]float32, error) {
	if !initialized {
		if err := Init(); err != nil {
			return nil, err
		}
		if inIDs == nil || inMask == nil || outTensor == nil {
			return nil, fmt.Errorf("embedder non initialisé : %v", persistErr)
		}
	}

	inputIDs, attentionMask, tokenTypeIDs := tokenize(text)

	ortMu.Lock()
	defer ortMu.Unlock()

	copy(inIDs.GetData(), inputIDs)
	copy(inMask.GetData(), attentionMask)
	if inTypes != nil {
		copy(inTypes.GetData(), tokenTypeIDs)
	}

	if err := session.Run(); err != nil {
		return nil, err
	}

	return meanPoolAndNormalize(outTensor.GetData(), attentionMask), nil
}

// ── Tokenisation Unigram SentencePiece (Metaspace / ▁) ─────────────────────

// tokenize tokenise `text` avec le tokenizer Unigram SentencePiece.
// Le modèle E5 attend un préfixe "query: " pour les requêtes.
func tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64) {
	inputIDs = make([]int64, MaxTokens)
	attentionMask = make([]int64, MaxTokens)
	tokenTypeIDs = make([]int64, MaxTokens)

	// Préfixe E5 pour les requêtes
	prefixed := "query: " + text

	tokens := unigramEncode(prefixed)
	// CLS + tokens + SEP
	all := make([]int64, 0, len(tokens)+2)
	all = append(all, int64(clsID))
	for _, t := range tokens {
		all = append(all, int64(t))
	}

	// Troncature
	if len(all) >= MaxTokens-1 {
		all = all[:MaxTokens-1]
	}
	all = append(all, int64(sepID))

	for i, t := range all {
		inputIDs[i] = t
		attentionMask[i] = 1
	}
	return
}

// unigramEncode encode une chaîne via Metaspace + Viterbi Unigram.
func unigramEncode(text string) []int32 {
	// Metaspace : remplacer espaces par ▁ (U+2581), préfixer avec ▁
	normalized := "▁" + strings.ReplaceAll(text, " ", "▁")
	return unigramViterbi(normalized)
}

// unigramViterbi décode le texte en tokens via l'algorithme de Viterbi.
// Cherche la séquence de tokens maximisant la somme des log-probabilités.
func unigramViterbi(text string) []int32 {
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
			if score, ok := vocabScores[sub]; ok {
				candidate := dp[i] + score
				if candidate > dp[j] {
					dp[j] = candidate
					from[j] = back{i, vocab[sub]}
				}
			}
		}
		// Caractère inconnu : avancer d'un rune avec unkID (pénalité)
		if dp[i+1] == negInf {
			dp[i+1] = dp[i] - 100.0
			from[i+1] = back{i, unkID}
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

func meanPoolAndNormalize(hidden []float32, mask []int64) []float32 {
	result := make([]float32, Dims)
	count := 0
	for t := 0; t < MaxTokens; t++ {
		if mask[t] == 0 {
			continue
		}
		count++
		for d := 0; d < Dims; d++ {
			result[d] += hidden[t*Dims+d]
		}
	}
	if count == 0 {
		return result
	}
	var norm float64
	for d := 0; d < Dims; d++ {
		result[d] /= float32(count)
		norm += float64(result[d]) * float64(result[d])
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for d := 0; d < Dims; d++ {
			result[d] = float32(float64(result[d]) / norm)
		}
	}
	return result
}

// ── Chargement du tokenizer.json (Unigram SentencePiece HuggingFace) ─────────

// loadTokenizer charge vocab Unigram + scores depuis tokenizer.json.
// Format : model.vocab = [[token, score], ...] où position = ID.
func loadTokenizer(path string) (v map[string]int32, scores map[string]float64, cls, sep, unk int32, err error) {
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

func findOrtLib() string {
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

// preTokenize découpe le texte en mots et encode chaque mot en BPE unicode.
// L'espace précédant un mot (sauf le premier) devient le caractère Ġ (U+0120).
