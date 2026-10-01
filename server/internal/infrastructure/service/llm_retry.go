// infrastructure/service/llm_retry.go — Retry et cadencement des appels LLM
//
// Les API LLM hébergées (Mistral, OpenAI) limitent le nombre de requêtes et de
// tokens par minute. On cadence les appels (pacer) pour éviter les rafales, et
// on réessaie les erreurs transitoires (429, 5xx) avec un backoff exponentiel.
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ask-rules-server/internal/domain/entity"
)

const (
	llmMaxRetries   = 3
	llmBaseBackoff  = time.Second
	llmMaxBackoff   = 8 * time.Second
	llmRetryMinLeft = 5 * time.Second // ne pas réessayer s'il reste moins que ça avant la deadline
)

// pacer espace les appels d'au moins `interval` (cadence max = 1/interval par seconde).
type pacer struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

func newPacer(requestsPerSecond float64) *pacer {
	if requestsPerSecond <= 0 {
		return &pacer{}
	}
	return &pacer{interval: time.Duration(float64(time.Second) / requestsPerSecond)}
}

// wait bloque jusqu'au prochain créneau libre (ou l'annulation du contexte).
func (p *pacer) wait(ctx context.Context) error {
	if p == nil || p.interval <= 0 {
		return nil
	}
	p.mu.Lock()
	now := time.Now()
	slot := p.next
	if slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(p.interval)
	p.mu.Unlock()

	return sleepCtx(ctx, time.Until(slot))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay : en-tête Retry-After s'il est fourni, sinon backoff exponentiel avec jitter.
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
			d := time.Duration(secs) * time.Second
			if d > llmMaxBackoff {
				d = llmMaxBackoff
			}
			return d
		}
	}
	d := llmBaseBackoff << attempt
	if d > llmMaxBackoff {
		d = llmMaxBackoff
	}
	return d + time.Duration(rand.Int63n(int64(d/2)+1))
}

// postJSONWithRetry envoie `body` en POST et renvoie la réponse 200.
// Les statuts 429/5xx sont réessayés ; un 429 persistant renvoie entity.ErrLLMRateLimited.
// L'appelant doit fermer resp.Body.
func postJSONWithRetry(ctx context.Context, client *http.Client, p *pacer, provider, url string, headers map[string]string, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		if err := p.wait(ctx); err != nil {
			return nil, fmt.Errorf("%s: %w", provider, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", provider, err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		lastErr = fmt.Errorf("%s status %d: %s", provider, resp.StatusCode, b)
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("%w: %v", entity.ErrLLMRateLimited, lastErr)
		}
		if !isRetryableStatus(resp.StatusCode) || attempt >= llmMaxRetries {
			return nil, lastErr
		}

		delay := retryDelay(resp, attempt)
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < delay+llmRetryMinLeft {
			return nil, lastErr
		}
		log.Printf("[WARN] %s - statut %d, nouvelle tentative %d/%d dans %s",
			provider, resp.StatusCode, attempt+1, llmMaxRetries, delay.Round(time.Millisecond))
		if err := sleepCtx(ctx, delay); err != nil {
			return nil, lastErr
		}
	}
}
