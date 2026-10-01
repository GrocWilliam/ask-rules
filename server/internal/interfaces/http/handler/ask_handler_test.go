package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ask-rules-server/internal/domain/entity"
)

func TestAskHandleError_UnwrapsDomainErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{fmt.Errorf("game 'x' not found: %w", entity.ErrGameNotFound), http.StatusNotFound},
		{entity.ErrNoSectionsFound, http.StatusNotFound},
		{fmt.Errorf("failed to generate answer: %w", fmt.Errorf("%w: mistral status 429", entity.ErrLLMRateLimited)), http.StatusServiceUnavailable},
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
