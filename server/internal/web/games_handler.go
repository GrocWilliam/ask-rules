// web/games_handler.go — Handler HTTP pour gérer les jeux
package web

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"ask-rules-server/internal/index"
	"ask-rules-server/internal/store"

	"github.com/go-chi/chi/v5"
)

// GamesHandler gère les requêtes HTTP sur les jeux.
type GamesHandler struct {
	gameRepo store.GameRepository
	index    *index.Service
}

// NewGamesHandler crée un nouveau handler.
func NewGamesHandler(gameRepo store.GameRepository, indexer *index.Service) *GamesHandler {
	return &GamesHandler{gameRepo: gameRepo, index: indexer}
}

// gameSummary : un jeu dans la liste publique (sans fichiers ni métadonnées).
type gameSummary struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	AddedAt       time.Time `json:"added_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	SectionsCount int       `json:"sections_count"`
}

// List traite les requêtes GET /api/games
func (h *GamesHandler) List(w http.ResponseWriter, r *http.Request) {
	games, err := h.gameRepo.List(r.Context())
	if err != nil {
		log.Printf("[ERROR] /api/games - Failed to list games: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to list games: "+err.Error())
		return
	}

	result := make([]gameSummary, len(games))
	for i, g := range games {
		result[i] = gameSummary{
			ID:            g.ID,
			Name:          g.Name,
			AddedAt:       g.AddedAt,
			UpdatedAt:     g.UpdatedAt,
			SectionsCount: g.SectionsCount,
		}
	}
	respondJSON(w, http.StatusOK, result)
}

// Upsert traite les requêtes POST /api/admin/games (create or update)
func (h *GamesHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] /api/admin/games - Failed to read request body: %v", err)
		respondError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req index.UpsertGameRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("[ERROR] /api/admin/games - Invalid request body: %v, body: %s", err, string(body))
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	game, err := h.index.Upsert(r.Context(), &req)
	if err != nil {
		if errors.Is(err, store.ErrInvalidGameName) {
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

	err := h.index.Delete(r.Context(), gameID)
	if err != nil {
		if errors.Is(err, store.ErrGameNotFound) {
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
