// cache/cache.go — Cache des réponses (Redis ou mémoire)
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"ask-rules-server/config"
	"ask-rules-server/models"

	"github.com/redis/go-redis/v9"
)

const (
	ttl             = 24 * time.Hour
	maxMemEntries   = 200 // Limite du cache mémoire pour éviter surconsommation RAM
	cleanupInterval = 5 * time.Minute
)

var (
	client   *redis.Client
	useRedis bool
	memMu    sync.RWMutex
	memStore = make(map[string]*cacheEntry)
)

type cacheEntry struct {
	Resp    models.AskResponse
	Expires time.Time
}

func Init() {
	if !config.C.RedisEnabled {
		useRedis = false
		return
	}
	opts, err := redis.ParseURL(config.C.RedisURL)
	if err != nil {
		log.Printf("Cache: URL Redis invalide: %v — fallback mémoire", err)
		useRedis = false
		return
	}
	client = redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("Cache: Redis injoignable: %v — fallback mémoire", err)
		client = nil
		useRedis = false
		return
	}
	useRedis = true
	// Lancer le nettoyage périodique du cache mémoire
	if !useRedis {
		go cleanupMemCache()
	}
}

func Key(question, jeu string) string {
	h := sha256.New()
	h.Write([]byte(question + "|" + jeu))
	return fmt.Sprintf("ask:%x", h.Sum(nil))
}

func Get(ctx context.Context, key string) *models.AskResponse {
	if useRedis && client != nil {
		val, err := client.Get(ctx, key).Bytes()
		if err != nil {
			return nil
		}
		var resp models.AskResponse
		if err := json.Unmarshal(val, &resp); err != nil {
			return nil
		}
		resp.Cached = true
		return &resp
	}
	memMu.RLock()
	entry, ok := memStore[key]
	memMu.RUnlock()
	if !ok || time.Now().After(entry.Expires) {
		return nil
	}
	copy := entry.Resp
	copy.Cached = true
	return &copy
}

func Set(ctx context.Context, key string, resp models.AskResponse) {
	if useRedis && client != nil {
		b, err := json.Marshal(resp)
		if err != nil {
			return
		}
		client.Set(ctx, key, b, ttl)
		return
	}
	memMu.Lock()
	defer memMu.Unlock()

	// Limiter la taille du cache en mémoire
	if len(memStore) >= maxMemEntries {
		// Supprimer 20% des entrées les plus anciennes
		var toDelete []string
		now := time.Now()
		for k, v := range memStore {
			if now.After(v.Expires) || len(toDelete) < maxMemEntries/5 {
				toDelete = append(toDelete, k)
			}
			if len(toDelete) >= maxMemEntries/5 {
				break
			}
		}
		for _, k := range toDelete {
			delete(memStore, k)
		}
	}

	memStore[key] = &cacheEntry{Resp: resp, Expires: time.Now().Add(ttl)}
}

func Invalidate(ctx context.Context, key string) {
	if useRedis && client != nil {
		client.Del(ctx, key)
		return
	}
	memMu.Lock()
	delete(memStore, key)
	memMu.Unlock()
}

// cleanupMemCache supprime périodiquement les entrées expirées du cache mémoire.
func cleanupMemCache() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		memMu.Lock()
		now := time.Now()
		for k, v := range memStore {
			if now.After(v.Expires) {
				delete(memStore, k)
			}
		}
		memMu.Unlock()
	}
}
