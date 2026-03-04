// interfaces/http/router/router.go — Configuration du routeur Chi
package router

import (
	"io/fs"
	"net/http"
	"time"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/interfaces/http/handler"
	customMiddleware "ask-rules-server/internal/interfaces/http/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Config contient toutes les dépendances nécessaires pour configurer le routeur.
type Config struct {
	AskHandler       *handler.AskHandler
	GamesHandler     *handler.GamesHandler
	ImportHandler    *handler.ImportHandler
	AdminHandler     *handler.AdminHandler
	LogsHandler      *handler.LogsHandler
	FilesHandler     *handler.FilesHandler
	AdminAuthUseCase *usecase.AdminAuthUseCase
	// StaticFS contient le front-end SvelteKit embarqué (go:embed build/).
	// Si nil, le front-end n'est pas servi (mode développement local).
	StaticFS fs.FS
}

// NewRouter crée et configure un nouveau routeur Chi.
func NewRouter(cfg *Config) *chi.Mux {
	r := chi.NewRouter()

	// ── Middlewares globaux ──────────────────────────────────────────────────

	// Logger
	r.Use(middleware.Logger)

	// Récupération des panics
	r.Use(middleware.Recoverer)

	// Request ID pour le traçage
	r.Use(middleware.RequestID)

	// Real IP extraction
	r.Use(middleware.RealIP)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:*", "http://127.0.0.1:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // 5 minutes
	}))

	// ── Routes publiques ─────────────────────────────────────────────────────

	r.Route("/api", func(r chi.Router) {
		// Questions & Réponses (avec timeout)
		r.With(middleware.Timeout(60*time.Second)).Post("/ask", cfg.AskHandler.Handle)

		// Import de jeux (SSE - Server-Sent Events, SANS timeout pour les opérations longues)
		r.Post("/import", cfg.ImportHandler.Import)

		// Consultation des jeux (avec timeout)
		r.With(middleware.Timeout(30*time.Second)).Get("/games", cfg.GamesHandler.List)
		r.With(middleware.Timeout(30*time.Second)).Get("/games/{id}", cfg.GamesHandler.Get)

		// Routes d'administration
		r.Route("/admin", func(r chi.Router) {
			// Authentification admin (sans middleware, avec timeout court)
			r.With(middleware.Timeout(10*time.Second)).Post("/login", cfg.AdminHandler.Login)
			r.With(middleware.Timeout(10*time.Second)).Post("/logout", cfg.AdminHandler.Logout)
			r.With(middleware.Timeout(10*time.Second)).Get("/check", cfg.AdminHandler.Check)

			// Routes protégées par authentification
			r.Group(func(r chi.Router) {
				// Middleware d'authentification admin
				r.Use(customMiddleware.AdminAuth(cfg.AdminAuthUseCase))

				// Gestion des jeux (avec timeout)
				r.With(middleware.Timeout(30*time.Second)).Get("/games", cfg.GamesHandler.List)
				r.With(middleware.Timeout(30*time.Second)).Post("/games", cfg.GamesHandler.Upsert)
				r.With(middleware.Timeout(30*time.Second)).Delete("/games/{id}", cfg.GamesHandler.Delete)

				// Reprocess (SSE - sans timeout)
				r.Post("/games/{id}/reprocess", cfg.ImportHandler.Reprocess)
				r.Post("/reprocess-all", cfg.ImportHandler.ReprocessAll)

				// Logs (avec timeout)
				r.With(middleware.Timeout(30*time.Second)).Get("/logs", cfg.LogsHandler.Get)

				// Fichiers uploadés (avec timeout)
				r.With(middleware.Timeout(30*time.Second)).Get("/files", cfg.FilesHandler.List)
				r.With(middleware.Timeout(30*time.Second)).Delete("/files/{slug}/{filename}", cfg.FilesHandler.Delete)
			})
		})
	})

	// ── Fichiers statiques ───────────────────────────────────────────────────
	// Timeout plus long pour permettre le téléchargement de gros PDF
	r.With(middleware.Timeout(120*time.Second)).Get("/files/{slug}/{filename}", cfg.FilesHandler.Serve)

	// ── Health check ─────────────────────────────────────────────────────────
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// ── Front-end SvelteKit (SPA fallback) ────────────────────────────────────
	// Toutes les routes non-API renvoient index.html pour le routing côté client.
	// Les assets statiques (_app/, favicon, etc.) sont servis directement.
	if cfg.StaticFS != nil {
		fileServer := http.FileServer(http.FS(cfg.StaticFS))
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			// Retirer le '/' initial ; la racine devient ""
			path := req.URL.Path
			if len(path) > 0 && path[0] == '/' {
				path = path[1:]
			}
			if path == "" {
				path = "index.html"
			}
			// Tenter de servir le fichier tel quel
			f, err := cfg.StaticFS.Open(path)
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, req)
				return
			}
			// Fichier inconnu → index.html (SPA client-side routing)
			http.ServeFileFS(w, req, cfg.StaticFS, "index.html")
		})
	}

	return r
}
