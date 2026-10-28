package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ask-rules-server/internal/infrastructure/config"
)

// fakeChat simule une API chat/completions qui répond toujours `status`.
func fakeChat(t *testing.T, status int, model string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("Authorization") != "Bearer key-"+model {
			t.Errorf("%s : clé d'API inattendue %q", model, r.Header.Get("Authorization"))
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"id":"x","model":"` + model + `","choices":[{"message":{"role":"assistant","content":"Réponse de ` + model + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
		} else {
			_, _ = w.Write([]byte(`{"message":"Rate limit exceeded","code":"1300"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func adapterFor(srv *httptest.Server) *ChatLLMAdapter {
	return &ChatLLMAdapter{client: http.DefaultClient, provider: &chatProvider{name: "mistral", url: srv.URL, apiKey: "key-mistral", model: "mistral"}}
}

func TestQuery_CanceledContext(t *testing.T) {
	mistral, calls := fakeChat(t, http.StatusOK, "mistral")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := adapterFor(mistral).Query(ctx, "question", "contexte", nil, nil); err == nil {
		t.Fatal("erreur attendue avec un contexte annulé")
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("client parti : pas d'appel attendu, %d appels", got)
	}
}

func TestQuery_NoProviderReturnsContext(t *testing.T) {
	m := &ChatLLMAdapter{client: http.DefaultClient}
	resp, err := m.Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil || resp.Answer != "contexte" {
		t.Fatalf("sans LLM, le contexte doit être renvoyé tel quel : %+v (err %v)", resp, err)
	}
}

func TestNewChatProvider(t *testing.T) {
	if _, ok := newChatProvider(config.LLMProvider{}, http.DefaultClient); ok {
		t.Error("sans BASE_URL, le fournisseur doit être désactivé")
	}
	p, ok := newChatProvider(config.LLMProvider{BaseURL: "http://llama.railway.internal:8080/v1/"}, http.DefaultClient)
	if !ok {
		t.Fatal("fournisseur attendu")
	}
	if p.url != "http://llama.railway.internal:8080/v1/chat/completions" {
		t.Errorf("url inattendue : %s", p.url)
	}
	if p.name != "llama.railway.internal:8080" {
		t.Errorf("nom inattendu : %s", p.name)
	}
}

func TestQueryChat_NoAPIKeyOmitsAuthorization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "" {
			t.Errorf("pas d'en-tête Authorization attendu, obtenu %q", h)
		}
		_, _ = w.Write([]byte(`{"model":"local.gguf","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	m := &ChatLLMAdapter{client: http.DefaultClient, provider: &chatProvider{name: "llama", url: srv.URL}}
	resp, err := m.Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil || resp.Answer != "ok" {
		t.Fatalf("réponse attendue, obtenu %+v (err %v)", resp, err)
	}
}
