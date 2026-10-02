// application/usecase/ask_question.go — Use case pour poser une question
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
	"ask-rules-server/internal/domain/service"
)

// AskQuestionUseCase gère la logique métier pour poser une question sur un jeu.
type AskQuestionUseCase struct {
	gameRepo  repository.GameRepository
	retriever service.RetrieverService
	llm       service.LLMService
	cache     service.CacheService
	logRepo   LogRepository
}

// NewAskQuestionUseCase crée un nouveau use case.
func NewAskQuestionUseCase(
	gameRepo repository.GameRepository,
	retriever service.RetrieverService,
	llm service.LLMService,
	cache service.CacheService,
	logRepo LogRepository,
) *AskQuestionUseCase {
	return &AskQuestionUseCase{
		gameRepo:  gameRepo,
		retriever: retriever,
		llm:       llm,
		cache:     cache,
		logRepo:   logRepo,
	}
}

// Limites appliquées à l'historique envoyé par le client : il est rejoué au
// LLM à chaque question, il doit donc rester court (tokens, latence).
const (
	maxHistoryTurns     = 3
	maxHistoryQuestion  = 500
	maxHistoryAnswerLen = 2000
)

// AskRequest est la requête pour poser une question.
type AskRequest struct {
	GameName string `json:"game"`
	Question string `json:"question"`
	// History contient les échanges précédents de la conversation, du plus
	// ancien au plus récent (optionnel).
	History []entity.ChatTurn `json:"history,omitempty"`
}

// AskResponse est la réponse à une question.
type AskResponse struct {
	Answer   string              `json:"answer"`
	Game     string              `json:"game"`
	Model    string              `json:"model"`
	Sections []*RetrievedSection `json:"sections"`
	Cached   bool                `json:"cached"`
	FilePath []string            `json:"file_path"`
}

// RetrievedSection représente une section récupérée pour la réponse.
type RetrievedSection struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
	PageNum int     `json:"page_num"`
	PageEnd int     `json:"page_end"`
	File    string  `json:"file"` // chemin relatif servi par /files/{file}
}

