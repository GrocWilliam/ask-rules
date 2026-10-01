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
)

// statusServer répond successivement avec les statuts donnés (le dernier est répété).
func statusServer(t *testing.T, statuses ...int) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&calls, 1)) - 1
		if n >= len(statuses) {
			n = len(statuses) - 1
		}
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(statuses[n])
		_, _ = w.Write([]byte(`{"message":"x"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestPostJSONWithRetry_RecoversAfter429(t *testing.T) {
	srv, calls := statusServer(t, http.StatusTooManyRequests, http.StatusOK)

	resp, err := postJSONWithRetry(context.Background(), srv.Client(), nil, "test", srv.URL, nil, []byte(`{}`))
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	resp.Body.Close()
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("attendu 2 appels, obtenu %d", got)
	}
}

func TestPostJSONWithRetry_Persistent429IsRateLimited(t *testing.T) {
	srv, calls := statusServer(t, http.StatusTooManyRequests)
	// La deadline empêche d'enchaîner toutes les tentatives : on vérifie l'abandon anticipé
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	_, err := postJSONWithRetry(ctx, srv.Client(), nil, "test", srv.URL, nil, []byte(`{}`))
	if !errors.Is(err, entity.ErrLLMRateLimited) {
		t.Fatalf("attendu ErrLLMRateLimited, obtenu %v", err)
	}
	if got := atomic.LoadInt32(calls); got < 2 || got > llmMaxRetries+1 {
		t.Fatalf("nombre d'appels inattendu : %d", got)
	}
}

func TestPostJSONWithRetry_NoRetryOnClientError(t *testing.T) {
	srv, calls := statusServer(t, http.StatusUnauthorized)

	_, err := postJSONWithRetry(context.Background(), srv.Client(), nil, "test", srv.URL, nil, []byte(`{}`))
	if err == nil || errors.Is(err, entity.ErrLLMRateLimited) {
		t.Fatalf("attendu une erreur 401 simple, obtenu %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("un 401 ne doit pas être réessayé : %d appels", got)
	}
}

func TestPacer_SpacesCalls(t *testing.T) {
	p := newPacer(20) // 50 ms entre deux appels
	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := p.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 140*time.Millisecond {
		t.Fatalf("4 appels à 20/s doivent prendre ≥ 150 ms, obtenu %s", elapsed)
	}
}

func TestPacer_DisabledWhenZero(t *testing.T) {
	p := newPacer(0)
	start := time.Now()
	for i := 0; i < 100; i++ {
		_ = p.wait(context.Background())
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("un pacer à 0 ne doit pas attendre")
	}
}
