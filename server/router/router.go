// router/router.go — Configuration du routeur Chi
package router

import (
	"embed"
	"io/fs"
	"net/http"

	"ask-rules-server/handlers"
	mw "ask-rules-server/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// New crée et configure le routeur Chi avec les assets statiques embarqués.
func New(staticFS embed.FS) http.Handler {
	r := chi.NewRouter()

	// ── Middleware global ──────────────────────────────────────────────────────
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// ── API publique ───────────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(mw.RateLimit)
		r.Post("/api/ask", handlers.Ask)
	})

	r.Post("/api/import", handlers.ImportSSE)

	r.Get("/api/games", handlers.ListGames)
	r.Get("/api/games/{id}", handlers.GetGame)

	// ── Fichiers uploadés ──────────────────────────────────────────────────────
	r.Get("/files/{slug}/{filename}", handlers.ServeFile)

	// ── Authentification admin ─────────────────────────────────────────────────
	r.Post("/api/admin/login", handlers.AdminLogin)
	r.Post("/api/admin/logout", handlers.AdminLogout)

	// ── API admin (protégée) ───────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(mw.RequireAdmin)

		r.Get("/api/admin/check", handlers.AdminCheck)
		r.Get("/api/admin/games", handlers.ListGames)
		r.Post("/api/admin/games", handlers.UpsertGame)
		r.Delete("/api/admin/games/{id}", handlers.DeleteGame)
		r.Get("/api/admin/logs", handlers.GetLogs)
		r.Get("/api/admin/files", handlers.ListFiles)
		r.Delete("/api/admin/files/{slug}/{filename}", handlers.DeleteFile)

		// Reprocess
		r.Post("/api/admin/reprocess", handlers.ReprocessGame)
		r.Post("/api/admin/reprocess-all", handlers.ReprocessAll)
	})

	// ── SPA SvelteKit (build statique) ────────────────────────────────────────
	buildFS, err := fs.Sub(staticFS, "build")
	if err == nil {
		fileServer := http.FileServer(http.FS(buildFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Tenter de servir le fichier; fallback vers index.html (SPA)
			path := r.URL.Path
			if path == "/" {
				path = "/index.html"
			}
			f, err := buildFS.Open(path[1:])
			if err != nil {
				// Fallback SPA
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}
			f.Close()
			fileServer.ServeHTTP(w, r)
		})
	}

	return r
}
