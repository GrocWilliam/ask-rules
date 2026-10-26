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

func TestBuildMessages_ReplaysHistory(t *testing.T) {
	msgs := buildMessages("Et ensuite ?", "Section 1: Tour", []entity.ChatTurn{
		{Question: "Comment jouer ?", Answer: "On pioche une carte (p. 2)."},
	})
	roles := make([]string, len(msgs))
	for i, m := range msgs {
		roles[i] = m.Role
	}
	if got := strings.Join(roles, ","); got != "system,user,assistant,user" {
		t.Fatalf("rôles inattendus : %s", got)
	}
	if msgs[1].Content != "Comment jouer ?" || msgs[2].Content != "On pioche une carte (p. 2)." {
		t.Fatalf("historique mal rejoué : %+v", msgs[1:3])
	}
	last := msgs[3].Content
	if !strings.Contains(last, "Section 1: Tour") || !strings.HasSuffix(last, "Question : Et ensuite ?") {
		t.Fatalf("dernière question sans contexte : %q", last)
	}
}

func TestBuildMessages_NoHistory(t *testing.T) {
	if msgs := buildMessages("Q", "ctx", nil); len(msgs) != 2 {
		t.Fatalf("attendu system + user, reçu %d messages", len(msgs))
	}
}

func TestBuildMessages_NoInstructionInQuestion(t *testing.T) {
	msgs := buildMessages("Comment gagner ?", "ctx", nil)
	if last := msgs[len(msgs)-1].Content; strings.Contains(last, "Réponds") {
		t.Fatalf("une consigne dans le message utilisateur fait répondre « Ok, … » : %q", last)
	}
}

func TestFillerStripper(t *testing.T) {
	cases := []struct {
		chunks []string
		want   string
	}{
		{[]string{"Ok", ", on pio", "che une carte ", "puis on la joue (p. 2)."}, "on pioche une carte puis on la joue (p. 2)."},
		{[]string{"D'accord.\n", "**Non** (p. 3)."}, "**Non** (p. 3)."},
		{[]string{"Okapi : ", "carte spéciale (p. 9)."}, "Okapi : carte spéciale (p. 9)."},
		{[]string{"Oui (p. 1)."}, "Oui (p. 1)."}, // réponse courte : envoyée au flush final
		{[]string{"Ok", "."}, "Ok."},             // rien d'autre que la formule : gardée
	}
	for _, c := range cases {
		var out strings.Builder
		f := &fillerStripper{out: func(s string) { out.WriteString(s) }}
		for _, chunk := range c.chunks {
			f.write(chunk)
		}
		f.flush()
		if out.String() != c.want {
			t.Errorf("%q → %q, attendu %q", c.chunks, out.String(), c.want)
		}
	}
}
