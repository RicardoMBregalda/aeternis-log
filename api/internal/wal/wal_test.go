package wal

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
)

func TestWALWrite(t *testing.T) {
	// Create temporary WAL directory
	tempDir := t.TempDir()

	// Create WAL
	wal, err := NewWriteAheadLog(tempDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	// Create test log
	now := time.Now().UTC()
	log := &models.Log{
		ID:        "test-001",
		Source:    "test-source",
		Level:     models.LogLevelInfo,
		Message:   "Test message",
		Timestamp: now.Format(time.RFC3339),
		CreatedAt: models.FlexTime{Time: now},
		Metadata:  make(map[string]interface{}),
	}

	// Write to WAL
	if err := wal.Write(log); err != nil {
		t.Fatalf("Failed to write to WAL: %v", err)
	}

	// Check statistics
	stats := wal.GetStats()
	if stats.TotalWritten != 1 {
		t.Errorf("Expected TotalWritten=1, got %d", stats.TotalWritten)
	}
	if stats.PendingCount == 0 {
		t.Errorf("Expected PendingCount>0, got %d", stats.PendingCount)
	}

	// Verify file exists
	if _, err := os.Stat(wal.pendingFile); os.IsNotExist(err) {
		t.Error("Pending file was not created")
	}
}

func TestWALRecovery(t *testing.T) {
	// Create temporary WAL directory
	tempDir := t.TempDir()

	// Create first WAL instance and write
	wal1, err := NewWriteAheadLog(tempDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	log := &models.Log{
		ID:        "test-recovery",
		Source:    "test",
		Level:     models.LogLevelInfo,
		Message:   "Recovery test",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: models.FlexTime{Time: time.Now().UTC()},
		Metadata:  make(map[string]interface{}),
	}

	if err := wal1.Write(log); err != nil {
		t.Fatalf("Failed to write to WAL: %v", err)
	}

	// Simulate crash - create new WAL instance
	wal2, err := NewWriteAheadLog(tempDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create second WAL: %v", err)
	}

	// Check if pending logs were recovered
	stats := wal2.GetStats()
	if stats.PendingCount == 0 {
		t.Error("Expected recovered pending logs, got 0")
	}
}

func TestWALProcessor(t *testing.T) {
	// Create temporary WAL directory
	tempDir := t.TempDir()

	// Create WAL
	wal, err := NewWriteAheadLog(tempDir, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	// Track processed logs (thread-safe with mutex)
	var mu sync.Mutex
	processedLogs := make(map[string]bool)
	var processedCount int

	// Mock MongoDB insert callback
	insertCallback := func(log *models.Log) error {
		mu.Lock()
		processedLogs[log.ID] = true
		processedCount++
		mu.Unlock()
		return nil
	}

	// Start processor
	wal.StartProcessor(insertCallback)
	defer wal.StopProcessor()

	// Write test logs
	for i := 0; i < 5; i++ {
		log := &models.Log{
			ID:        string(rune('A' + i)),
			Source:    "test",
			Level:     models.LogLevelInfo,
			Message:   "Test message",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			CreatedAt: models.FlexTime{Time: time.Now().UTC()},
			Metadata:  make(map[string]interface{}),
		}
		if err := wal.Write(log); err != nil {
			t.Fatalf("Failed to write log %d: %v", i, err)
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify all logs were processed (thread-safe read)
	mu.Lock()
	count := processedCount
	mu.Unlock()
	
	if count != 5 {
		t.Errorf("Expected 5 processed logs, got %d", count)
	}

	// Check statistics
	stats := wal.GetStats()
	if stats.TotalProcessed != 5 {
		t.Errorf("Expected TotalProcessed=5, got %d", stats.TotalProcessed)
	}
	if stats.PendingCount != 0 {
		t.Errorf("Expected PendingCount=0, got %d", stats.PendingCount)
	}
}

func TestWALGracefulShutdown(t *testing.T) {
	tempDir := t.TempDir()

	wal, err := NewWriteAheadLog(tempDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	insertCallback := func(log *models.Log) error {
		time.Sleep(50 * time.Millisecond) // Simulate slow processing
		return nil
	}

	wal.StartProcessor(insertCallback)

	// Write logs
	for i := 0; i < 3; i++ {
		log := &models.Log{
			ID:        string(rune('X' + i)),
			Source:    "test",
			Level:     models.LogLevelInfo,
			Message:   "Test",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			CreatedAt: models.FlexTime{Time: time.Now().UTC()},
			Metadata:  make(map[string]interface{}),
		}
		wal.Write(log)
	}

	// Stop processor gracefully
	wal.StopProcessor()

	// Verify processor stopped
	if wal.IsRunning() {
		t.Error("Processor should be stopped")
	}
}

func BenchmarkWALWrite(b *testing.B) {
	tempDir := b.TempDir()

	wal, err := NewWriteAheadLog(tempDir, 10*time.Second)
	if err != nil {
		b.Fatalf("Failed to create WAL: %v", err)
	}

	log := &models.Log{
		ID:        "bench-log",
		Source:    "benchmark",
		Level:     models.LogLevelInfo,
		Message:   "Benchmark message",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: models.FlexTime{Time: time.Now().UTC()},
		Metadata:  make(map[string]interface{}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := wal.Write(log); err != nil {
			b.Fatalf("Write failed: %v", err)
		}
	}
}
