package wal

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"github.com/go-redis/redis/v8"
)

// newTestRedis returns a Redis client, skipping the test if Redis is not
// reachable (so the suite stays green without external infrastructure).
func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		host := os.Getenv("REDIS_HOST")
		if host == "" {
			host = "localhost"
		}
		addr = host + ":6379"
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("Redis not reachable at %s, skipping: %v", addr, err)
	}
	return client
}

func testLog(i int) *models.Log {
	now := time.Now().UTC()
	return &models.Log{
		ID:        fmt.Sprintf("redis-wal-%d", i),
		Source:    "test",
		Level:     models.LogLevelInfo,
		Message:   "msg",
		Timestamp: now.Format(time.RFC3339),
		CreatedAt: models.FlexTime{Time: now},
		Metadata:  map[string]interface{}{},
	}
}

// TestNewFactory covers backend selection and runs without Redis.
func TestNewFactory(t *testing.T) {
	// Disabled -> NoopWAL.
	w, err := New(&config.WALConfig{Enabled: false}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := w.(NoopWAL); !ok {
		t.Errorf("disabled WAL: expected NoopWAL, got %T", w)
	}

	// Redis backend without a client -> error (no silent durability downgrade).
	if _, err := New(&config.WALConfig{Enabled: true, Backend: "redis", StreamKey: "k", ConsumerGroup: "g"}, nil); err == nil {
		t.Error("redis backend without client: expected error, got nil")
	}

	// Unknown backend -> error.
	if _, err := New(&config.WALConfig{Enabled: true, Backend: "bogus"}, nil); err == nil {
		t.Error("unknown backend: expected error, got nil")
	}

	// File backend -> *WriteAheadLog.
	w, err = New(&config.WALConfig{Enabled: true, Backend: "file", Directory: t.TempDir(), CheckInterval: time.Second}, nil)
	if err != nil {
		t.Fatalf("file backend: unexpected error: %v", err)
	}
	if _, ok := w.(*WriteAheadLog); !ok {
		t.Errorf("file backend: expected *WriteAheadLog, got %T", w)
	}
}

// TestRedisWALEndToEnd writes entries and asserts the processor drains them.
func TestRedisWALEndToEnd(t *testing.T) {
	client := newTestRedis(t)
	defer client.Close()

	streamKey := fmt.Sprintf("test:wal:%d", time.Now().UnixNano())
	cfg := &config.WALConfig{
		Enabled:       true,
		Backend:       "redis",
		StreamKey:     streamKey,
		ConsumerGroup: "test-processors",
		ReadCount:     128,
		BlockTimeout:  200 * time.Millisecond,
		ClaimMinIdle:  time.Second,
	}
	w := NewRedisWAL(client, cfg)

	defer func() {
		ctx := context.Background()
		client.XGroupDestroy(ctx, streamKey, cfg.ConsumerGroup)
		client.Del(ctx, streamKey)
	}()

	const n = 5
	var mu sync.Mutex
	got := make(map[string]bool)

	w.StartProcessor(func(l *models.Log) error {
		mu.Lock()
		got[l.ID] = true
		mu.Unlock()
		return nil
	})
	defer w.StopProcessor()

	for i := 0; i < n; i++ {
		if err := w.Write(testLog(i)); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := len(got)
		mu.Unlock()
		if c == n {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	c := len(got)
	mu.Unlock()
	if c != n {
		t.Fatalf("expected %d entries processed, got %d", n, c)
	}

	stats := w.GetStats()
	if stats.TotalWritten != int64(n) {
		t.Errorf("expected TotalWritten=%d, got %d", n, stats.TotalWritten)
	}
	if stats.TotalProcessed != int64(n) {
		t.Errorf("expected TotalProcessed=%d, got %d", n, stats.TotalProcessed)
	}

	// Stream should be empty: entries are deleted after a successful ack.
	if l, err := client.XLen(context.Background(), streamKey).Result(); err == nil && l != 0 {
		t.Errorf("expected stream drained, XLEN=%d", l)
	}
}

// TestRedisWALRetryOnFailure verifies that a failing insert keeps the entry and
// it is processed once the callback starts succeeding.
func TestRedisWALRetryOnFailure(t *testing.T) {
	client := newTestRedis(t)
	defer client.Close()

	streamKey := fmt.Sprintf("test:wal-retry:%d", time.Now().UnixNano())
	cfg := &config.WALConfig{
		Enabled:       true,
		Backend:       "redis",
		StreamKey:     streamKey,
		ConsumerGroup: "test-processors",
		ReadCount:     128,
		BlockTimeout:  200 * time.Millisecond,
		ClaimMinIdle:  300 * time.Millisecond,
	}
	w := NewRedisWAL(client, cfg)

	defer func() {
		ctx := context.Background()
		client.XGroupDestroy(ctx, streamKey, cfg.ConsumerGroup)
		client.Del(ctx, streamKey)
	}()

	var mu sync.Mutex
	fail := true
	attempts := 0
	done := make(chan struct{})

	w.StartProcessor(func(l *models.Log) error {
		mu.Lock()
		attempts++
		shouldFail := fail
		mu.Unlock()
		if shouldFail {
			return fmt.Errorf("simulated insert failure")
		}
		close(done)
		return nil
	})
	defer w.StopProcessor()

	if err := w.Write(testLog(0)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Let it fail at least once, then allow success; the reclaim loop retries.
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	fail = false
	mu.Unlock()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("entry was not retried/processed after failure cleared")
	}

	mu.Lock()
	a := attempts
	mu.Unlock()
	if a < 2 {
		t.Errorf("expected at least 2 attempts (>=1 failure + 1 success), got %d", a)
	}
}
