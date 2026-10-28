// ask/ask.go — Use case pour poser une question
package ask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"ask-rules-server/internal/llm"
	"ask-rules-server/internal/store"
)

var (
	// ErrEmptyQuestion est retourné quand la question est vide.
	ErrEmptyQuestion = errors.New("question is required")

	// ErrNoSectionsFound est retourné quand aucune section pertinente n'est trouvée.
	ErrNoSectionsFound = errors.New("no relevant sections found")
)

// Searcher retrouve les sections d'un jeu pour une question (search.Retriever).
type Searcher interface {
	Search(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error)
}

// LLM génère la réponse à partir de la question et des extraits du livret.
// onToken, s'il n'est pas nil, reçoit la réponse au fil de sa génération
// (streaming) ; la réponse complète est retournée à la fin dans tous les cas.
type LLM interface {
	Query(ctx context.Context, question, contextText string, history []llm.ChatTurn, onToken func(string)) (*llm.Response, error)
}

// Cache garde les réponses aux premières questions (Redis ou mémoire).
// Get retourne nil si la clé est absente ou expirée.
type Cache interface {
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}) error
}

// Service gère la logique métier pour poser une question sur un jeu.
type Service struct {
	gameRepo  store.GameRepository
	retriever Searcher
	llm       LLM
	cache     Cache
	logRepo   store.LogRepository
}

// NewService crée le service.
func NewService(
	gameRepo store.GameRepository,
	retriever Searcher,
	llm LLM,
	cache Cache,
	logRepo store.LogRepository,
) *Service {
	return &Service{
		gameRepo:  gameRepo,
		retriever: retriever,
		llm:       llm,
		cache:     cache,
		logRepo:   logRepo,
	}
}

// Limites appliquées à la question et à l'historique envoyés par le client :
// ils partent au LLM à chaque question, ils doivent rester courts (tokens, latence).
const (
	maxQuestionLen      = 500 // comme MAX_QUESTION_LENGTH côté interface
	maxHistoryTurns     = 3
	maxHistoryQuestion  = 500
	maxHistoryAnswerLen = 2000
)

// Request est la requête pour poser une question.
type Request struct {
	GameName string `json:"game"`
	Question string `json:"question"`
	// History contient les échanges précédents de la conversation, du plus
	// ancien au plus récent (optionnel).
	History []llm.ChatTurn `json:"history,omitempty"`
}

// Response est la réponse à une question.
type Response struct {
	Answer   string     `json:"answer"`
	Game     string     `json:"game"`
	Model    string     `json:"model"`
	Sections []*Section `json:"sections"`
	Cached   bool       `json:"cached"`
	FilePath []string   `json:"file_path"`
}

// Section représente une section récupérée pour la réponse.
type Section struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
	PageNum int     `json:"page_num"`
	PageEnd int     `json:"page_end"`
	File    string  `json:"file"` // chemin relatif servi par /files/{file}
}

// Execute recherche le contexte et génère la réponse.
func (s *Service) Execute(ctx context.Context, req *Request) (*Response, error) {
	return s.ExecuteStream(ctx, req, nil)
}

