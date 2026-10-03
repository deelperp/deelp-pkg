package ttlcache

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errLoadAborted = errors.New("ttlcache: carga interrompida")

type item[V any] struct {
	value     V
	expiresAt time.Time
}

type call[V any] struct {
	done  chan struct{}
	value V
	err   error
}

type Cache[K comparable, V any] struct {
	mu       sync.Mutex
	capacity int
	now      func() time.Time
	items    map[K]item[V]
	inflight map[K]*call[V]
}

func New[K comparable, V any](capacity int, now func() time.Time) *Cache[K, V] {
	if capacity < 1 {
		capacity = 1
	}
	return &Cache[K, V]{
		capacity: capacity,
		now:      now,
		items:    map[K]item[V]{},
		inflight: map[K]*call[V]{},
	}
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getLocked(key)
}

func (c *Cache[K, V]) getLocked(key K) (V, bool) {
	entry, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.items, key)
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(key, value, ttl)
}

func (c *Cache[K, V]) setLocked(key K, value V, ttl time.Duration) {
	now := c.now()
	if _, exists := c.items[key]; !exists && len(c.items) >= c.capacity {
		c.purgeLocked(now)
		if len(c.items) >= c.capacity {
			c.evictSoonestLocked()
		}
	}
	c.items[key] = item[V]{value: value, expiresAt: now.Add(ttl)}
}

func (c *Cache[K, V]) purgeLocked(now time.Time) {
	for key, entry := range c.items {
		if !now.Before(entry.expiresAt) {
			delete(c.items, key)
		}
	}
}

func (c *Cache[K, V]) evictSoonestLocked() {
	var (
		victim  K
		soonest time.Time
		found   bool
	)
	for key, entry := range c.items {
		if !found || entry.expiresAt.Before(soonest) {
			victim, soonest, found = key, entry.expiresAt, true
		}
	}
	if found {
		delete(c.items, victim)
	}
}

func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func (c *Cache[K, V]) DeleteFunc(match func(K, V) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.items {
		if match(key, entry.value) {
			delete(c.items, key)
		}
	}
}

func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Load devolve o valor em cache ou executa load uma única vez por chave, mesmo
// com chamadas concorrentes. ttl <= 0 no retorno de load não guarda o valor.
func (c *Cache[K, V]) Load(ctx context.Context, key K, load func(context.Context) (V, time.Duration, error)) (V, error) {
	c.mu.Lock()
	if value, ok := c.getLocked(key); ok {
		c.mu.Unlock()
		return value, nil
	}
	if pending, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-pending.done:
			return pending.value, pending.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}
	pending := &call[V]{done: make(chan struct{})}
	c.inflight[key] = pending
	c.mu.Unlock()

	var (
		value V
		ttl   time.Duration
		err   = errLoadAborted
	)
	defer func() {
		c.mu.Lock()
		delete(c.inflight, key)
		if err == nil && ttl > 0 {
			c.setLocked(key, value, ttl)
		}
		c.mu.Unlock()
		pending.value, pending.err = value, err
		close(pending.done)
	}()
	value, ttl, err = load(ctx)
	return value, err
}
