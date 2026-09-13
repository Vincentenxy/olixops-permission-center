package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginLimiterRefillAndCapacity(t *testing.T) {
	now := time.Unix(100, 0)
	l := &loginLimiter{buckets: make(map[string]loginBucket), now: func() time.Time { return now }}
	for range 10 {
		if !l.allow("first") {
			t.Fatal("initial allowance lost")
		}
	}
	if l.allow("first") {
		t.Fatal("burst exceeded")
	}
	if !l.allow("second") {
		t.Fatal("unrelated peer blocked")
	}
	now = now.Add(6 * time.Second)
	if !l.allow("first") || l.allow("first") {
		t.Fatal("refill incorrect")
	}
	for len(l.buckets) < 4096 {
		l.buckets[time.Duration(len(l.buckets)).String()] = loginBucket{updated: now}
	}
	if l.allow("new") {
		t.Fatal("capacity unbounded")
	}
	now = now.Add(11 * time.Minute)
	if !l.allow("new") {
		t.Fatal("stale peers not pruned")
	}
}
func TestLoginLimiterDoesNotTrustForwardedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.POST("/login", LoginRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	for i := range 11 {
		q := httptest.NewRequest(http.MethodPost, "/login", nil)
		q.RemoteAddr = "192.0.2.1:1234"
		q.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		want := http.StatusOK
		if i == 10 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("request %d: status %d", i, w.Code)
		}
	}
}

func TestTrustedProxyUsesIndependentClientBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := r.SetTrustedProxies([]string{"192.0.2.0/24"}); err != nil {
		t.Fatal(err)
	}
	r.POST("/login", LoginRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	for i := range 20 {
		q := httptest.NewRequest(http.MethodPost, "/login", nil)
		q.RemoteAddr = "192.0.2.1:1234"
		q.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i%2+1))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if w.Code != http.StatusOK {
			t.Fatalf("independent proxy client %d blocked", i)
		}
	}
	q := httptest.NewRequest(http.MethodPost, "/login", nil)
	q.RemoteAddr = "192.0.2.1:1234"
	q.Header.Set("X-Forwarded-For", "198.51.100.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("trusted proxy client exceeded its own bucket")
	}
}
