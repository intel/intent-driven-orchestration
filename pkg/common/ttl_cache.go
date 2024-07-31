package common

import (
	"runtime"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// TTLCache represents a super simple TTL cache.
type TTLCache struct {
	entries map[string]int64
	mLock   sync.Mutex
}

// NewCache initializes a new TTL based cache that actively evicts old entries.
func NewCache(ttl int, tick time.Duration) (*TTLCache, chan struct{}) {
	cache := &TTLCache{entries: make(map[string]int64)}
	done := make(chan struct{})
	if tick <= 0 || ttl <= 0 || tick > MaxPlanCacheTimeout || ttl > MaxPlanCacheTTL {
		klog.Error("invalid timing values.")
		return cache, done
	}

	go func() {
		ticker := time.NewTicker(time.Millisecond * tick)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				cache.mLock.Lock()
				for k, v := range cache.entries {
					if now.UnixMilli()-v > int64(ttl) {
						delete(cache.entries, k)
					}
				}
				cache.mLock.Unlock()
			case <-done:
				return
			}
			runtime.Gosched()
		}
	}()
	return cache, done
}

// Put adds an entry to the Cache.
func (c *TTLCache) Put(key string) {
	c.mLock.Lock()
	c.entries[key] = time.Now().UnixMilli()
	c.mLock.Unlock()
}

// IsIn checks if an entry still exists.
func (c *TTLCache) IsIn(key string) bool {
	res := false
	c.mLock.Lock()
	if _, ok := c.entries[key]; ok {
		res = true
	}
	c.mLock.Unlock()
	return res
}
