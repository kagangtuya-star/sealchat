package api

import (
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	mcpOAuthAuthorizePerMinute = 30
	mcpOAuthTokenPerMinute     = 60
	mcpOAuthPublicBucketLimit  = 4096
	mcpOAuthPublicWindow       = time.Minute
)

type mcpOAuthPublicBucketKey struct {
	ip    string
	token bool
}

type mcpOAuthPublicBucket struct {
	expiresAt time.Time
	count     int
}

type mcpOAuthPublicLimiter struct {
	mu          sync.Mutex
	buckets     map[mcpOAuthPublicBucketKey]mcpOAuthPublicBucket
	nextCleanup time.Time
	now         func() time.Time
}

func newMCPOAuthPublicLimiter() *mcpOAuthPublicLimiter {
	return &mcpOAuthPublicLimiter{buckets: map[mcpOAuthPublicBucketKey]mcpOAuthPublicBucket{}, now: time.Now}
}

func (l *mcpOAuthPublicLimiter) allow(ip string, token bool) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !l.nextCleanup.After(now) || len(l.buckets) >= mcpOAuthPublicBucketLimit {
		for key, bucket := range l.buckets {
			if !bucket.expiresAt.After(now) {
				delete(l.buckets, key)
			}
		}
		l.nextCleanup = now.Add(mcpOAuthPublicWindow)
	}
	key := mcpOAuthPublicBucketKey{ip: ip, token: token}
	bucket, exists := l.buckets[key]
	if !exists || !bucket.expiresAt.After(now) {
		if !exists && len(l.buckets) >= mcpOAuthPublicBucketLimit {
			return false, 60
		}
		bucket = mcpOAuthPublicBucket{expiresAt: now.Add(mcpOAuthPublicWindow)}
	}
	limit := mcpOAuthAuthorizePerMinute
	if token {
		limit = mcpOAuthTokenPerMinute
	}
	if bucket.count >= limit {
		return false, max(1, int(math.Ceil(bucket.expiresAt.Sub(now).Seconds())))
	}
	bucket.count++
	l.buckets[key] = bucket
	return true, 0
}

func mcpOAuthClientIP(c *fiber.Ctx) string {
	// Fiber owns the configured proxy header and trusted peer checks. Never
	// consult forwarded headers directly, including when trust checks are off.
	if c.App().Config().EnableTrustedProxyCheck && c.IsProxyTrusted() {
		if ip := net.ParseIP(c.IP()); ip != nil {
			return ip.String()
		}
	}
	return c.Context().RemoteIP().String()
}

func mcpOAuthPublicLimit(l *mcpOAuthPublicLimiter, token bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if allowed, retry := l.allow(mcpOAuthClientIP(c), token); !allowed {
			c.Set("Cache-Control", "no-store")
			c.Set("Pragma", "no-cache")
			c.Set("Retry-After", strconv.Itoa(retry))
			// No client/redirect has been validated at this point, so authorize
			// also returns a local error without inspecting any request-store ID.
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "temporarily_unavailable"})
		}
		return c.Next()
	}
}
