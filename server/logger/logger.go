// logger/logger.go — Helpers de journalisation métier
package logger

import (
	"context"
	"fmt"
	"time"

	"ask-rules-server/db"
	"ask-rules-server/models"
)

func log(_ context.Context, level, eventType, message string, details map[string]interface{}) {
	if details == nil {
		details = map[string]interface{}{}
	}
	details["level"] = level
	entry := models.LogEntry{
		EventType: eventType,
		Message:   message,
		Metadata:  details,
	}
	// Utiliser un contexte indépendant de la requête HTTP : l'insertion de log
	// ne doit pas échouer parce que le client SSE a fermé la connexion.
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.InsertLog(dbCtx, entry); err != nil {
		fmt.Printf("[logger] erreur insertion log: %v\n", err)
	}
}

// Info journal un événement informationnel.
func Info(ctx context.Context, eventType, message string, details map[string]interface{}) {
	log(ctx, "info", eventType, message, details)
}

// Error journal une erreur.
func Error(ctx context.Context, eventType, message string, details map[string]interface{}) {
	log(ctx, "error", eventType, message, details)
}

// GameAdded journal l'ajout d'un jeu.
func GameAdded(ctx context.Context, gameName string, fileCount int) {
	Info(ctx, "game_added", fmt.Sprintf("Jeu ajouté : %s", gameName), map[string]interface{}{
		"game": gameName, "file_count": fileCount,
	})
}

// GameDeleted journal la suppression d'un jeu.
func GameDeleted(ctx context.Context, gameName string) {
	Info(ctx, "game_deleted", fmt.Sprintf("Jeu supprimé : %s", gameName), map[string]interface{}{
		"game": gameName,
	})
}

// GameReprocessed journal le retraitement d'un jeu.
func GameReprocessed(ctx context.Context, gameName string) {
	Info(ctx, "game_reprocessed", fmt.Sprintf("Jeu retraité : %s", gameName), map[string]interface{}{
		"game": gameName,
	})
}

// ImportError journal une erreur d'import.
func ImportError(ctx context.Context, gameName, errMsg string) {
	Error(ctx, "import_error", fmt.Sprintf("Erreur import %s : %s", gameName, errMsg), map[string]interface{}{
		"game": gameName, "error": errMsg,
	})
}

// LLMQuery journal une requête LLM avec la question et la réponse anonymisées.
func LLMQuery(ctx context.Context, question, answer, jeu, model string, tokens *models.TokenUsage, durationMs int64) {
	q := []rune(question)
	if len(q) > 300 {
		q = q[:300]
	}
	a := []rune(answer)
	if len(a) > 600 {
		a = a[:600]
	}
	details := map[string]interface{}{
		"question":    string(q),
		"answer":      string(a),
		"jeu":         jeu,
		"model":       model,
		"duration_ms": durationMs,
	}
	if tokens != nil {
		details["tokens_prompt"] = tokens.Prompt
		details["tokens_completion"] = tokens.Completion
		details["tokens_total"] = tokens.Total
	}
	Info(ctx, "llm_query", fmt.Sprintf("Question sur %s (%dms)", jeu, durationMs), details)
}

// CacheHit journal un cache hit.
func CacheHit(ctx context.Context, jeu string) {
	Info(ctx, "cache_hit", fmt.Sprintf("Cache hit pour %s", jeu), map[string]interface{}{"jeu": jeu})
}
