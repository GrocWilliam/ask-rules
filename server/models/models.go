// models/models.go — Types partagés (schéma PostgreSQL existant)
package models

import "time"

// ── Jeux ────────────────────────────────────────────────────────────────────

// Game correspond à la table `games`.
// Les colonnes PG sont en snake_case ; on utilise des tags JSON pour l'API.
type Game struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`      // colonne: jeu
	FilePath string                 `json:"file_path"` // colonne: fichier
	AddedAt  time.Time              `json:"added_at"`  // colonne: date_ajout
	Metadata map[string]interface{} `json:"metadata"`
	Stats    map[string]interface{} `json:"stats"` // colonne: statistiques
	Gameplay map[string]interface{} `json:"gameplay"`
}

type GameWithStats struct {
	Game
	SectionsCount int `json:"sections_count"`
}

// ── Sections ─────────────────────────────────────────────────────────────────

// Section correspond à la table `sections`.
type Section struct {
	ID            string    `json:"id"`
	GameID        string    `json:"game_id"`
	Title         string    `json:"title"`        // colonne: titre
	SectionType   string    `json:"section_type"` // colonne: type_section
	Text          string    `json:"text"`         // colonne: contenu
	Summary       string    `json:"summary"`      // colonne: resume
	Mechanics     []string  `json:"mechanics"`    // colonne: mecaniques — inclus dans search_vector (poids B)
	Embedding     []float64 `json:"embedding,omitempty"`
	PageStart     *int      `json:"page_start,omitempty"` // colonne: page_debut
	PageEnd       *int      `json:"page_end,omitempty"`   // colonne: page_fin
	HierarchyPath string    `json:"hierarchy_path"`       // colonne: hierarchy_path — type de section pour FTS (poids B)
	ChunkIndex    int       `json:"chunk_index"`
	TotalChunks   int       `json:"total_chunks"`
}

// ScoredSection est une section avec un score de pertinence (résultat de recherche).
type ScoredSection struct {
	ID          string                 `json:"id"`
	GameID      string                 `json:"game_id"`
	Title       string                 `json:"title"`
	SectionType string                 `json:"section_type"`
	Text        string                 `json:"text"`
	Summary     string                 `json:"summary"`
	PageStart   *int                   `json:"page_start,omitempty"`
	PageEnd     *int                   `json:"page_end,omitempty"`
	Score       float64                `json:"score"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ── LLM ──────────────────────────────────────────────────────────────────────

type LLMResponse struct {
	Answer  string      `json:"answer"`
	Model   string      `json:"model"`
	UsedLLM bool        `json:"used_llm"`
	Tokens  *TokenUsage `json:"tokens,omitempty"`
}

type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// ── Requêtes / Réponses API ──────────────────────────────────────────────────

type AskRequest struct {
	Question string `json:"question"`
	Jeu      string `json:"jeu,omitempty"`
	JeuID    string `json:"jeu_id,omitempty"`
}

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

type SectionResult struct {
	Title       string  `json:"title"`
	SectionType string  `json:"section_type"`
	Summary     string  `json:"summary"`
	Text        string  `json:"text"`
	Score       float64 `json:"score"`
	PageStart   *int    `json:"page_start,omitempty"`
}

// ── Logs ─────────────────────────────────────────────────────────────────────

// LogEntry correspond à la table `logs`.
type LogEntry struct {
	ID        int                    `json:"id"`
	EventType string                 `json:"event_type"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// ── Import SSE ───────────────────────────────────────────────────────────────

type SSEEvent struct {
	Type    string                 `json:"type"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Message string                 `json:"message,omitempty"`
}