// ExecuteStream fait comme Execute, en transmettant la réponse à onToken au fil
// de sa génération (en une fois si elle vient du cache). onToken n'est appelé
// qu'une fois le jeu et les sections trouvés : une erreur de validation ou de
// recherche arrive toujours avant le premier fragment.
func (s *Service) ExecuteStream(ctx context.Context, req *Request, onToken func(string)) (*Response, error) {
	// 1. Valider la requête
	if strings.TrimSpace(req.GameName) == "" {
		return nil, fmt.Errorf("game name is required: %w", store.ErrInvalidGameName)
	}
	if strings.TrimSpace(req.Question) == "" {
		return nil, ErrEmptyQuestion
	}
	req.Question = truncateRunes(strings.TrimSpace(req.Question), maxQuestionLen)

	// 2. Chercher le jeu par nom
	game, err := s.gameRepo.FindByName(ctx, req.GameName)
	if err != nil {
		if errors.Is(err, store.ErrGameNotFound) {
			log.Printf("[ERROR] AskQuestion - Game '%s' not found", req.GameName)
			return nil, fmt.Errorf("game '%s' not found: %w", req.GameName, store.ErrGameNotFound)
		}
		log.Printf("[ERROR] AskQuestion - Failed to find game '%s': %v", req.GameName, err)
		return nil, fmt.Errorf("failed to find game: %w", err)
	}

	history := trimHistory(req.History)

	// 3. Vérifier le cache — uniquement pour une première question : avec un
	// historique, la réponse dépend de la conversation
	useCache := len(history) == 0
	cacheKey := s.buildCacheKey(game, req.Question)
	if useCache {
		if cached, err := s.cache.Get(ctx, cacheKey); err == nil && cached != nil {
			// Convertir map[string]interface{} en *Response via JSON
			if cachedMap, ok := cached.(map[string]interface{}); ok {
				b, _ := json.Marshal(cachedMap)
				var response Response
				if json.Unmarshal(b, &response) == nil {
					response.Cached = true
					if onToken != nil {
						onToken(response.Answer)
					}
					return &response, nil
				}
			}
		}
	}

	// 4-5. Rechercher les sections pertinentes et construire le contexte pour le LLM
	retrieved, err := s.RetrieveContext(ctx, game.ID, req.Question, history)
	if err != nil {
		log.Printf("[ERROR] AskQuestion - Failed to search sections for game '%s': %v", game.Name, err)
		return nil, fmt.Errorf("failed to search sections: %w", err)
	}

	if len(retrieved.Sections) == 0 {
		log.Printf("[WARN] AskQuestion - No sections found for game '%s' with question: %s", game.Name, req.Question)
		return nil, ErrNoSectionsFound
	}
	// Les sources affichées au joueur sont celles que le LLM a reçues
	sections := retrieved.Sections

	// 6. Générer la réponse avec le LLM
	llmResponse, err := s.llm.Query(ctx, req.Question, retrieved.Text, history, onToken)
	if err != nil {
		log.Printf("[ERROR] AskQuestion - Failed to generate LLM answer for game '%s': %v", game.Name, err)
		return nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	// 7. Construire la réponse
	response := &Response{
		Answer:   llmResponse.Answer,
		Game:     game.Name,
		Model:    llmResponse.Model,
		Sections: s.mapSections(sections),
		Cached:   false,
		FilePath: game.Files(),
	}

	// 8. Mettre en cache
	if useCache {
		_ = s.cache.Set(ctx, cacheKey, response)
	}

	// 9. Logger la question en base de données
	if s.logRepo != nil {
		metadata := map[string]interface{}{
			"game":     game.Name,
			"question": req.Question,
			"model":    llmResponse.Model,
		}
		if len(history) > 0 {
			metadata["history_turns"] = len(history)
		}
		if llmResponse.Tokens != nil {
			metadata["tokens_input"] = llmResponse.Tokens.Prompt
			metadata["tokens_output"] = llmResponse.Tokens.Completion
			metadata["tokens_total"] = llmResponse.Tokens.Total
		}
		entry := &store.LogEntry{
			EventType: "ask_question",
			Message:   fmt.Sprintf("Question sur le jeu '%s'", game.Name),
			Metadata:  metadata,
		}
		if err := s.logRepo.Save(ctx, entry); err != nil {
			log.Printf("[WARN] AskQuestion - Failed to save log: %v", err)
		}
	}

	return response, nil
}

// RetrievedContext est ce que le LLM reçoit pour répondre à une question.
type RetrievedContext struct {
	// Candidates : sections trouvées, par pertinence, avant le budget de contexte.
	Candidates []*store.ScoredSection
	// Sections : candidates retenues dans le budget, envoyées au LLM.
	Sections []*store.ScoredSection
	// Text : contexte envoyé au LLM, construit à partir de Sections.
	Text string
}

// RetrieveContext recherche les sections de la question et construit le
// contexte du LLM, tel que l'utilise ExecuteStream (aussi utilisé par cmd/eval).
// history doit déjà être borné (trimHistory).
func (s *Service) RetrieveContext(ctx context.Context, gameID, question string, history []llm.ChatTurn) (*RetrievedContext, error) {
	candidates, err := s.search(ctx, gameID, question, history)
	if err != nil {
		return nil, err
	}
	sections := fitContextBudget(candidates)
	return &RetrievedContext{
		Candidates: candidates,
		Sections:   sections,
		Text:       s.buildContext(sections),
	}, nil
}

// trimHistory ne garde que les derniers échanges complets, tronqués, pour
// borner la taille du prompt quelle que soit la requête du client.
func trimHistory(history []llm.ChatTurn) []llm.ChatTurn {
	result := make([]llm.ChatTurn, 0, maxHistoryTurns)
	for _, turn := range history {
		q, a := strings.TrimSpace(turn.Question), strings.TrimSpace(turn.Answer)
		if q == "" || a == "" {
			continue
		}
		result = append(result, llm.ChatTurn{
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

// search retrouve les sections pour la question. Pour une question de suivi,
// deux recherches sont fusionnées : la question seule (nouveau sujet) et la
// question complétée par la précédente (question elliptique, « et en fin de
// partie ? »). Enrichir seule faisait dériver vers le sujet précédent.
// Toutes les questions de l'historique sont ajoutées : la précédente peut être
// elle-même elliptique, le sujet est alors porté par une plus ancienne.
func (s *Service) search(ctx context.Context, gameID, question string, history []llm.ChatTurn) ([]*store.ScoredSection, error) {
	// Candidats : fitContextBudget garde ensuite ce qui tient dans le budget
	const limit = 6
	own, err := s.retriever.Search(ctx, gameID, question, limit)
	if err != nil || len(history) == 0 {
		return own, err
	}
	// Question en premier, puis de la plus récente à la plus ancienne
	parts := []string{question}
	for i := len(history) - 1; i >= 0; i-- {
		parts = append(parts, history[i].Question)
	}
	enriched, err := s.retriever.Search(ctx, gameID, strings.Join(parts, " "), limit)
	if err != nil {
		return nil, err
	}
	return interleave(own, enriched, limit), nil
}

// maxContextRunes : taille max des extraits envoyés au LLM (~1 300 tokens).
// C'est la taille, pas le nombre de sections, qui fixe la latence : mesuré sur
// Gemma 3 4B (CPU), ~3 300 caractères → premier mot en 4,5-8 s, ~5 800 → 9-15 s.
// 4 500 laisse passer les 2 sections étendues (jusqu'à ~2 100 chacune).
const maxContextRunes = 4500

// fitContextBudget garde les sections, dans l'ordre de pertinence, tant
// qu'elles tiennent dans maxContextRunes. Une section trop longue est sautée
// au profit des suivantes, plus courtes ; la première est toujours gardée.
func fitContextBudget(sections []*store.ScoredSection) []*store.ScoredSection {
	var kept []*store.ScoredSection
	used := 0
	for _, s := range sections {
		n := utf8.RuneCountInString(s.Text)
		if len(kept) > 0 && used+n > maxContextRunes {
			continue
		}
		kept = append(kept, s)
		used += n
	}
	return kept
}

// interleave alterne les résultats de a et b, sans doublon, jusqu'à limit.
func interleave(a, b []*store.ScoredSection, limit int) []*store.ScoredSection {
	result := make([]*store.ScoredSection, 0, limit)
	seen := map[string]bool{}
	for i := 0; len(result) < limit && (i < len(a) || i < len(b)); i++ {
		for _, list := range [][]*store.ScoredSection{a, b} {
			if i < len(list) && !seen[list[i].ID] && len(result) < limit {
				seen[list[i].ID] = true
				result = append(result, list[i])
			}
		}
	}
	return result
}

// buildCacheKey génère une clé de cache unique pour la question. date_update
// en fait partie : réimporter ou reprocesser le jeu la met à jour, les
// réponses calculées sur les anciennes sections ne sont plus servies.
func (s *Service) buildCacheKey(game *store.Game, question string) string {
	normalized := strings.ToLower(strings.TrimSpace(question))
	hash := sha256.Sum256(fmt.Appendf(nil, "%s:%d:%s", game.ID, game.UpdatedAt.UnixNano(), normalized))
	return "ask:" + hex.EncodeToString(hash[:])
}

// buildContext construit le texte de contexte pour le LLM.
// Chaque section indique sa page pour que le LLM puisse la citer. Les sections
// sont regroupées par chapitre, annoncé en tête de groupe : une même règle
// peut exister dans deux chapitres (mode solo / multijoueur…), le LLM doit
// voir qu'il s'agit de deux contextes distincts.
func (s *Service) buildContext(sections []*store.ScoredSection) string {
	var order []string
	groups := map[string][]*store.ScoredSection{}
	for _, sec := range sections {
		if _, ok := groups[sec.HierarchyPath]; !ok {
			order = append(order, sec.HierarchyPath)
		}
		groups[sec.HierarchyPath] = append(groups[sec.HierarchyPath], sec)
	}

	var builder strings.Builder
	i := 0
	for _, path := range order {
		if path != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n\n")
			}
			builder.WriteString("Chapitre : " + path)
		}
		for _, sec := range groups[path] {
			if builder.Len() > 0 {
				builder.WriteString("\n\n")
			}
			i++
			builder.WriteString(fmt.Sprintf("Section %d: %s", i, sec.Title))
			if ref := pageRef(sec.PageStart, sec.PageEnd); ref != "" {
				builder.WriteString(" (" + ref + ")")
			}
			builder.WriteString("\n" + sec.Text)
		}
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

// mapSections convertit les sections trouvées en sections de la réponse.
func (s *Service) mapSections(sections []*store.ScoredSection) []*Section {
	result := make([]*Section, len(sections))
	for i, sec := range sections {
		pageNum, pageEnd := 0, 0
		if sec.PageStart != nil {
			pageNum = *sec.PageStart
		}
		if sec.PageEnd != nil {
			pageEnd = *sec.PageEnd
		}
		result[i] = &Section{
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
