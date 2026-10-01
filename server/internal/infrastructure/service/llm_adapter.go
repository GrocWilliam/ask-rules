// infrastructure/service/mistral_llm_adapter.go — Client LLM intégré (Mistral/OpenAI/Ollama)
package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/infrastructure/config"
)

const systemPrompt = `Tu es un assistant expert en jeux de société. Tu dois répondre uniquement en te basant sur les informations du contexte fourni.

Règles importantes :
- Réponds toujours en français
- Cite des règles précises quand tu les énonces, avec la page indiquée dans le contexte (ex : « p. 4 »)
- Si l'information n'est pas dans le contexte, dis-le clairement plutôt qu'inventer
- Structure ta réponse de façon claire avec des listes si nécessaire
- Sois précis et concis`

// llmMaxTokens : longueur max d'une réponse (~750 mots), largement suffisante
// pour une règle de jeu. Limite aussi la consommation du quota de tokens.
const llmMaxTokens = 1024

// MistralLLMAdapter implémente LLMService avec support Mistral/OpenAI/Ollama.
type MistralLLMAdapter struct {
	client *http.Client // partagé : réutilise les connexions TLS
	pacer  *pacer       // cadence les appels aux API hébergées (Mistral, OpenAI)
}

// NewMistralLLM crée un nouvel adapter pour le service LLM.
func NewMistralLLM() service.LLMService {
	return &MistralLLMAdapter{
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: config.C.Env == "development", // Permet d'ignorer les erreurs TLS en dev
				},
			},
		},
		pacer: newPacer(config.C.LLMRequestsPerSecond),
	}
}

// Query génère une réponse à partir de la question et du contexte fourni.
func (m *MistralLLMAdapter) Query(ctx context.Context, question, contextText string) (*service.LLMResponse, error) {
	var resp entity.LLMResponse
	var err error

	switch {
	case config.C.MistralAPIKey != "":
		resp, err = m.queryMistral(ctx, question, contextText)
	case config.C.OpenAIAPIKey != "":
		resp, err = m.queryOpenAI(ctx, question, contextText)
	case config.C.OllamaHost != "":
		resp, err = m.queryOllama(ctx, question, contextText)
	default:
		resp = entity.LLMResponse{Answer: contextText, UsedLLM: false}
	}

	if err != nil {
		return nil, err
	}

	// Convertir entity.LLMResponse → service.LLMResponse
	var tokensUsed *service.TokenUsage
	if resp.Tokens != nil {
		tokensUsed = &service.TokenUsage{
			Input:  resp.Tokens.Prompt,
			Output: resp.Tokens.Completion,
			Total:  resp.Tokens.Total,
		}
	}

	return &service.LLMResponse{
		Answer:     resp.Answer,
		Model:      resp.Model,
		UsedLLM:    resp.UsedLLM,
		TokensUsed: tokensUsed,
	}, nil
}

// ModelName retourne le nom du modèle utilisé.
func (m *MistralLLMAdapter) ModelName() string {
	if config.C.MistralAPIKey != "" {
		return config.C.MistralModel
	} else if config.C.OpenAIAPIKey != "" {
		return config.C.OpenAIModel
	} else if config.C.OllamaHost != "" {
		return config.C.OllamaModel
	}
	return "none"
}

// ── Mistral API ──────────────────────────────────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mistralRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type mistralResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

func (m *MistralLLMAdapter) queryMistral(ctx context.Context, question, ctxText string) (entity.LLMResponse, error) {
	prompt := fmt.Sprintf("Contexte du jeu :\n%s\n\nQuestion : %s", ctxText, question)

	reqBody := mistralRequest{
		Model: config.C.MistralModel,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   llmMaxTokens,
		Temperature: 0.3,
	}

	body, _ := json.Marshal(reqBody)

	resp, err := postJSONWithRetry(ctx, m.client, m.pacer, "mistral",
		"https://api.mistral.ai/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + config.C.MistralAPIKey}, body)
	if err != nil {
		return entity.LLMResponse{}, err
	}
	defer resp.Body.Close()

	var result mistralResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.LLMResponse{}, err
	}
	if len(result.Choices) == 0 {
		return entity.LLMResponse{}, fmt.Errorf("mistral: pas de réponse")
	}

	tokens := &entity.TokenUsage{
		Prompt:     result.Usage.PromptTokens,
		Completion: result.Usage.CompletionTokens,
		Total:      result.Usage.TotalTokens,
	}

	return entity.LLMResponse{
		Answer:  result.Choices[0].Message.Content,
		Model:   result.Model,
		UsedLLM: true,
		Tokens:  tokens,
	}, nil
}

// ── OpenAI API ───────────────────────────────────────────────────────────────

type openAIRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type openAIResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

func (m *MistralLLMAdapter) queryOpenAI(ctx context.Context, question, ctxText string) (entity.LLMResponse, error) {
	prompt := fmt.Sprintf("Contexte du jeu :\n%s\n\nQuestion : %s", ctxText, question)
	reqBody := openAIRequest{
		Model: config.C.OpenAIModel,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   llmMaxTokens,
		Temperature: 0.3,
	}
	body, _ := json.Marshal(reqBody)
	fmt.Printf("[DEBUG] OpenAI request body: %s\n", string(body))

	resp, err := postJSONWithRetry(ctx, m.client, m.pacer, "openai",
		"https://api.openai.com/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + config.C.OpenAIAPIKey}, body)
	if err != nil {
		return entity.LLMResponse{}, err
	}
	defer resp.Body.Close()
	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.LLMResponse{}, err
	}
	if len(result.Choices) == 0 {
		return entity.LLMResponse{}, fmt.Errorf("openai: pas de réponse")
	}
	tokens := &entity.TokenUsage{
		Prompt:     result.Usage.PromptTokens,
		Completion: result.Usage.CompletionTokens,
		Total:      result.Usage.TotalTokens,
	}
	return entity.LLMResponse{
		Answer:  result.Choices[0].Message.Content,
		Model:   result.Model,
		UsedLLM: true,
		Tokens:  tokens,
	}, nil
}

// ── Ollama API ───────────────────────────────────────────────────────────────

type ollamaRequest struct {
	Model      string `json:"model"`
	Prompt     string `json:"prompt"`
	Stream     bool   `json:"stream"`
	NumPredict int    `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Model    string `json:"model"`
}

func (m *MistralLLMAdapter) queryOllama(ctx context.Context, question, ctxText string) (entity.LLMResponse, error) {
	prompt := fmt.Sprintf("%s\n\nContexte du jeu :\n%s\n\nQuestion : %s", systemPrompt, ctxText, question)
	reqBody := ollamaRequest{
		Model:      config.C.OllamaModel,
		Prompt:     prompt,
		Stream:     false,
		NumPredict: llmMaxTokens,
	}
	body, _ := json.Marshal(reqBody)
	fmt.Printf("[DEBUG] Ollama request body: %s\n", string(body))

	req, err := http.NewRequestWithContext(ctx, "POST",
		config.C.OllamaHost+"/api/generate",
		bytes.NewReader(body))
	if err != nil {
		return entity.LLMResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return entity.LLMResponse{}, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return entity.LLMResponse{}, fmt.Errorf("ollama status %d: %s", resp.StatusCode, b)
	}
	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.LLMResponse{}, err
	}
	return entity.LLMResponse{
		Answer:  result.Response,
		Model:   result.Model,
		UsedLLM: true,
	}, nil
}
