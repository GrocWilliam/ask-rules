// interfaces/http/middleware/ratelimit.go — Limitation du nombre de requêtes par IP
package middleware

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimit limite chaque adresse IP à `perMinute` requêtes par minute (seau à
// jetons : jusqu'à `perMinute` requêtes d'affilée, puis une toutes les
// 60/perMinute secondes). Protège le quota du LLM contre les abus.
// L'IP vient de r.RemoteAddr : placer middleware.RealIP avant, derrière un
// proxy de confiance (nginx écrase X-Real-IP).
// perMinute <= 0 désactive la limite.
func RateLimit(perMinute int) func(http.Handler) http.Handler {
	if perMinute <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	l := newRateLimiter(float64(perMinute), time.Now)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if wait := l.allow(clientIP(r)); wait > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Trop de questions d'affilée. Patientez quelques secondes avant de réessayer.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP retire le port de RemoteAddr ("1.2.3.4:5678" → "1.2.3.4") ; RealIP
// peut y avoir mis une IP sans port.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

type bucket struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	mu        sync.Mutex
	capacity  float64
	perSecond float64
	buckets   map[string]*bucket
	now       func() time.Time
	lastSweep time.Time
}

func newRateLimiter(perMinute float64, now func() time.Time) *rateLimiter {
	return &rateLimiter{
		capacity:  perMinute,
		perSecond: perMinute / 60,
		buckets:   map[string]*bucket{},
		now:       now,
		lastSweep: now(),
	}
}

// allow consomme un jeton pour key ; retourne 0 si la requête passe, sinon
// le temps à attendre avant le prochain jeton.
func (l *rateLimiter) allow(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.capacity, b.tokens+now.Sub(b.last).Seconds()*l.perSecond)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	return time.Duration((1 - b.tokens) / l.perSecond * float64(time.Second))
}

// sweep oublie, au plus une fois par minute, les IP dont le seau est plein :
// la mémoire reste bornée au nombre de clients actifs.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	full := time.Duration(l.capacity / l.perSecond * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) >= full {
			delete(l.buckets, k)
		}
	}
}
