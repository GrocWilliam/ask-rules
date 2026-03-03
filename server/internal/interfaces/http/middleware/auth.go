// interfaces/http/middleware/auth.go — Middleware d'authentification admin
package middleware

import (
	"encoding/json"
	"net/http"

	"ask-rules-server/internal/application/usecase"
)

// AdminAuth crée un middleware qui vérifie l'authentification admin.
func AdminAuth(authUseCase *usecase.AdminAuthUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Récupérer le cookie de session
			cookie, err := r.Cookie("admin_session")
			if err != nil {
				respondUnauthorized(w, "Authentication required")
				return
			}

			// Vérifier la validité de la session
			valid, err := authUseCase.CheckSession(r.Context(), cookie.Value)
			if err != nil || !valid {
				respondUnauthorized(w, "Session invalid or expired")
				return
			}

			// Session valide, continuer
			next.ServeHTTP(w, r)
		})
	}
}

// respondUnauthorized envoie une réponse 401 Unauthorized.
func respondUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
