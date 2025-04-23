// Copyright 2023 Henrik Hedlund. All rights reserved.
// Use of this source code is governed by the GNU Affero
// GPL license that can be found in the LICENSE file.

package mcache

import (
	"sync"
	"time"
)

const (
	NoExpiration time.Duration = -1
)

// New creates a new Cache instance with the specified expiration duration.
// If expiration is set to NoExpiration, items will not expire.
func New[K comparable, V any](expiration time.Duration) *Cache[K, V] {
	return &Cache[K, V]{
		expiry: expiration,
		items:  make(map[K]item[V]),
		now:    time.Now,
	}
}

type Cache[K comparable, V any] struct {
	expiry time.Duration
	mu     sync.RWMutex
	items  map[K]item[V]
	now    func() time.Time
}

// Get retrieves the value associated with the given key from the cache.
// Returns the value and true if the key exists and is not expired, otherwise returns the zero value and false.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if item, ok := c.items[key]; ok {
		now := c.now().UnixNano()
		if !item.expired(now) {
			return item.value, true
		}
	}

	var empty V
	return empty, false
}

// Set adds a key-value pair to the cache with an optional expiration duration.
// If no duration is provided, the default cache expiration is used.
func (c *Cache[K, V]) Set(key K, value V, d ...time.Duration) {
	expiry := c.expiry
	if len(d) > 0 {
		expiry = d[0]
	}

	var expires int64
	if expiry > 0 {
		expires = c.now().Add(expiry).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = item[V]{
		value, expires,
	}
}

// Delete removes the key-value pair associated with the given key from the cache.
// Returns true if the key existed and was deleted, otherwise false.
func (c *Cache[K, V]) Delete(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.items[key]; ok {
		delete(c.items, key)
		return true
	}
	return false
}

// Count returns the number of items currently stored in the cache.
func (c *Cache[K, V]) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// Cleanup removes all expired items from the cache.
// Returns the number of items that were removed.
func (c *Cache[K, V]) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	var count int
	now := c.now().UnixNano()
	for k, i := range c.items {
		if i.expired(now) {
			delete(c.items, k)
			count++
		}
	}
	return count
}

// Flush removes all items from the cache, regardless of expiration.
func (c *Cache[K, V]) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[K]item[V])
}

// StartCleanupLoop starts a background loop that periodically calls the Cleanup method on the cache.
// The loop runs at the specified interval and can be stopped by calling the returned stop function.
func StartCleanupLoop(c interface{ Cleanup() int }, interval time.Duration) (stop func()) {
	var wg sync.WaitGroup
	sc := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		loop(c.Cleanup, interval, sc)
	}()

	return func() {
		close(sc)
		wg.Wait()
	}
}

func loop(cleanup func() int, d time.Duration, stop chan struct{}) {
	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cleanup()
		case <-stop:
			return
		}
	}
}

type item[V any] struct {
	value   V
	expires int64
}

func (i *item[V]) expired(now int64) bool {
	if i.expires == 0 {
		return false
	}
	return now > i.expires
}
