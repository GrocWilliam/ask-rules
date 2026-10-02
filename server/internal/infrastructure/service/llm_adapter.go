// infrastructure/service/llm_adapter.go — Client LLM compatible OpenAI (principal + secours)
package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/infrastructure/config"
)

const systemPrompt = `Tu es un assistant expert en jeux de société. Les joueurs te consultent en pleine partie : ils veulent la règle tout de suite. Tu dois répondre uniquement en te basant sur les informations du contexte fourni.

Règles importantes :
- Commence par la réponse directe en une phrase, en gras (par « Oui » ou « Non » quand la question s'y prête), avec la page. Ajoute ensuite seulement les précisions utiles
- Réponds toujours en français
- Cite des règles précises quand tu les énonces, avec la page indiquée dans le contexte (ex : « p. 4 »)
- Si l'information n'est pas dans le contexte, dis-le clairement plutôt qu'inventer
- La question peut faire suite aux échanges précédents : interprète-la à leur lumière
- Structure ta réponse de façon claire avec des listes si nécessaire
- Sois précis et concis`

// llmMaxTokens : longueur max d'une réponse (~750 mots), largement suffisante
// pour une règle de jeu. Limite aussi la consommation du quota de tokens.
const llmMaxTokens = 1024

// chatProvider : une API au format chat/completions (llama.cpp, Mistral…).
type chatProvider struct {
	name    string // hôte de l'API, pour les logs
	url     string
	apiKey  string
	model   string
	timeout time.Duration
	pacer   *pacer
	health  *healthState // nil si LLM_HEALTH_URL n'est pas défini
}

func newChatProvider(c config.LLMProvider, client *http.Client) (chatProvider, bool) {
	if c.BaseURL == "" {
		return chatProvider{}, false
	}
	name := c.BaseURL
	if u, err := url.Parse(c.BaseURL); err == nil && u.Host != "" {
		name = u.Host
	}
	return chatProvider{
		name:    name,
		url:     strings.TrimRight(c.BaseURL, "/") + "/chat/completions",
		apiKey:  c.APIKey,
		model:   c.Model,
		timeout: c.Timeout,
		pacer:   newPacer(c.RequestsPerSecond),
		health:  newHealthState(name, c.HealthURL, c.WakeTimeout, client),
	}, true
}

// ChatLLMAdapter implémente LLMService avec un fournisseur principal et un
// secours optionnel (quota atteint, panne, serveur en veille…).
type ChatLLMAdapter struct {
	client    *http.Client // partagé : réutilise les connexions TLS
	providers []chatProvider
}

// NewLLM crée l'adapter LLM à partir de la configuration LLM_* / LLM_FALLBACK_*.
func NewLLM() service.LLMService {
	m := &ChatLLMAdapter{
		client: &http.Client{
			// Les délais sont gérés par fournisseur via le contexte
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: config.C.Env == "development", // Permet d'ignorer les erreurs TLS en dev
				},
			},
		},
	}
	for _, c := range []config.LLMProvider{config.C.LLM, config.C.LLMFallback} {
		if p, ok := newChatProvider(c, m.client); ok {
			m.providers = append(m.providers, p)
		}
	}
	return m
}

