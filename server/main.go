// main.go — Point d'entrée du serveur Go ask-rules
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ask-rules-server/cache"
	"ask-rules-server/config"
	"ask-rules-server/db"
	"ask-rules-server/embedder"
	"ask-rules-server/router"
)

//go:embed all:build
var staticFS embed.FS

func main() {
	if err := config.Load(); err != nil {
		log.Printf("Avertissement config: %v", err)
	}
	if err := db.Connect(); err != nil {
		log.Fatalf("Impossible de se connecter à PostgreSQL: %v", err)
	}
	defer db.Close()
	log.Println("✔ PostgreSQL connecté")
	log.Println("Migration du schéma...")
	if err := db.Migrate(context.Background()); err != nil {
		log.Fatalf("Échec migration: %v", err)
	}
	log.Println("✔ Schéma à jour")
	if err := embedder.Init(); err != nil {
		log.Printf("Avertissement: modèle d'embedding non initialisé: %v", err)
	} else {
		log.Printf("✔ Modèle d'embedding chargé depuis %s", config.C.ModelPath)
	}
	cache.Init()
	if config.C.RedisEnabled {
		log.Println("✔ Redis activé")
	}
	handler := router.New(staticFS)
	srv := &http.Server{
		Addr:         ":" + config.C.Port,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		fmt.Printf("🚀 Serveur sur http://localhost:%s\n", config.C.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erreur serveur: %v", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Arrêt du serveur...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("Serveur arrêté")
}
