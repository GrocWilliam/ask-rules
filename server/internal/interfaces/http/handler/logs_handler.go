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
	getLogsUseCase *usecase.GetLogsUseCase
}

// NewLogsHandler crée un nouveau handler.
func NewLogsHandler(getLogsUseCase *usecase.GetLogsUseCase) *LogsHandler {
	return &LogsHandler{getLogsUseCase: getLogsUseCase}
}

// Get traite les requêtes GET /api/admin/logs
func (h *LogsHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Parser le paramètre limit
	limitStr := r.URL.Query().Get("limit")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	logs, err := h.getLogsUseCase.Execute(r.Context(), limit)
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
