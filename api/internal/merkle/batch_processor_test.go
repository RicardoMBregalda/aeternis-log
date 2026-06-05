package merkle

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/google/uuid"
)

// TestBatchProcessorCreation tests batch processor creation
func TestBatchProcessorCreation(t *testing.T) {
	cfg := &config.BatchingConfig{
		Enabled:              true,
		AutoBatchSize:        10,
		AutoBatchInterval:    5 * time.Second,
		BatchExecutorWorkers: 3,
	}

	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient, _ := fabric.NewFabricClient(fabricCfg)

	processor := NewBatchProcessor(nil, fabricClient, cfg)

	if processor == nil {
		t.Fatal("Expected processor to be created")
	}

	if processor.workers != 3 {
		t.Errorf("Expected 3 workers, got %d", processor.workers)
	}
}

// TestBatchProcessorStartStop tests starting and stopping the processor
func TestBatchProcessorStartStop(t *testing.T) {
	cfg := &config.BatchingConfig{
		Enabled:              false, // Disable auto-batch to avoid nil pointer with collections
		AutoBatchSize:        10,
		AutoBatchInterval:    100 * time.Millisecond,
		BatchExecutorWorkers: 2,
	}

	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient, _ := fabric.NewFabricClient(fabricCfg)

	processor := NewBatchProcessor(nil, fabricClient, cfg)

	ctx := context.Background()

	// Start processor
	if err := processor.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	if !processor.IsRunning() {
		t.Error("Expected processor to be running")
	}

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Stop processor
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
	cfg := &config.BatchingConfig{
		Enabled:              true,
		AutoBatchSize:        10,
		AutoBatchInterval:    5 * time.Second,
		BatchExecutorWorkers: 2,
	}

	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient, _ := fabric.NewFabricClient(fabricCfg)

	processor := NewBatchProcessor(nil, fabricClient, cfg)

	// Update stats
	processor.updateStats("batch_123", 50, time.Now())

	stats := processor.GetStats()

	if stats.TotalBatches != 1 {
		t.Errorf("Expected 1 total batch, got %d", stats.TotalBatches)
	}

	if stats.TotalLogs != 50 {
		t.Errorf("Expected 50 total logs, got %d", stats.TotalLogs)
	}

	if stats.LastBatchID != "batch_123" {
		t.Errorf("Expected last batch ID 'batch_123', got %s", stats.LastBatchID)
	}

	// Increment error
	processor.incrementError()
	stats = processor.GetStats()

	if stats.ProcessingErrors != 1 {
		t.Errorf("Expected 1 processing error, got %d", stats.ProcessingErrors)
	}
}

// TestVerifyBatchWithMockData tests batch verification with mock data
func TestVerifyBatchWithMockData(t *testing.T) {
	// This test would require a mock MongoDB connection
	// Skipping for unit test, should be tested in integration tests
	t.Skip("Requires MongoDB connection")
}

// TestMerkleTreeCalculation tests Merkle tree root calculation
func TestMerkleTreeCalculation(t *testing.T) {
	// Create test logs
	now := time.Now()
	logs := []*models.Log{
		{
			ID:        uuid.New().String(),
			Timestamp: now.Format(time.RFC3339),
			Level:     models.LogLevelInfo,
			Message:   "Test log 1",
			Source:    "test",
			CreatedAt: models.FlexTime{Time: now},
		},
		{
			ID:        uuid.New().String(),
			Timestamp: now.Format(time.RFC3339),
			Level:     models.LogLevelInfo,
			Message:   "Test log 2",
			Source:    "test",
			CreatedAt: models.FlexTime{Time: now},
		},
	}

	// Calculate Merkle root
	merkleRoot, hashes := models.CalculateMerkleRoot(logs)

	if merkleRoot == "" {
		t.Error("Expected non-empty Merkle root")
	}

	if len(hashes) != len(logs) {
		t.Errorf("Expected %d hashes, got %d", len(logs), len(hashes))
	}

	// Verify consistency - same logs should produce same root
	merkleRoot2, _ := models.CalculateMerkleRoot(logs)
	if merkleRoot != merkleRoot2 {
		t.Error("Merkle root should be consistent for same logs")
	}
}

// BenchmarkBatchProcessing benchmarks batch processing
func BenchmarkBatchProcessing(b *testing.B) {
	// Create test logs
	now := time.Now()
	logs := make([]*models.Log, 100)
	for i := 0; i < 100; i++ {
		logs[i] = &models.Log{
			ID:        uuid.New().String(),
			Timestamp: now.Format(time.RFC3339),
			Level:     models.LogLevelInfo,
			Message:   "Benchmark log",
			Source:    "benchmark",
			CreatedAt: models.FlexTime{Time: now},
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		models.CalculateMerkleRoot(logs)
	}
}
