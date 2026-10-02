// entity/llm.go — Structures LLM du domaine
package entity

// LLMResponse est la réponse d'un modèle de langage.
type LLMResponse struct {
	Answer  string      `json:"answer"`
	Model   string      `json:"model"`
	UsedLLM bool        `json:"used_llm"`
	Tokens  *TokenUsage `json:"tokens,omitempty"`
}

// TokenUsage comptabilise les tokens utilisés lors d'un appel LLM.
type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// ChatTurn est un échange précédent de la conversation (question + réponse),
// renvoyé au LLM pour qu'il comprenne les questions de suivi.
type ChatTurn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
