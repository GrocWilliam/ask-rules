// config/config.go — Chargement de la configuration depuis .env
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Serveur
	Port string

	// Base de données
	DatabaseURL string

	// LLM : API compatible OpenAI (chat/completions) — llama.cpp, Mistral,
	// Ollama, OpenAI… Le secours n'est utilisé que si le principal échoue.
	LLM         LLMProvider
	LLMFallback LLMProvider

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

// LLMProvider : un fournisseur actif dès que BaseURL est défini.
type LLMProvider struct {
	// BaseURL : racine de l'API, ex. https://api.mistral.ai/v1 (sans /chat/completions)
	BaseURL string
	// APIKey : optionnelle (llama.cpp sans --api-key, Ollama)
	APIKey string
	// Model : optionnel pour llama.cpp, qui sert le modèle qu'il a chargé
	Model string
	// RequestsPerSecond : cadence max des appels (0 = illimitée)
	RequestsPerSecond float64
	// Timeout : délai max d'une requête (tentatives comprises)
	Timeout time.Duration
	// HealthURL : endpoint de santé d'un serveur pouvant être mis en veille
	// (llama.cpp : <hôte>/health). S'il ne répond pas, le serveur est réveillé
	// en arrière-plan et la question part sur le secours.
	HealthURL string
	// WakeTimeout : durée max du réveil en arrière-plan
	WakeTimeout time.Duration
}

var C Config

func Load() error {
	// Cherche le .env à la racine du projet (../   par rapport au binaire dans server/)
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	C = Config{
		Port:          getEnv("PORT", "3001"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/ask-rules"),
		LLM:           loadLLMProvider("LLM_"),
		LLMFallback:   loadLLMProvider("LLM_FALLBACK_"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin"),
		RedisEnabled:  getEnvBool("REDIS_ENABLED", false),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		UploadsDir:    getEnv("UPLOADS_DIR", "../uploads"),
		ModelPath:     getEnv("MODEL_PATH", "../models/multilingual-e5-small"),
		OnnxThreads:   getEnvInt("ONNX_THREADS", 2),
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

func loadLLMProvider(prefix string) LLMProvider {
	return LLMProvider{
		BaseURL:           getEnv(prefix+"BASE_URL", ""),
		APIKey:            getEnv(prefix+"API_KEY", ""),
		Model:             getEnv(prefix+"MODEL", ""),
		RequestsPerSecond: getEnvFloat(prefix+"REQUESTS_PER_SECOND", 0),
		Timeout:           time.Duration(getEnvInt(prefix+"TIMEOUT_SECONDS", 60)) * time.Second,
		HealthURL:         getEnv(prefix+"HEALTH_URL", ""),
		WakeTimeout:       time.Duration(getEnvInt(prefix+"WAKE_TIMEOUT_SECONDS", 300)) * time.Second,
	}
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
