// middleware/auth.go — Authentification admin (cookie de session)
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"ask-rules-server/config"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const adminKey contextKey = "admin"

var (
	sessionsMu sync.RWMutex
	sessions   = map[string]time.Time{}
	sessionTTL = 8 * time.Hour
)

// RequireAdmin vérifie le cookie de session admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("admin_session")
		if err != nil || !isValidSession(cookie.Value) {
			http.Error(w, "Non autorisé", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), adminKey, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Login vérifie le mot de passe et crée une session.
func Login(password string) (string, bool) {
	if len(config.C.AdminPassword) == 0 {
		return "", false
	}

	// Comparer avec bcrypt si le mot de passe stocké commence par $2
	expected := config.C.AdminPassword
	if len(expected) > 3 && expected[:3] == "$2a" || len(expected) > 3 && expected[:3] == "$2b" {
		if err := bcrypt.CompareHashAndPassword([]byte(expected), []byte(password)); err != nil {
			return "", false
		}
	} else if password != expected {
		return "", false
	}

	token := newToken()
	sessionsMu.Lock()
	sessions[token] = time.Now().Add(sessionTTL)
	sessionsMu.Unlock()
	return token, true
}

// Logout révoque un token de session.
func Logout(token string) {
	sessionsMu.Lock()
	delete(sessions, token)
	sessionsMu.Unlock()
}

func isValidSession(token string) bool {
	sessionsMu.RLock()
	exp, ok := sessions[token]
	sessionsMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		sessionsMu.Lock()
		delete(sessions, token)
		sessionsMu.Unlock()
		return false
	}
	return true
}

func newToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}
