package merkle

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/fabric"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

func newTestProcessor(t *testing.T, cfg *config.BatchingConfig) *BatchProcessor {
	t.Helper()
	fabricClient, _ := fabric.NewFabricClient(&config.FabricConfig{SyncEnabled: false})
	return NewBatchProcessor(nil, fabricClient, cfg)
}

// TestBatchProcessorCreation tests batch processor creation
func TestBatchProcessorCreation(t *testing.T) {
	processor := newTestProcessor(t, &config.BatchingConfig{
		Enabled:           true,
		AutoBatchSize:     10,
		AutoBatchInterval: 5 * time.Second,
	})
	if processor == nil {
		t.Fatal("Expected processor to be created")
	}
	if processor.IsRunning() {
		t.Error("Expected a fresh processor to not be running")
	}
}

// TestBatchProcessorStartStop tests starting and stopping the processor
func TestBatchProcessorStartStop(t *testing.T) {
	processor := newTestProcessor(t, &config.BatchingConfig{
		Enabled:           false, // disable auto-batch to avoid touching a nil collections
		AutoBatchSize:     10,
		AutoBatchInterval: 100 * time.Millisecond,
	})

	ctx := context.Background()
	if err := processor.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	if !processor.IsRunning() {
		t.Error("Expected processor to be running")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := processor.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop processor: %v", err)
	}
	if processor.IsRunning() {
		t.Error("Expected processor to be stopped")
	}
}

// TestBatchProcessorStatistics tests statistics tracking
func TestBatchProcessorStatistics(t *testing.T) {
	processor := newTestProcessor(t, &config.BatchingConfig{
		Enabled:           true,
		AutoBatchSize:     10,
		AutoBatchInterval: 5 * time.Second,
	})

	processor.updateStats("batch_123", 50, time.Now())
	stats := processor.GetStats()

	if stats.TotalBatches != 1 {
		t.Errorf("Expected 1 total batch, got %d", stats.TotalBatches)
	}
	if stats.TotalRecords != 50 {
		t.Errorf("Expected 50 total records, got %d", stats.TotalRecords)
	}
	if stats.LastBatchID != "batch_123" {
		t.Errorf("Expected last batch ID 'batch_123', got %s", stats.LastBatchID)
	}

	processor.incrementError()
	if stats = processor.GetStats(); stats.ProcessingErrors != 1 {
		t.Errorf("Expected 1 processing error, got %d", stats.ProcessingErrors)
	}
}

// TestMerkleTreeDeterminism checks the shared Merkle tree construction is
// deterministic and structure-sensitive.
func TestMerkleTreeDeterminism(t *testing.T) {
	hashes := []string{"a", "b", "c"}

	root1 := models.BuildMerkleTree(hashes)
	root2 := models.BuildMerkleTree(hashes)
	if root1 == "" {
		t.Error("Expected non-empty Merkle root")
	}
	if root1 != root2 {
		t.Error("Merkle root should be deterministic for the same leaves")
	}
	if got := models.BuildMerkleTree([]string{"x"}); got != "x" {
		t.Errorf("single leaf should be its own root, got %q", got)
	}
	if models.BuildMerkleTree([]string{"a", "b"}) == models.BuildMerkleTree([]string{"a", "c"}) {
		t.Error("different leaves should yield different roots")
	}
}

// BenchmarkMerkleTree benchmarks Merkle tree construction over 100 leaves.
func BenchmarkMerkleTree(b *testing.B) {
	hashes := make([]string, 100)
	for i := range hashes {
		hashes[i] = models.CombineHashes("leaf", time.Now().String())
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		models.BuildMerkleTree(hashes)
	}
}
