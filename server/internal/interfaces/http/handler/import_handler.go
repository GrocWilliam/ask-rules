package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"ask-rules-server/internal/application/usecase"

	"github.com/go-chi/chi/v5"
)

const maxUploadSize = 50 << 20 // 50 MB

// ImportHandler gère les requêtes HTTP pour l'import de jeux.
type ImportHandler struct {
	importUseCase    *usecase.ImportGameUseCase
	reprocessUseCase *usecase.ReprocessGameUseCase
}

// NewImportHandler crée un nouveau handler.
func NewImportHandler(importUseCase *usecase.ImportGameUseCase, reprocessUseCase *usecase.ReprocessGameUseCase) *ImportHandler {
	return &ImportHandler{importUseCase: importUseCase, reprocessUseCase: reprocessUseCase}
}

// Import traite les requêtes POST /api/import avec Server-Sent Events.
func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	// Configurer SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("[ERROR] /api/import - SSE not supported")
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Fonction d'envoi SSE
	var sendMu sync.Mutex
	send := func(eventType string, data map[string]interface{}) {
		sendMu.Lock()
		defer sendMu.Unlock()
		merged := map[string]interface{}{"type": eventType}
		for k, v := range data {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	sendError := func(msg string) {
		send("error", map[string]interface{}{"error": msg})
	}

	// Heartbeat pour garder la connexion SSE ouverte
	// Envoie un ping toutes les 15 secondes pour éviter les timeouts proxy/navigateur
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				send("ping", map[string]interface{}{"timestamp": time.Now().Unix()})
			}
		}
	}()

	// Parser le formulaire multipart
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		log.Printf("[ERROR] /api/import - File too large or parse error: %v", err)
		sendError("File too large (max 50 MB)")
		return
	}

	// Récupérer le nom du jeu
	gameName := r.FormValue("gameName")
	if gameName == "" {
		gameName = r.FormValue("game")
	}
	if gameName == "" {
		gameName = r.FormValue("jeu")
	}
	if gameName == "" {
		log.Printf("[ERROR] /api/import - Missing game name")
		sendError("Game name is required")
		return
	}

	// Récupérer les fichiers
	files := r.MultipartForm.File["fichier"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		log.Printf("[ERROR] /api/import - No files provided for game '%s'", gameName)
		sendError("No files provided")
		return
	}

	// Mode d'import
	mode := r.FormValue("mode")
	if mode == "" {
		mode = "replace"
	}

	// Créer la requête
	req := &usecase.ImportRequest{
		GameName: gameName,
		Files:    files,
		Mode:     mode,
		OnEvent:  send,
	}

	// Exécuter le use case
	if err := h.importUseCase.Execute(r.Context(), req); err != nil {
		log.Printf("[ERROR] /api/import - Failed to import game '%s': %v", gameName, err)
		sendError(err.Error())
		return
	}
}

// Reprocess traite les requêtes POST /api/admin/games/{id}/reprocess avec Server-Sent Events.
func (h *ImportHandler) Reprocess(w http.ResponseWriter, r *http.Request) {
	// Configurer SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	var sendMu sync.Mutex
	send := func(eventType string, data map[string]interface{}) {
		sendMu.Lock()
		defer sendMu.Unlock()
		merged := map[string]interface{}{"type": eventType}
		for k, v := range data {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				send("ping", map[string]interface{}{"timestamp": time.Now().Unix()})
			}
		}
	}()

	gameID := chi.URLParam(r, "id")
	if gameID == "" {
		send("error", map[string]interface{}{"error": "Missing game ID"})
		return
	}

	req := &usecase.ReprocessRequest{
		GameID:  gameID,
		OnEvent: send,
	}

	if err := h.reprocessUseCase.Execute(r.Context(), req); err != nil {
		log.Printf("[ERROR] /api/admin/games/%s/reprocess - Failed: %v", gameID, err)
		send("error", map[string]interface{}{"error": err.Error()})
		return
	}
}

// ReprocessAll traite les requêtes POST /api/admin/reprocess-all avec Server-Sent Events.
func (h *ImportHandler) ReprocessAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	var sendMu sync.Mutex
	send := func(eventType string, data map[string]interface{}) {
		sendMu.Lock()
		defer sendMu.Unlock()
		merged := map[string]interface{}{"type": eventType}
		for k, v := range data {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				send("ping", map[string]interface{}{"timestamp": time.Now().Unix()})
			}
		}
	}()

	log.Printf("[INFO] /api/admin/reprocess-all - Starting reprocess of all games")
	h.reprocessUseCase.ExecuteAll(ctx, send)
}
