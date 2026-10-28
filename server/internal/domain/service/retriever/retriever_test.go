package retriever

import (
	"context"
	"fmt"
	"testing"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
)

func sec(id string, score float64) entity.ScoredSection {
	s := entity.ScoredSection{Score: score}
	s.ID = id
	return s
}

func TestMergeResults_TextOnlyHitsAreKept(t *testing.T) {
	// ts_rank renvoie des scores très faibles : ils ne doivent pas être filtrés
	vec := []entity.ScoredSection{sec("a", 0.85), sec("b", 0.80)}
	text := []entity.ScoredSection{sec("c", 0.02)}

	got := mergeResults(vec, text, 4)
	if len(got) != 3 {
		t.Fatalf("attendu 3 sections, obtenu %d", len(got))
	}
}

func TestMergeResults_BothListsRankFirst(t *testing.T) {
	vec := []entity.ScoredSection{sec("a", 0.90), sec("b", 0.88)}
	text := []entity.ScoredSection{sec("b", 0.30), sec("a", 0.10)}

	got := mergeResults(vec, text, 4)
	if got[0].Score != got[1].Score {
		t.Fatalf("a et b ont des rangs symétriques, scores attendus égaux : %v / %v", got[0].Score, got[1].Score)
	}

	vec = []entity.ScoredSection{sec("a", 0.90), sec("b", 0.88)}
	text = []entity.ScoredSection{sec("a", 0.30)}
	got = mergeResults(vec, text, 4)
	if got[0].ID != "a" || got[0].Score != 1 {
		t.Fatalf("a premier dans les deux listes : attendu score 1, obtenu %s=%v", got[0].ID, got[0].Score)
	}
}

func TestMergeResults_LowCosineDropped(t *testing.T) {
	vec := []entity.ScoredSection{sec("a", 0.85), sec("noise", 0.60)}

	got := mergeResults(vec, nil, 4)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("le résultat vectoriel sous le seuil doit être écarté : %+v", got)
	}
}

func TestMergeResults_Limit(t *testing.T) {
	vec := []entity.ScoredSection{sec("a", 0.9), sec("b", 0.9), sec("c", 0.9)}
	if got := mergeResults(vec, nil, 2); len(got) != 2 {
		t.Fatalf("attendu 2 sections, obtenu %d", len(got))
	}
}

func TestMergeResults_EarlierPageWinsCloseRanks(t *testing.T) {
	p := func(v int) *int { return &v }
	late, early := sec("late", 0.9), sec("early", 0.89)
	late.PageStart, early.PageStart = p(20), p(3)

	got := mergeResults([]entity.ScoredSection{late, early}, nil, 4)
	if got[0].ID != "early" {
		t.Fatalf("à rangs voisins, la page la plus tôt doit passer devant : %+v", got)
	}

	// Mais un écart de rang net l'emporte sur la page
	vec := []entity.ScoredSection{late}
	for _, id := range []string{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10"} {
		x := sec(id, 0.85)
		x.PageStart = p(20)
		vec = append(vec, x)
	}
	early.Score = 0.80
	vec = append(vec, early)
	if got := mergeResults(vec, nil, 20); got[0].ID != "late" {
		t.Fatalf("la page ne doit pas écraser un meilleur rang : premier %s", got[0].ID)
	}
}

// chapterRepo : un chapitre de chunks consécutifs ; Neighbors renvoie le
// chunk et ses voisins immédiats, comme la requête SQL.
type chapterRepo struct {
	repository.SectionRepository
	chunks []entity.ScoredSection
}

func (r chapterRepo) Neighbors(ctx context.Context, s *entity.ScoredSection) ([]*entity.ScoredSection, error) {
	var out []*entity.ScoredSection
	for i := range r.chunks {
		if d := r.chunks[i].ChunkIndex - s.ChunkIndex; d >= -1 && d <= 1 {
			out = append(out, &r.chunks[i])
		}
	}
	return out, nil
}

func TestExpandTop_MergesChapterNeighbors(t *testing.T) {
	p := func(v int) *int { return &v }
	chunk := func(i int) entity.ScoredSection {
		s := sec(fmt.Sprint("c", i), 0)
		s.Text, s.PageStart, s.PageEnd, s.HierarchyPath, s.ChunkIndex = fmt.Sprint("Phase ", i), p(i), p(i), "TOUR DE JEU", i
		return s
	}
	repo := chapterRepo{}
	for i := 1; i <= 5; i++ {
		repo.chunks = append(repo.chunks, chunk(i))
	}
	other := sec("other", 0)
	other.Text = "Combat"

	// c2 absorbe c1 et c3 ; c3, déjà utilisé, disparaît ; c5 est la 2e section
	// étendue, avec c4 seulement ; other (3e) n'est pas étendue
	got := expandTop(context.Background(), repo, []entity.ScoredSection{chunk(2), chunk(3), chunk(5), other})
	var texts []string
	for _, s := range got {
		texts = append(texts, s.Text)
	}
	want := []string{"Phase 1\n\nPhase 2\n\nPhase 3", "Phase 4\n\nPhase 5", "Combat"}
	if fmt.Sprint(texts) != fmt.Sprint(want) {
		t.Fatalf("sections : %q, attendu %q", texts, want)
	}
	if *got[0].PageStart != 1 || *got[0].PageEnd != 3 {
		t.Fatalf("pages de la 1re section : p.%d-%d, attendu p.1-3", *got[0].PageStart, *got[0].PageEnd)
	}

	// Sans chapitre connu, pas d'extension
	noPath := chunk(2)
	noPath.HierarchyPath = ""
	if got := expandTop(context.Background(), repo, []entity.ScoredSection{noPath}); got[0].Text != "Phase 2" {
		t.Fatalf("section sans chapitre étendue : %q", got[0].Text)
	}
}

func TestBuildSearchQuery(t *testing.T) {
	got := buildSearchQuery("Combien de règles pour la défausse ?")
	want := "règles or défausse" // "combien" est un stopword ; accents conservés
	if got != want {
		t.Fatalf("buildSearchQuery = %q, attendu %q", got, want)
	}
}