// Query génère une réponse à partir de la question et du contexte fourni.
func (m *ChatLLMAdapter) Query(ctx context.Context, question, contextText string, history []entity.ChatTurn, onToken func(string)) (*service.LLMResponse, error) {
	if len(m.providers) == 0 {
		if onToken != nil {
			onToken(contextText)
		}
		return &service.LLMResponse{Answer: contextText, UsedLLM: false}, nil
	}

	resp, err := m.queryWithFallback(ctx, question, contextText, history, onToken)
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

// Warmup réveille en arrière-plan les fournisseurs mis en veille, pour qu'ils
// soient prêts quand l'utilisateur posera sa question.
func (m *ChatLLMAdapter) Warmup() {
	for _, p := range m.providers {
		if p.health != nil {
			go p.health.ready(context.Background())
		}
	}
}

// queryWithFallback interroge les fournisseurs dans l'ordre jusqu'au premier succès.
// Quand un secours existe, le fournisseur principal n'est tenté qu'une fois :
// mieux vaut basculer tout de suite que faire patienter l'utilisateur.
// En streaming, pas de bascule une fois du texte transmis : le secours
// recommencerait la réponse depuis le début.
func (m *ChatLLMAdapter) queryWithFallback(ctx context.Context, question, ctxText string, history []entity.ChatTurn, onToken func(string)) (entity.LLMResponse, error) {
	streamed := false
	if onToken != nil {
		forward := onToken
		onToken = func(text string) {
			streamed = true
			forward(text)
		}
	}
	var errs []string
	for i, p := range m.providers {
		isLast := i == len(m.providers)-1
		retries := llmMaxRetries
		if !isLast {
			retries = 0
		}
		if p.health != nil {
			if isLast {
				// Pas d'autre fournisseur : attendre le réveil (dans la limite de la requête)
				if err := p.health.waitReady(ctx); err != nil {
					return entity.LLMResponse{}, fmt.Errorf("%s: en veille, réveil trop long : %w", p.name, err)
				}
			} else if !p.health.ready(ctx) {
				log.Printf("[INFO] LLM - %s en veille, bascule sur %s", p.name, m.providers[i+1].name)
				errs = append(errs, p.name+": en veille")
				continue
			}
		}
		resp, err := m.queryChat(ctx, p, retries, question, ctxText, history, onToken)
		if p.health != nil {
			if err == nil {
				p.health.markOK()
			} else {
				p.health.markDown()
			}
		}
		if err == nil {
			if i > 0 {
				log.Printf("[INFO] LLM - réponse fournie par %s (secours)", p.name)
			}
			return resp, nil
		}
		// Client parti, délai de la requête dépassé ou réponse déjà entamée :
		// inutile d'essayer le suivant
		if ctx.Err() != nil || isLast || streamed {
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
func (m *ChatLLMAdapter) ModelName() string {
	if len(m.providers) == 0 {
		return "none"
	}
	if m.providers[0].model == "" {
		return m.providers[0].name
	}
	return m.providers[0].model
}

// ── API chat/completions ────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model,omitempty"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream,omitempty"`
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

// buildMessages construit la conversation envoyée au LLM : les échanges
// précédents sont rejoués tels quels, et seule la dernière question porte le
// contexte des règles (les réponses précédentes citent déjà leurs pages).
func buildMessages(question, ctxText string, history []entity.ChatTurn) []chatMessage {
	messages := make([]chatMessage, 0, 2+2*len(history))
	messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	for _, turn := range history {
		messages = append(messages,
			chatMessage{Role: "user", Content: turn.Question},
			chatMessage{Role: "assistant", Content: turn.Answer},
		)
	}
	prompt := fmt.Sprintf("Contexte du jeu :\n%s\n\nQuestion : %s", ctxText, question)
	return append(messages, chatMessage{Role: "user", Content: prompt})
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

func (m *ChatLLMAdapter) queryChat(ctx context.Context, p chatProvider, maxRetries int, question, ctxText string, history []entity.ChatTurn, onToken func(string)) (entity.LLMResponse, error) {
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	body, _ := json.Marshal(chatRequest{
		Model:       p.model,
		Messages:    buildMessages(question, ctxText, history),
		MaxTokens:   llmMaxTokens,
		Temperature: 0.3,
		Stream:      onToken != nil,
	})

	headers := map[string]string{}
	if p.apiKey != "" {
		headers["Authorization"] = "Bearer " + p.apiKey
	}
	resp, err := postJSONWithRetry(ctx, m.client, p.pacer, p.name, p.url, headers, body, maxRetries)
	if err != nil {
		return entity.LLMResponse{}, err
	}
	defer resp.Body.Close()

	if onToken != nil {
		return readChatStream(p.name, resp.Body, onToken)
	}

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

// ── Streaming (Server-Sent Events) ──────────────────────

// chatStreamChunk : un événement « data: » du flux chat/completions.
type chatStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			// Chaîne ou tableau de chunks, comme Message.Content
			Content json.RawMessage `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	// Usage : présent dans le dernier événement (Mistral, llama.cpp), sinon absent
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// readChatStream lit un flux SSE chat/completions, transmet chaque fragment de
// texte à onToken et retourne la réponse complète.
func readChatStream(provider string, body io.Reader, onToken func(string)) (entity.LLMResponse, error) {
	var (
		answer       strings.Builder
		model, id    string
		finishReason string
		tokens       *entity.TokenUsage
	)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // lignes vides, commentaires « : keep-alive », event:…
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return entity.LLMResponse{}, fmt.Errorf("%s: flux invalide : %w", provider, err)
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.ID != "" {
			id = chunk.ID
		}
		if chunk.Usage != nil {
			tokens = &entity.TokenUsage{
				Prompt:     chunk.Usage.PromptTokens,
				Completion: chunk.Usage.CompletionTokens,
				Total:      chunk.Usage.TotalTokens,
			}
		}
		for _, choice := range chunk.Choices {
			text, err := chatContentText(choice.Delta.Content)
			if err != nil {
				return entity.LLMResponse{}, fmt.Errorf("%s: %w", provider, err)
			}
			if text != "" {
				answer.WriteString(text)
				onToken(text)
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishReason = *choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return entity.LLMResponse{}, fmt.Errorf("%s: flux interrompu : %w", provider, err)
	}
	if strings.TrimSpace(answer.String()) == "" {
		return entity.LLMResponse{}, fmt.Errorf("%s: réponse vide (finish_reason=%s)", provider, finishReason)
	}
	if finishReason == "length" {
		log.Printf("[WARN] %s - réponse tronquée à %d tokens (id=%s)", provider, llmMaxTokens, id)
	}
	return entity.LLMResponse{Answer: answer.String(), Model: model, UsedLLM: true, Tokens: tokens}, nil
}
