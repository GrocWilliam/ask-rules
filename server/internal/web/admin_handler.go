// web/admin_handler.go — Handlers HTTP pour l'admin
package web

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"
)

// AdminHandler gère les requêtes HTTP pour l'authentification admin.
type AdminHandler struct {
	auth *Auth
}

// NewAdminHandler crée un nouveau handler.
func NewAdminHandler(auth *Auth) *AdminHandler {
	return &AdminHandler{auth: auth}
}

// Login traite les requêtes POST /api/admin/login
func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] /api/admin/login - Invalid request body: %v", err)
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	response, err := h.auth.Login(r.Context(), &req)
	if err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			log.Printf("[WARN] /api/admin/login - Invalid password attempt")
			respondError(w, http.StatusUnauthorized, "Invalid password")
			return
		}
		log.Printf("[ERROR] /api/admin/login - Internal error: %v", err)
		respondError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Définir le cookie de session
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    response.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
		Expires:  response.Expires,
	})

	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Logout traite les requêtes POST /api/admin/logout
func (h *AdminHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("admin_session")
	if err == nil {
		_ = h.auth.Logout(r.Context(), cookie.Value)
	}

	// Supprimer le cookie
	http.SetCookie(w, &http.Cookie{
		Name:    "admin_session",
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})

	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Check traite les requêtes GET /api/admin/check
func (h *AdminHandler) Check(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("admin_session")
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	valid, err := h.auth.CheckSession(r.Context(), cookie.Value)
	if err != nil || !valid {
		respondError(w, http.StatusUnauthorized, "Session invalid or expired")
		return
	}

	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
