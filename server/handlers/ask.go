// handlers/ask.go — POST /api/ask
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	appctx "ask-rules-server/context"
	"ask-rules-server/cache"
	"ask-rules-server/db"
	"ask-rules-server/llm"
	"ask-rules-server/logger"
	"ask-rules-server/models"
	"ask-rules-server/retriever"
)

// Ask traite une question sur un jeu.
func Ask(w http.ResponseWriter, r *http.Request) {
	var req models.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Corps de requête invalide", http.StatusBadRequest)
		return
	}
	if req.Question == "" {
		jsonError(w, "La question est vide", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	start := time.Now()

	// Chercher le jeu
	var game *models.Game
	if req.JeuID != "" {
		g, err := db.FindGame(ctx, req.JeuID)
		if err != nil || g == nil {
			jsonError(w, "Jeu introuvable", http.StatusNotFound)
			return
		}
		game = g
	} else if req.Jeu != "" {
		g, err := db.FindGameByName(ctx, req.Jeu)
		if err != nil || g == nil {
			jsonError(w, "Jeu introuvable", http.StatusNotFound)
			return
		}
		game = g
	} else {
		jsonError(w, "Veuillez sélectionner un jeu", http.StatusBadRequest)
		return
	}

	// Cache
	cacheKey := cache.Key(req.Question, game.Name)
	if cached := cache.Get(ctx, cacheKey); cached != nil {
		cached.Cached = true
		logger.CacheHit(ctx, game.Name)
		jsonOK(w, cached)
		return
	}

	// Recherche hybride
	sections, err := retriever.Search(ctx, retriever.SearchOptions{
		GameID:   game.ID,
		Question: req.Question,
		Limit:    6,
	})
	if err != nil || len(sections) == 0 {
		resp := models.AskResponse{
			OK:    false,
			Error: "Aucune section de règles trouvée pour cette question.",
		}
		jsonOK(w, resp)
		return
	}

	// Construire le contexte
	ctxText := appctx.BuildContext(game, sections)

	// Appel LLM
	llmResp, llmErr := llm.Query(ctx, req.Question, ctxText)
	if llmErr != nil {
		// Fallback : retourner le contexte brut
		llmResp = models.LLMResponse{Answer: ctxText, Model: "fallback", UsedLLM: false}
	}

	// Construire la réponse
	sectionResults := make([]models.SectionResult, len(sections))
	for i, s := range sections {
		sectionResults[i] = models.SectionResult{
			Title:       s.Title,
			SectionType: s.SectionType,
			Summary:     s.Summary,
			Text:        s.Text,
			Score:       s.Score,
			PageStart:   s.PageStart,
		}
	}

	resp := models.AskResponse{
		OK:       true,
		Jeu:      game.Name,
		JeuID:    game.ID,
		Answer:   llmResp.Answer,
		Model:    llmResp.Model,
		UsedLLM:  llmResp.UsedLLM,
		Sections: sectionResults,
		Cached:   false,
	}

	// Mettre en cache
	cache.Set(ctx, cacheKey, resp)

	// Logger
	durationMs := time.Since(start).Milliseconds()
	logger.LLMQuery(ctx, req.Question, llmResp.Answer, game.Name, llmResp.Model, llmResp.Tokens, durationMs)

	jsonOK(w, resp)
}
