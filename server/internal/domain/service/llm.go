// domain/service/llm.go — Interface service pour les LLM
package service

import (
	"context"
)

// LLMService génère des réponses à partir d'un contexte et d'une question.
// Implémentation : toute API compatible OpenAI (llama.cpp, Mistral, Ollama…).
type LLMService interface {
	// Query génère une réponse à partir de la question et du contexte fourni.
	Query(ctx context.Context, question, context string) (*LLMResponse, error)

	// Warmup réveille en arrière-plan un LLM mis en veille (sans bloquer).
	Warmup()

	// ModelName retourne le nom du modèle utilisé (ex: "mistral-small-latest").
	ModelName() string
}

// LLMResponse représente la réponse d'un LLM.
type LLMResponse struct {
	// Answer est la réponse générée par le LLM.
	Answer string

	// Model est le nom du modèle utilisé.
	Model string

	// UsedLLM indique si un LLM a été utilisé (false = fallback).
	UsedLLM bool

	// TokensUsed compte les tokens consommés (input + output).
	TokensUsed *TokenUsage
}

// TokenUsage représente la consommation de tokens.
type TokenUsage struct {
	Input  int
	Output int
	Total  int
}
