package database

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/google/uuid"
)

// TestMongoClientConnection tests MongoDB connection
func TestMongoClientConnection(t *testing.T) {
	cfg := &config.MongoDBConfig{
		URL:                       "mongodb://localhost:27017",
		Database:                  "logdb_test",
		Collection:                "logs_test",
		SyncControlCollection:     "sync_control_test",
		MinPoolSize:               5,
		MaxPoolSize:               10,
		MaxIdleTimeMS:             60000,
		ServerSelectionTimeoutMS:  5000,
		ConnectTimeout:            10 * time.Second,
		SocketTimeout:             30 * time.Second,
	}

	client, err := NewMongoClient(cfg)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
		return
	}
	defer client.Close(context.Background())

	// Test ping
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Errorf("Failed to ping MongoDB: %v", err)
	}

	// Test health check
	if err := client.HealthCheck(ctx); err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	// Test stats
	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Errorf("Failed to get stats: %v", err)
	}
	if !stats["connected"].(bool) {
		t.Error("Expected connected to be true")
	}
}

// TestCollectionsOperations tests CRUD operations
func TestCollectionsOperations(t *testing.T) {
	cfg := &config.MongoDBConfig{
		URL:                       "mongodb://localhost:27017",
		Database:                  "logdb_test",
		Collection:                "logs_test",
		SyncControlCollection:     "sync_control_test",
		MinPoolSize:               5,
		MaxPoolSize:               10,
		MaxIdleTimeMS:             60000,
		ServerSelectionTimeoutMS:  5000,
		ConnectTimeout:            10 * time.Second,
		SocketTimeout:             30 * time.Second,
	}

	client, err := NewMongoClient(cfg)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
		return
	}
	defer client.Close(context.Background())

	collections := NewCollections(client)
	ctx := context.Background()

	// Clean up before test
	client.Database.Collection(cfg.Collection).Drop(ctx)
	client.Database.Collection(cfg.SyncControlCollection).Drop(ctx)

	// Re-create indexes
	if err := client.CreateIndexes(ctx); err != nil {
		t.Fatalf("Failed to create indexes: %v", err)
	}

	// Test log insertion
	log := &models.Log{
		ID:        uuid.New().String(),
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     models.LogLevelInfo,
		Message:   "Test log message",
		Source:    "test-service",
		Metadata: map[string]interface{}{
			"test": "data",
		},
		CreatedAt: models.FlexTime{Time: time.Now()},
	}
	log.Hash = log.CalculateHash()

	if err := collections.InsertLog(ctx, log); err != nil {
		t.Fatalf("Failed to insert log: %v", err)
	}

	// Test find by ID
	foundLog, err := collections.FindLogByID(ctx, log.ID)
	if err != nil {
		t.Fatalf("Failed to find log: %v", err)
	}
	if foundLog.ID != log.ID {
		t.Errorf("Expected log ID %s, got %s", log.ID, foundLog.ID)
	}

	// Test count
	count, err := collections.CountLogs(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Failed to count logs: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 log, got %d", count)
	}

	// Test sync control insertion
	syncControl := &models.SyncControl{
		LogID:      log.ID,
		SyncStatus: models.SyncStatusPending,
		CreatedAt:  time.Now(),
	}

	if err := collections.InsertSyncControl(ctx, syncControl); err != nil {
		t.Fatalf("Failed to insert sync control: %v", err)
	}

	// Test find sync control
	foundSync, err := collections.FindSyncControlByLogID(ctx, log.ID)
	if err != nil {
		t.Fatalf("Failed to find sync control: %v", err)
	}
	if foundSync.LogID != log.ID {
		t.Errorf("Expected log ID %s, got %s", log.ID, foundSync.LogID)
	}

	// Test update sync status
	if err := collections.UpdateSyncStatus(ctx, log.ID, models.SyncStatusSynced); err != nil {
		t.Fatalf("Failed to update sync status: %v", err)
	}

	// Verify update
	foundSync, err = collections.FindSyncControlByLogID(ctx, log.ID)
	if err != nil {
		t.Fatalf("Failed to find sync control after update: %v", err)
	}
	if foundSync.SyncStatus != models.SyncStatusSynced {
		t.Errorf("Expected status %s, got %s", models.SyncStatusSynced, foundSync.SyncStatus)
	}

	// Test aggregate stats
	stats, err := collections.AggregateSyncStats(ctx)
	if err != nil {
		t.Fatalf("Failed to aggregate sync stats: %v", err)
	}
	if stats.Synced != 1 {
		t.Errorf("Expected 1 synced log, got %d", stats.Synced)
	}
}

// TestConnectWithRetry tests retry mechanism
func TestConnectWithRetry(t *testing.T) {
	cfg := &config.MongoDBConfig{
		URL:                       "mongodb://invalid-host:27017",
		Database:                  "logdb_test",
		Collection:                "logs_test",
		SyncControlCollection:     "sync_control_test",
		MinPoolSize:               5,
		MaxPoolSize:               10,
		MaxIdleTimeMS:             60000,
		ServerSelectionTimeoutMS:  1000,
		ConnectTimeout:            2 * time.Second,
		SocketTimeout:             2 * time.Second,
	}

	// This should fail after 2 retries
	start := time.Now()
	_, err := ConnectWithRetry(cfg, 2)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error for invalid host, got nil")
	}

	// Should retry with backoff (1s + 2s = 3s minimum)
	if duration < 3*time.Second {
		t.Errorf("Expected at least 3 seconds for 2 retries, got %v", duration)
	}
}
