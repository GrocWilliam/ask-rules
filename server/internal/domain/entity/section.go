// domain/entity/section.go — Entité Section (domaine métier)
package entity

// Section représente une section de règles dans le domaine métier.
// Correspond à la table `sections` en base de données.
type Section struct {
	ID            string    `json:"id"`
	GameID        string    `json:"game_id"`
	Title         string    `json:"title"`        // colonne PG: titre
	SectionType   string    `json:"section_type"` // colonne PG: type_section
	Text          string    `json:"text"`         // colonne PG: contenu
	Summary       string    `json:"summary"`      // colonne PG: resume
	Mechanics     []string  `json:"mechanics"`    // colonne PG: mecaniques
	Embedding     []float64 `json:"embedding,omitempty"`
	PageStart     *int      `json:"page_start,omitempty"` // colonne PG: page_debut
	PageEnd       *int      `json:"page_end,omitempty"`   // colonne PG: page_fin
	SourceFile    string    `json:"source_file"`          // colonne PG: fichier_source (chemin relatif à UPLOADS_DIR)
	HierarchyPath string    `json:"hierarchy_path"`       // colonne PG: hierarchy_path
	ChunkIndex    int       `json:"chunk_index"`
	TotalChunks   int       `json:"total_chunks"`
}

// ScoredSection est une section avec un score de pertinence.
// Utilisée pour les résultats de recherche (vectorielle ou full-text).
type ScoredSection struct {
	ID          string                 `json:"id"`
	GameID      string                 `json:"game_id"`
	Title       string                 `json:"title"`
	SectionType string                 `json:"section_type"`
	Text        string                 `json:"text"`
	Summary     string                 `json:"summary"`
	PageStart   *int                   `json:"page_start,omitempty"`
	PageEnd     *int                   `json:"page_end,omitempty"`
	SourceFile  string                 `json:"source_file"`
	Score       float64                `json:"score"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
