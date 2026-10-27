// domain/service/retriever/retriever.go — Recherche hybride (vectorielle + plein texte)
package retriever

import (
	"context"
	"log"
	"sort"
	"strings"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/domain/service/nlp"
)

const (
	defaultLimit = 4
	// rrfK : constante de Reciprocal Rank Fusion (valeur standard de la littérature).
	rrfK = 60
	// minCandidates : nombre minimum de candidats demandés à chaque recherche.
	minCandidates = 20
	// minVectorScore : similarité cosinus en dessous de laquelle un résultat
	// uniquement vectoriel est écarté. E5 produit des cosinus élevés même pour
	// des textes sans rapport (~0.7), le seuil ne filtre que le bruit évident.
	minVectorScore = 0.72
	// pageDecay : pénalité par page du score fusionné. La règle de référence
	// est le plus souvent au début du livret, les pages suivantes la détaillent
	// ou la répètent (exemples, résumé). 0.005 : page 10 ≈ -3 rangs, page 30 ≈ -9.
	// ponytail: page absolue, ignore la longueur du livret ; à régler avec cmd/eval.
	pageDecay = 0.005
)

// SearchOptions paramètres de recherche.
type SearchOptions struct {
	GameID      string
	Question    string
	Limit       int
	Embedder    service.EmbedderService
	SectionRepo repository.SectionRepository
}

// Search effectue une recherche hybride et retourne les sections les plus pertinentes.
func Search(ctx context.Context, opts SearchOptions) ([]entity.ScoredSection, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	// Embed la question
	vec, embErr := opts.Embedder.Embed(ctx, opts.Question)

	var vectorResults, textResults []entity.ScoredSection

	// Recherche vectorielle
	if embErr == nil && vec != nil {
		f64vec := make([]float64, len(vec))
		for i, f := range vec {
			f64vec[i] = float64(f)
		}
		vr, err := opts.SectionRepo.VectorSearch(ctx, opts.GameID, f64vec, candidates(limit))
		if err != nil {
			log.Printf("[retriever] VectorSearch: %v", err)
		} else {
			// Convertir []*entity.ScoredSection en []entity.ScoredSection
			for _, s := range vr {
				vectorResults = append(vectorResults, *s)
			}
		}
	} else if embErr != nil {
		log.Printf("[retriever] Embed: %v", embErr)
	}

	// Recherche plein texte (OR sur les mots-clés)
	keywords := buildSearchQuery(opts.Question)
	if keywords != "" {
		tr, err := opts.SectionRepo.FullTextSearch(ctx, opts.GameID, keywords, candidates(limit))
		if err != nil {
			log.Printf("[retriever] FullTextSearch: %v", err)
		} else {
			// Convertir []*entity.ScoredSection en []entity.ScoredSection
			for _, s := range tr {
				textResults = append(textResults, *s)
			}
		}
	}

	merged := mergeResults(vectorResults, textResults, limit)
	return expandTop(ctx, opts.SectionRepo, merged), nil
}

// expandTop complète la meilleure section avec ses voisines du même chapitre :
// une question générale (« quelles sont les phases ? ») porte sur un chapitre
// découpé en plusieurs chunks, le meilleur n'en contient qu'une partie.
// Les voisines déjà présentes dans les résultats en sont retirées.
func expandTop(ctx context.Context, repo repository.SectionRepository, sections []entity.ScoredSection) []entity.ScoredSection {
	if len(sections) == 0 || sections[0].HierarchyPath == "" {
		return sections // sans chapitre connu, les voisines peuvent parler d'autre chose
	}
	neighbors, err := repo.Neighbors(ctx, &sections[0])
	if err != nil {
		log.Printf("[retriever] Neighbors: %v", err)
		return sections
	}
	if len(neighbors) < 2 {
		return sections
	}

	top := sections[0]
	ids := map[string]bool{}
	texts := make([]string, len(neighbors))
	for i, n := range neighbors {
		ids[n.ID] = true
		texts[i] = n.Text
	}
	top.Text = strings.Join(texts, "\n\n")
	top.PageStart = neighbors[0].PageStart
	top.PageEnd = neighbors[len(neighbors)-1].PageEnd

	result := []entity.ScoredSection{top}
	for _, s := range sections[1:] {
		if !ids[s.ID] {
			result = append(result, s)
		}
	}
	return result
}

func candidates(limit int) int {
	if limit*4 > minCandidates {
		return limit * 4
	}
	return minCandidates
}

// buildSearchQuery construit une requête websearch_to_tsquery en OR : une section
// est retournée dès qu'elle contient AU MOINS UN des termes (classement par
// ts_rank ensuite). Les accents sont conservés pour que le stemmer 'french'
// produise les mêmes lexèmes que ceux indexés.
func buildSearchQuery(question string) string {
	terms := nlp.ExtractSearchTerms(question)
	if len(terms) == 0 {
		return question
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return strings.Join(terms, " or ")
}

// mergeResults fusionne les deux classements par Reciprocal Rank Fusion.
// Les scores bruts (cosinus vs ts_rank) ne sont pas comparables : seul le rang
// compte. Le score final est ramené dans [0, 1] (1 = premier dans les deux listes).
func mergeResults(vecResults, textResults []entity.ScoredSection, limit int) []entity.ScoredSection {
	scores := map[string]float64{}
	sectionMap := map[string]entity.ScoredSection{}

	for i, s := range vecResults {
		if s.Score < minVectorScore {
			break // résultats triés par similarité décroissante
		}
		scores[s.ID] += 1.0 / float64(rrfK+i+1)
		sectionMap[s.ID] = s
	}

	for i, s := range textResults {
		scores[s.ID] += 1.0 / float64(rrfK+i+1)
		if _, exists := sectionMap[s.ID]; !exists {
			sectionMap[s.ID] = s
		}
	}

	maxScore := 2.0 / float64(rrfK+1)
	merged := make([]entity.ScoredSection, 0, len(scores))
	for id, score := range scores {
		s := sectionMap[id]
		s.Score = score / maxScore * pageFactor(s.PageStart)
		merged = append(merged, s)
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].ID < merged[j].ID
	})

	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// pageFactor favorise légèrement les premières pages du livret (voir pageDecay).
func pageFactor(page *int) float64 {
	if page == nil || *page <= 1 {
		return 1
	}
	return 1 / (1 + pageDecay*float64(*page-1))
}
