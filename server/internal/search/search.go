// search/search.go — Recherche hybride (vectorielle + plein texte)
package search

import (
	"context"
	"log"
	"sort"
	"strings"

	"ask-rules-server/internal/nlp"
	"ask-rules-server/internal/store"
)

const (
	defaultLimit = 4
	// rrfK : constante de Reciprocal Rank Fusion (valeur standard de la littérature).
	rrfK = 60
	// minCandidates : nombre minimum de candidats demandés à chaque recherche.
	// Une section bien classée dans une seule des deux listes doit pouvoir
	// apparaître dans l'autre pour recevoir les deux votes RRF : à 20, une
	// section 3e en vectoriel mais 28e en plein texte était écartée (cmd/eval).
	minCandidates = 40
	// defaultMinVectorScore : similarité cosinus en dessous de laquelle un
	// résultat vectoriel est écarté (SearchOptions.MinVectorScore). E5 produit
	// des cosinus élevés même pour des textes sans rapport (~0.7), le seuil ne
	// filtre que le bruit évident. Il dépend du modèle : d'autres (Arctic,
	// Bekko) donnent ~0.6 au meilleur résultat, 0.72 les écarterait tous.
	defaultMinVectorScore = 0.72
	// pageDecay : pénalité par page du score fusionné. La règle de référence
	// est le plus souvent au début du livret, les pages suivantes la détaillent
	// ou la répètent (exemples, résumé). 0.005 : page 10 ≈ -3 rangs, page 30 ≈ -9.
	// ponytail: page absolue, ignore la longueur du livret ; à régler avec cmd/eval.
	pageDecay = 0.005
)

// Embedder calcule le vecteur d'une question.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Retriever recherche les sections d'un jeu les plus pertinentes pour une question.
type Retriever struct {
	Embedder Embedder
	Sections store.SectionRepository
	// MinVectorScore : seuil de similarité des résultats vectoriels
	// (0 : defaultMinVectorScore, négatif : aucun seuil).
	MinVectorScore float64
}

// Search effectue une recherche hybride et retourne les sections les plus pertinentes.
func (r *Retriever) Search(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error) {
	if limit <= 0 {
		limit = defaultLimit
	}

	// Embed la question
	vec, embErr := r.Embedder.Embed(ctx, question)

	var vectorResults, textResults []store.ScoredSection

	// Recherche vectorielle
	if embErr == nil && vec != nil {
		f64vec := make([]float64, len(vec))
		for i, f := range vec {
			f64vec[i] = float64(f)
		}
		vr, err := r.Sections.VectorSearch(ctx, gameID, f64vec, candidates(limit))
		if err != nil {
			log.Printf("[retriever] VectorSearch: %v", err)
		} else {
			for _, s := range vr {
				vectorResults = append(vectorResults, *s)
			}
		}
	} else if embErr != nil {
		log.Printf("[retriever] Embed: %v", embErr)
	}

	// Recherche plein texte (OR sur les mots-clés)
	keywords := buildSearchQuery(question)
	if keywords != "" {
		tr, err := r.Sections.FullTextSearch(ctx, gameID, keywords, candidates(limit))
		if err != nil {
			log.Printf("[retriever] FullTextSearch: %v", err)
		} else {
			for _, s := range tr {
				textResults = append(textResults, *s)
			}
		}
	}

	minScore := r.MinVectorScore
	if minScore == 0 {
		minScore = defaultMinVectorScore
	}
	merged := expandTop(ctx, r.Sections, mergeResults(vectorResults, textResults, limit, minScore))

	result := make([]*store.ScoredSection, len(merged))
	for i := range merged {
		result[i] = &merged[i]
	}
	return result, nil
}

// expandTopN : nombre de meilleures sections complétées par leurs voisines.
const expandTopN = 2

// expandTop complète les expandTopN meilleures sections avec leurs voisines du
// même chapitre : une réponse (« quelles sont les phases ? », les deux effets
// d'une tuile) s'étale souvent sur plusieurs chunks, le meilleur n'en contient
// qu'une partie. Un chunk n'apparaît qu'une fois : les voisines déjà utilisées
// ou déjà présentes dans les résultats en sont retirées.
func expandTop(ctx context.Context, repo store.SectionRepository, sections []store.ScoredSection) []store.ScoredSection {
	used := map[string]bool{}
	var result []store.ScoredSection
	for _, s := range sections {
		if used[s.ID] {
			continue // déjà inclus comme voisine d'une section mieux classée
		}
		used[s.ID] = true
		if len(result) < expandTopN {
			s = expand(ctx, repo, s, used)
		}
		result = append(result, s)
	}
	return result
}

// expand ajoute à s ses voisines non encore utilisées, dans l'ordre du livret.
func expand(ctx context.Context, repo store.SectionRepository, s store.ScoredSection, used map[string]bool) store.ScoredSection {
	if s.HierarchyPath == "" {
		return s // sans chapitre connu, les voisines peuvent parler d'autre chose
	}
	neighbors, err := repo.Neighbors(ctx, &s)
	if err != nil {
		log.Printf("[retriever] Neighbors: %v", err)
		return s
	}

	var texts []string
	for _, n := range neighbors {
		if n.ID != s.ID && used[n.ID] {
			continue
		}
		used[n.ID] = true
		texts = append(texts, n.Text)
		if len(texts) == 1 {
			s.PageStart = n.PageStart
		}
		s.PageEnd = n.PageEnd
	}
	if len(texts) > 0 {
		s.Text = strings.Join(texts, "\n\n")
	}
	return s
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
func mergeResults(vecResults, textResults []store.ScoredSection, limit int, minVectorScore float64) []store.ScoredSection {
	scores := map[string]float64{}
	sectionMap := map[string]store.ScoredSection{}

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
	merged := make([]store.ScoredSection, 0, len(scores))
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
