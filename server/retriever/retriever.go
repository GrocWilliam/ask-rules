// retriever/retriever.go — Recherche hybride (vectorielle + plein texte)
package retriever

import (
	"context"
	"sort"
	"strings"

	"ask-rules-server/db"
	"ask-rules-server/embedder"
	"ask-rules-server/models"
	"ask-rules-server/nlp"
)

const (
	vectorWeight  = 0.65
	textWeight    = 0.35
	defaultLimit  = 6
	minScore      = 0.1
)

// SearchOptions paramètres de recherche.
type SearchOptions struct {
	GameID   string
	Question string
	Limit    int
}

// Search effectue une recherche hybride et retourne les sections les plus pertinentes.
func Search(ctx context.Context, opts SearchOptions) ([]models.ScoredSection, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	// Embed la question
	vec, embErr := embedder.Embed(ctx, opts.Question)

	var vectorResults, textResults []models.ScoredSection

	// Recherche vectorielle
	if embErr == nil && vec != nil {
		f64vec := make([]float64, len(vec))
		for i, f := range vec {
			f64vec[i] = float64(f)
		}
		vr, err := db.VectorSearch(ctx, opts.GameID, f64vec, limit*2)
		if err == nil {
			vectorResults = vr
		}
	}

	// Recherche plein texte
	keywords := buildSearchQuery(opts.Question)
	if keywords != "" {
		tr, err := db.FullTextSearch(ctx, opts.GameID, keywords, limit*2)
		if err == nil {
			textResults = tr
		}
	}

	merged := mergeResults(vectorResults, textResults, limit)
	return merged, nil
}

// buildSearchQuery construit une requête texte depuis les mots-clés.
func buildSearchQuery(question string) string {
	keywords := nlp.ExtractKeywords(question)
	if len(keywords) == 0 {
		return question
	}
	// Prendre les 5 premiers mots-clés
	if len(keywords) > 5 {
		keywords = keywords[:5]
	}
	return strings.Join(keywords, " & ")
}

// mergeResults fusionne les résultats vectoriels et textuels avec pondération.
func mergeResults(vecResults, textResults []models.ScoredSection, limit int) []models.ScoredSection {
	scores := map[string]float64{}
	sectionMap := map[string]models.ScoredSection{}

	// Pondérer résultats vectoriels
	for i, s := range vecResults {
		rrScore := 1.0 / float64(i+1+10) // Reciprocal Rank Fusion
		scores[s.ID] += vectorWeight * (s.Score + rrScore)
		sectionMap[s.ID] = s
	}

	// Pondérer résultats textuels
	for i, s := range textResults {
		rrScore := 1.0 / float64(i+1+10)
		scores[s.ID] += textWeight * (s.Score + rrScore)
		if _, exists := sectionMap[s.ID]; !exists {
			sectionMap[s.ID] = s
		}
	}

	// Construire la liste triée
	var merged []models.ScoredSection
	for id, score := range scores {
		if score < minScore {
			continue
		}
		s := sectionMap[id]
		s.Score = score
		merged = append(merged, s)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
