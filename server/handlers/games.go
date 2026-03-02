// handlers/games.go — CRUD jeux
package handlers

import (
	"encoding/json"
	"net/http"

	"ask-rules-server/db"
	"ask-rules-server/logger"
	"ask-rules-server/models"

	"github.com/go-chi/chi/v5"
)

// ListGames retourne la liste de tous les jeux avec leur compte de sections.
func ListGames(w http.ResponseWriter, r *http.Request) {
	games, err := db.ListGames(r.Context())
	if err != nil {
		jsonError(w, "Erreur base de données", http.StatusInternalServerError)
		return
	}
	if games == nil {
		games = []models.GameWithStats{}
	}
	jsonOK(w, games)
}

// GetGame retourne un jeu par son ID.
func GetGame(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	game, err := db.FindGame(r.Context(), id)
	if err != nil || game == nil {
		jsonError(w, "Jeu introuvable", http.StatusNotFound)
		return
	}
	jsonOK(w, game)
}

// DeleteGame supprime un jeu et toutes ses sections.
func DeleteGame(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	game, err := db.FindGame(r.Context(), id)
	if err != nil || game == nil {
		jsonError(w, "Jeu introuvable", http.StatusNotFound)
		return
	}
	name := game.Name
	if err := db.DeleteGame(r.Context(), id); err != nil {
		jsonError(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}
	logger.GameDeleted(r.Context(), name)
	jsonOK(w, map[string]bool{"ok": true})
}

// UpsertGame crée ou met à jour un jeu (sans réindexer).
func UpsertGame(w http.ResponseWriter, r *http.Request) {
	var g models.Game
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		jsonError(w, "Corps invalide", http.StatusBadRequest)
		return
	}
	if err := db.UpsertGame(r.Context(), &g); err != nil {
		jsonError(w, "Erreur base de données", http.StatusInternalServerError)
		return
	}
	jsonOK(w, g)
}
