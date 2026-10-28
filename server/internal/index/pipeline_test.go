package index

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

func TestChunkPages_HeadingsSplitChapters(t *testing.T) {
	rule := strings.Repeat("Le joueur actif pioche deux cartes et les ajoute à sa main. ", 4)
	pages := []pageInfo{
		{Number: 2, Text: "MODE SOLO\n\nCombat\n" + rule},
		{Number: 5, Text: "MODE MULTIJOUEUR\nCombat\n" + rule},
	}

	chunks := chunkPages(pages)
	if len(chunks) != 2 {
		t.Fatalf("attendu 1 chunk par chapitre, obtenu %d : %+v", len(chunks), chunks)
	}
	if chunks[0].Heading != "MODE SOLO > Combat" || chunks[1].Heading != "MODE MULTIJOUEUR > Combat" {
		t.Errorf("chapitres : %q / %q", chunks[0].Heading, chunks[1].Heading)
	}
	if !strings.HasPrefix(chunks[1].Text, "MODE MULTIJOUEUR") {
		t.Errorf("le titre doit rester dans le texte du chunk : %q", chunks[1].Text[:40])
	}
}

func TestIsHeading(t *testing.T) {
	cases := []struct {
		line, prev, next string
		want             bool
	}{
		{"TOUR DE JEU", "Fin de la mise en place.", "Chaque tour comporte 5 phases.", true},
		{"Mise en place", "", "Chaque joueur prend un plateau.", true},
		{"2. PHASE D'ACTION", "", "Le joueur actif…", true},
		// Ligne de colonne étroite : la phrase continue
		{"Le joueur actif pioche", "", "deux cartes.", false},
		{"Le joueur actif pioche", "Il commence.", "Deux cartes.", false}, // casse normale hors début de bloc
		{"MISSION", "au maximum 3 cartes", "en main.", false},
		{"Fin de partie.", "", "Le joueur…", false},
		{"1. Piocher deux cartes", "", "Puis jouer.", false}, // élément de liste
		// Relevés sur Les Châteaux de Bourgogne
		{"Monastères (jaunes) : voir pages 9 à 11", "", "Navires (bleu)", false},
		{"4 joueurs : 2 boucliers par dépôt", "", "", false},
		{"H 55h", "", "Animal (vert clair)", false},
		{"Les tuiles Monastère", "", "Le jeu base inclut 26 tuiles.", true},
	}
	for _, c := range cases {
		if got := isHeading(c.line, c.prev, c.next); got != c.want {
			t.Errorf("isHeading(%q, %q, %q) = %v, attendu %v", c.line, c.prev, c.next, got, c.want)
		}
	}
}
