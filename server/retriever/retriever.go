// retriever/retriever.go — Recherche hybride (vectorielle + plein texte)
package retriever

import (
	"context"
	"log"
	"sort"
	"strings"

	"ask-rules-server/db"
	"ask-rules-server/embedder"
	"ask-rules-server/models"
	"ask-rules-server/nlp"
)

const (
	vectorWeight = 0.65
	textWeight   = 0.35
	defaultLimit = 4
	minScore     = 0.50 // seuil de pertinence : seules les sections à ≥ 50% sont retenues
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
		if err != nil {
			log.Printf("[retriever] VectorSearch: %v", err)
		} else {
			vectorResults = vr
		}
	} else if embErr != nil {
		log.Printf("[retriever] Embed: %v", embErr)
	}

	// Recherche plein texte (OR sur les mots-clés)
	keywords := buildSearchQuery(opts.Question)
	if keywords != "" {
		tr, err := db.FullTextSearch(ctx, opts.GameID, keywords, limit*2)
		if err != nil {
			log.Printf("[retriever] FullTextSearch: %v", err)
		} else {
			textResults = tr
		}
	}

	merged := mergeResults(vectorResults, textResults, limit)
	return merged, nil
}

// buildSearchQuery construit une requête to_tsquery avec OR (|) pour être permissif.
// Chaque mot-clé est séparé par " | " : une section est retournée dès qu'elle
// contient AU MOINS UN des mots-clés (classement par ts_rank ensuite).
func buildSearchQuery(question string) string {
	keywords := nlp.ExtractKeywords(question)
	if len(keywords) == 0 {
		// Fallback : tous les mots de la question (plainto_tsquery tolère n'importe quoi)
		return question
	}
	// Prendre les 8 premiers mots-clés
	if len(keywords) > 8 {
		keywords = keywords[:8]
	}
	// Échapper les caractères spéciaux pour to_tsquery
	for i, k := range keywords {
		keywords[i] = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1 // supprimer tout caractère non alphanumérique
		}, k)
	}
	// Filtrer les mots vides après nettoyage
	var clean []string
	for _, k := range keywords {
		if len(k) >= 2 {
			clean = append(clean, k)
		}
	}
	if len(clean) == 0 {
		return question
	}
	return strings.Join(clean, " | ")
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
