package ask

import (
	"strings"
	"testing"

	"ask-rules-server/internal/store"
)

func TestPageRef(t *testing.T) {
	p := func(v int) *int { return &v }
	cases := []struct {
		start, end *int
		want       string
	}{
		{nil, nil, ""},
		{p(0), p(0), ""},
		{p(4), nil, "p. 4"},
		{p(4), p(4), "p. 4"},
		{p(4), p(5), "p. 4-5"},
	}
	for _, c := range cases {
		if got := pageRef(c.start, c.end); got != c.want {
			t.Errorf("pageRef(%v, %v) = %q, attendu %q", c.start, c.end, got, c.want)
		}
	}
}

func TestBuildContextAndMapSections_IncludePages(t *testing.T) {
	start, end := 7, 8
	sections := []*store.ScoredSection{{
		Title:      "Fin de partie",
		Text:       "La partie se termine…",
		PageStart:  &start,
		PageEnd:    &end,
		SourceFile: "jeu/regles.pdf",
	}}
	svc := &Service{}

	if ctx := svc.buildContext(sections); !strings.HasPrefix(ctx, "Section 1: Fin de partie (p. 7-8)\n") {
		t.Fatalf("contexte sans référence de page : %q", ctx)
	}
	got := svc.mapSections(sections)[0]
	if got.PageNum != 7 || got.PageEnd != 8 || got.File != "jeu/regles.pdf" {
		t.Fatalf("section mal convertie : %+v", got)
	}
}

func TestBuildContext_GroupsByChapter(t *testing.T) {
	sec := func(path, title string) *store.ScoredSection {
		return &store.ScoredSection{HierarchyPath: path, Title: title, Text: title + "…"}
	}
	sections := []*store.ScoredSection{
		sec("MODE SOLO > Combat", "Attaque solo"),
		sec("MODE MULTIJOUEUR > Combat", "Attaque multi"),
		sec("MODE SOLO > Combat", "Défense solo"),
	}

	got := (&Service{}).buildContext(sections)
	want := "Chapitre : MODE SOLO > Combat\n\nSection 1: Attaque solo\nAttaque solo…\n\nSection 2: Défense solo\nDéfense solo…" +
		"\n\nChapitre : MODE MULTIJOUEUR > Combat\n\nSection 3: Attaque multi\nAttaque multi…"
	if got != want {
		t.Fatalf("contexte :\n%s\n\nattendu :\n%s", got, want)
	}
}

func TestFitContextBudget(t *testing.T) {
	sec := func(id string, runes int) *store.ScoredSection {
		return &store.ScoredSection{ID: id, Text: strings.Repeat("é", runes)}
	}
	ids := func(sections []*store.ScoredSection) string {
		var b strings.Builder
		for _, s := range sections {
			b.WriteString(s.ID)
		}
		return b.String()
	}

	// c dépasse le budget restant et est sautée, d (courte) passe encore
	got := fitContextBudget([]*store.ScoredSection{sec("a", 2000), sec("b", 2000), sec("c", 1000), sec("d", 400)})
	if ids(got) != "abd" {
		t.Fatalf("sections gardées : %s, attendu abd", ids(got))
	}

	// La première est gardée même seule au-dessus du budget
	if got := fitContextBudget([]*store.ScoredSection{sec("a", maxContextRunes+1), sec("b", 10)}); ids(got) != "a" {
		t.Fatalf("sections gardées : %s, attendu a", ids(got))
	}
}
