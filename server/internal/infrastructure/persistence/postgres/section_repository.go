// infrastructure/persistence/postgres/section_repository.go — Repository pour les sections
package postgres

import (
	"context"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// SectionRepositoryImpl implémente repository.SectionRepository avec PostgreSQL.
type SectionRepositoryImpl struct {
	pool *pgxpool.Pool
}

// NewSectionRepository crée un nouveau repository PostgreSQL pour les sections.
func NewSectionRepository(pool *pgxpool.Pool) repository.SectionRepository {
	return &SectionRepositoryImpl{pool: pool}
}

// Insert insère une nouvelle section dans la base.
func (r *SectionRepositoryImpl) Insert(ctx context.Context, section *entity.Section) error {
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
func (r *SectionRepositoryImpl) DeleteByGameID(ctx context.Context, gameID string) error {
	query := `DELETE FROM sections WHERE game_id = $1`
	_, err := r.pool.Exec(ctx, query, gameID)
	return err
}

// scoredColumns : colonnes lues par scanScored, suivies du score.
const scoredColumns = `id, game_id, titre, type_section, contenu, resume,
		       page_debut, page_fin, fichier_source, hierarchy_path, chunk_index`

// VectorSearch effectue une recherche sémantique par embedding.
func (r *SectionRepositoryImpl) VectorSearch(ctx context.Context, gameID string, embedding []float64, limit int) ([]*entity.ScoredSection, error) {
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
func (r *SectionRepositoryImpl) FullTextSearch(ctx context.Context, gameID string, query string, limit int) ([]*entity.ScoredSection, error) {
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
func (r *SectionRepositoryImpl) Neighbors(ctx context.Context, sec *entity.ScoredSection) ([]*entity.ScoredSection, error) {
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
func scanScored(rows pgx.Rows, err error) ([]*entity.ScoredSection, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sections []*entity.ScoredSection
	for rows.Next() {
		var sec entity.ScoredSection

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
