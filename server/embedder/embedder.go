// embedder/embedder.go — Génération d'embeddings via ONNX Runtime
//
// Utilise Xenova/multilingual-e5-small en format ONNX.
// Le modèle doit être téléchargé dans le répertoire models/ :
//
//	scripts/download-model.sh
//
// Format attendu :
//
//	models/multilingual-e5-small/
//	  model.onnx          (ou model_quantized.onnx)
//	  tokenizer.json
//	  tokenizer_config.json
//	  special_tokens_map.json
//	  vocab.txt (ou sentencepiece.bpe.model)
package embedder

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"ask-rules-server/config"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	Dims      = 384
	MaxTokens = 512
)

var (
	once  sync.Once
	ortMu sync.Mutex

	vocab   map[string]int32
	unkID   int32 = 0
	clsID   int32 = 101
	sepID   int32 = 102
	_       int32 = 0 // padID — unused but kept for reference
	session *ort.AdvancedSession

	// Tensors pré-alloués (réutilisés à chaque inférence)
	inIDs     *ort.Tensor[int32]
	inMask    *ort.Tensor[int32]
	inTypes   *ort.Tensor[int32]
	outTensor *ort.Tensor[float32]

	initialized bool
	persistErr  error // erreur d'init conservée pour les appels suivants
)

// Init charge le modèle ONNX et le vocabulaire.
func Init() error {
	once.Do(func() {
		modelDir := config.C.ModelPath

		// Chargement du vocabulaire : tokenizer.json (HuggingFace BPE/SPM) en priorité
		vocabCandidates := []string{
			filepath.Join(modelDir, "tokenizer.json"),
			filepath.Join(modelDir, "vocab.txt"),
		}
		var vocabErr error
		for _, candidate := range vocabCandidates {
			vocab, vocabErr = loadVocab(candidate)
			if vocabErr == nil && len(vocab) > 0 {
				break
			}
		}
		if vocabErr != nil || len(vocab) == 0 {
			persistErr = fmt.Errorf("chargement vocab (tokenizer.json / vocab.txt) : %w", vocabErr)
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
		inIDs, err = ort.NewEmptyTensor[int32](ort.NewShape(1, MaxTokens))
		if err != nil {
			persistErr = fmt.Errorf("tensor input_ids : %w", err)
			return
		}
		inMask, err = ort.NewEmptyTensor[int32](ort.NewShape(1, MaxTokens))
		if err != nil {
			persistErr = fmt.Errorf("tensor attention_mask : %w", err)
			return
		}
		inTypes, err = ort.NewEmptyTensor[int32](ort.NewShape(1, MaxTokens))
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
			persistErr = fmt.Errorf("création session ORT : %w", err)
			return
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
		// Après once.Do, vérifier que les tensors sont bien alloués
		if inIDs == nil || inMask == nil || inTypes == nil || outTensor == nil {
			return nil, fmt.Errorf("embedder non initialisé : %v", persistErr)
		}
	}

	// Tokenisation WordPiece
	inputIDs, attentionMask, tokenTypeIDs := tokenize(text)

	ortMu.Lock()
	defer ortMu.Unlock()

	// Copie des données dans les tensors pré-alloués
	copy(inIDs.GetData(), inputIDs)
	copy(inMask.GetData(), attentionMask)
	copy(inTypes.GetData(), tokenTypeIDs)

	if err := session.Run(); err != nil {
		return nil, err
	}

	hidden := outTensor.GetData()

	// Mean pooling sur les tokens non-padding, puis normalisation L2
	return meanPoolAndNormalize(hidden, attentionMask), nil
}

// ── Tokenisation WordPiece minimale ─────────────────────────────────────────

func tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int32) {
	inputIDs = make([]int32, MaxTokens)
	attentionMask = make([]int32, MaxTokens)
	tokenTypeIDs = make([]int32, MaxTokens)

	words := strings.Fields(strings.ToLower(text))
	tokens := []int32{clsID}

	for _, word := range words {
		wt := wordPieceTokenize(word)
		for _, t := range wt {
			tokens = append(tokens, t)
			if len(tokens) >= MaxTokens-1 {
				goto done
			}
		}
	}
done:
	tokens = append(tokens, sepID)
	if len(tokens) > MaxTokens {
		tokens = tokens[:MaxTokens-1]
		tokens = append(tokens, sepID)
	}

	for i, t := range tokens {
		inputIDs[i] = t
		attentionMask[i] = 1
	}
	return
}

func wordPieceTokenize(word string) []int32 {
	if id, ok := vocab[word]; ok {
		return []int32{id}
	}

	var ids []int32
	runes := []rune(word)
	start := 0
	for start < len(runes) {
		found := false
		for end := len(runes); end > start; end-- {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := vocab[sub]; ok {
				ids = append(ids, id)
				start = end
				found = true
				break
			}
		}
		if !found {
			// Caractère par caractère en fallback
			for _, r := range string(runes[start]) {
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					ids = append(ids, unkID)
				}
			}
			start++
		}
	}
	if len(ids) == 0 {
		return []int32{unkID}
	}
	return ids
}

// ── Mean pooling + normalisation L2 ─────────────────────────────────────────

func meanPoolAndNormalize(hidden []float32, mask []int32) []float32 {
	// hidden shape : [1, MaxTokens, Dims] → flatten
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

// ── Chargement du vocabulaire ─────────────────────────────────────────────

func loadVocab(path string) (map[string]int32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := make(map[string]int32)

	// Essaie d'abord le format JSON (tokenizer.json de HuggingFace)
	if filepath.Ext(path) == ".json" {
		var tj struct {
			Model struct {
				Vocab map[string]int32 `json:"vocab"`
			} `json:"model"`
		}
		if err := json.Unmarshal(data, &tj); err == nil && len(tj.Model.Vocab) > 0 {
			return tj.Model.Vocab, nil
		}
	}

	// Format texte ligne par ligne (vocab.txt BERT)
	for i, line := range bytes.Split(data, []byte("\n")) {
		token := strings.TrimSpace(string(line))
		if token != "" {
			m[token] = int32(i)
		}
	}
	return m, nil
}

// ── Utilitaire : lecture float32 LE ─────────────────────────────────────────

func float32FromBytes(b []byte) float32 {
	bits := binary.LittleEndian.Uint32(b)
	return math.Float32frombits(bits)
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
	// Laisse ORT chercher dans le PATH système
	return "libonnxruntime.so"
}
