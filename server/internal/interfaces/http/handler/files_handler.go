// interfaces/http/handler/files_handler.go — Handlers HTTP pour les fichiers
package handler

import (
	"log"
	"net/http"

	"ask-rules-server/internal/application/usecase"

	"github.com/go-chi/chi/v5"
)

// FilesHandler gère les requêtes HTTP pour les fichiers.
type FilesHandler struct {
	listUseCase   *usecase.ListFilesUseCase
	deleteUseCase *usecase.DeleteFileUseCase
	serveUseCase  *usecase.ServeFileUseCase
}

// NewFilesHandler crée un nouveau handler.
func NewFilesHandler(
	listUseCase *usecase.ListFilesUseCase,
	deleteUseCase *usecase.DeleteFileUseCase,
	serveUseCase *usecase.ServeFileUseCase,
) *FilesHandler {
	return &FilesHandler{
		listUseCase:   listUseCase,
		deleteUseCase: deleteUseCase,
		serveUseCase:  serveUseCase,
	}
}

// List traite les requêtes GET /api/admin/files
func (h *FilesHandler) List(w http.ResponseWriter, r *http.Request) {
	files, err := h.listUseCase.Execute(r.Context())
	if err != nil {
		log.Printf("[ERROR] /api/admin/files - Failed to list files: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to list files")
		return
	}

	if files == nil {
		files = []*usecase.FileInfo{}
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

	err := h.deleteUseCase.Execute(r.Context(), slug, filename)
	if err != nil {
		if err.Error() == "file not found" || err.Error() == "file not found: stat" {
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

	absPath, err := h.serveUseCase.Execute(r.Context(), slug, filename)
	if err != nil {
		log.Printf("[ERROR] /files - File not found: %s/%s: %v", slug, filename, err)
		respondError(w, http.StatusNotFound, "File not found")
		return
	}

	http.ServeFile(w, r, absPath)
}
