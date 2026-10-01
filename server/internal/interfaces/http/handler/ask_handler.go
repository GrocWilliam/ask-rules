// interfaces/http/handler/ask_handler.go — Handler HTTP pour /api/ask
package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
)

// AskHandler gère les requêtes HTTP pour poser une question.
// C'est un adaptateur entre HTTP et le use case métier.
type AskHandler struct {
	askUseCase *usecase.AskQuestionUseCase
}

// NewAskHandler crée un nouveau handler.
func NewAskHandler(askUseCase *usecase.AskQuestionUseCase) *AskHandler {
	return &AskHandler{askUseCase: askUseCase}
}

// Handle traite les requêtes POST /api/ask
func (h *AskHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// 1. Décoder la requête JSON
	var req usecase.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] /api/ask - Invalid request body: %v", err)
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// 2. Exécuter le use case
	response, err := h.askUseCase.Execute(r.Context(), &req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// 3. Renvoyer la réponse JSON
	h.respondJSON(w, http.StatusOK, response)
}

// handleError convertit les erreurs métier en codes HTTP appropriés.
func (h *AskHandler) handleError(w http.ResponseWriter, err error) {
	// errors.Is : le use case enveloppe les erreurs (fmt.Errorf("...: %w"))
	switch {
	case errors.Is(err, entity.ErrGameNotFound):
		log.Printf("[ERROR] /api/ask - Game not found: %v", err)
		h.respondError(w, http.StatusNotFound, entity.ErrGameNotFound.Error())
	case errors.Is(err, entity.ErrNoSectionsFound):
		log.Printf("[ERROR] /api/ask - No sections found: %v", err)
		h.respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, entity.ErrLLMRateLimited):
		log.Printf("[WARN] /api/ask - LLM rate limited: %v", err)
		w.Header().Set("Retry-After", "10")
		h.respondError(w, http.StatusServiceUnavailable,
			"Le service de réponse est très sollicité. Réessayez dans quelques secondes.")
	default:
		log.Printf("[ERROR] /api/ask - Internal error: %v", err)
		h.respondError(w, http.StatusInternalServerError, "Internal server error")
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