// Execute exécute le use case : recherche le contexte, génère la réponse.
func (uc *AskQuestionUseCase) Execute(ctx context.Context, req *AskRequest) (*AskResponse, error) {
	// 1. Valider la requête
	if req.GameName == "" {
		return nil, fmt.Errorf("game name is required")
	}
	if req.Question == "" {
		return nil, fmt.Errorf("question is required")
	}

	// 2. Chercher le jeu par nom
	game, err := uc.gameRepo.FindByName(ctx, req.GameName)
	if err != nil {
		if err == entity.ErrGameNotFound {
			log.Printf("[ERROR] AskQuestion - Game '%s' not found", req.GameName)
			return nil, fmt.Errorf("game '%s' not found: %w", req.GameName, entity.ErrGameNotFound)
		}
		log.Printf("[ERROR] AskQuestion - Failed to find game '%s': %v", req.GameName, err)
		return nil, fmt.Errorf("failed to find game: %w", err)
	}

	history := trimHistory(req.History)

	// 3. Vérifier le cache — uniquement pour une première question : avec un
	// historique, la réponse dépend de la conversation
	useCache := len(history) == 0
	cacheKey := uc.buildCacheKey(game.ID, req.Question)
	if useCache {
		if cached, err := uc.cache.Get(ctx, cacheKey); err == nil && cached != nil {
			// Convertir map[string]interface{} en *AskResponse via JSON
			if cachedMap, ok := cached.(map[string]interface{}); ok {
				b, _ := json.Marshal(cachedMap)
				var response AskResponse
				if json.Unmarshal(b, &response) == nil {
					response.Cached = true
					return &response, nil
				}
			}
		}
	}

	// 4. Rechercher les sections pertinentes
	sections, err := uc.retriever.Search(ctx, &service.SearchRequest{
		GameID:   game.ID,
		Question: searchQuery(req.Question, history),
		Limit:    6,
	})

	if err != nil {
		log.Printf("[ERROR] AskQuestion - Failed to search sections for game '%s': %v", game.Name, err)
		return nil, fmt.Errorf("failed to search sections: %w", err)
	}

	if len(sections) == 0 {
		log.Printf("[WARN] AskQuestion - No sections found for game '%s' with question: %s", game.Name, req.Question)
		return nil, entity.ErrNoSectionsFound
	}

	// 5. Construire le contexte pour le LLM
	contextText := uc.buildContext(sections)

	// 6. Générer la réponse avec le LLM
	llmResponse, err := uc.llm.Query(ctx, req.Question, contextText, history)
	if err != nil {
		log.Printf("[ERROR] AskQuestion - Failed to generate LLM answer for game '%s': %v", game.Name, err)
		return nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	// 7. Construire la réponse
	response := &AskResponse{
		Answer:   llmResponse.Answer,
		Game:     game.Name,
		Model:    llmResponse.Model,
		Sections: uc.mapSections(sections),
		Cached:   false,
		FilePath: toStringSlice(game.Stats["files"]),
	}

	// 8. Mettre en cache
	if useCache {
		_ = uc.cache.Set(ctx, cacheKey, response)
	}

	// 9. Logger la question en base de données
	if uc.logRepo != nil {
		metadata := map[string]interface{}{
			"game":     game.Name,
			"question": req.Question,
			"model":    llmResponse.Model,
		}
		if len(history) > 0 {
			metadata["history_turns"] = len(history)
		}
		if llmResponse.TokensUsed != nil {
			metadata["tokens_input"] = llmResponse.TokensUsed.Input
			metadata["tokens_output"] = llmResponse.TokensUsed.Output
			metadata["tokens_total"] = llmResponse.TokensUsed.Total
		}
		entry := &LogEntry{
			EventType: "ask_question",
			Message:   fmt.Sprintf("Question sur le jeu '%s'", game.Name),
			Metadata:  metadata,
		}
		if err := uc.logRepo.Save(ctx, entry); err != nil {
			log.Printf("[WARN] AskQuestion - Failed to save log: %v", err)
		}
	}

	return response, nil
}

// trimHistory ne garde que les derniers échanges complets, tronqués, pour
// borner la taille du prompt quelle que soit la requête du client.
func trimHistory(history []entity.ChatTurn) []entity.ChatTurn {
	result := make([]entity.ChatTurn, 0, maxHistoryTurns)
	for _, turn := range history {
		q, a := strings.TrimSpace(turn.Question), strings.TrimSpace(turn.Answer)
		if q == "" || a == "" {
			continue
		}
		result = append(result, entity.ChatTurn{
			Question: truncateRunes(q, maxHistoryQuestion),
			Answer:   truncateRunes(a, maxHistoryAnswerLen),
		})
	}
	if len(result) > maxHistoryTurns {
		result = result[len(result)-maxHistoryTurns:]
	}
	return result
}

// truncateRunes coupe s à max caractères (sans casser un caractère UTF-8).
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// searchQuery enrichit une question de suivi (« et en fin de partie ? ») avec
// la question précédente, pour que la recherche retrouve le bon sujet.
func searchQuery(question string, history []entity.ChatTurn) string {
	if len(history) == 0 {
		return question
	}
	return history[len(history)-1].Question + " " + question
}

// buildCacheKey génère une clé de cache unique pour la question.
func (uc *AskQuestionUseCase) buildCacheKey(gameID, question string) string {
	normalized := strings.ToLower(strings.TrimSpace(question))
	hash := sha256.Sum256([]byte(gameID + ":" + normalized))
	return "ask:" + hex.EncodeToString(hash[:])
}

// buildContext construit le texte de contexte pour le LLM.
// Chaque section indique sa page pour que le LLM puisse la citer.
func (uc *AskQuestionUseCase) buildContext(sections []*entity.ScoredSection) string {
	var builder strings.Builder
	for i, sec := range sections {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("Section %d: %s", i+1, sec.Title))
		if ref := pageRef(sec.PageStart, sec.PageEnd); ref != "" {
			builder.WriteString(" (" + ref + ")")
		}
		builder.WriteString("\n" + sec.Text)
	}
	return builder.String()
}

// pageRef formate la page d'une section : "p. 4" ou "p. 4-5".
func pageRef(start, end *int) string {
	if start == nil || *start <= 0 {
		return ""
	}
	if end != nil && *end > *start {
		return fmt.Sprintf("p. %d-%d", *start, *end)
	}
	return fmt.Sprintf("p. %d", *start)
}

// mapSections convertit les sections du domaine en DTO.
func (uc *AskQuestionUseCase) mapSections(sections []*entity.ScoredSection) []*RetrievedSection {
	result := make([]*RetrievedSection, len(sections))
	for i, sec := range sections {
		pageNum, pageEnd := 0, 0
		if sec.PageStart != nil {
			pageNum = *sec.PageStart
		}
		if sec.PageEnd != nil {
			pageEnd = *sec.PageEnd
		}
		result[i] = &RetrievedSection{
			Title:   sec.Title,
			Content: sec.Text,
			Score:   sec.Score,
			PageNum: pageNum,
			PageEnd: pageEnd,
			File:    sec.SourceFile,
		}
	}
	return result
}
