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
	"regexp"
	"strings"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/infrastructure/config"
)

// systemPrompt vise un petit modèle (Gemma 3 1B) : consignes courtes et numérotées.
// Pas d'exemple de réponse : quand la question le déroute, un petit modèle
// recopie les exemples mot pour mot (« Le premier joueur à 10 points… »).
const systemPrompt = `Tu es l'arbitre d'une partie de jeu de société. Les joueurs te posent une question de règle en pleine partie.

Consignes :
1. Réponds uniquement avec les extraits du livret fournis avec la question. N'invente jamais une règle ni une page.
2. Utilise seulement les extraits qui concernent la question, ignore les autres.
3. Si la question est courte (« et pour… ? »), elle porte sur le sujet des questions précédentes.
4. Si les extraits ne donnent pas la réponse, réponds : « Je ne trouve pas cette règle dans le livret. »
5. Réponds en français, en 80 mots au plus (150 pour une liste).
6. Commence directement par la règle : jamais « Ok », « D'accord », « Bien sûr » ni reformulation de la question.
7. « Oui » ou « Non » seulement pour une question fermée (« Peut-on… ? », « Est-ce que… ? »).
8. Les extraits sont rangés par chapitre. Ne mélange jamais deux chapitres : si la règle diffère selon le chapitre (mode de jeu, extension…), donne-la pour chacun en le nommant.

Format :
- Première ligne : la réponse directe en gras, avec la page entre parenthèses.
- Ensuite, seulement si utile : 1 à 3 précisions en liste, chacune avec sa page.
- Si la question demande de présenter ou lister (types, phases, actions…) : une ligne de liste par élément, avec sa page.`

// fillerOpening : accusé de réception en tête de réponse (« Ok, », « D'accord. »…),
// retiré malgré la consigne du prompt qu'un petit modèle ne suit pas toujours.
var fillerOpening = regexp.MustCompile(`(?i)^\s*(ok|okay|d'accord|d’accord|bien sûr|très bien)\b\s*[,.!:]?\s*`)

// llmMaxTokens : longueur max d'une réponse (~300 mots), le double des 150 mots
// demandés par le prompt. Coupe aussi vite une réponse qui tourne en boucle.
const llmMaxTokens = 400

// llmFrequencyPenalty : pénalise les tokens déjà produits, contre les boucles
// d'un petit modèle (même paragraphe répété jusqu'à max_tokens). Faible pour
// ne pas gêner les mots normalement répétés (« tuile », « joueur »).
const llmFrequencyPenalty = 0.3

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

