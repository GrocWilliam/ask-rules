// interfaces/http/handler/games_handler.go — Handler HTTP pour gérer les jeux
package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"

	"github.com/go-chi/chi/v5"
)

// GamesHandler gère les requêtes HTTP sur les jeux.
type GamesHandler struct {
	listUseCase   *usecase.ListGamesUseCase
	getUseCase    *usecase.GetGameUseCase
	upsertUseCase *usecase.UpsertGameUseCase
	deleteUseCase *usecase.DeleteGameUseCase
}

// NewGamesHandler crée un nouveau handler.
func NewGamesHandler(
	listUseCase *usecase.ListGamesUseCase,
	getUseCase *usecase.GetGameUseCase,
	upsertUseCase *usecase.UpsertGameUseCase,
	deleteUseCase *usecase.DeleteGameUseCase,
) *GamesHandler {
	return &GamesHandler{
		listUseCase:   listUseCase,
		getUseCase:    getUseCase,
		upsertUseCase: upsertUseCase,
		deleteUseCase: deleteUseCase,
	}
}

// List traite les requêtes GET /api/games
func (h *GamesHandler) List(w http.ResponseWriter, r *http.Request) {
	games, err := h.listUseCase.Execute(r.Context())
	if err != nil {
		log.Printf("[ERROR] /api/games - Failed to list games: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to list games: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, games)
}

// Get traite les requêtes GET /api/games/{id}
func (h *GamesHandler) Get(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "id")
	if gameID == "" {
		log.Printf("[ERROR] /api/games/{id} - Missing game ID")
		respondError(w, http.StatusBadRequest, "Missing game ID")
		return
	}

	game, err := h.getUseCase.Execute(r.Context(), gameID)
	if err != nil {
		if err == entity.ErrGameNotFound {
			log.Printf("[ERROR] /api/games/{id} - Game not found: %s", gameID)
			respondError(w, http.StatusNotFound, "Game not found")
			return
		}
		log.Printf("[ERROR] /api/games/{id} - Failed to get game %s: %v", gameID, err)
		respondError(w, http.StatusInternalServerError, "Failed to get game")
		return
	}

	respondJSON(w, http.StatusOK, game)
}

// Upsert traite les requêtes POST /api/admin/games (create or update)
func (h *GamesHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] /api/admin/games - Failed to read request body: %v", err)
		respondError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req usecase.UpsertGameRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("[ERROR] /api/admin/games - Invalid request body: %v, body: %s", err, string(body))
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	game, err := h.upsertUseCase.Execute(r.Context(), &req)
	if err != nil {
		if err == entity.ErrInvalidGameName {
			log.Printf("[ERROR] /api/admin/games - Invalid game name: %v", err)
			respondError(w, http.StatusBadRequest, "Invalid game name")
			return
		}
		log.Printf("[ERROR] /api/admin/games - Failed to save game: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to save game")
		return
	}

	respondJSON(w, http.StatusOK, game)
}

// Delete traite les requêtes DELETE /api/games/{id}
func (h *GamesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "id")
	if gameID == "" {
		respondError(w, http.StatusBadRequest, "Missing game ID")
		return
	}

	err := h.deleteUseCase.Execute(r.Context(), gameID)
	if err != nil {
		if err == entity.ErrGameNotFound {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to delete game")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Game deleted successfully"})
}

// Fonctions helpers (partagées entre handlers)
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
