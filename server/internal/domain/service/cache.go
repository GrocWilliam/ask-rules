// domain/service/cache.go — Interface service pour le cache
package service

import (
	"context"
)

// CacheService gère le cache des réponses.
// Implémentations possibles : Redis, Memcached, cache mémoire, etc.
type CacheService interface {
	// Get récupère une valeur du cache.
	// Retourne nil si la clé n'existe pas ou est expirée.
	Get(ctx context.Context, key string) (interface{}, error)

	// Set stocke une valeur dans le cache avec un TTL.
	Set(ctx context.Context, key string, value interface{}) error

	// Invalidate supprime une clé du cache.
	Invalidate(ctx context.Context, key string) error
}
