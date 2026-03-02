// llm/llm.go — Client LLM (Mistral, OpenAI, Ollama)
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ask-rules-server/config"
	"ask-rules-server/models"
)

var systemPrompt = `Tu es un assistant expert en jeux de société. Tu dois répondre uniquement en te basant sur les informations du contexte fourni.

Règles importantes :
- Réponds toujours en français
- Cite des règles précises quand tu les énonces
- Si l'information n'est pas dans le contexte, dis-le clairement plutôt qu'inventer
- Structure ta réponse de façon claire avec des listes si nécessaire
- Sois précis et concis`

func Query(ctx context.Context, question, ctxText string) (models.LLMResponse, error) {
	switch {
	case config.C.MistralAPIKey != "":
		return queryMistral(ctx, question, ctxText)
	case config.C.OpenAIAPIKey != "":
		return queryOpenAI(ctx, question, ctxText)
	case config.C.OllamaHost != "":
		return queryOllama(ctx, question, ctxText)
	default:
		return models.LLMResponse{Answer: ctxText, UsedLLM: false}, nil
	}
}

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

func queryMistral(ctx context.Context, question, ctxText string) (models.LLMResponse, error) {
	prompt := fmt.Sprintf("Contexte du jeu :\n%s\n\nQuestion : %s", ctxText, question)
	reqBody := mistralRequest{
		Model: config.C.MistralModel,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   1024,
		Temperature: 0.3,
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.mistral.ai/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return models.LLMResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.C.MistralAPIKey)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return models.LLMResponse{}, fmt.Errorf("mistral: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return models.LLMResponse{}, fmt.Errorf("mistral status %d: %s", resp.StatusCode, b)
	}
	var result mistralResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return models.LLMResponse{}, err
	}
	if len(result.Choices) == 0 {
		return models.LLMResponse{}, fmt.Errorf("mistral: pas de réponse")
	}
	tokens := &models.TokenUsage{
		Prompt:     result.Usage.PromptTokens,
		Completion: result.Usage.CompletionTokens,
		Total:      result.Usage.TotalTokens,
	}
	return models.LLMResponse{
		Answer:  result.Choices[0].Message.Content,
		Model:   result.Model,
		UsedLLM: true,
		Tokens:  tokens,
	}, nil
}

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

func queryOpenAI(ctx context.Context, question, ctxText string) (models.LLMResponse, error) {
	prompt := fmt.Sprintf("Contexte du jeu :\n%s\n\nQuestion : %s", ctxText, question)
	reqBody := openAIRequest{
		Model: config.C.OpenAIModel,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   1024,
		Temperature: 0.3,
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return models.LLMResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.C.OpenAIAPIKey)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return models.LLMResponse{}, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return models.LLMResponse{}, fmt.Errorf("openai status %d: %s", resp.StatusCode, b)
	}
	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return models.LLMResponse{}, err
	}
	if len(result.Choices) == 0 {
		return models.LLMResponse{}, fmt.Errorf("openai: pas de réponse")
	}
	tokens := &models.TokenUsage{
		Prompt:     result.Usage.PromptTokens,
		Completion: result.Usage.CompletionTokens,
		Total:      result.Usage.TotalTokens,
	}
	return models.LLMResponse{
		Answer:  result.Choices[0].Message.Content,
		Model:   result.Model,
		UsedLLM: true,
		Tokens:  tokens,
	}, nil
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Model    string `json:"model"`
}

func queryOllama(ctx context.Context, question, ctxText string) (models.LLMResponse, error) {
	prompt := fmt.Sprintf("%s\n\nContexte du jeu :\n%s\n\nQuestion : %s", systemPrompt, ctxText, question)
	reqBody := ollamaRequest{
		Model:  config.C.OllamaModel,
		Prompt: prompt,
		Stream: false,
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST",
		config.C.OllamaHost+"/api/generate",
		bytes.NewReader(body))
	if err != nil {
		return models.LLMResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return models.LLMResponse{}, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return models.LLMResponse{}, fmt.Errorf("ollama status %d: %s", resp.StatusCode, b)
	}
	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return models.LLMResponse{}, err
	}
	return models.LLMResponse{
		Answer:  result.Response,
		Model:   result.Model,
		UsedLLM: true,
	}, nil
}
