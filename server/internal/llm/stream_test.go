package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeStream simule une API chat/completions en streaming : envoie `events`
// (lignes « data: » déjà formées), puis coupe la connexion si `cut` est vrai.
func fakeStream(t *testing.T, events []string, cut bool) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Errorf("stream:true attendu dans la requête")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			_, _ = w.Write([]byte(e + "\n\n"))
			w.(http.Flusher).Flush()
		}
		if cut {
			// Coupure brutale au milieu de la réponse
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestQuery_StreamsTokens(t *testing.T) {
	srv, _ := fakeStream(t, []string{
		`: keep-alive`,
		`data: {"id":"c1","model":"mistral-small","choices":[{"delta":{"role":"assistant","content":""}}]}`,
		`data: {"id":"c1","model":"mistral-small","choices":[{"delta":{"content":"**Oui**"}}]}`,
		`data: {"id":"c1","model":"mistral-small","choices":[{"delta":{"content":[{"type":"thinking","thinking":[]},{"type":"text","text":", p. 4."}]}}]}`,
		`data: {"id":"c1","model":"mistral-small","choices":[{"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`,
		`data: [DONE]`,
	}, false)

	var got []string
	resp, err := clientFor(srv).Query(context.Background(), "q", "ctx", nil, func(s string) { got = append(got, s) })
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	// Le début du flux est regroupé par fillerStripper : on vérifie le texte transmis
	if strings.Join(got, "") != "**Oui**, p. 4." {
		t.Fatalf("fragments inattendus : %q", got)
	}
	if resp.Answer != "**Oui**, p. 4." || resp.Model != "mistral-small" {
		t.Fatalf("réponse complète inattendue : %+v", resp)
	}
	if resp.Tokens == nil || resp.Tokens.Total != 14 {
		t.Fatalf("usage non lu : %+v", resp.Tokens)
	}
}

func TestQuery_StreamNoRetryAfterFirstToken(t *testing.T) {
	llama, calls := fakeStream(t, []string{
		`data: {"model":"llama","choices":[{"delta":{"content":"Début de réponse"}}]}`,
	}, true)
	m := clientFor(llama)
	m.provider.health = newHealthState("llama", healthyURL(t), time.Minute, http.DefaultClient)

	_, err := m.Query(context.Background(), "q", "ctx", nil, func(string) {})
	if err == nil {
		t.Fatal("une coupure en cours de réponse doit remonter une erreur")
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("pas de nouvel essai une fois la réponse entamée : %d appels", n)
	}
}
