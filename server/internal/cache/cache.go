// cache/cache.go — Cache des réponses en mémoire
package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"
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

// Cache met en cache les réponses en mémoire.
type Cache struct {
	memMu    sync.RWMutex
	memStore map[string]*cacheEntry
}

// New crée le cache et lance le nettoyage périodique des entrées expirées.
func New() *Cache {
	c := &Cache{
		memStore: make(map[string]*cacheEntry),
	}
	go c.cleanupMemCache()
	return c
}

// Get récupère une valeur du cache.
func (c *Cache) Get(ctx context.Context, key string) (interface{}, error) {
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
func (c *Cache) Set(ctx context.Context, key string, value interface{}) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}

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

// cleanupMemCache supprime périodiquement les entrées expirées.
func (c *Cache) cleanupMemCache() {
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
