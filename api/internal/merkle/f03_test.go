package merkle

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/database"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/fabric"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

// fakeAnchorer is an injectable Anchorer so the anchor-failure and reconciliation
// paths can be driven without a live Fabric.
type fakeAnchorer struct {
	store func(batchID, root string) (*fabric.InvokeResponse, error)
}

func (f *fakeAnchorer) Enabled() bool                  { return true }
func (f *fakeAnchorer) ChannelForTenant(string) string { return "logchannel" }
func (f *fakeAnchorer) StoreMerkleBatch(_ context.Context, _, _, batchID, root string, _ int, _ []string) (*fabric.InvokeResponse, error) {
	return f.store(batchID, root)
}
func (f *fakeAnchorer) VerifyMerkleBatch(context.Context, string, string, string) (*fabric.QueryResponse, error) {
	return nil, fmt.Errorf("does not exist")
}

func f03Collections(t *testing.T, coll string) (*database.Collections, context.Context) {
	t.Helper()
	cfg := &config.MongoDBConfig{
		URL: "mongodb://localhost:27017", Database: "logdb_test",
		Collection: "logs_test", RecordsCollection: coll, SyncControlCollection: "sync_control_test",
		MinPoolSize: 5, MaxPoolSize: 20, MaxIdleTimeMS: 60000,
		ServerSelectionTimeoutMS: 5000, ConnectTimeout: 10 * time.Second, SocketTimeout: 30 * time.Second,
	}
	client, err := database.NewMongoClient(cfg)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
	}
	ctx := context.Background()
	client.Database.Collection(coll).Drop(ctx)
	t.Cleanup(func() { client.Close(context.Background()) })
	return database.NewCollections(client), ctx
}

func insertPending(t *testing.T, c *database.Collections, ctx context.Context, tenant, domain string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		r := &models.Record{
			Tenant: tenant, Domain: domain, ID: fmt.Sprintf("rec-%02d", i),
			Timestamp: time.Now().Format(time.RFC3339), Source: "t",
			Payload: map[string]interface{}{"n": i}, CreatedAt: models.FlexTime{Time: time.Now()},
		}
		r.Hash = r.CalculateHash()
		if err := c.InsertRecord(ctx, r); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
}

// TestProcessRecordBatchAnchorFailureNotLost: when the anchor fails, the claimed
// records must NOT be lost — they end up marked failed (batch_id kept), so a
// reconciler can re-drive them, and they are not silently dropped from batching.
func TestProcessRecordBatchAnchorFailureNotLost(t *testing.T) {
	c, ctx := f03Collections(t, "records_f03_fail")
	insertPending(t, c, ctx, "acme", "d", 5)

	anchor := &fakeAnchorer{store: func(string, string) (*fabric.InvokeResponse, error) {
		return nil, fmt.Errorf("peer unreachable")
	}}
	bp := NewBatchProcessor(c, anchor, &config.BatchingConfig{AutoBatchSize: 100})

	result, err := bp.ProcessRecordBatch(ctx, "acme", "d", 100)
	if err == nil {
		t.Error("expected an anchor error to be surfaced")
	}
	if result == nil || result.BatchID == "" {
		t.Fatal("expected a result with a batch id even on anchor failure")
	}
	if result.Anchored {
		t.Error("result must not report anchored on failure")
	}

	batch, err := c.FindRecordsByBatchID(ctx, "acme", "d", result.BatchID)
	if err != nil {
		t.Fatalf("find batch: %v", err)
	}
	if len(batch) != 5 {
		t.Fatalf("records lost: batch has %d, want 5", len(batch))
	}
	for _, r := range batch {
		if r.AnchorStatus != models.RecordAnchorFailed {
			t.Errorf("record %s status = %q, want failed", r.ID, r.AnchorStatus)
		}
	}
	if pending, _ := c.FindRecordsWithoutBatch(ctx, "acme", "d", 100); len(pending) != 0 {
		t.Errorf("failed records must not return to the pending pool, got %d", len(pending))
	}
}

// TestReconcileRetriesFailedAndAnchors: the reconciler re-drives a failed batch
// and, when the anchor now succeeds, marks it anchored with the tx id.
func TestReconcileRetriesFailedAndAnchors(t *testing.T) {
	c, ctx := f03Collections(t, "records_f03_reconcile")
	insertPending(t, c, ctx, "acme", "d", 4)

	failing := &fakeAnchorer{store: func(string, string) (*fabric.InvokeResponse, error) {
		return nil, fmt.Errorf("peer unreachable")
	}}
	bp := NewBatchProcessor(c, failing, &config.BatchingConfig{AutoBatchSize: 100})
	result, _ := bp.ProcessRecordBatch(ctx, "acme", "d", 100) // leaves a failed batch

	// Anchor recovers.
	bp.anchorer = &fakeAnchorer{store: func(batchID, root string) (*fabric.InvokeResponse, error) {
		return &fabric.InvokeResponse{TxID: "tx-recovered"}, nil
	}}
	if err := bp.ReconcileBatches(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	batch, _ := c.FindRecordsByBatchID(ctx, "acme", "d", result.BatchID)
	if len(batch) == 0 {
		t.Fatal("batch vanished")
	}
	for _, r := range batch {
		if r.AnchorStatus != models.RecordAnchorAnchored {
			t.Errorf("record %s status = %q, want anchored", r.ID, r.AnchorStatus)
		}
		if r.TxID != "tx-recovered" {
			t.Errorf("record %s tx_id = %q, want tx-recovered", r.ID, r.TxID)
		}
	}
}

// TestReconcileAlreadyAnchoredTreatedAsSuccess: if a re-anchor hits the F02
// write-once guard ("already anchored"), the batch IS on-chain — the reconciler
// must treat that as success and stop retrying.
func TestReconcileAlreadyAnchoredTreatedAsSuccess(t *testing.T) {
	c, ctx := f03Collections(t, "records_f03_already")
	insertPending(t, c, ctx, "acme", "d", 3)

	failing := &fakeAnchorer{store: func(string, string) (*fabric.InvokeResponse, error) {
		return nil, fmt.Errorf("peer unreachable")
	}}
	bp := NewBatchProcessor(c, failing, &config.BatchingConfig{AutoBatchSize: 100})
	result, _ := bp.ProcessRecordBatch(ctx, "acme", "d", 100)

	bp.anchorer = &fakeAnchorer{store: func(batchID, root string) (*fabric.InvokeResponse, error) {
		return nil, fmt.Errorf("batch %s is already anchored and cannot be overwritten", batchID)
	}}
	if err := bp.ReconcileBatches(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	batch, _ := c.FindRecordsByBatchID(ctx, "acme", "d", result.BatchID)
	for _, r := range batch {
		if r.AnchorStatus != models.RecordAnchorAnchored {
			t.Errorf("record %s status = %q, want anchored (already-anchored is success)", r.ID, r.AnchorStatus)
		}
	}
}
