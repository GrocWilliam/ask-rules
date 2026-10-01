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
	// Plugsky : fournisseur de secours si Mistral échoue
	PlugskyAPIKey  string
	PlugskyModel   string
	PlugskyBaseURL string
	OllamaHost     string
	OllamaModel    string
	// LLMRequestsPerSecond : cadence max des appels Mistral (0 = illimitée)
	LLMRequestsPerSecond float64

	// Admin
	AdminPassword string

	// Redis
	RedisEnabled bool
	RedisURL     string

	// Stockage
	UploadsDir string

	// Embedder (chemin vers le modèle ONNX)
	ModelPath string
	// OnnxThreads : threads ONNX Runtime pour une inférence (peu = moins de RAM)
	OnnxThreads int

	Env string
}

var C Config

func Load() error {
	// Cherche le .env à la racine du projet (../   par rapport au binaire dans server/)
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	C = Config{
		Port:                 getEnv("PORT", "3001"),
		DatabaseURL:          getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/ask-rules"),
		MistralAPIKey:        getEnv("MISTRAL_API_KEY", ""),
		MistralModel:         getEnv("MISTRAL_MODEL", "mistral-small-latest"),
		PlugskyAPIKey:        getEnv("PLUGSKY_API_KEY", ""),
		PlugskyModel:         getEnv("PLUGSKY_MODEL", "plugsky-lite"),
		PlugskyBaseURL:       getEnv("PLUGSKY_BASE_URL", "https://plugsky.com/v1"),
		OllamaHost:           getEnv("OLLAMA_HOST", "http://localhost:11434"),
		OllamaModel:          getEnv("OLLAMA_MODEL", ""),
		LLMRequestsPerSecond: getEnvFloat("LLM_REQUESTS_PER_SECOND", 1),
		AdminPassword:        getEnv("ADMIN_PASSWORD", "admin"),
		RedisEnabled:         getEnvBool("REDIS_ENABLED", false),
		RedisURL:             getEnv("REDIS_URL", "redis://localhost:6379"),
		UploadsDir:           getEnv("UPLOADS_DIR", "../uploads"),
		ModelPath:            getEnv("MODEL_PATH", "../models/multilingual-e5-small"),
		OnnxThreads:          getEnvInt("ONNX_THREADS", 2),
		Env:                  getEnv("ENV", "production"),
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

func getEnvInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func getEnvFloat(key string, fallback float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
