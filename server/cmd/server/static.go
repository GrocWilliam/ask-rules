package main

import (
	"embed"
	"io/fs"
	"log"
)

// buildFiles embarque le répertoire build/ (SvelteKit adapter-static) dans le binaire.
// Le dossier build/ est généré par `pnpm run build:web` et copié ici par le Dockerfile.
//
//go:embed all:build
var buildFiles embed.FS

// staticFS retourne un fs.FS pointant sur le contenu du dossier build/,
// avec le sous-dossier supprimé pour que les chemins commencent directement
// par les fichiers (ex: "index.html", "_app/...", etc.).
func staticFS() fs.FS {
	sub, err := fs.Sub(buildFiles, "build")
	if err != nil {
		log.Fatalf("Impossible d'ouvrir le répertoire build/ embarqué : %v", err)
	}
	return sub
}
