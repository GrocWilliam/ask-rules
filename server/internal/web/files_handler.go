// web/files_handler.go — Handlers HTTP pour les fichiers
package web

import (
	"errors"
	"io/fs"
	"log"
	"net/http"

	"ask-rules-server/internal/files"

	"github.com/go-chi/chi/v5"
)

// FilesHandler gère les requêtes HTTP pour les fichiers uploadés (UPLOADS_DIR).
type FilesHandler struct{}

// NewFilesHandler crée un nouveau handler.
func NewFilesHandler() *FilesHandler {
	return &FilesHandler{}
}

// List traite les requêtes GET /api/admin/files
func (h *FilesHandler) List(w http.ResponseWriter, r *http.Request) {
	files, err := files.ListFiles()
	if err != nil {
		log.Printf("[ERROR] /api/admin/files - Failed to list files: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to list files")
		return
	}

	respondJSON(w, http.StatusOK, files)
}

// Delete traite les requêtes DELETE /api/admin/files/{slug}/{filename}
func (h *FilesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	filename := chi.URLParam(r, "filename")

	if slug == "" || filename == "" {
		log.Printf("[ERROR] /api/admin/files - Missing slug or filename")
		respondError(w, http.StatusBadRequest, "Missing slug or filename")
		return
	}

	err := files.DeleteGameFile(slug, filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.Printf("[ERROR] /api/admin/files - File not found: %s/%s", slug, filename)
			respondError(w, http.StatusNotFound, "File not found")
			return
		}
		log.Printf("[ERROR] /api/admin/files - Failed to delete %s/%s: %v", slug, filename, err)
		respondError(w, http.StatusInternalServerError, "Failed to delete file")
		return
	}

	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Serve traite les requêtes GET /files/{slug}/{filename}
func (h *FilesHandler) Serve(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	filename := chi.URLParam(r, "filename")

	if slug == "" || filename == "" {
		log.Printf("[ERROR] /files - Missing slug or filename")
		respondError(w, http.StatusBadRequest, "Missing slug or filename")
		return
	}

	absPath, err := files.GameFilePath(slug, filename)
	if err != nil {
		log.Printf("[ERROR] /files - File not found: %s/%s: %v", slug, filename, err)
		respondError(w, http.StatusNotFound, "File not found")
		return
	}

	http.ServeFile(w, r, absPath)
}
