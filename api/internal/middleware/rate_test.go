package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestRateLimiterAllowsThenBlocks (F17): a client is allowed up to max requests
// per window; the next one is rejected with 429.
func TestRateLimiterAllowsThenBlocks(t *testing.T) {
	r := gin.New()
	r.Use(RateLimiter(NewMemoryRateStore(), 2, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	hit := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "1.2.3.4:5555"
		r.ServeHTTP(w, req)
		return w.Code
	}
	if c := hit(); c != http.StatusOK {
		t.Errorf("request 1 = %d, want 200", c)
	}
	if c := hit(); c != http.StatusOK {
		t.Errorf("request 2 = %d, want 200", c)
	}
	if c := hit(); c != http.StatusTooManyRequests {
		t.Errorf("request 3 = %d, want 429", c)
	}
}

// TestMemoryRateStoreResetsAfterWindow: the counter resets once the window passes.
func TestMemoryRateStoreResetsAfterWindow(t *testing.T) {
	store := NewMemoryRateStore()
	ctx := context.Background()
	if n, _ := store.Incr(ctx, "k", 40*time.Millisecond); n != 1 {
		t.Fatalf("first incr = %d, want 1", n)
	}
	if n, _ := store.Incr(ctx, "k", 40*time.Millisecond); n != 2 {
		t.Fatalf("second incr = %d, want 2", n)
	}
	time.Sleep(60 * time.Millisecond)
	if n, _ := store.Incr(ctx, "k", 40*time.Millisecond); n != 1 {
		t.Errorf("incr after window = %d, want 1 (reset)", n)
	}
}

// TestMemoryRateStoreEvictsIdle (F17): expired keys are evicted, so memory does
// not grow unbounded with distinct/spoofed client IPs.
func TestMemoryRateStoreEvictsIdle(t *testing.T) {
	store := NewMemoryRateStore()
	ctx := context.Background()
	store.Incr(ctx, "idle", 20*time.Millisecond)
	store.Incr(ctx, "active", time.Hour)
	time.Sleep(40 * time.Millisecond)
	store.Incr(ctx, "active", time.Hour) // touching the store sweeps expired keys
	if got := store.size(); got != 1 {
		t.Errorf("after eviction store has %d keys, want 1 (idle evicted)", got)
	}
}
