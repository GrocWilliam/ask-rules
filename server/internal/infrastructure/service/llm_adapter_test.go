package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ask-rules-server/internal/domain/entity"
)

func TestChatContentText(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"chaîne", `"Réponse simple"`, "Réponse simple"},
		{"chunks", `[{"type":"thinking","thinking":[{"type":"text","text":"raisonnement"}]},{"type":"text","text":"Partie 1. "},{"type":"text","text":"Partie 2."}]`, "Partie 1. Partie 2."},
		{"null", `null`, ""},
	}
	for _, c := range cases {
		got, err := chatContentText([]byte(c.raw))
		if err != nil || got != c.want {
			t.Errorf("%s : obtenu %q (err %v), attendu %q", c.name, got, err, c.want)
		}
	}
	if _, err := chatContentText([]byte(`42`)); err == nil {
		t.Error("un contenu numérique doit être rejeté")
	}
}

func TestPostJSONWithRetry_ExhaustedMonthlyQuotaNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("X-Ratelimit-Limit-Tokens-Month", "500000")
		w.Header().Set("X-Ratelimit-Remaining-Tokens-Month", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"Rate limit exceeded","code":"1300"}`))
	}))
	defer srv.Close()

	_, err := postJSONWithRetry(context.Background(), srv.Client(), nil, "mistral", srv.URL, nil, []byte(`{}`), llmMaxRetries)
	if !errors.Is(err, entity.ErrLLMRateLimited) {
		t.Fatalf("attendu ErrLLMRateLimited, obtenu %v", err)
	}
	if !strings.Contains(err.Error(), "x-ratelimit-remaining-tokens-month=0") {
		t.Errorf("les en-têtes de limite doivent figurer dans l'erreur : %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("quota mensuel épuisé : 1 seul appel attendu, obtenu %d", got)
	}
}
