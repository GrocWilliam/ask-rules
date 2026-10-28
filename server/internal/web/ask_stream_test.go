package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ask-rules-server/internal/ask"
	"ask-rules-server/internal/llm"
	"ask-rules-server/internal/store"
)

// Fakes minimaux : l'interface embarquée fournit les méthodes non utilisées.
type fakeGames struct{ store.GameRepository }

func (fakeGames) FindByName(_ context.Context, name string) (*store.Game, error) {
	if name != "Wingspan" {
		return nil, store.ErrGameNotFound
	}
	return &store.Game{ID: "g1", Name: name}, nil
}

type fakeRetriever struct{}

func (fakeRetriever) Search(context.Context, string, string, int) ([]*store.ScoredSection, error) {
	return []*store.ScoredSection{{ID: "s1", Title: "Score", Text: "…"}}, nil
}

type fakeCache struct{}

func (fakeCache) Get(context.Context, string) (interface{}, error) { return nil, nil }
func (fakeCache) Set(context.Context, string, interface{}) error   { return nil }

type fakeLLM struct{}

func (fakeLLM) Query(_ context.Context, _, _ string, _ []llm.ChatTurn, onToken func(string)) (*llm.Response, error) {
	onToken("**Oui**")
	onToken(" (p. 4).")
	return &llm.Response{Answer: "**Oui** (p. 4).", Model: "mock"}, nil
}

func streamHandler() *AskHandler {
	return NewAskHandler(ask.NewService(fakeGames{}, fakeRetriever{}, fakeLLM{}, fakeCache{}, nil))
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
