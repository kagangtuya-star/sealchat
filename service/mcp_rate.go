package service

import (
	"sealchat/utils"
	"sync"
	"time"
)

type mcpRateBucket struct {
	minute        int64
	calls, writes int
	httpCalls     int
}

// Process-local and bounded. A full active table rejects new users instead of
// evicting active counters and allowing the quota to be bypassed.
type MCPRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]mcpRateBucket
}

func NewMCPRateLimiter() *MCPRateLimiter { return &MCPRateLimiter{buckets: map[string]mcpRateBucket{}} }
func (l *MCPRateLimiter) Allow(userID string, write, business bool, cfg utils.MCPConfig) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().Unix() / 60
	cfg = utils.NormalizeMCPConfig(cfg)
	if len(l.buckets) >= 10000 {
		for id, b := range l.buckets {
			if b.minute != now {
				delete(l.buckets, id)
			}
		}
	}
	b, exists := l.buckets[userID]
	if !exists && len(l.buckets) >= 10000 {
		return false
	}
	if b.minute != now {
		b = mcpRateBucket{minute: now}
	}
	if business {
		if b.calls >= cfg.CallsPerMinute {
			return false
		}
		if write && b.writes >= cfg.WritesPerMinute {
			return false
		}
		b.calls++
		if write {
			b.writes++
		}
	} else {
		if b.httpCalls >= cfg.CallsPerMinute*4 {
			return false
		}
		b.httpCalls++
	}
	l.buckets[userID] = b
	return true
}
