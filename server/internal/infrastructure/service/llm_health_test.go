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

func sleepyAdapter(srv *httptest.Server) *ChatLLMAdapter {
	return &ChatLLMAdapter{client: http.DefaultClient, provider: &chatProvider{
		name:   "llama",
		url:    srv.URL + "/v1/chat/completions",
		health: newHealthState("llama", srv.URL+"/health", time.Minute, http.DefaultClient),
	}}
}

// healthyURL : endpoint de santé qui répond toujours 200.
func healthyURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	return srv.URL
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

func TestHealth_AsleepWaitsForWake(t *testing.T) {
	llama, _, _ := sleepyLLM(t, 1)
	m := sleepyAdapter(llama)

	resp, err := m.Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil || resp.Model != "llama" {
		t.Fatalf("en veille : attente du réveil attendue, obtenu %+v (err %v)", resp, err)
	}
}

func TestHealth_AsleepRespectsDeadline(t *testing.T) {
	llama, _, _ := sleepyLLM(t, 1000)
	m := sleepyAdapter(llama)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := m.Query(ctx, "question", "contexte", nil, nil); err == nil {
		t.Fatal("erreur attendue : réveil plus long que la requête")
	}
}

func TestWarmup_PingsHealth(t *testing.T) {
	llama, healthCalls, _ := sleepyLLM(t, 0)
	m := sleepyAdapter(llama)

	m.Warmup()
	eventually(t, func() bool { return atomic.LoadInt32(healthCalls) > 0 })
}

func TestHealth_WaitBoundedByWakeTimeout(t *testing.T) {
	llama, _, _ := sleepyLLM(t, 1000)
	m := sleepyAdapter(llama)
	m.provider.health.wakeTimeout = 200 * time.Millisecond

	if _, err := m.Query(context.Background(), "question", "contexte", nil, nil); err == nil {
		t.Fatal("erreur attendue : réveil plus long que LLM_WAKE_TIMEOUT_SECONDS")
	}
}

func TestHealth_WaitRelaunchesEndedWake(t *testing.T) {
	llama, _, _ := sleepyLLM(t, 1)
	h := newHealthState("llama", llama.URL+"/health", time.Minute, http.DefaultClient)
	// Boucle de réveil précédente arrêtée sans succès : l'attente doit la relancer
	h.wakeTimeout = 0
	h.wake()
	eventually(t, func() bool { h.mu.Lock(); defer h.mu.Unlock(); return !h.waking })
	h.wakeTimeout = 10 * time.Second

	if err := h.waitReady(context.Background()); err != nil {
		t.Fatalf("réveil attendu, obtenu %v", err)
	}
}

func TestHealth_FailureAfterHealthyCheckRetriesOnce(t *testing.T) {
	var chatCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			return
		}
		// Premier appel : le serveur se rendort juste après le contrôle de santé
		if atomic.AddInt32(&chatCalls, 1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"model":"llama","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	resp, err := sleepyAdapter(srv).Query(context.Background(), "question", "contexte", nil, nil)
	if err != nil || resp.Answer != "ok" {
		t.Fatalf("nouvel essai attendu après réveil, obtenu %+v (err %v)", resp, err)
	}
	if got := atomic.LoadInt32(&chatCalls); got != 2 {
		t.Errorf("2 appels attendus, obtenu %d", got)
	}
}
