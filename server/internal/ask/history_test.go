// ask/history_test.go — Tests de l'historique de conversation
package ask_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ask-rules-server/internal/ask"
	"ask-rules-server/internal/llm"
	"ask-rules-server/internal/store"
)

// historyFixture prépare un Service qui enregistre la recherche et l'historique reçus par le LLM.
func historyFixture(searchQueries *[]string, llmHistory *[]llm.ChatTurn, llmCalls *int) *ask.Service {
	gameRepo := &mockGameRepo{
		findByNameFunc: func(ctx context.Context, name string) (*store.Game, error) {
			return &store.Game{ID: "game-1", Name: "Wingspan"}, nil
		},
	}
	retriever := &mockRetriever{
		searchFunc: func(ctx context.Context, gameID, question string, limit int) ([]*store.ScoredSection, error) {
			*searchQueries = append(*searchQueries, question)
			id := fmt.Sprintf("s%d", len(*searchQueries))
			return []*store.ScoredSection{
				{ID: id, Title: "Score", Text: "Les oiseaux rapportent des points."},
				{ID: "shared", Title: "Fin", Text: "Décompte final."},
			}, nil
		},
	}
	llmSvc := &mockLLM{
		queryFunc: func(ctx context.Context, question, context string, history []llm.ChatTurn, onToken func(string)) (*llm.Response, error) {
			*llmCalls++
			*llmHistory = history
			return &llm.Response{Answer: "Réponse", Model: "mock-model"}, nil
		},
	}
	return ask.NewService(gameRepo, retriever, llmSvc, newMockCache(), &mockLogRepo{})
}

func TestAskQuestion_HistoryPassedToLLMAndSearch(t *testing.T) {
	var queries []string
	var history []llm.ChatTurn
	var calls int
	svc := historyFixture(&queries, &history, &calls)

	resp, err := svc.Execute(context.Background(), &ask.Request{
		GameName: "Wingspan",
		Question: "Et en fin de partie ?",
		History:  []llm.ChatTurn{{Question: "Comment marque-t-on des points ?", Answer: "Avec les oiseaux (p. 4)."}},
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
	var history []llm.ChatTurn
	var calls int
	svc := historyFixture(&queries, &history, &calls)

	req := &ask.Request{GameName: "Wingspan", Question: "Et ensuite ?"}
	for i := 0; i < 5; i++ {
		req.History = append(req.History, llm.ChatTurn{Question: "Q" + string(rune('1'+i)), Answer: "R"})
	}
	req.History = append(req.History,
		llm.ChatTurn{Question: "  ", Answer: "réponse sans question"}, // ignoré
		llm.ChatTurn{Question: "Longue", Answer: strings.Repeat("é", 5000)},
	)

	if _, err := svc.Execute(context.Background(), req); err != nil {
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
	var history []llm.ChatTurn
	var calls int
	svc := historyFixture(&queries, &history, &calls)

	first := &ask.Request{GameName: "Wingspan", Question: "Combien de points ?"}
	if _, err := svc.Execute(context.Background(), first); err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}

	followUp := &ask.Request{
		GameName: "Wingspan",
		Question: "Combien de points ?",
		History:  []llm.ChatTurn{{Question: "Et pour les œufs ?", Answer: "1 point par œuf."}},
	}
	resp, err := svc.Execute(context.Background(), followUp)
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	if resp.Cached || calls != 2 {
		t.Fatalf("une question avec historique ne doit pas venir du cache (cached=%v, appels LLM=%d)", resp.Cached, calls)
	}
}
