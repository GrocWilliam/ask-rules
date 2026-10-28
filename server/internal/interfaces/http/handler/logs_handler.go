// interfaces/http/handler/logs_handler.go — Handler HTTP pour les logs
package handler

import (
	"log"
	"net/http"
	"strconv"

	"ask-rules-server/internal/application/usecase"
)

// LogsHandler gère les requêtes HTTP pour les logs.
type LogsHandler struct {
	logRepo usecase.LogRepository
}

// NewLogsHandler crée un nouveau handler.
func NewLogsHandler(logRepo usecase.LogRepository) *LogsHandler {
	return &LogsHandler{logRepo: logRepo}
}

// Get traite les requêtes GET /api/admin/logs
func (h *LogsHandler) Get(w http.ResponseWriter, r *http.Request) {
	// limit : 100 par défaut, 500 au plus
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 500 {
		limit = 100
	}

	logs, err := h.logRepo.GetRecent(r.Context(), limit)
	if err != nil {
		log.Printf("[ERROR] /api/admin/logs - Failed to get logs: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to get logs")
		return
	}

	if logs == nil {
		logs = []*usecase.LogEntry{}
	}

	respondJSON(w, http.StatusOK, logs)
}
