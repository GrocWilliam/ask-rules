// entity/api.go — DTOs pour l'API REST
package entity

// AskRequest est la requête pour poser une question.
type AskRequest struct {
	Question string `json:"question"`
	Jeu      string `json:"jeu,omitempty"`
	JeuID    string `json:"jeu_id,omitempty"`
}

// AskResponse est la réponse à une question.
type AskResponse struct {
	OK       bool            `json:"ok"`
	Jeu      string          `json:"jeu,omitempty"`
	JeuID    string          `json:"jeu_id,omitempty"`
	Answer   string          `json:"answer,omitempty"`
	UsedLLM  bool            `json:"used_llm,omitempty"`
	Model    string          `json:"model,omitempty"`
	Sections []SectionResult `json:"sections,omitempty"`
	Cached   bool            `json:"cached,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// SectionResult est une section retournée dans l'API.
type SectionResult struct {
	Title       string  `json:"title"`
	SectionType string  `json:"section_type"`
	Summary     string  `json:"summary"`
	Text        string  `json:"text"`
	Score       float64 `json:"score"`
	PageStart   *int    `json:"page_start,omitempty"`
}
