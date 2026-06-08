package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

// TestSoftDeleteLog verifies that soft delete preserves the document, hides it
// from active listings, and is idempotent for already-deleted/missing logs.
func TestSoftDeleteLog(t *testing.T) {
	cfg := &config.MongoDBConfig{
		URL:                      "mongodb://localhost:27017",
		Database:                 "logdb_test",
		Collection:               "logs_test",
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
	client.Database.Collection(cfg.Collection).Drop(ctx)

	log := &models.Log{
		ID:        uuid.New().String(),
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     models.LogLevelInfo,
		Message:   "soft delete test",
		Source:    "test-service",
		CreatedAt: models.FlexTime{Time: time.Now()},
	}
	log.Hash = log.CalculateHash()
	if err := collections.InsertLog(ctx, log); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Soft delete sets deleted_at and keeps the document.
	if err := collections.SoftDeleteLog(ctx, log.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	found, err := collections.FindLogByID(ctx, log.ID)
	if err != nil {
		t.Fatalf("document should still exist after soft delete: %v", err)
	}
	if found.DeletedAt == nil {
		t.Error("expected deleted_at to be set")
	}

	// Listings that exclude deleted must not return it.
	active, err := collections.FindLogs(ctx, bson.M{"deleted_at": bson.M{"$exists": false}}, NewFindOptions())
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	for _, l := range active {
		if l.ID == log.ID {
			t.Error("soft-deleted log should not appear in active listing")
		}
	}

	// Re-deleting or deleting a missing log reports ErrNoDocuments.
	if err := collections.SoftDeleteLog(ctx, log.ID); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Errorf("re-delete: expected ErrNoDocuments, got %v", err)
	}
	if err := collections.SoftDeleteLog(ctx, "does-not-exist"); !errors.Is(err, mongo.ErrNoDocuments) {
		t.Errorf("missing log: expected ErrNoDocuments, got %v", err)
	}
}

// TestKeysetPagination walks the (created_at desc, id desc) keyset filter used
// by cursor pagination and verifies every log is returned exactly once, in
// order, with no overlap across pages — including ties on the same millisecond.
func TestKeysetPagination(t *testing.T) {
	cfg := &config.MongoDBConfig{
		URL:                      "mongodb://localhost:27017",
		Database:                 "logdb_test",
		Collection:               "logs_test",
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
	client.Database.Collection(cfg.Collection).Drop(ctx)

	const total = 10
	base := time.Now().UTC().Truncate(time.Millisecond)
	for i := 0; i < total; i++ {
		ts := base.Add(time.Duration(i) * time.Millisecond)
		if i == 4 {
			ts = base.Add(3 * time.Millisecond) // share a millisecond with i==3
		}
		log := &models.Log{
			ID:        fmt.Sprintf("log-%02d-%s", i, uuid.New().String()),
			Timestamp: ts.Format(time.RFC3339),
			Level:     models.LogLevelInfo,
			Message:   "x",
			Source:    "keyset",
			CreatedAt: models.FlexTime{Time: ts},
		}
		log.Hash = log.CalculateHash()
		if err := collections.InsertLog(ctx, log); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	baseFilter := bson.M{"deleted_at": bson.M{"$exists": false}, "source": "keyset"}
	sort := bson.D{{Key: "created_at", Value: -1}, {Key: "id", Value: -1}}
	const limit = int64(3)

	seen := make(map[string]bool)
	var paged []string
	var lastMs int64
	var lastID string
	first := true

	for {
		filter := bson.M{}
		for k, v := range baseFilter {
			filter[k] = v
		}
		if !first {
			boundary := primitive.NewDateTimeFromTime(time.UnixMilli(lastMs).UTC())
			filter["$or"] = bson.A{
				bson.M{"created_at": bson.M{"$lt": boundary}},
				bson.M{"created_at": boundary, "id": bson.M{"$lt": lastID}},
			}
		}

		page, err := collections.FindLogs(ctx, filter, NewFindOptions().SetSort(sort).SetLimit(limit))
		if err != nil {
			t.Fatalf("page query: %v", err)
		}
		if len(page) == 0 {
			break
		}
		for _, l := range page {
			if seen[l.ID] {
				t.Errorf("duplicate id across pages: %s", l.ID)
			}
			seen[l.ID] = true
			paged = append(paged, l.ID)
		}
		last := page[len(page)-1]
		lastMs = last.CreatedAt.UnixMilli()
		lastID = last.ID
		first = false
		if len(page) < int(limit) {
			break
		}
	}

	if len(seen) != total {
		t.Errorf("expected %d unique logs across pages, got %d", total, len(seen))
	}

	// Paged order must match a single sorted query over the whole set.
	all, err := collections.FindLogs(ctx, baseFilter, NewFindOptions().SetSort(sort))
	if err != nil {
		t.Fatalf("sorted query: %v", err)
	}
	if len(all) != len(paged) {
		t.Fatalf("sorted=%d paged=%d", len(all), len(paged))
	}
	for i := range all {
		if all[i].ID != paged[i] {
			t.Errorf("order mismatch at %d: paged=%s sorted=%s", i, paged[i], all[i].ID)
		}
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
