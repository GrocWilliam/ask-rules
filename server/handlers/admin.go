// handlers/admin.go — Authentification admin
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"ask-rules-server/middleware"
)

// AdminLogin vérifie le mot de passe et crée un cookie de session.
func AdminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "Corps invalide", http.StatusBadRequest)
		return
	}

	token, ok := middleware.Login(body.Password)
	if !ok {
		jsonError(w, "Mot de passe incorrect", http.StatusUnauthorized)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(8 * time.Hour),
	})
	jsonOK(w, map[string]bool{"ok": true})
}

// AdminLogout révoque la session admin.
func AdminLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("admin_session")
	if err == nil {
		middleware.Logout(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "admin_session",
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
	jsonOK(w, map[string]bool{"ok": true})
}

// AdminCheck vérifie si la session admin est valide.
func AdminCheck(w http.ResponseWriter, r *http.Request) {
	// La requête arrive ici seulement si le middleware RequireAdmin a passé.
	jsonOK(w, map[string]bool{"ok": true})
}
