// store/sections.go — Repository pour les sections
package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// Section représente une section (chunk) des règles d'un jeu.
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
	ID          string `json:"id"`
	GameID      string `json:"game_id"`
	Title       string `json:"title"`
	SectionType string `json:"section_type"`
	Text        string `json:"text"`
	Summary     string `json:"summary"`
	PageStart   *int   `json:"page_start,omitempty"`
	PageEnd     *int   `json:"page_end,omitempty"`
	SourceFile  string `json:"source_file"`
	// HierarchyPath : chapitre du livret (« COMBAT > Attaque »), vide si inconnu.
	HierarchyPath string                 `json:"hierarchy_path,omitempty"`
	ChunkIndex    int                    `json:"chunk_index"`
	Score         float64                `json:"score"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// SectionRepository définit les opérations sur les sections de règles.
type SectionRepository interface {
	// Insert insère une nouvelle section dans la base.
	Insert(ctx context.Context, section *Section) error

	// DeleteByGameID supprime toutes les sections d'un jeu.
	DeleteByGameID(ctx context.Context, gameID string) error

	// ListIDsByGameID retourne les IDs des sections d'un jeu.
	ListIDsByGameID(ctx context.Context, gameID string) ([]string, error)

	// DeleteByIDs supprime les sections dont l'ID est dans ids.
	DeleteByIDs(ctx context.Context, ids []string) error

	// VectorSearch effectue une recherche sémantique par embedding.
	// Retourne les sections les plus similaires triées par score décroissant.
	VectorSearch(ctx context.Context, gameID string, embedding []float64, limit int) ([]*ScoredSection, error)

	// FullTextSearch effectue une recherche full-text (PostgreSQL tsvector).
	// Retourne les sections correspondantes triées par pertinence.
	FullTextSearch(ctx context.Context, gameID string, query string, limit int) ([]*ScoredSection, error)

	// Neighbors retourne la section et ses voisines immédiates (chunk_index ± 1)
	// du même fichier et du même chapitre, dans l'ordre du livret.
	Neighbors(ctx context.Context, sec *ScoredSection) ([]*ScoredSection, error)
}

// sectionRepository implémente SectionRepository avec PostgreSQL.
type sectionRepository struct {
	pool *pgxpool.Pool
}

// NewSectionRepository crée un nouveau repository PostgreSQL pour les sections.
func NewSectionRepository(pool *pgxpool.Pool) SectionRepository {
	return &sectionRepository{pool: pool}
}

// Insert insère une nouvelle section dans la base.
func (r *sectionRepository) Insert(ctx context.Context, section *Section) error {
	query := `
		INSERT INTO sections (
			id, game_id, titre, type_section, contenu, resume, mecaniques, 
			embedding, page_debut, page_fin, fichier_source, hierarchy_path, chunk_index, total_chunks
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	// Garantir que Mechanics n'est jamais nil (contrainte NOT NULL)
	if section.Mechanics == nil {
		section.Mechanics = []string{}
	}

	// Convertir []float64 → pgvector.Vector
	vecData := make([]float32, len(section.Embedding))
	for i, v := range section.Embedding {
		vecData[i] = float32(v)
	}
	vec := pgvector.NewVector(vecData)

	_, err := r.pool.Exec(ctx, query,
		section.ID,
		section.GameID,
		section.Title,
		section.SectionType,
		section.Text,
		section.Summary,
		section.Mechanics,
		vec,
		section.PageStart,
		section.PageEnd,
		section.SourceFile,
		section.HierarchyPath,
		section.ChunkIndex,
		section.TotalChunks,
	)

	return err
}

// DeleteByGameID supprime toutes les sections d'un jeu.
func (r *sectionRepository) DeleteByGameID(ctx context.Context, gameID string) error {
	query := `DELETE FROM sections WHERE game_id = $1`
	_, err := r.pool.Exec(ctx, query, gameID)
	return err
}

// ListIDsByGameID retourne les IDs des sections d'un jeu.
func (r *sectionRepository) ListIDsByGameID(ctx context.Context, gameID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM sections WHERE game_id = $1`, gameID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// DeleteByIDs supprime les sections dont l'ID est dans ids.
func (r *sectionRepository) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM sections WHERE id = ANY($1)`, ids)
	return err
}

// scoredColumns : colonnes lues par scanScored, suivies du score.
const scoredColumns = `id, game_id, titre, type_section, contenu, resume,
		       page_debut, page_fin, fichier_source, hierarchy_path, chunk_index`

// VectorSearch effectue une recherche sémantique par embedding.
func (r *sectionRepository) VectorSearch(ctx context.Context, gameID string, embedding []float64, limit int) ([]*ScoredSection, error) {
	query := `
		SELECT ` + scoredColumns + `,
		       1 - (embedding <=> $1) AS score
		FROM sections
		WHERE game_id = $2
		ORDER BY embedding <=> $1
		LIMIT $3
	`

	// Convertir []float64 → []float32 pour pgvector
	vecData := make([]float32, len(embedding))
	for i, v := range embedding {
		vecData[i] = float32(v)
	}
	vec := pgvector.NewVector(vecData)

	return scanScored(r.pool.Query(ctx, query, vec, gameID, limit))
}

// FullTextSearch effectue une recherche full-text (PostgreSQL tsvector).
func (r *sectionRepository) FullTextSearch(ctx context.Context, gameID string, query string, limit int) ([]*ScoredSection, error) {
	sqlQuery := `
		SELECT ` + scoredColumns + `,
		       ts_rank(search_vector, websearch_to_tsquery('french', $1)) AS score
		FROM sections
		WHERE game_id = $2
		  AND search_vector @@ websearch_to_tsquery('french', $1)
		ORDER BY score DESC
		LIMIT $3
	`

	return scanScored(r.pool.Query(ctx, sqlQuery, query, gameID, limit))
}

// Neighbors retourne la section et ses voisines (chunk_index ± 1) du même
// fichier et du même chapitre, dans l'ordre du livret.
func (r *sectionRepository) Neighbors(ctx context.Context, sec *ScoredSection) ([]*ScoredSection, error) {
	query := `
		SELECT ` + scoredColumns + `, 0::float8 AS score
		FROM sections
		WHERE game_id = $1 AND fichier_source = $2 AND hierarchy_path = $3
		  AND chunk_index BETWEEN $4::int - 1 AND $4::int + 1
		ORDER BY chunk_index
	`

	return scanScored(r.pool.Query(ctx, query, sec.GameID, sec.SourceFile, sec.HierarchyPath, sec.ChunkIndex))
}

// scanScored lit les lignes SELECT scoredColumns, score.
func scanScored(rows pgx.Rows, err error) ([]*ScoredSection, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sections []*ScoredSection
	for rows.Next() {
		var sec ScoredSection

		err := rows.Scan(
			&sec.ID,
			&sec.GameID,
			&sec.Title,
			&sec.SectionType,
			&sec.Text,
			&sec.Summary,
			&sec.PageStart,
			&sec.PageEnd,
			&sec.SourceFile,
			&sec.HierarchyPath,
			&sec.ChunkIndex,
			&sec.Score,
		)
		if err != nil {
			return nil, err
		}

		sections = append(sections, &sec)
	}

	return sections, rows.Err()
}
