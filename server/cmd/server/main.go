// cmd/server/main.go — Point d'entrée de l'application (Clean Architecture)
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	// Domain (interfaces uniquement)
	"ask-rules-server/internal/application/usecase"

	// Infrastructure (implémentations)
	"ask-rules-server/internal/infrastructure/config"
	"ask-rules-server/internal/infrastructure/persistence/db"
	postgresRepo "ask-rules-server/internal/infrastructure/persistence/postgres"
	infraService "ask-rules-server/internal/infrastructure/service"

	// Interfaces HTTP
	"ask-rules-server/internal/interfaces/http/handler"
	"ask-rules-server/internal/interfaces/http/router"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// 0. Charger la configuration depuis .env
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 1. Initialiser la base de données
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, config.C.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Assigner le pool global pour les migrations
	db.Pool = pool

	// Exécuter les migrations automatiques
	log.Println("Applying database migrations...")
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Database migrations completed successfully")

	// 2. Créer les repositories (Clean Architecture)
	gameRepo := postgresRepo.NewGameRepository(pool)
	sectionRepo := postgresRepo.NewSectionRepository(pool)
	logRepo := postgresRepo.NewLogRepository(pool)

	// 3. Créer les services via adapters
	embedderSvc := infraService.NewONNXEmbedder() // Instance unique — ORT ne peut être initialisé qu'une fois
	llmSvc := infraService.NewMistralLLM()
	cacheSvc := infraService.NewRedisCache()
	retrieverSvc := infraService.NewHybridRetriever(sectionRepo, embedderSvc)
	pipelineSvc := infraService.NewPipeline(gameRepo, sectionRepo, embedderSvc)

	// 4. Créer les use cases
	// Game use cases
	askUseCase := usecase.NewAskQuestionUseCase(gameRepo, retrieverSvc, llmSvc, cacheSvc, logRepo)
	listGamesUseCase := usecase.NewListGamesUseCase(gameRepo)
	getGameUseCase := usecase.NewGetGameUseCase(gameRepo)
	upsertGameUseCase := usecase.NewUpsertGameUseCase(gameRepo)
	deleteGameUseCase := usecase.NewDeleteGameUseCase(gameRepo, sectionRepo, cacheSvc)
	importGameUseCase := usecase.NewImportGameUseCase(gameRepo, sectionRepo, pipelineSvc, config.C.UploadsDir, logRepo)
	reprocessGameUseCase := usecase.NewReprocessGameUseCase(gameRepo, sectionRepo, pipelineSvc, config.C.UploadsDir, logRepo)

	// Admin use cases
	adminAuthUseCase := usecase.NewAdminAuthUseCase(config.C.AdminPassword, logRepo)

	// Logs use cases
	getLogsUseCase := usecase.NewGetLogsUseCase(logRepo)

	// Files use cases
	listFilesUseCase := usecase.NewListFilesUseCase(config.C.UploadsDir)
	deleteFileUseCase := usecase.NewDeleteFileUseCase(config.C.UploadsDir)
	serveFileUseCase := usecase.NewServeFileUseCase(config.C.UploadsDir)

	// 6. Créer les handlers HTTP
	askHandler := handler.NewAskHandler(askUseCase)
	gamesHandler := handler.NewGamesHandler(listGamesUseCase, getGameUseCase, upsertGameUseCase, deleteGameUseCase)
	importHandler := handler.NewImportHandler(importGameUseCase, reprocessGameUseCase)
	adminHandler := handler.NewAdminHandler(adminAuthUseCase)
	logsHandler := handler.NewLogsHandler(getLogsUseCase)
	filesHandler := handler.NewFilesHandler(listFilesUseCase, deleteFileUseCase, serveFileUseCase)

	// 7. Configurer le routeur Chi
	r := router.NewRouter(&router.Config{
		AskHandler:       askHandler,
		GamesHandler:     gamesHandler,
		ImportHandler:    importHandler,
		AdminHandler:     adminHandler,
		LogsHandler:      logsHandler,
		FilesHandler:     filesHandler,
		AdminAuthUseCase: adminAuthUseCase,
	})

	// 8. Démarrer le serveur
	addr := ":" + config.C.Port
	fmt.Printf("🚀 Server starting on %s\n", addr)
	fmt.Printf("📂 Uploads dir  : %s\n", config.C.UploadsDir)
	fmt.Printf("🤖 Model path   : %s\n", config.C.ModelPath)
	fmt.Printf("📊 Repositories: Game, Section, Log\n")
	fmt.Printf("🔧 Services: ONNX Embedder, Mistral LLM, Hybrid Retriever, Redis Cache, Pipeline\n")
	fmt.Printf("📝 Use Cases: Ask, Import, ListGames, GetGame, UpsertGame, DeleteGame, AdminAuth, GetLogs, ManageFiles\n")
	fmt.Printf("🌐 Endpoints:\n")
	fmt.Printf("  GET    /health                               - Health check\n")
	fmt.Printf("  POST   /api/ask                              - Ask a question about a game\n")
	fmt.Printf("  POST   /api/import                           - Import game files (SSE)\n")
	fmt.Printf("  GET    /api/games                            - List all games\n")
	fmt.Printf("  GET    /api/games/{id}                       - Get game details\n")
	fmt.Printf("  GET    /files/{slug}/{filename}              - Serve uploaded files\n")
	fmt.Printf("  POST   /api/admin/login                      - Admin login\n")
	fmt.Printf("  POST   /api/admin/logout                     - Admin logout\n")
	fmt.Printf("  GET    /api/admin/check                      - Check admin session\n")
	fmt.Printf("  POST   /api/admin/games                      - Create/update game\n")
	fmt.Printf("  DELETE /api/admin/games/{id}                 - Delete game\n")
	fmt.Printf("  POST   /api/admin/games/{id}/reprocess        - Reprocess existing game files (SSE)\n")
	fmt.Printf("  POST   /api/admin/reprocess-all               - Reprocess all games (SSE)\n")
	fmt.Printf("  GET    /api/admin/logs                       - Get logs\n")
	fmt.Printf("  GET    /api/admin/files                      - List uploaded files\n")
	fmt.Printf("  DELETE /api/admin/files/{slug}/{filename}    - Delete file\n")

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
