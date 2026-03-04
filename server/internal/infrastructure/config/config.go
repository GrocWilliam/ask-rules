// config/config.go — Chargement de la configuration depuis .env
package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Serveur
	Port string

	// Base de données
	DatabaseURL string

	// LLM
	MistralAPIKey string
	MistralModel  string
	OpenAIAPIKey  string
	OpenAIModel   string
	OllamaHost    string
	OllamaModel   string

	// Admin
	AdminPassword string

	// Redis
	RedisEnabled bool
	RedisURL     string

	// Stockage
	UploadsDir string

	// Embedder (chemin vers le modèle ONNX)
	ModelPath string

	Env string
}

var C Config

func Load() error {
	// Cherche le .env à la racine du projet (../   par rapport au binaire dans server/)
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	C = Config{
		Port:          getEnv("PORT", "3001"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/ask-rules"),
		MistralAPIKey: getEnv("MISTRAL_API_KEY", ""),
		MistralModel:  getEnv("MISTRAL_MODEL", "mistral-small-latest"),
		OpenAIAPIKey:  getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:   getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		OllamaHost:    getEnv("OLLAMA_HOST", "http://localhost:11434"),
		OllamaModel:   getEnv("OLLAMA_MODEL", ""),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin"),
		RedisEnabled:  getEnvBool("REDIS_ENABLED", false),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		UploadsDir:    getEnv("UPLOADS_DIR", "../uploads"),
		ModelPath:     getEnv("MODEL_PATH", "../models/multilingual-e5-small"),
		Env:           getEnv("ENV", "production"),
	}

	// Résoudre les chemins relatifs en chemins absolus
	// afin que la résolution ne dépende pas du CWD au moment du lancement.
	if abs, err := filepath.Abs(C.UploadsDir); err == nil {
		C.UploadsDir = abs
	}
	if abs, err := filepath.Abs(C.ModelPath); err == nil {
		C.ModelPath = abs
	}

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
