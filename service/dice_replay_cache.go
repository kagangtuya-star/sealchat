package service

import (
	"sync"
	"time"
)

type diceReplayCacheItem struct {
	value     *DiceReplaySnapshot
	expiresAt time.Time
}

type diceReplayCache struct {
	mu    sync.RWMutex
	items map[string]diceReplayCacheItem
	ttl   time.Duration
	max   int
}

func newDiceReplayCache(max int, ttl time.Duration) *diceReplayCache {
	if max <= 0 {
		max = 128
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &diceReplayCache{
		items: make(map[string]diceReplayCacheItem),
		ttl:   ttl,
		max:   max,
	}
}

func (c *diceReplayCache) Get(key string) (*DiceReplaySnapshot, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	now := time.Now()
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}
	return item.value, item.value != nil
}

func (c *diceReplayCache) Set(key string, value *DiceReplaySnapshot) {
	if c == nil || key == "" || value == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		for existingKey := range c.items {
			delete(c.items, existingKey)
			break
		}
	}
	c.items[key] = diceReplayCacheItem{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

var globalDiceReplayCache = newDiceReplayCache(256, 15*time.Minute)
