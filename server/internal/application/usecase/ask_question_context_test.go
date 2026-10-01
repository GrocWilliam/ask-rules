package usecase

import (
	"strings"
	"testing"

	"ask-rules-server/internal/domain/entity"
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
	sections := []*entity.ScoredSection{{
		Title:      "Fin de partie",
		Text:       "La partie se termine…",
		PageStart:  &start,
		PageEnd:    &end,
		SourceFile: "jeu/regles.pdf",
	}}
	uc := &AskQuestionUseCase{}

	if ctx := uc.buildContext(sections); !strings.HasPrefix(ctx, "Section 1: Fin de partie (p. 7-8)\n") {
		t.Fatalf("contexte sans référence de page : %q", ctx)
	}
	got := uc.mapSections(sections)[0]
	if got.PageNum != 7 || got.PageEnd != 8 || got.File != "jeu/regles.pdf" {
		t.Fatalf("section mal convertie : %+v", got)
	}
}
