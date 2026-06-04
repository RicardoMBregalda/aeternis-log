package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/go-redis/redis/v8"
	zlog "github.com/rs/zerolog/log"
)

// RedisCache wraps the Redis client with caching operations
type RedisCache struct {
	Client  *redis.Client
	Config  *config.RedisConfig
	Enabled bool
}

// NewRedisClient creates a new Redis client
func NewRedisClient(cfg *config.RedisConfig) (*RedisCache, error) {
	if !cfg.CacheEnabled {
		return &RedisCache{
			Enabled: false,
			Config:  cfg,
		}, nil
	}

	// Configure Redis client
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// Graceful degradation - cache unavailable but don't fail
		zlog.Warn().Err(err).Msg("Redis unavailable, cache disabled")
		return &RedisCache{
			Enabled: false,
			Config:  cfg,
		}, nil
	}

	zlog.Info().Msg("Redis cache connected")

	return &RedisCache{
		Client:  client,
		Config:  cfg,
		Enabled: true,
	}, nil
}

// Get retrieves a value from cache
func (rc *RedisCache) Get(ctx context.Context, key string) (string, error) {
	if !rc.Enabled {
		return "", redis.Nil
	}

	val, err := rc.Client.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}

	return val, nil
}

// GetJSON retrieves and unmarshals a JSON value from cache
func (rc *RedisCache) GetJSON(ctx context.Context, key string, dest interface{}) error {
	if !rc.Enabled {
		return redis.Nil
	}

	val, err := rc.Get(ctx, key)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(val), dest)
}

// Set stores a value in cache with TTL
func (rc *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !rc.Enabled {
		return nil // Silent no-op
	}

	// Use default TTL if not specified
	if ttl == 0 {
		ttl = time.Duration(rc.Config.CacheTTL) * time.Second
	}

	return rc.Client.Set(ctx, key, value, ttl).Err()
}

// SetJSON marshals and stores a JSON value in cache
func (rc *RedisCache) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !rc.Enabled {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return rc.Set(ctx, key, data, ttl)
}

// Delete removes a key from cache
func (rc *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if !rc.Enabled {
		return nil
	}

	return rc.Client.Del(ctx, keys...).Err()
}

// DeletePattern deletes all keys matching a pattern
func (rc *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	if !rc.Enabled {
		return nil
	}

	// Scan for matching keys
	var cursor uint64
	var deletedCount int

	for {
		keys, nextCursor, err := rc.Client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys: %w", err)
		}

		if len(keys) > 0 {
			if err := rc.Client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("failed to delete keys: %w", err)
			}
			deletedCount += len(keys)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if deletedCount > 0 {
		zlog.Debug().Int("keys", deletedCount).Str("pattern", pattern).Msg("cache invalidated")
	}

	return nil
}

// Exists checks if a key exists in cache
func (rc *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	if !rc.Enabled {
		return false, nil
	}

	count, err := rc.Client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Expire sets a TTL on an existing key
func (rc *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if !rc.Enabled {
		return nil
	}

	return rc.Client.Expire(ctx, key, ttl).Err()
}

// Close closes the Redis connection
func (rc *RedisCache) Close() error {
	if !rc.Enabled || rc.Client == nil {
		return nil
	}

	return rc.Client.Close()
}

// Ping checks if Redis is reachable
func (rc *RedisCache) Ping(ctx context.Context) error {
	if !rc.Enabled {
		return fmt.Errorf("cache is disabled")
	}

	return rc.Client.Ping(ctx).Err()
}

// GetStats returns cache statistics
func (rc *RedisCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	if !rc.Enabled {
		return map[string]interface{}{
			"enabled": false,
		}, nil
	}

	// Get Redis INFO
	info, err := rc.Client.Info(ctx, "stats").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get redis info: %w", err)
	}

	// Count cache keys
	var cursor uint64
	totalKeys := 0
	
	for {
		keys, nextCursor, err := rc.Client.Scan(ctx, cursor, "logs:*", 100).Result()
		if err != nil {
			break
		}
		totalKeys += len(keys)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	stats := map[string]interface{}{
		"enabled":    true,
		"total_keys": totalKeys,
		"ttl":        fmt.Sprintf("%ds", rc.Config.CacheTTL),
		"pool_size":  rc.Config.PoolSize,
	}

	// Parse INFO for hits/misses (simplified)
	// In production, you'd parse the info string properly
	stats["info"] = info

	return stats, nil
}

// HealthCheck performs a health check on Redis
func (rc *RedisCache) HealthCheck(ctx context.Context) error {
	if !rc.Enabled {
		return nil // Cache disabled is not an error
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := rc.Ping(pingCtx); err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}

	return nil
}

// ========================================
// CACHE KEY BUILDERS
// ========================================

// BuildLogListKey builds cache key for log list queries
func BuildLogListKey(source, level string, limit, offset int) string {
	return fmt.Sprintf("logs:list:%s:%s:%d:%d", source, level, limit, offset)
}

// BuildLogListCursorKey builds the cache key for a cursor-paginated log list.
// It shares the "logs:list:" prefix so InvalidateLogCache clears it too.
func BuildLogListCursorKey(source, level string, limit int, cursor string) string {
	return fmt.Sprintf("logs:list:%s:%s:%d:cursor:%s", source, level, limit, cursor)
}

// BuildLogKey builds cache key for individual log
func BuildLogKey(logID string) string {
	return fmt.Sprintf("log:%s", logID)
}

// BuildBatchKey builds cache key for batch
func BuildBatchKey(batchID string) string {
	return fmt.Sprintf("batch:%s", batchID)
}

// ========================================
// HELPER METHODS
// ========================================

// InvalidateLogCache invalidates all log-related cache entries
func (rc *RedisCache) InvalidateLogCache(ctx context.Context, source string) error {
	if !rc.Enabled {
		return nil
	}

	// Invalidate list caches
	if source != "" {
		if err := rc.DeletePattern(ctx, fmt.Sprintf("logs:list:%s:*", source)); err != nil {
			return err
		}
	}
	
	// Invalidate general list caches
	return rc.DeletePattern(ctx, "logs:list:*")
}

// SetWithDefaultTTL sets a value with the default TTL from config
func (rc *RedisCache) SetWithDefaultTTL(ctx context.Context, key string, value interface{}) error {
	ttl := time.Duration(rc.Config.CacheTTL) * time.Second
	return rc.Set(ctx, key, value, ttl)
}