func newChatProvider(llmConfig config.LLMProvider, client *http.Client) (chatProvider, bool) {
	if llmConfig.BaseURL == "" {
		return chatProvider{}, false
	}

	name := llmConfig.BaseURL
	if u, err := url.Parse(llmConfig.BaseURL); err == nil && u.Host != "" {
		name = u.Host
	}

	return chatProvider{
		name:    name,
		url:     strings.TrimRight(llmConfig.BaseURL, "/") + "/chat/completions",
		apiKey:  llmConfig.APIKey,
		model:   llmConfig.Model,
		timeout: llmConfig.Timeout,
		pacer:   newPacer(llmConfig.RequestsPerSecond),
		health:  newHealthState(name, llmConfig.HealthURL, llmConfig.WakeTimeout, client),
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
	for _, llmConfig := range []config.LLMProvider{config.C.LLM, config.C.LLMFallback} {
		if llmProvider, ok := newChatProvider(llmConfig, m.client); ok {
			m.providers = append(m.providers, llmProvider)
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

	var strip *fillerStripper
	if onToken != nil {
		strip = &fillerStripper{out: onToken}
		onToken = strip.write
	}
	resp, err := m.queryWithFallback(ctx, question, contextText, history, onToken)
	if err != nil {
		return nil, err
	}
	if strip != nil {
		strip.flush()
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
		Answer:     stripFiller(resp.Answer),
		Model:      resp.Model,
		UsedLLM:    resp.UsedLLM,
		TokensUsed: tokensUsed,
	}, nil
}

// stripFiller retire fillerOpening, sauf si la réponse ne contient rien d'autre.
func stripFiller(s string) string {
	if t := fillerOpening.ReplaceAllString(s, ""); strings.TrimSpace(t) != "" {
		return t
	}
	return s
}

// fillerStripper retient le début du flux, le temps de pouvoir retirer
// fillerOpening, puis laisse passer le reste tel quel.
type fillerStripper struct {
	out  func(string)
	head strings.Builder
	done bool
}

// fillerHeadLen : assez d'octets pour contenir la plus longue formule retirée
const fillerHeadLen = 24

func (f *fillerStripper) write(s string) {
	if f.done {
		f.out(s)
		return
	}
	f.head.WriteString(s)
	if f.head.Len() >= fillerHeadLen {
		f.flush()
	}
}

// flush envoie le début retenu (appelé aussi en fin de flux, pour une réponse courte).
func (f *fillerStripper) flush() {
	if f.done {
		return
	}
	f.done = true
	if head := stripFiller(f.head.String()); head != "" {
		f.out(head)
	}
}

// Warmup réveille en arrière-plan les fournisseurs mis en veille, pour qu'ils
// soient prêts quand l'utilisateur posera sa question.
func (m *ChatLLMAdapter) Warmup() {
	for _, provider := range m.providers {
		if provider.health != nil {
			go provider.health.ready(context.Background())
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
	for i, llmProvider := range m.providers {
		isLast := i == len(m.providers)-1
		retries := llmMaxRetries

		if !isLast {
			retries = 0
		}

		if llmProvider.health != nil {
			if isLast {
				// Pas d'autre fournisseur : attendre le réveil (dans la limite de la requête)
				if err := llmProvider.health.waitReady(ctx); err != nil {
					return entity.LLMResponse{}, fmt.Errorf("%s: en veille, réveil trop long : %w", llmProvider.name, err)
				}
			} else if !llmProvider.health.ready(ctx) {
				log.Printf("[INFO] LLM - %s en veille, bascule sur %s", llmProvider.name, m.providers[i+1].name)
				errs = append(errs, llmProvider.name+": en veille")
				continue
			}
		}

		resp, err := m.queryChat(ctx, llmProvider, retries, question, ctxText, history, onToken)
		if llmProvider.health != nil {
			if err == nil {
				llmProvider.health.markOK()
			} else {
				llmProvider.health.markDown()
			}
		}

		if err == nil {
			if i > 0 {
				log.Printf("[INFO] LLM - réponse fournie par %s (secours)", llmProvider.name)
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

		log.Printf("[WARN] LLM - %s indisponible, bascule sur %s : %v", llmProvider.name, m.providers[i+1].name, err)
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
	Model            string        `json:"model,omitempty"`
	Messages         []chatMessage `json:"messages"`
	MaxTokens        int           `json:"max_tokens"`
	Temperature      float64       `json:"temperature"`
	FrequencyPenalty float64       `json:"frequency_penalty"`
	Stream           bool          `json:"stream,omitempty"`
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

// buildMessages construit un seul message utilisateur, même en conversation :
// rejouer les échanges en tours user/assistant faisait imiter les réponses
// précédentes à un petit modèle, jusqu'à répondre « Oui » tout court.
// Seuls le sujet (questions précédentes) et la dernière réponse sont rappelés.
func buildMessages(question, ctxText string, history []entity.ChatTurn) []chatMessage {
	var b strings.Builder
	// Pas de consigne à l'impératif ici : un petit modèle y répond par « Ok, … ».
	// Le rappel de ne citer que les extraits reste dans le titre, au plus près de la question.
	fmt.Fprintf(&b, "Extraits du livret (seule source autorisée) :\n---\n%s\n---\n\n", ctxText)
	if len(history) > 0 {
		b.WriteString("Questions précédentes :\n")
		for _, turn := range history {
			b.WriteString("- " + turn.Question + "\n")
		}
		fmt.Fprintf(&b, "Dernière réponse donnée : %s\n\n", history[len(history)-1].Answer)
	}

	b.WriteString("Question : " + question)
	return []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: b.String()},
	}
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

func (m *ChatLLMAdapter) queryChat(ctx context.Context, llmProvider chatProvider, maxRetries int, question, ctxText string, history []entity.ChatTurn, onToken func(string)) (entity.LLMResponse, error) {
	if llmProvider.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, llmProvider.timeout)
		defer cancel()
	}

	body, _ := json.Marshal(chatRequest{
		Model:            llmProvider.model,
		Messages:         buildMessages(question, ctxText, history),
		MaxTokens:        llmMaxTokens,
		Temperature:      0.3,
		FrequencyPenalty: llmFrequencyPenalty,
		Stream:           onToken != nil,
	})

	headers := map[string]string{}
	if llmProvider.apiKey != "" {
		headers["Authorization"] = "Bearer " + llmProvider.apiKey
	}

	resp, err := postJSONWithRetry(ctx, m.client, llmProvider.pacer, llmProvider.name, llmProvider.url, headers, body, maxRetries)
	if err != nil {
		return entity.LLMResponse{}, err
	}

	defer resp.Body.Close()

	if onToken != nil {
		return readChatStream(llmProvider.name, resp.Body, onToken)
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.LLMResponse{}, fmt.Errorf("%s: %w", llmProvider.name, err)
	}

	if len(result.Choices) == 0 {
		return entity.LLMResponse{}, fmt.Errorf("%s: pas de réponse", llmProvider.name)
	}

	choice := result.Choices[0]
	answer, err := chatContentText(choice.Message.Content)

	if err != nil {
		return entity.LLMResponse{}, fmt.Errorf("%s: %w", llmProvider.name, err)
	}

	if strings.TrimSpace(answer) == "" {
		return entity.LLMResponse{}, fmt.Errorf("%s: réponse vide (finish_reason=%s)", llmProvider.name, choice.FinishReason)
	}

	if choice.FinishReason == "length" {
		log.Printf("[WARN] %s - réponse tronquée à %d tokens (id=%s)", llmProvider.name, llmMaxTokens, result.ID)
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
