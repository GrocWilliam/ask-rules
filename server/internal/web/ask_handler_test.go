package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ask-rules-server/internal/ask"
	"ask-rules-server/internal/llm"
	"ask-rules-server/internal/store"
)

func TestAskHandleError_UnwrapsDomainErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{fmt.Errorf("game 'x' not found: %w", store.ErrGameNotFound), http.StatusNotFound},
		{ask.ErrNoSectionsFound, http.StatusNotFound},
		{fmt.Errorf("failed to generate answer: %w", fmt.Errorf("%w: mistral status 429", llm.ErrRateLimited)), http.StatusServiceUnavailable},
		{fmt.Errorf("boom"), http.StatusInternalServerError},
	}
	h := &AskHandler{}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.handleError(rec, c.err)
		if rec.Code != c.want {
			t.Errorf("%v : statut %d, attendu %d", c.err, rec.Code, c.want)
		}
	}
}
