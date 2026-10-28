// web/auth.go — Use cases pour l'authentification admin
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"ask-rules-server/internal/store"
)

var (
	// ErrInvalidPassword est retourné quand le mot de passe est incorrect.
	ErrInvalidPassword = errors.New("invalid password")

	// ErrSessionNotFound est retourné quand la session n'existe pas.
	ErrSessionNotFound = errors.New("session not found")
)

// Auth gère la logique métier pour l'authentification admin.
type Auth struct {
	adminPassword string
	sessions      map[string]time.Time
	mu            sync.RWMutex
	logRepo       store.LogRepository
}

// NewAuth crée le gestionnaire de sessions ; sans mot de passe, l'admin est désactivé.
func NewAuth(adminPassword string, logRepo store.LogRepository) *Auth {
	a := &Auth{
		adminPassword: adminPassword,
		sessions:      make(map[string]time.Time),
		logRepo:       logRepo,
	}

	// Nettoyage périodique des sessions expirées
	go a.cleanupExpiredSessions()

	return a
}

// LoginRequest représente la requête de login.
type LoginRequest struct {
	Password string `json:"password"`
}

// LoginResponse représente la réponse de login.
type LoginResponse struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

// Login vérifie le mot de passe et crée une session.
func (a *Auth) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// Sans ADMIN_PASSWORD, l'admin est désactivé (pas de mot de passe par défaut).
	// Comparaison à temps constant : la durée ne révèle pas le préfixe correct.
	if a.adminPassword == "" || subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.adminPassword)) != 1 {
		if a.logRepo != nil {
			_ = a.logRepo.Save(ctx, &store.LogEntry{
				EventType: "admin_login_failed",
				Message:   "Tentative de connexion admin échouée",
			})
		}
		return nil, ErrInvalidPassword
	}

	// Générer un token de session
	token := generateToken()
	expires := time.Now().Add(8 * time.Hour)

	a.mu.Lock()
	a.sessions[token] = expires
	a.mu.Unlock()

	if a.logRepo != nil {
		_ = a.logRepo.Save(ctx, &store.LogEntry{
			EventType: "admin_login",
			Message:   "Connexion admin réussie",
		})
	}

	return &LoginResponse{
		Token:   token,
		Expires: expires,
	}, nil
}

// Logout révoque une session.
func (a *Auth) Logout(ctx context.Context, token string) error {
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
	return nil
}

// CheckSession vérifie si une session est valide.
func (a *Auth) CheckSession(ctx context.Context, token string) (bool, error) {
	a.mu.RLock()
	expires, exists := a.sessions[token]
	a.mu.RUnlock()

	if !exists {
		return false, nil
	}

	if time.Now().After(expires) {
		// Session expirée
		a.mu.Lock()
		delete(a.sessions, token)
		a.mu.Unlock()
		return false, nil
	}

	return true, nil
}

// cleanupExpiredSessions supprime périodiquement les sessions expirées.
func (a *Auth) cleanupExpiredSessions() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		a.mu.Lock()
		for token, expires := range a.sessions {
			if now.After(expires) {
				delete(a.sessions, token)
			}
		}
		a.mu.Unlock()
	}
}

// generateToken génère un token de session aléatoire.
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
