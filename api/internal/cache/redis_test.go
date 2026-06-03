package cache

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

// TestNewRedisClient tests Redis client creation
func TestNewRedisClient(t *testing.T) {
	cfg := &config.RedisConfig{
		Host:         "localhost",
		Port:         6379,
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		CacheTTL:     600,
		CacheEnabled: true,
	}

	cache, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	if !cache.Enabled {
		t.Skip("Redis not available, skipping tests")
		return
	}

	defer cache.Close()

	// Test ping
	ctx := context.Background()
	if err := cache.Ping(ctx); err != nil {
		t.Errorf("Failed to ping Redis: %v", err)
	}
}

// TestRedisCacheOperations tests cache operations
func TestRedisCacheOperations(t *testing.T) {
	cfg := &config.RedisConfig{
		Host:         "localhost",
		Port:         6379,
		DB:           1, // Use different DB for testing
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		CacheTTL:     5,
		CacheEnabled: true,
	}

	cache, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	if !cache.Enabled {
		t.Skip("Redis not available, skipping tests")
		return
	}

	defer cache.Close()

	ctx := context.Background()

	// Test Set and Get
	key := "test:key:1"
	value := "test-value"

	if err := cache.Set(ctx, key, value, 10*time.Second); err != nil {
		t.Errorf("Failed to set value: %v", err)
	}

	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Errorf("Failed to get value: %v", err)
	}
	if got != value {
		t.Errorf("Expected %s, got %s", value, got)
	}

	// Test Exists
	exists, err := cache.Exists(ctx, key)
	if err != nil {
		t.Errorf("Failed to check existence: %v", err)
	}
	if !exists {
		t.Error("Expected key to exist")
	}

	// Test Delete
	if err := cache.Delete(ctx, key); err != nil {
		t.Errorf("Failed to delete key: %v", err)
	}

	// Verify deletion
	exists, err = cache.Exists(ctx, key)
	if err != nil {
		t.Errorf("Failed to check existence after delete: %v", err)
	}
	if exists {
		t.Error("Expected key to not exist after deletion")
	}
}

// TestRedisJSONOperations tests JSON serialization
func TestRedisJSONOperations(t *testing.T) {
	cfg := &config.RedisConfig{
		Host:         "localhost",
		Port:         6379,
		DB:           1,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		CacheTTL:     5,
		CacheEnabled: true,
	}

	cache, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	if !cache.Enabled {
		t.Skip("Redis not available, skipping tests")
		return
	}

	defer cache.Close()

	ctx := context.Background()

	// Test SetJSON and GetJSON
	type TestData struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Count   int    `json:"count"`
	}

	key := "test:json:1"
	data := TestData{
		ID:      "123",
		Message: "Test message",
		Count:   42,
	}

	if err := cache.SetJSON(ctx, key, data, 10*time.Second); err != nil {
		t.Errorf("Failed to set JSON: %v", err)
	}

	var got TestData
	if err := cache.GetJSON(ctx, key, &got); err != nil {
		t.Errorf("Failed to get JSON: %v", err)
	}

	if got.ID != data.ID || got.Message != data.Message || got.Count != data.Count {
		t.Errorf("Expected %+v, got %+v", data, got)
	}

	// Clean up
	cache.Delete(ctx, key)
}

// TestRedisDeletePattern tests pattern deletion
func TestRedisDeletePattern(t *testing.T) {
	cfg := &config.RedisConfig{
		Host:         "localhost",
		Port:         6379,
		DB:           1,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		CacheTTL:     5,
		CacheEnabled: true,
	}

	cache, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	if !cache.Enabled {
		t.Skip("Redis not available, skipping tests")
		return
	}

	defer cache.Close()

	ctx := context.Background()

	// Create multiple keys
	keys := []string{"test:pattern:1", "test:pattern:2", "test:pattern:3"}
	for _, key := range keys {
		if err := cache.Set(ctx, key, "value", 60*time.Second); err != nil {
			t.Errorf("Failed to set key %s: %v", key, err)
		}
	}

	// Delete with pattern
	if err := cache.DeletePattern(ctx, "test:pattern:*"); err != nil {
		t.Errorf("Failed to delete pattern: %v", err)
	}

	// Verify deletion
	for _, key := range keys {
		exists, err := cache.Exists(ctx, key)
		if err != nil {
			t.Errorf("Failed to check existence of %s: %v", key, err)
		}
		if exists {
			t.Errorf("Expected key %s to be deleted", key)
		}
	}
}

// TestRedisCacheDisabled tests graceful degradation when cache is disabled
func TestRedisCacheDisabled(t *testing.T) {
	cfg := &config.RedisConfig{
		CacheEnabled: false,
	}

	cache, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create disabled cache: %v", err)
	}

	if cache.Enabled {
		t.Error("Expected cache to be disabled")
	}

	ctx := context.Background()

	// All operations should be no-ops
	if err := cache.Set(ctx, "key", "value", 10*time.Second); err != nil {
		t.Error("Expected nil error for disabled cache Set")
	}

	_, err = cache.Get(ctx, "key")
	if err == nil {
		t.Error("Expected error for disabled cache Get")
	}

	if err := cache.Delete(ctx, "key"); err != nil {
		t.Error("Expected nil error for disabled cache Delete")
	}
}

// TestRedisHealthCheck tests health check
func TestRedisHealthCheck(t *testing.T) {
	cfg := &config.RedisConfig{
		Host:         "localhost",
		Port:         6379,
		DB:           1,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		CacheTTL:     5,
		CacheEnabled: true,
	}

	cache, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	if !cache.Enabled {
		t.Skip("Redis not available, skipping tests")
		return
	}

	defer cache.Close()

	ctx := context.Background()

	// Test health check
	if err := cache.HealthCheck(ctx); err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	// Test stats
	stats, err := cache.GetStats(ctx)
	if err != nil {
		t.Errorf("Failed to get stats: %v", err)
	}

	if !stats["enabled"].(bool) {
		t.Error("Expected enabled to be true in stats")
	}
}
