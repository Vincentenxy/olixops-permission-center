package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"olixops-permission-center/pkg/response"
)

type loginBucket struct {
	tokens  float64
	updated time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginBucket
	now     func() time.Time
}

func (l *loginLimiter) allow(address string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	bucket, exists := l.buckets[address]
	if !exists {
		if len(l.buckets) >= 4096 {
			for key, v := range l.buckets {
				if now.Sub(v.updated) > 10*time.Minute {
					delete(l.buckets, key)
				}
			}
			if len(l.buckets) >= 4096 {
				return false
			}
		}
		bucket = loginBucket{tokens: 10, updated: now}
	}
	bucket.tokens = min(10, bucket.tokens+now.Sub(bucket.updated).Seconds()/6)
	bucket.updated = now
	allowed := bucket.tokens >= 1
	if allowed {
		bucket.tokens--
	}
	l.buckets[address] = bucket
	return allowed
}

// LoginRateLimit bounds expensive bcrypt work per client: ten initial attempts,
// then one every six seconds. ClientIP honors only explicitly configured trusted
// proxy peers. Production ingress should add an independent shared limit.
func LoginRateLimit() gin.HandlerFunc {
	l := &loginLimiter{buckets: make(map[string]loginBucket), now: time.Now}
	return func(c *gin.Context) {
		address := c.ClientIP()
		if !l.allow(address) {
			c.Header("Retry-After", "6")
			response.HTTPError(c, http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}
