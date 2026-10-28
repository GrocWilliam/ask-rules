// cmd/eval/memory_vectors.go — Recherche vectorielle en mémoire pour comparer des modèles d'embedding
package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"ask-rules-server/internal/embed"
	"ask-rules-server/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

// memoryVectors remplace la recherche vectorielle de la base par des vecteurs
// calculés en mémoire avec un autre modèle (-model) : on compare des modèles
// sans réindexer ni modifier le schéma (vector(384)). Plein texte et voisines
// restent ceux de la base.
type memoryVectors struct {
	store.SectionRepository
	sections map[string][]memorySection // par game_id
}

type memorySection struct {
	section store.ScoredSection
	vector  []float32
}

// newMemoryVectors lit les sections des jeux et calcule leurs vecteurs avec
// embedder, sur le même texte que le pipeline d'import (chapitre + contenu).
func newMemoryVectors(ctx context.Context, pool *pgxpool.Pool, repo store.SectionRepository,
	embedder *embed.Embedder, gameIDs []string) (*memoryVectors, error) {
	m := &memoryVectors{SectionRepository: repo, sections: map[string][]memorySection{}}
	for _, gameID := range gameIDs {
		rows, err := pool.Query(ctx, `
			SELECT id, game_id, titre, type_section, contenu, resume,
			       page_debut, page_fin, fichier_source, hierarchy_path, chunk_index
			FROM sections
			WHERE game_id = $1 AND embedding IS NOT NULL`, gameID)
		if err != nil {
			return nil, err
		}
		var sections []store.ScoredSection
		for rows.Next() {
			var s store.ScoredSection
			if err := rows.Scan(&s.ID, &s.GameID, &s.Title, &s.SectionType, &s.Text, &s.Summary,
				&s.PageStart, &s.PageEnd, &s.SourceFile, &s.HierarchyPath, &s.ChunkIndex); err != nil {
				rows.Close()
				return nil, err
			}
			sections = append(sections, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		start := time.Now()
		for _, s := range sections {
			text := s.Text
			if s.HierarchyPath != "" {
				text = s.HierarchyPath + "\n" + s.Text
			}
			vec, err := embedder.EmbedPassage(ctx, text)
			if err != nil {
				return nil, fmt.Errorf("embedding section %s : %w", s.ID, err)
			}
			m.sections[gameID] = append(m.sections[gameID], memorySection{section: s, vector: vec})
		}
		if len(sections) > 0 {
			elapsed := time.Since(start)
			log.Printf("%d sections de %s vectorisées en %s (%s par section)", len(sections), gameID,
				elapsed.Round(time.Second), (elapsed / time.Duration(len(sections))).Round(time.Millisecond))
		}
	}
	return m, nil
}

// VectorSearch classe les sections du jeu par similarité cosinus (vecteurs normalisés).
func (m *memoryVectors) VectorSearch(_ context.Context, gameID string, embedding []float64, limit int) ([]*store.ScoredSection, error) {
	var results []*store.ScoredSection
	for _, ms := range m.sections[gameID] {
		if len(ms.vector) != len(embedding) {
			return nil, fmt.Errorf("dimension %d au lieu de %d", len(embedding), len(ms.vector))
		}
		var dot float64
		for i, v := range ms.vector {
			dot += float64(v) * embedding[i]
		}
		s := ms.section
		s.Score = dot
		results = append(results, &s)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
