package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_BurstThenRefill(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(6, func() time.Time { return now }) // 6/min = 1 toutes les 10 s

	for i := 0; i < 6; i++ {
		if wait := l.allow("a"); wait != 0 {
			t.Fatalf("requête %d refusée dans la rafale", i+1)
		}
	}
	if wait := l.allow("a"); wait <= 0 || wait > 10*time.Second {
		t.Fatalf("7e requête : attente attendue ≤ 10 s, reçu %v", wait)
	}
	if wait := l.allow("b"); wait != 0 {
		t.Fatal("une autre IP ne doit pas être limitée")
	}

	now = now.Add(10 * time.Second)
	if wait := l.allow("a"); wait != 0 {
		t.Fatalf("un jeton doit être revenu après 10 s, attente %v", wait)
	}
}

func TestRateLimiter_SweepForgetsIdleClients(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(6, func() time.Time { return now })
	l.allow("a")
	now = now.Add(2 * time.Minute)
	l.allow("b")
	if _, ok := l.buckets["a"]; ok {
		t.Fatal("le client inactif aurait dû être oublié")
	}
}

func TestRateLimit_Middleware(t *testing.T) {
	h := RateLimit(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	do := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/ask", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := do(); rec.Code != http.StatusOK {
		t.Fatalf("1re requête : %d", rec.Code)
	}
	rec := do()
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("2e requête : attendu 429 avec Retry-After, reçu %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestClientIP(t *testing.T) {
	for addr, want := range map[string]string{
		"1.2.3.4:5678": "1.2.3.4",
		"1.2.3.4":      "1.2.3.4",
		"[::1]:5678":   "::1",
		"2001:db8::1":  "2001:db8::1",
	} {
		r := &http.Request{RemoteAddr: addr}
		if got := clientIP(r); got != want {
			t.Errorf("clientIP(%q) = %q, attendu %q", addr, got, want)
		}
	}
}
