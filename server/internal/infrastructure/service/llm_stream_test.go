package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
	resp, err := adapterFor(srv, nil).Query(context.Background(), "q", "ctx", nil, func(s string) { got = append(got, s) })
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	if strings.Join(got, "|") != "**Oui**|, p. 4." {
		t.Fatalf("fragments inattendus : %q", got)
	}
	if resp.Answer != "**Oui**, p. 4." || resp.Model != "mistral-small" {
		t.Fatalf("réponse complète inattendue : %+v", resp)
	}
	if resp.TokensUsed == nil || resp.TokensUsed.Total != 14 {
		t.Fatalf("usage non lu : %+v", resp.TokensUsed)
	}
}

func TestQuery_StreamFallsBackBeforeFirstToken(t *testing.T) {
	mistral, _ := fakeChat(t, http.StatusTooManyRequests, "mistral")
	plugsky, _ := fakeStream(t, []string{
		`data: {"model":"plugsky","choices":[{"delta":{"content":"Réponse"},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, false)

	var got strings.Builder
	resp, err := adapterFor(mistral, plugsky).Query(context.Background(), "q", "ctx", nil, func(s string) { got.WriteString(s) })
	if err != nil {
		t.Fatalf("erreur inattendue : %v", err)
	}
	if got.String() != "Réponse" || resp.Model != "plugsky" {
		t.Fatalf("bascule attendue sur plugsky : %q %+v", got.String(), resp)
	}
}

func TestQuery_StreamNoFallbackAfterFirstToken(t *testing.T) {
	mistral, _ := fakeStream(t, []string{
		`data: {"model":"mistral","choices":[{"delta":{"content":"Début de réponse"}}]}`,
	}, true)
	plugsky, plugskyCalls := fakeStream(t, []string{`data: [DONE]`}, false)

	_, err := adapterFor(mistral, plugsky).Query(context.Background(), "q", "ctx", nil, func(string) {})
	if err == nil {
		t.Fatal("une coupure en cours de réponse doit remonter une erreur")
	}
	if n := atomic.LoadInt32(plugskyCalls); n != 0 {
		t.Fatalf("pas de bascule une fois la réponse entamée : %d appels au secours", n)
	}
}
