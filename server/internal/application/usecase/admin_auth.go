// application/usecase/admin_auth.go — Use cases pour l'authentification admin
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	// ErrInvalidPassword est retourné quand le mot de passe est incorrect.
	ErrInvalidPassword = errors.New("invalid password")

	// ErrSessionNotFound est retourné quand la session n'existe pas.
	ErrSessionNotFound = errors.New("session not found")
)

// AdminAuthUseCase gère la logique métier pour l'authentification admin.
type AdminAuthUseCase struct {
	adminPassword string
	sessions      map[string]time.Time
	mu            sync.RWMutex
}

// NewAdminAuthUseCase crée un nouveau use case.
func NewAdminAuthUseCase(adminPassword string) *AdminAuthUseCase {
	uc := &AdminAuthUseCase{
		adminPassword: adminPassword,
		sessions:      make(map[string]time.Time),
	}

	// Nettoyage périodique des sessions expirées
	go uc.cleanupExpiredSessions()

	return uc
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
func (uc *AdminAuthUseCase) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	if req.Password != uc.adminPassword {
		return nil, ErrInvalidPassword
	}

	// Générer un token de session
	token := generateToken()
	expires := time.Now().Add(8 * time.Hour)

	uc.mu.Lock()
	uc.sessions[token] = expires
	uc.mu.Unlock()

	return &LoginResponse{
		Token:   token,
		Expires: expires,
	}, nil
}

// Logout révoque une session.
func (uc *AdminAuthUseCase) Logout(ctx context.Context, token string) error {
	uc.mu.Lock()
	delete(uc.sessions, token)
	uc.mu.Unlock()
	return nil
}

// CheckSession vérifie si une session est valide.
func (uc *AdminAuthUseCase) CheckSession(ctx context.Context, token string) (bool, error) {
	uc.mu.RLock()
	expires, exists := uc.sessions[token]
	uc.mu.RUnlock()

	if !exists {
		return false, nil
	}

	if time.Now().After(expires) {
		// Session expirée
		uc.mu.Lock()
		delete(uc.sessions, token)
		uc.mu.Unlock()
		return false, nil
	}

	return true, nil
}

// cleanupExpiredSessions supprime périodiquement les sessions expirées.
func (uc *AdminAuthUseCase) cleanupExpiredSessions() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		uc.mu.Lock()
		for token, expires := range uc.sessions {
			if now.After(expires) {
				delete(uc.sessions, token)
			}
		}
		uc.mu.Unlock()
	}
}

// generateToken génère un token de session aléatoire.
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
