package mqtt

import (
	"context"
	"sync"
	"time"

	"iot-perform/internal/platform/domain"
)

// productCache is a small TTL cache in front of GetProductByKey. The product
// model is read-mostly (it is only changed by create operations today), so a
// per-instance TTL cache removes a PostgreSQL round trip from every telemetry
// message without requiring invalidation on update.
type productCache struct {
	mu     sync.RWMutex
	ttl    time.Duration
	items  map[string]productCacheEntry
	loader func(context.Context, string) (domain.Product, error)
}

type productCacheEntry struct {
	product   domain.Product
	expiresAt time.Time
}

func newProductCache(loader func(context.Context, string) (domain.Product, error), ttl time.Duration) *productCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &productCache{ttl: ttl, items: make(map[string]productCacheEntry), loader: loader}
}

func (c *productCache) Get(ctx context.Context, productKey string) (domain.Product, error) {
	c.mu.RLock()
	entry, ok := c.items[productKey]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.product, nil
	}
	product, err := c.loader(ctx, productKey)
	if err != nil {
		return domain.Product{}, err
	}
	c.mu.Lock()
	c.items[productKey] = productCacheEntry{product: product, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return product, nil
}
