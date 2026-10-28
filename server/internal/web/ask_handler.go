// web/ask_handler.go — Handler HTTP pour /api/ask
package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"ask-rules-server/internal/ask"
	"ask-rules-server/internal/llm"
	"ask-rules-server/internal/store"
)

// AskHandler gère les requêtes HTTP pour poser une question.
type AskHandler struct {
	ask *ask.Service
}

// NewAskHandler crée un nouveau handler.
func NewAskHandler(asker *ask.Service) *AskHandler {
	return &AskHandler{ask: asker}
}

// maxAskBody : question + historique (3 échanges) tiennent largement en 64 Ko
const maxAskBody = 64 << 10

// Handle traite les requêtes POST /api/ask
func (h *AskHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// 1. Décoder la requête JSON
	var req ask.Request
	r.Body = http.MaxBytesReader(w, r.Body, maxAskBody)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] /api/ask - Invalid request body: %v", err)
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 2. Poser la question
	response, err := h.ask.Execute(r.Context(), &req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// 3. Renvoyer la réponse JSON
	h.respondJSON(w, http.StatusOK, response)
}

// HandleStream traite les requêtes POST /api/ask/stream : la réponse est
// envoyée en Server-Sent Events au fil de sa génération.
//
//	event: delta  data: {"text": "…"}       fragment de réponse
//	event: done   data: {AskResponse}       réponse complète (sections, modèle…)
//	event: error  data: {"error": "…"}      échec après le début du flux
//
// Une erreur survenue avant le premier fragment (jeu introuvable, aucune
// section, LLM indisponible…) est renvoyée en JSON avec son code HTTP, comme
// pour /api/ask.
func (h *AskHandler) HandleStream(w http.ResponseWriter, r *http.Request) {
	var req ask.Request
	r.Body = http.MaxBytesReader(w, r.Body, maxAskBody)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] /api/ask/stream - Invalid request body: %v", err)
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	stream := &sseStream{w: w}
	response, err := h.ask.ExecuteStream(r.Context(), &req, func(text string) {
		stream.send("delta", map[string]string{"text": text})
	})
	if err != nil {
		if !stream.started {
			h.handleError(w, err)
			return
		}
		_, message := errorStatus(err)
		log.Printf("[ERROR] /api/ask/stream - Stream interrupted: %v", err)
		stream.send("error", map[string]string{"error": message})
		return
	}
	stream.send("done", response)
}

// sseStream écrit des événements SSE ; les en-têtes partent au premier envoi.
type sseStream struct {
	w       http.ResponseWriter
	started bool
}

func (s *sseStream) send(event string, data interface{}) {
	if !s.started {
		s.started = true
		h := s.w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no") // pas de mise en tampon par nginx
		s.w.WriteHeader(http.StatusOK)
	}
	payload, _ := json.Marshal(data)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, payload)
	_ = http.NewResponseController(s.w).Flush()
}

// handleError convertit les erreurs métier en codes HTTP appropriés.
func (h *AskHandler) handleError(w http.ResponseWriter, err error) {
	status, message := errorStatus(err)
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "10")
	}
	h.respondError(w, status, message)
}

// errorStatus associe une erreur métier à un code HTTP et un message affichable.
func errorStatus(err error) (int, string) {
	// errors.Is : ask enveloppe les erreurs (fmt.Errorf("...: %w"))
	switch {
	case errors.Is(err, store.ErrInvalidGameName):
		return http.StatusBadRequest, "Choisissez un jeu avant de poser votre question."
	case errors.Is(err, ask.ErrEmptyQuestion):
		return http.StatusBadRequest, "La question est vide."
	case errors.Is(err, store.ErrGameNotFound):
		log.Printf("[ERROR] /api/ask - Game not found: %v", err)
		return http.StatusNotFound, store.ErrGameNotFound.Error()
	case errors.Is(err, ask.ErrNoSectionsFound):
		log.Printf("[ERROR] /api/ask - No sections found: %v", err)
		return http.StatusNotFound, err.Error()
	case errors.Is(err, llm.ErrRateLimited):
		log.Printf("[WARN] /api/ask - LLM rate limited: %v", err)
		return http.StatusServiceUnavailable,
			"Le service de réponse est très sollicité. Réessayez dans quelques secondes."
	default:
		log.Printf("[ERROR] /api/ask - Internal error: %v", err)
		return http.StatusInternalServerError, "Internal server error"
	}
}

// respondJSON envoie une réponse JSON.
func (h *AskHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError envoie une erreur JSON.
func (h *AskHandler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, map[string]string{"error": message})
}
