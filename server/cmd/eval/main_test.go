package main

import (
	"slices"
	"testing"
)

func TestFactFound(t *testing.T) {
	tests := []struct {
		answer, fact string
		want         bool
	}{
		{"**Au maximum 3 cartes MISSION** en main.", "3|trois", true},
		{"Trois cartes au plus.", "3|trois", true},
		{"Il faut 13 cartes.", "3|trois", false},
		{"Après la 4e Époque.", "quatrième|4e", true},
		{"Le dé « déjà vu »", "déjà vu", true},
		{"On paie avec des cubes R&D.", "r&d", true},
		{"Elle n’est jamais contrôlée.", "jamais contrôlée", true},
		{"La réserve du joueur.", "réserves", false},
	}
	for _, tt := range tests {
		if got := factFound(normalizeAnswer(tt.answer), tt.fact); got != tt.want {
			t.Errorf("factFound(%q, %q) = %v, want %v", tt.answer, tt.fact, got, tt.want)
		}
	}
}

func TestCitedPages(t *testing.T) {
	tests := []struct {
		answer string
		want   []int
	}{
		{"**3 cartes** (p. 6)", []int{6}},
		{"Voir p.4-5 et page 12.", []int{4, 5, 12}},
		{"pages 3 à 5", []int{3, 5}},
		{"(p. 6) puis (p. 6)", []int{6}},
		{"Aucune page, 3 cartes.", nil},
	}
	for _, tt := range tests {
		if got := citedPages(tt.answer); !slices.Equal(got, tt.want) {
			t.Errorf("citedPages(%q) = %v, want %v", tt.answer, got, tt.want)
		}
	}
}

func TestFactIgnoresCitedPage(t *testing.T) {
	answer := "**Au maximum 2 cartes** (p. 3)"
	if factFound(normalizeAnswer(pageRef.ReplaceAllString(answer, "")), "3|trois") {
		t.Error("la page citée ne doit pas valider le fait")
	}
}
