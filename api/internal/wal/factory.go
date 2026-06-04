package wal

import (
	"fmt"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/go-redis/redis/v8"
)

// Compile-time guarantee that the Redis backend satisfies the interface.
var _ WAL = (*RedisWAL)(nil)

// New builds the WAL backend selected by cfg.Backend. When the WAL is disabled
// it returns a NoopWAL so callers always get a usable, non-nil value.
//
// The "redis" backend requires a live Redis connection (redisClient must be
// non-nil). Selecting it without Redis is an error rather than a silent
// downgrade, because that would weaken the durability guarantee.
func New(cfg *config.WALConfig, redisClient *redis.Client) (WAL, error) {
	if !cfg.Enabled {
		return NoopWAL{}, nil
	}

	switch cfg.Backend {
	case "file":
		return NewWriteAheadLog(cfg.Directory, cfg.CheckInterval)
	case "redis":
		if redisClient == nil {
			return nil, fmt.Errorf("wal backend %q requires Redis, but no Redis connection is available", cfg.Backend)
		}
		return NewRedisWAL(redisClient, cfg), nil
	default:
		return nil, fmt.Errorf("unknown wal backend %q", cfg.Backend)
	}
}
