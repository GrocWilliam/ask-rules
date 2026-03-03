// infrastructure/service/redis_cache_adapter.go — Cache intégré (Redis ou mémoire)
package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/infrastructure/config"

	"github.com/redis/go-redis/v9"
)

const (
	cacheTTL        = 24 * time.Hour
	maxMemEntries   = 200
	cleanupInterval = 5 * time.Minute
)

type cacheEntry struct {
	Data    []byte
	Expires time.Time
}

// RedisCacheAdapter implémente CacheService avec Redis ou fallback mémoire.
type RedisCacheAdapter struct {
	client   *redis.Client
	useRedis bool
	memMu    sync.RWMutex
	memStore map[string]*cacheEntry
	once     sync.Once
}

// NewRedisCache crée un nouvel adapter pour le cache.
func NewRedisCache() service.CacheService {
	adapter := &RedisCacheAdapter{
		memStore: make(map[string]*cacheEntry),
	}
	adapter.init()
	return adapter
}

// init initialise la connexion Redis ou le cache mémoire.
func (c *RedisCacheAdapter) init() {
	c.once.Do(func() {
		if !config.C.RedisEnabled {
			c.useRedis = false
			go c.cleanupMemCache()
			return
		}
		opts, err := redis.ParseURL(config.C.RedisURL)
		if err != nil {
			log.Printf("Cache: URL Redis invalide: %v — fallback mémoire", err)
			c.useRedis = false
			go c.cleanupMemCache()
			return
		}
		c.client = redis.NewClient(opts)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.client.Ping(ctx).Err(); err != nil {
			log.Printf("Cache: Redis injoignable: %v — fallback mémoire", err)
			c.client = nil
			c.useRedis = false
			go c.cleanupMemCache()
			return
		}
		c.useRedis = true
	})
}

// Get récupère une valeur du cache.
func (c *RedisCacheAdapter) Get(ctx context.Context, key string) (interface{}, error) {
	if c.useRedis && c.client != nil {
		val, err := c.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				return nil, nil
			}
			return nil, err
		}
		// Retourner les données brutes JSON sous forme de map
		var result map[string]interface{}
		if err := json.Unmarshal(val, &result); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Fallback mémoire
	c.memMu.RLock()
	entry, ok := c.memStore[key]
	c.memMu.RUnlock()
	if !ok || time.Now().After(entry.Expires) {
		return nil, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(entry.Data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Set stocke une valeur dans le cache.
func (c *RedisCacheAdapter) Set(ctx context.Context, key string, value interface{}) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if c.useRedis && c.client != nil {
		return c.client.Set(ctx, key, b, cacheTTL).Err()
	}

	// Fallback mémoire
	c.memMu.Lock()
	defer c.memMu.Unlock()

	// Limiter la taille du cache en mémoire
	if len(c.memStore) >= maxMemEntries {
		var toDelete []string
		now := time.Now()
		for k, v := range c.memStore {
			if now.After(v.Expires) || len(toDelete) < maxMemEntries/5 {
				toDelete = append(toDelete, k)
			}
			if len(toDelete) >= maxMemEntries/5 {
				break
			}
		}
		for _, k := range toDelete {
			delete(c.memStore, k)
		}
	}

	c.memStore[key] = &cacheEntry{Data: b, Expires: time.Now().Add(cacheTTL)}
	return nil
}

// Invalidate supprime une entrée du cache.
func (c *RedisCacheAdapter) Invalidate(ctx context.Context, key string) error {
	if c.useRedis && c.client != nil {
		return c.client.Del(ctx, key).Err()
	}
	c.memMu.Lock()
	delete(c.memStore, key)
	c.memMu.Unlock()
	return nil
}

// GenerateKey génère une clé de cache.
func (c *RedisCacheAdapter) GenerateKey(question, gameName string) string {
	h := sha256.New()
	h.Write([]byte(question + "|" + gameName))
	return fmt.Sprintf("ask:%x", h.Sum(nil))
}

// cleanupMemCache supprime périodiquement les entrées expirées.
func (c *RedisCacheAdapter) cleanupMemCache() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.memMu.Lock()
		now := time.Now()
		for k, v := range c.memStore {
			if now.After(v.Expires) {
				delete(c.memStore, k)
			}
		}
		c.memMu.Unlock()
	}
}
