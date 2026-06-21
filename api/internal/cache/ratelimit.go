package cache

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

// rateLimitScript atomically increments a key and, on the first hit of a window,
// sets its expiry. Atomicity guarantees the key always gets a TTL (so it is
// evicted), avoiding the INCR-then-EXPIRE race where a crash leaves a key that
// never expires.
var rateLimitScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return c
`)

// RedisRateStore is a Redis-backed fixed-window rate-limit counter. Because the
// count lives in Redis, the limit is shared across all API instances; the key's
// TTL evicts idle clients automatically.
type RedisRateStore struct {
	client *redis.Client
}

// NewRedisRateStore creates a Redis-backed rate-limit store.
func NewRedisRateStore(client *redis.Client) *RedisRateStore {
	return &RedisRateStore{client: client}
}

// Incr increments the per-key counter for the current window and returns the new
// count. The key expires after window.
func (s *RedisRateStore) Incr(ctx context.Context, key string, window time.Duration) (int, error) {
	return rateLimitScript.Run(ctx, s.client, []string{key}, window.Milliseconds()).Int()
}
