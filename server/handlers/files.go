// handlers/files.go — Servir et lister les fichiers uplos
package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ask-rules-server/config"

	"github.com/go-chi/chi/v5"
)

// ServeFile sert un fichier depuis UPLOADS_DIR.
// Route : GET /files/{slug}/{filename}
func ServeFile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	filename := chi.URLParam(r, "filename")

	// Sécurité : interdire les path traversals
	if strings.Contains(slug, "..") || strings.Contains(filename, "..") {
		http.Error(w, "Chemin invalide", http.StatusBadRequest)
		return
	}

	absPath := filepath.Join(config.C.UploadsDir, slug, filename)
	http.ServeFile(w, r, absPath)
}

// FileInfo représente les métadonnées d'un fichier uploadé.
type FileInfo struct {
	Game         string `json:"game"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Modified     string `json:"modified"`
	RelativePath string `json:"relativePath"`
	Path         string `json:"path"`
}

// DeleteFile supprime un fichier uploadé depuis le disque.
// Route : DELETE /api/admin/files/{slug}/{filename}
func DeleteFile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	filename := chi.URLParam(r, "filename")

	if strings.Contains(slug, "..") || strings.Contains(filename, "..") {
		http.Error(w, "Chemin invalide", http.StatusBadRequest)
		return
	}

	absPath := filepath.Join(config.C.UploadsDir, slug, filename)

	if err := os.Remove(absPath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Fichier introuvable", http.StatusNotFound)
		} else {
			http.Error(w, "Erreur lors de la suppression", http.StatusInternalServerError)
		}
		return
	}

	// Supprimer le répertoire parent s'il est vide
	parentDir := filepath.Join(config.C.UploadsDir, slug)
	os.Remove(parentDir) // échoue silencieusement si non vide

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// ListFiles scanne le répertoire uploads et retourne la liste des fichiers.
// Route : GET /api/admin/files
func ListFiles(w http.ResponseWriter, r *http.Request) {
	uploadsDir := config.C.UploadsDir

	entries, err := os.ReadDir(uploadsDir)
	if err != nil {
		// Le répertoire peut ne pas encore exister
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}

	var files []FileInfo

	for _, gameDir := range entries {
		if !gameDir.IsDir() {
			continue
		}
		gameName := gameDir.Name()
		gamePath := filepath.Join(uploadsDir, gameName)

		fileEntries, err := os.ReadDir(gamePath)
		if err != nil {
			continue
		}

		for _, fe := range fileEntries {
			if fe.IsDir() {
				continue
			}
			info, err := fe.Info()
			if err != nil {
				continue
			}
			relPath := gameName + "/" + fe.Name()
			files = append(files, FileInfo{
				Game:         gameName,
				Name:         fe.Name(),
				Size:         info.Size(),
				Modified:     info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
				RelativePath: relPath,
				Path:         relPath,
			})
		}
	}

	if files == nil {
		files = []FileInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}
