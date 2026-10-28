// llm/types.go — Historique, réponse et consommation de tokens
package llm

// Response est la réponse d'un modèle de langage.
type Response struct {
	Answer string `json:"answer"`
	Model  string `json:"model"`
	Tokens *Usage `json:"tokens,omitempty"`
}

// Usage comptabilise les tokens utilisés lors d'un appel LLM.
type Usage struct {
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
