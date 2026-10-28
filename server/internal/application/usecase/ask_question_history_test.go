// application/usecase/ask_question_history_test.go — Tests de l'historique de conversation
package usecase_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
)

// historyFixture prépare un use case qui enregistre la recherche et l'historique reçus par le LLM.
func historyFixture(searchQueries *[]string, llmHistory *[]entity.ChatTurn, llmCalls *int) *usecase.AskQuestionUseCase {
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*entity.Game, error) {
			return &entity.Game{ID: "game-1", Name: "Wingspan"}, nil
		},
	}
	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, gameID, question string, limit int) ([]*entity.ScoredSection, error) {
			*searchQueries = append(*searchQueries, question)
			id := fmt.Sprintf("s%d", len(*searchQueries))
			return []*entity.ScoredSection{
				{ID: id, Title: "Score", Text: "Les oiseaux rapportent des points."},
				{ID: "shared", Title: "Fin", Text: "Décompte final."},
			}, nil
		},
	}
	llmSvc := &mockLLM{
		queryFunc: func(ctx context.Context, question, context string, history []entity.ChatTurn, onToken func(string)) (*entity.LLMResponse, error) {
			*llmCalls++
			*llmHistory = history
			return &entity.LLMResponse{Answer: "Réponse", Model: "mock-model"}, nil
		},
	}
	return usecase.NewAskQuestionUseCase(gameRepo, retriever, llmSvc, newMockCache(), &mockLogRepo{})
}

func TestAskQuestion_HistoryPassedToLLMAndSearch(t *testing.T) {
	var queries []string
	var history []entity.ChatTurn
	var calls int
	uc := historyFixture(&queries, &history, &calls)

	resp, err := uc.Execute(context.Background(), &usecase.AskRequest{
		GameName: "Wingspan",
		Question: "Et en fin de partie ?",
		History:  []entity.ChatTurn{{Question: "Comment marque-t-on des points ?", Answer: "Avec les oiseaux (p. 4)."}},
	})
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	if len(history) != 1 || history[0].Question != "Comment marque-t-on des points ?" {
		t.Fatalf("historique non transmis au LLM : %+v", history)
	}
	// La question seule (nouveau sujet) et complétée par la précédente (suivi elliptique)
	want := []string{"Et en fin de partie ?", "Et en fin de partie ? Comment marque-t-on des points ?"}
	if len(queries) != 2 || queries[0] != want[0] || queries[1] != want[1] {
		t.Fatalf("recherches inattendues : %q", queries)
	}
	var ids []string
	for _, s := range resp.Sections {
		ids = append(ids, s.Title)
	}
	if len(resp.Sections) != 3 {
		t.Fatalf("résultats des deux recherches fusionnés sans doublon attendus, reçu %v", ids)
	}
}

func TestAskQuestion_HistoryTrimmed(t *testing.T) {
	var queries []string
	var history []entity.ChatTurn
	var calls int
	uc := historyFixture(&queries, &history, &calls)

	req := &usecase.AskRequest{GameName: "Wingspan", Question: "Et ensuite ?"}
	for i := 0; i < 5; i++ {
		req.History = append(req.History, entity.ChatTurn{Question: "Q" + string(rune('1'+i)), Answer: "R"})
	}
	req.History = append(req.History,
		entity.ChatTurn{Question: "  ", Answer: "réponse sans question"}, // ignoré
		entity.ChatTurn{Question: "Longue", Answer: strings.Repeat("é", 5000)},
	)

	if _, err := uc.Execute(context.Background(), req); err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("attendu les 3 derniers échanges, reçu %d : %+v", len(history), history)
	}
	if history[0].Question != "Q4" || history[2].Question != "Longue" {
		t.Fatalf("mauvais échanges conservés : %+v", history)
	}
	if n := len([]rune(history[2].Answer)); n > 2001 {
		t.Fatalf("réponse non tronquée : %d caractères", n)
	}
	// Le sujet (Q4) reste dans la recherche même si les questions suivantes sont elliptiques
	if len(queries) != 2 || queries[1] != "Et ensuite ? Longue Q5 Q4" {
		t.Fatalf("la recherche enrichie doit reprendre tout l'historique gardé : %q", queries)
	}
}

func TestAskQuestion_HistoryBypassesCache(t *testing.T) {
	var queries []string
	var history []entity.ChatTurn
	var calls int
	uc := historyFixture(&queries, &history, &calls)

	first := &usecase.AskRequest{GameName: "Wingspan", Question: "Combien de points ?"}
	if _, err := uc.Execute(context.Background(), first); err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}

	followUp := &usecase.AskRequest{
		GameName: "Wingspan",
		Question: "Combien de points ?",
		History:  []entity.ChatTurn{{Question: "Et pour les œufs ?", Answer: "1 point par œuf."}},
	}
	resp, err := uc.Execute(context.Background(), followUp)
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	if resp.Cached || calls != 2 {
		t.Fatalf("une question avec historique ne doit pas venir du cache (cached=%v, appels LLM=%d)", resp.Cached, calls)
	}
}
