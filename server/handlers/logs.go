// handlers/logs.go — Consultation des journaux
package handlers

import (
	"net/http"
	"strconv"

	"ask-rules-server/db"
	"ask-rules-server/models"
)

// GetLogs retourne les entrées de journal récentes.
func GetLogs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 500 {
		limit = 100
	}

	logs, err := db.GetRecentLogs(r.Context(), limit)
	if err != nil {
		jsonError(w, "Erreur base de données", http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []models.LogEntry{}
	}
	jsonOK(w, logs)
}
