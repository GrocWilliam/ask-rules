// infrastructure/service/llm_adapter.go — Client LLM intégré (Mistral, Plugsky en secours, Ollama)
package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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

// chatProvider : une API au format chat/completions (Mistral, Plugsky).
type chatProvider struct {
	name   string
	url    string
	apiKey string
	model  string
	pacer  *pacer
}

// MistralLLMAdapter implémente LLMService : Mistral en principal, Plugsky en
// secours (quota Mistral atteint, panne…), Ollama si aucune API n'est configurée.
type MistralLLMAdapter struct {
	client    *http.Client // partagé : réutilise les connexions TLS
	providers []chatProvider
}

// NewMistralLLM crée un nouvel adapter pour le service LLM.
func NewMistralLLM() service.LLMService {
	m := &MistralLLMAdapter{
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: config.C.Env == "development", // Permet d'ignorer les erreurs TLS en dev
				},
			},
		},
	}
	if config.C.MistralAPIKey != "" {
		m.providers = append(m.providers, chatProvider{
			name:   "mistral",
			url:    "https://api.mistral.ai/v1/chat/completions",
			apiKey: config.C.MistralAPIKey,
			model:  config.C.MistralModel,
			pacer:  newPacer(config.C.LLMRequestsPerSecond),
		})
	}
	if config.C.PlugskyAPIKey != "" {
		m.providers = append(m.providers, chatProvider{
			name:   "plugsky",
			url:    strings.TrimRight(config.C.PlugskyBaseURL, "/") + "/chat/completions",
			apiKey: config.C.PlugskyAPIKey,
			model:  config.C.PlugskyModel,
		})
	}
	return m
}

// Query génère une réponse à partir de la question et du contexte fourni.
func (m *MistralLLMAdapter) Query(ctx context.Context, question, contextText string) (*service.LLMResponse, error) {
	var resp entity.LLMResponse
	var err error

	switch {
	case len(m.providers) > 0:
		resp, err = m.queryWithFallback(ctx, question, contextText)
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

// queryWithFallback interroge les fournisseurs dans l'ordre jusqu'au premier succès.
// Quand un secours existe, le fournisseur principal n'est tenté qu'une fois :
// mieux vaut basculer tout de suite que faire patienter l'utilisateur.
func (m *MistralLLMAdapter) queryWithFallback(ctx context.Context, question, ctxText string) (entity.LLMResponse, error) {
	var errs []string
	for i, p := range m.providers {
		isLast := i == len(m.providers)-1
		retries := llmMaxRetries
		if !isLast {
			retries = 0
		}
		resp, err := m.queryChat(ctx, p, retries, question, ctxText)
		if err == nil {
			if i > 0 {
				log.Printf("[INFO] LLM - réponse fournie par %s (secours)", p.name)
			}
			return resp, nil
		}
		// Client parti ou délai de la requête dépassé : inutile d'essayer le suivant
		if ctx.Err() != nil || isLast {
			if len(errs) > 0 {
				return entity.LLMResponse{}, fmt.Errorf("%w (après échec : %s)", err, strings.Join(errs, " ; "))
			}
			return entity.LLMResponse{}, err
		}
		log.Printf("[WARN] LLM - %s indisponible, bascule sur %s : %v", p.name, m.providers[i+1].name, err)
		errs = append(errs, err.Error())
	}
	return entity.LLMResponse{}, fmt.Errorf("aucun fournisseur LLM configuré")
}

// ModelName retourne le nom du modèle principal utilisé.
func (m *MistralLLMAdapter) ModelName() string {
	if len(m.providers) > 0 {
		return m.providers[0].model
	}
	if config.C.OllamaHost != "" {
		return config.C.OllamaModel
	}
	return "none"
}

// ── API chat/completions (Mistral, Plugsky) ────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role string `json:"role"`
			// Chaîne, ou tableau de chunks ({"type":"text","text":…}, "thinking"…)
			// selon le modèle (spécification actuelle de l'API Mistral).
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

// chatContentText extrait le texte de la réponse : `content` est soit une
// chaîne, soit un tableau de chunks dont seuls les chunks "text" sont gardés
// (les chunks de raisonnement "thinking" ne sont pas montrés à l'utilisateur).
func chatContentText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var chunks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return "", fmt.Errorf("contenu de réponse inattendu : %w", err)
	}
	var b strings.Builder
	for _, c := range chunks {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String(), nil
}

func (m *MistralLLMAdapter) queryChat(ctx context.Context, p chatProvider, maxRetries int, question, ctxText string) (entity.LLMResponse, error) {
	prompt := fmt.Sprintf("Contexte du jeu :\n%s\n\nQuestion : %s", ctxText, question)

	body, _ := json.Marshal(chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   llmMaxTokens,
		Temperature: 0.3,
	})

	resp, err := postJSONWithRetry(ctx, m.client, p.pacer, p.name, p.url,
		map[string]string{"Authorization": "Bearer " + p.apiKey}, body, maxRetries)
	if err != nil {
		return entity.LLMResponse{}, err
	}
	defer resp.Body.Close()

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.LLMResponse{}, fmt.Errorf("%s: %w", p.name, err)
	}
	if len(result.Choices) == 0 {
		return entity.LLMResponse{}, fmt.Errorf("%s: pas de réponse", p.name)
	}
	choice := result.Choices[0]
	answer, err := chatContentText(choice.Message.Content)
	if err != nil {
		return entity.LLMResponse{}, fmt.Errorf("%s: %w", p.name, err)
	}
	if strings.TrimSpace(answer) == "" {
		return entity.LLMResponse{}, fmt.Errorf("%s: réponse vide (finish_reason=%s)", p.name, choice.FinishReason)
	}
	if choice.FinishReason == "length" {
		log.Printf("[WARN] %s - réponse tronquée à %d tokens (id=%s)", p.name, llmMaxTokens, result.ID)
	}

	return entity.LLMResponse{
		Answer:  answer,
		Model:   result.Model,
		UsedLLM: true,
		Tokens: &entity.TokenUsage{
			Prompt:     result.Usage.PromptTokens,
			Completion: result.Usage.CompletionTokens,
			Total:      result.Usage.TotalTokens,
		},
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
