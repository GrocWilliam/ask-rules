package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
	"ask-rules-server/internal/domain/service"
)

// Fakes minimaux : l'interface embarquée fournit les méthodes non utilisées.
type fakeGames struct{ repository.GameRepository }

func (fakeGames) FindByName(_ context.Context, name string) (*entity.Game, error) {
	if name != "Wingspan" {
		return nil, entity.ErrGameNotFound
	}
	return &entity.Game{ID: "g1", Name: name}, nil
}

type fakeRetriever struct{}

func (fakeRetriever) Search(context.Context, *service.SearchRequest) ([]*entity.ScoredSection, error) {
	return []*entity.ScoredSection{{ID: "s1", Title: "Score", Text: "…"}}, nil
}

type fakeCache struct{ service.CacheService }

func (fakeCache) Get(context.Context, string) (interface{}, error) { return nil, nil }
func (fakeCache) Set(context.Context, string, interface{}) error   { return nil }

type fakeLLM struct{ service.LLMService }

func (fakeLLM) Query(_ context.Context, _, _ string, _ []entity.ChatTurn, onToken func(string)) (*service.LLMResponse, error) {
	onToken("**Oui**")
	onToken(" (p. 4).")
	return &service.LLMResponse{Answer: "**Oui** (p. 4).", Model: "mock", UsedLLM: true}, nil
}

func streamHandler() *AskHandler {
	return NewAskHandler(usecase.NewAskQuestionUseCase(fakeGames{}, fakeRetriever{}, fakeLLM{}, fakeCache{}, nil))
}

func postStream(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	streamHandler().HandleStream(rec, httptest.NewRequest(http.MethodPost, "/api/ask/stream", strings.NewReader(body)))
	return rec
}

func TestHandleStream_SendsDeltasThenDone(t *testing.T) {
	rec := postStream(`{"game":"Wingspan","question":"Peut-on rejouer ?"}`)

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("statut %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	want := []string{
		"event: delta\ndata: {\"text\":\"**Oui**\"}\n\n",
		"event: delta\ndata: {\"text\":\" (p. 4).\"}\n\n",
		"event: done\ndata: {\"answer\":\"**Oui** (p. 4).\"",
	}
	pos := 0
	for _, w := range want {
		i := strings.Index(body[pos:], w)
		if i < 0 {
			t.Fatalf("événement manquant ou dans le désordre : %q\ncorps : %s", w, body)
		}
		pos += i + len(w)
	}
}

func TestHandleStream_ErrorBeforeFirstTokenIsJSON(t *testing.T) {
	cases := map[string]int{
		`{"game":"","question":"Q ?"}`:        http.StatusBadRequest,
		`{"game":"Inconnu","question":"Q ?"}`: http.StatusNotFound,
		`pas du json`:                         http.StatusBadRequest,
	}
	for body, status := range cases {
		rec := postStream(body)
		if rec.Code != status || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s : statut %d (%s), attendu %d en JSON", body, rec.Code, rec.Header().Get("Content-Type"), status)
		}
	}
}
