package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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

// TestRecordsCRUD verifies generic record insert/find, domain isolation and
// soft delete against MongoDB.
func TestRecordsCRUD(t *testing.T) {
	cfg := &config.MongoDBConfig{
		URL:                      "mongodb://localhost:27017",
		Database:                 "logdb_test",
		Collection:               "logs_test",
		RecordsCollection:        "records_test",
		SyncControlCollection:    "sync_control_test",
		MinPoolSize:              5,
		MaxPoolSize:              10,
		MaxIdleTimeMS:            60000,
		ServerSelectionTimeoutMS: 5000,
		ConnectTimeout:           10 * time.Second,
		SocketTimeout:            30 * time.Second,
	}

	client, err := NewMongoClient(cfg)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
		return
	}
	defer client.Close(context.Background())

	collections := NewCollections(client)
	ctx := context.Background()
	client.Database.Collection(cfg.RecordsCollection).Drop(ctx)

	rec := &models.Record{
		Tenant:    "acme",
		Domain:    "contracts",
		ID:        uuid.New().String(),
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    "crm",
		Payload:   map[string]interface{}{"amount": 100, "party": "acme"},
		CreatedAt: models.FlexTime{Time: time.Now()},
	}
	rec.Hash = rec.CalculateHash()
	if err := collections.InsertRecord(ctx, rec); err != nil {
		t.Fatalf("insert: %v", err)
	}

	found, err := collections.FindRecordByID(ctx, "acme", "contracts", rec.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found.Hash != rec.Hash {
		t.Errorf("hash mismatch: got %s want %s", found.Hash, rec.Hash)
	}

	// Tenant isolation: the same id under another tenant must not be found.
	if _, err := collections.FindRecordByID(ctx, "globex", "contracts", rec.ID); err == nil {
		t.Error("expected not found for the same id under a different tenant")
	}
	// Domain isolation: the same id under another domain must not be found.
	if _, err := collections.FindRecordByID(ctx, "acme", "other", rec.ID); err == nil {
		t.Error("expected not found for the same id in a different domain")
	}

	// Soft delete hides the record from active listings but keeps it.
	if err := collections.SoftDeleteRecord(ctx, "acme", "contracts", rec.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	active, err := collections.FindRecords(ctx, bson.M{"tenant": "acme", "domain": "contracts", "deleted_at": bson.M{"$exists": false}}, NewFindOptions())
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	for _, r := range active {
		if r.ID == rec.ID {
			t.Error("soft-deleted record should not appear in active listing")
		}
	}
	if err := collections.SoftDeleteRecord(ctx, "acme", "contracts", rec.ID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Errorf("re-delete: expected ErrNoDocuments, got %v", err)
	}
}

// TestInsertRecordDuplicate verifies a duplicate (tenant, domain, id) — including a
// re-used id — surfaces ErrDuplicateRecord (mapped to 409 by the handler), not a
// generic error, while the same id under another tenant is allowed.
func TestInsertRecordDuplicate(t *testing.T) {
	cfg := &config.MongoDBConfig{
		URL:                      "mongodb://localhost:27017",
		Database:                 "logdb_test",
		Collection:               "logs_test",
		RecordsCollection:        "records_dup_test",
		SyncControlCollection:    "sync_control_test",
		MinPoolSize:              5,
		MaxPoolSize:              10,
		MaxIdleTimeMS:            60000,
		ServerSelectionTimeoutMS: 5000,
		ConnectTimeout:           10 * time.Second,
		SocketTimeout:            30 * time.Second,
	}
	client, err := NewMongoClient(cfg)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
		return
	}
	defer client.Close(context.Background())
	ctx := context.Background()
	client.Database.Collection(cfg.RecordsCollection).Drop(ctx)
	if err := client.CreateIndexes(ctx); err != nil {
		t.Fatalf("create indexes: %v", err)
	}

	collections := NewCollections(client)
	rec := &models.Record{
		Tenant:    "acme",
		Domain:    "contracts",
		ID:        "dup-1",
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    "crm",
		Payload:   map[string]interface{}{"x": 1},
		CreatedAt: models.FlexTime{Time: time.Now()},
	}
	rec.Hash = rec.CalculateHash()
	if err := collections.InsertRecord(ctx, rec); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Same (tenant, domain, id) -> duplicate.
	if err := collections.InsertRecord(ctx, rec); !errors.Is(err, ErrDuplicateRecord) {
		t.Errorf("duplicate insert: expected ErrDuplicateRecord, got %v", err)
	}
	// Same id under a different tenant is allowed.
	other := *rec
	other.Tenant = "globex"
	if err := collections.InsertRecord(ctx, &other); err != nil {
		t.Errorf("same id under another tenant should be allowed, got %v", err)
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
