// cmd/server/main.go — Point d'entrée : configuration, base, services, routes HTTP
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"ask-rules-server/internal/ask"
	"ask-rules-server/internal/cache"
	"ask-rules-server/internal/config"
	"ask-rules-server/internal/embed"
	"ask-rules-server/internal/index"
	"ask-rules-server/internal/llm"
	"ask-rules-server/internal/search"
	"ask-rules-server/internal/store"
	"ask-rules-server/internal/web"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Base de données et migrations automatiques
	ctx := context.Background()
	if err := store.Connect(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer store.Close()

	log.Println("Applying database migrations...")
	if err := store.Migrate(ctx); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Database migrations completed successfully")

	games := store.NewGameRepository(store.Pool)
	sections := store.NewSectionRepository(store.Pool)
	logs := store.NewLogRepository(store.Pool)

	embedder := embed.New() // Instance unique — ORT ne peut être initialisé qu'une fois
	llmClient := llm.New()
	retriever := &search.Retriever{Embedder: embedder, Sections: sections, MinVectorScore: config.C.VectorMinScore}

	asker := ask.NewService(games, retriever, llmClient, cache.New(), logs)
	indexer := index.NewService(games, sections, index.NewPipeline(games, sections, embedder), logs)
	auth := web.NewAuth(config.C.AdminPassword, logs)

	r := web.NewRouter(&web.Config{
		AskHandler:            web.NewAskHandler(asker),
		GamesHandler:          web.NewGamesHandler(games, indexer),
		ImportHandler:         web.NewImportHandler(indexer),
		AdminHandler:          web.NewAdminHandler(auth),
		LogsHandler:           web.NewLogsHandler(logs),
		FilesHandler:          web.NewFilesHandler(),
		Auth:                  auth,
		AskRateLimitPerMinute: config.C.AskRateLimitPerMinute,
		LLMWarmup:             llmClient.Warmup,
	})

	addr := ":" + config.C.Port
	fmt.Printf("🚀 Server starting on %s\n", addr)
	fmt.Printf("📂 Uploads dir  : %s\n", config.C.UploadsDir)
	fmt.Printf("🤖 Model path   : %s\n", config.C.ModelPath)

	// Configurer le serveur HTTP avec des timeouts adaptés aux imports longs (SSE)
	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Pas de timeout d'écriture pour les SSE de longue durée
		IdleTimeout:  120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
