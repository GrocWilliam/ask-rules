package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// sleepyLLM simule llama.cpp en veille : /health répond 503 pour les
// `asleepChecks` premiers contrôles, puis 200.
func sleepyLLM(t *testing.T, asleepChecks int32) (srv *httptest.Server, healthCalls, chatCalls *int32) {
	t.Helper()
	healthCalls, chatCalls = new(int32), new(int32)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if atomic.AddInt32(healthCalls, 1) <= asleepChecks {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			return
		}
		atomic.AddInt32(chatCalls, 1)
		_, _ = w.Write([]byte(`{"model":"llama","choices":[{"message":{"content":"Réponse de llama"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, healthCalls, chatCalls
}

func withHealth(p chatProvider, srv *httptest.Server) chatProvider {
	p.url = srv.URL + "/v1/chat/completions"
	p.health = newHealthState(p.name, srv.URL+"/health", time.Minute, http.DefaultClient)
	return p
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition jamais atteinte")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHealth_AsleepPrimaryUsesFallbackThenWakes(t *testing.T) {
	llama, _, llamaChat := sleepyLLM(t, 1)
	plugsky, _ := fakeChat(t, http.StatusOK, "plugsky")
	m := adapterFor(llama, plugsky)
	m.providers[0] = withHealth(m.providers[0], llama)

	resp, err := m.Query(context.Background(), "question", "contexte", nil)
	if err != nil || resp.Model != "plugsky" {
		t.Fatalf("en veille : réponse attendue du secours, obtenu %+v (err %v)", resp, err)
	}
	if got := atomic.LoadInt32(llamaChat); got != 0 {
		t.Errorf("serveur en veille : pas d'appel chat attendu, %d appels", got)
	}

	// Le réveil en arrière-plan aboutit : les questions suivantes vont au principal
	eventually(t, m.providers[0].health.fresh)
	resp, err = m.Query(context.Background(), "question", "contexte", nil)
	if err != nil || resp.Model != "llama" {
		t.Fatalf("réveillé : réponse attendue du principal, obtenu %+v (err %v)", resp, err)
	}
}

func TestHealth_AsleepWithoutFallbackWaits(t *testing.T) {
	llama, _, _ := sleepyLLM(t, 1)
	m := &ChatLLMAdapter{client: http.DefaultClient}
	m.providers = []chatProvider{withHealth(chatProvider{name: "llama"}, llama)}

	resp, err := m.Query(context.Background(), "question", "contexte", nil)
	if err != nil || resp.Model != "llama" {
		t.Fatalf("sans secours : attente du réveil attendue, obtenu %+v (err %v)", resp, err)
	}
}

func TestHealth_AsleepWithoutFallbackRespectsDeadline(t *testing.T) {
	llama, _, _ := sleepyLLM(t, 1000)
	m := &ChatLLMAdapter{client: http.DefaultClient}
	m.providers = []chatProvider{withHealth(chatProvider{name: "llama"}, llama)}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := m.Query(ctx, "question", "contexte", nil); err == nil {
		t.Fatal("erreur attendue : réveil plus long que la requête")
	}
}

func TestWarmup_PingsHealth(t *testing.T) {
	llama, healthCalls, _ := sleepyLLM(t, 0)
	m := &ChatLLMAdapter{client: http.DefaultClient}
	m.providers = []chatProvider{withHealth(chatProvider{name: "llama"}, llama)}

	m.Warmup()
	eventually(t, func() bool { return atomic.LoadInt32(healthCalls) > 0 })
}
