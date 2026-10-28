package web

import (
	"context"
	"net/http/httptest"
	"testing"

	"ask-rules-server/internal/store"
)

type fakeLogs struct{ limit int }

func (f *fakeLogs) GetRecent(ctx context.Context, limit int) ([]*store.LogEntry, error) {
	f.limit = limit
	return nil, nil
}

func (f *fakeLogs) Save(context.Context, *store.LogEntry) error { return nil }

func TestLogsHandler_Limit(t *testing.T) {
	for query, want := range map[string]int{"": 100, "?limit=50": 50, "?limit=0": 100, "?limit=abc": 100, "?limit=501": 100, "?limit=500": 500} {
		repo := &fakeLogs{}
		rec := httptest.NewRecorder()
		NewLogsHandler(repo).Get(rec, httptest.NewRequest("GET", "/api/admin/logs"+query, nil))
		if repo.limit != want {
			t.Errorf("%q : limit = %d, attendu %d", query, repo.limit, want)
		}
		if body := rec.Body.String(); body != "[]\n" {
			t.Errorf("%q : corps = %q, attendu une liste vide", query, body)
		}
	}
}
