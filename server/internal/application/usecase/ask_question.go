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

// AskRequest est la requête pour poser une question.
type AskRequest struct {
	GameName string `json:"game"`
	Question string `json:"question"`
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

	// 3. Vérifier le cache
	cacheKey := uc.buildCacheKey(game.ID, req.Question)
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

	// 4. Rechercher les sections pertinentes
	sections, err := uc.retriever.Search(ctx, &service.SearchRequest{
		GameID:   game.ID,
		Question: req.Question,
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
	llmResponse, err := uc.llm.Query(ctx, req.Question, contextText)
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
	_ = uc.cache.Set(ctx, cacheKey, response)

	// 9. Logger la question en base de données
	if uc.logRepo != nil {
		metadata := map[string]interface{}{
			"game":     game.Name,
			"question": req.Question,
			"model":    llmResponse.Model,
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

// buildCacheKey génère une clé de cache unique pour la question.
func (uc *AskQuestionUseCase) buildCacheKey(gameID, question string) string {
	normalized := strings.ToLower(strings.TrimSpace(question))
	hash := sha256.Sum256([]byte(gameID + ":" + normalized))
	return "ask:" + hex.EncodeToString(hash[:])
}

// buildContext construit le texte de contexte pour le LLM.
func (uc *AskQuestionUseCase) buildContext(sections []*entity.ScoredSection) string {
	var builder strings.Builder
	for i, sec := range sections {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("Section %d: %s\n%s", i+1, sec.Title, sec.Text))
	}
	return builder.String()
}

// mapSections convertit les sections du domaine en DTO.
func (uc *AskQuestionUseCase) mapSections(sections []*entity.ScoredSection) []*RetrievedSection {
	result := make([]*RetrievedSection, len(sections))
	for i, sec := range sections {
		pageNum := 0
		if sec.PageStart != nil {
			pageNum = *sec.PageStart
		}
		result[i] = &RetrievedSection{
			Title:   sec.Title,
			Content: sec.Text,
			Score:   sec.Score,
			PageNum: pageNum,
		}
	}
	return result
}
