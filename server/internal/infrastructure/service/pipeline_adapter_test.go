package service

import (
	"strings"
	"testing"
)

func TestChunkPages_TracksPages(t *testing.T) {
	para := func(topic string) string {
		return strings.Repeat("Les joueurs doivent placer leurs jetons "+topic+" sur le plateau principal. ", 6)
	}
	pages := []pageInfo{
		{Number: 1, Text: para("rouges")},
		{Number: 2, Text: para("bleus")},
		{Number: 4, Text: para("verts") + "\n\n" + para("jaunes")},
	}

	chunks := chunkPages(pages)
	if len(chunks) == 0 {
		t.Fatal("aucun chunk")
	}
	for _, c := range chunks {
		for _, pg := range pages {
			if strings.Contains(c.Text, strings.TrimSpace(pg.Text)[:60]) && (pg.Number < c.PageStart || pg.Number > c.PageEnd) {
				t.Errorf("chunk p.%d-%d contient le texte de la page %d", c.PageStart, c.PageEnd, pg.Number)
			}
		}
	}
	last := chunks[len(chunks)-1]
	if last.PageEnd != 4 {
		t.Errorf("dernier chunk : page de fin %d, attendu 4", last.PageEnd)
	}
}

func TestChunkPages_ContinuationAcrossPages(t *testing.T) {
	// Une phrase coupée par un saut de page doit être recollée et couvrir les deux pages
	pages := []pageInfo{
		{Number: 3, Text: strings.Repeat("Chaque joueur reçoit des ressources au début du tour. ", 4) + "À la fin de la manche, chaque joueur"},
		{Number: 4, Text: "défausse les cartes restantes de sa main et pioche cinq nouvelles cartes pour la manche suivante."},
	}

	chunks := chunkPages(pages)
	if len(chunks) != 1 {
		t.Fatalf("attendu 1 chunk, obtenu %d", len(chunks))
	}
	if chunks[0].PageStart != 3 || chunks[0].PageEnd != 4 {
		t.Errorf("chunk p.%d-%d, attendu p.3-4", chunks[0].PageStart, chunks[0].PageEnd)
	}
}
