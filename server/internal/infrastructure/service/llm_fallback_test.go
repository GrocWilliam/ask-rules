package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ask-rules-server/internal/domain/entity"
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

func adapterFor(primary, fallback *httptest.Server) *ChatLLMAdapter {
	m := &ChatLLMAdapter{client: http.DefaultClient}
	m.providers = append(m.providers, chatProvider{name: "mistral", url: primary.URL, apiKey: "key-mistral", model: "mistral"})
	if fallback != nil {
		m.providers = append(m.providers, chatProvider{name: "plugsky", url: fallback.URL, apiKey: "key-plugsky", model: "plugsky"})
	}
	return m
}

func TestFallback_MistralRateLimitedUsesPlugsky(t *testing.T) {
	mistral, mistralCalls := fakeChat(t, http.StatusTooManyRequests, "mistral")
	plugsky, plugskyCalls := fakeChat(t, http.StatusOK, "plugsky")

	resp, err := adapterFor(mistral, plugsky).Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	if resp.Answer != "Réponse de plugsky" || resp.Model != "plugsky" {
		t.Fatalf("réponse attendue de plugsky, obtenu %+v", resp)
	}
	if got := atomic.LoadInt32(mistralCalls); got != 1 {
		t.Errorf("avec un secours, Mistral ne doit être tenté qu'une fois : %d appels", got)
	}
	if got := atomic.LoadInt32(plugskyCalls); got != 1 {
		t.Errorf("plugsky : 1 appel attendu, obtenu %d", got)
	}
}

func TestFallback_MistralOKDoesNotCallPlugsky(t *testing.T) {
	mistral, _ := fakeChat(t, http.StatusOK, "mistral")
	plugsky, plugskyCalls := fakeChat(t, http.StatusOK, "plugsky")

	resp, err := adapterFor(mistral, plugsky).Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil || resp.Model != "mistral" {
		t.Fatalf("réponse attendue de mistral, obtenu %+v (err %v)", resp, err)
	}
	if got := atomic.LoadInt32(plugskyCalls); got != 0 {
		t.Errorf("plugsky ne doit pas être appelé : %d appels", got)
	}
}

func TestFallback_BothFailReturnsFallbackError(t *testing.T) {
	mistral, _ := fakeChat(t, http.StatusTooManyRequests, "mistral")
	plugsky, _ := fakeChat(t, http.StatusUnauthorized, "plugsky")

	_, err := adapterFor(mistral, plugsky).Query(context.Background(), "question", "contexte", nil, nil)
	if err == nil {
		t.Fatal("erreur attendue")
	}
	// L'erreur finale est celle du secours (401), pas un rate limit
	if errors.Is(err, entity.ErrLLMRateLimited) {
		t.Errorf("l'erreur finale doit être celle de plugsky : %v", err)
	}
}

func TestFallback_CanceledContextSkipsFallback(t *testing.T) {
	mistral, _ := fakeChat(t, http.StatusOK, "mistral")
	plugsky, plugskyCalls := fakeChat(t, http.StatusOK, "plugsky")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := adapterFor(mistral, plugsky).Query(ctx, "question", "contexte", nil, nil); err == nil {
		t.Fatal("erreur attendue avec un contexte annulé")
	}
	if got := atomic.LoadInt32(plugskyCalls); got != 0 {
		t.Errorf("client parti : pas de bascule attendue, %d appels plugsky", got)
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

	m := &ChatLLMAdapter{client: http.DefaultClient, providers: []chatProvider{{name: "llama", url: srv.URL}}}
	resp, err := m.Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil || resp.Answer != "ok" {
		t.Fatalf("réponse attendue, obtenu %+v (err %v)", resp, err)
	}
}

func TestFallback_PrimaryTimeoutUsesFallback(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	plugsky, _ := fakeChat(t, http.StatusOK, "plugsky")

	m := adapterFor(slow, plugsky)
	m.providers[0].timeout = 50 * time.Millisecond
	resp, err := m.Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil || resp.Model != "plugsky" {
		t.Fatalf("bascule attendue sur le secours, obtenu %+v (err %v)", resp, err)
	}
}
