package retriever

import (
	"testing"

	"ask-rules-server/internal/domain/entity"
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

func TestBuildSearchQuery(t *testing.T) {
	got := buildSearchQuery("Combien de règles pour la défausse ?")
	want := "règles or défausse" // "combien" est un stopword ; accents conservés
	if got != want {
		t.Fatalf("buildSearchQuery = %q, attendu %q", got, want)
	}
}
