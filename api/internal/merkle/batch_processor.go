package merkle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/database"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/webhook"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/google/uuid"
	zlog "github.com/rs/zerolog/log"
)

// BatchProcessor handles automatic batching and Merkle tree generation
type BatchProcessor struct {
	collections   *database.Collections
	fabricClient  *fabric.FabricClient
	config        *config.BatchingConfig
	notifier      *webhook.Notifier

	// Worker pool
	workers       int
	jobQueue      chan *BatchJob
	stopChan      chan struct{}
	wg            sync.WaitGroup
	running       bool
	mu            sync.RWMutex
	
	// Statistics
	stats         *ProcessorStats
	statsMu       sync.RWMutex
}

// BatchJob represents a batching job
type BatchJob struct {
	BatchSize int
	Context   context.Context
}

// ProcessorStats holds processor statistics
type ProcessorStats struct {
	TotalBatches     int       `json:"total_batches"`
	TotalLogs        int       `json:"total_logs"`
	FailedBatches    int       `json:"failed_batches"`
	LastBatchTime    time.Time `json:"last_batch_time"`
	LastBatchSize    int       `json:"last_batch_size"`
	LastBatchID      string    `json:"last_batch_id"`
	ProcessingErrors int       `json:"processing_errors"`
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(collections *database.Collections, fabricClient *fabric.FabricClient, cfg *config.BatchingConfig) *BatchProcessor {
	workers := cfg.BatchExecutorWorkers
	if workers <= 0 {
		workers = 5
	}

	return &BatchProcessor{
		collections:  collections,
		fabricClient: fabricClient,
		config:       cfg,
		workers:      workers,
		jobQueue:     make(chan *BatchJob, 100),
		stopChan:     make(chan struct{}),
		stats:        &ProcessorStats{},
	}
}

// SetNotifier attaches a webhook notifier fired when a batch is anchored.
func (bp *BatchProcessor) SetNotifier(n *webhook.Notifier) {
	bp.notifier = n
}

// notifyAnchored fires the batch-anchored webhook (best-effort, async).
func (bp *BatchProcessor) notifyAnchored(tenant, domain, batchID, merkleRoot string, numRecords int, txID string) {
	if bp.notifier == nil {
		return
	}
	bp.notifier.NotifyBatchAnchored(webhook.BatchAnchoredEvent{
		Tenant:     tenant,
		Domain:     domain,
		BatchID:    batchID,
		MerkleRoot: merkleRoot,
		NumRecords: numRecords,
		TxID:       txID,
		AnchoredAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// Start starts the batch processor
func (bp *BatchProcessor) Start(ctx context.Context) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.running {
		return fmt.Errorf("batch processor already running")
	}

	bp.running = true

	// Start worker pool
	for i := 0; i < bp.workers; i++ {
		bp.wg.Add(1)
		go bp.worker(ctx, i)
	}

	// Start auto-batch ticker if enabled
	if bp.config.Enabled && bp.config.AutoBatchInterval > 0 {
		bp.wg.Add(1)
		go bp.autoBatchTicker(ctx)
	}

	return nil
}

// Stop stops the batch processor
func (bp *BatchProcessor) Stop(ctx context.Context) error {
	bp.mu.Lock()
	if !bp.running {
		bp.mu.Unlock()
		return fmt.Errorf("batch processor not running")
	}
	bp.running = false
	bp.mu.Unlock()

	// Signal stop
	close(bp.stopChan)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		bp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for workers to stop")
	}
}

// worker processes batch jobs
func (bp *BatchProcessor) worker(ctx context.Context, workerID int) {
	defer bp.wg.Done()

	for {
		select {
		case <-bp.stopChan:
			return
		case <-ctx.Done():
			return
		case job := <-bp.jobQueue:
			if err := bp.processBatch(job.Context, job.BatchSize); err != nil {
				bp.incrementError()
				zlog.Error().Err(err).Int("worker", workerID).Msg("batch processing error")
			}
		}
	}
}

// autoBatchTicker periodically triggers automatic batching
func (bp *BatchProcessor) autoBatchTicker(ctx context.Context) {
	defer bp.wg.Done()

	ticker := time.NewTicker(bp.config.AutoBatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bp.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Submit auto-batch job for logs.
			select {
			case bp.jobQueue <- &BatchJob{
				BatchSize: bp.config.AutoBatchSize,
				Context:   ctx,
			}:
			default:
				// Queue is full, skip this tick
			}

			// Auto-batch pending records per (tenant, domain); each anchored to Fabric.
			if scopes, err := bp.collections.DistinctPendingRecordScopes(ctx); err == nil {
				for _, s := range scopes {
					if _, err := bp.ProcessRecordBatch(ctx, s.Tenant, s.Domain, bp.config.AutoBatchSize); err != nil {
						zlog.Error().Err(err).Str("tenant", s.Tenant).Str("domain", s.Domain).Msg("record auto-batch error")
					}
				}
			}
		}
	}
}

// ProcessBatch submits a batch job for processing
func (bp *BatchProcessor) ProcessBatch(ctx context.Context, batchSize int) error {
	bp.mu.RLock()
	if !bp.running {
		bp.mu.RUnlock()
		return fmt.Errorf("batch processor not running")
	}
	bp.mu.RUnlock()

	if batchSize <= 0 {
		batchSize = bp.config.AutoBatchSize
	}

	// The job is processed asynchronously by a worker, so it must NOT inherit
	// the caller's (HTTP request) context — that gets cancelled as soon as the
	// handler returns, which would kill the in-flight batch. Only the enqueue
	// below is gated by the caller's context.
	job := &BatchJob{
		BatchSize: batchSize,
		Context:   context.Background(),
	}

	select {
	case bp.jobQueue <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("job queue is full")
	}
}

// processBatch processes a single batch
func (bp *BatchProcessor) processBatch(ctx context.Context, batchSize int) error {
	startTime := time.Now()

	// Find logs without batch
	logs, err := bp.collections.FindLogsWithoutBatch(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("failed to find logs: %w", err)
	}

	if len(logs) == 0 {
		return nil // No logs to batch
	}

	// Generate batch ID
	batchID := fmt.Sprintf("batch_%s", uuid.New().String()[:8])

	// Calculate Merkle root
	merkleRoot, _ := models.CalculateMerkleRoot(logs)

	// Extract log IDs
	logIDs := make([]string, len(logs))
	for i, log := range logs {
		logIDs[i] = log.ID
	}

	// Update logs with batch information
	if err := bp.collections.UpdateLogBatch(ctx, logIDs, batchID, merkleRoot); err != nil {
		return fmt.Errorf("failed to update logs: %w", err)
	}

	// Store batch in Fabric blockchain
	if bp.fabricClient != nil {
		fabricCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		channel := bp.fabricClient.Config.ChannelForTenant("default")
		inv, err := bp.fabricClient.StoreMerkleBatch(fabricCtx, channel, batchID, merkleRoot, len(logs), logIDs)
		if err != nil {
			bp.incrementFailedBatch()
			return fmt.Errorf("failed to store batch in Fabric: %w", err)
		}

		// Update sync control status
		if err := bp.collections.UpdateSyncStatusBatch(ctx, logIDs, models.SyncStatusSynced, batchID); err != nil {
			return fmt.Errorf("failed to update sync status: %w", err)
		}

		bp.notifyAnchored("default", "logs", batchID, merkleRoot, len(logs), inv.TxID)
	}

	// Update statistics
	bp.updateStats(batchID, len(logs), startTime)

	zlog.Info().
		Str("batch_id", batchID).
		Int("logs", len(logs)).
		Str("merkle_root", merkleRoot).
		Dur("took", time.Since(startTime)).
		Msg("batch created")

	return nil
}

// VerifyBatch verifies a batch's integrity
func (bp *BatchProcessor) VerifyBatch(ctx context.Context, batchID string) (*models.VerifyBatchResponse, error) {
	// Find logs in batch
	logs, err := bp.collections.FindLogsByBatchID(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to find logs: %w", err)
	}

	if len(logs) == 0 {
		return nil, fmt.Errorf("batch not found: %s", batchID)
	}

	// Get original Merkle root
	originalMerkleRoot := logs[0].MerkleRoot

	// Recalculate Merkle root
	recalculatedMerkleRoot, _ := models.CalculateMerkleRoot(logs)

	// Compare
	isValid := originalMerkleRoot == recalculatedMerkleRoot

	integrity := "VALID"
	message := "Batch integrity verified successfully"
	if !isValid {
		integrity = "CORRUPTED"
		message = "Batch integrity check failed - Merkle root mismatch"
	}

	response := &models.VerifyBatchResponse{
		BatchID:                batchID,
		IsValid:                isValid,
		NumLogs:                len(logs),
		OriginalMerkleRoot:     originalMerkleRoot,
		RecalculatedMerkleRoot: recalculatedMerkleRoot,
		Integrity:              integrity,
		Message:                message,
	}

	return response, nil
}

// GetBatch retrieves a batch with its logs
func (bp *BatchProcessor) GetBatch(ctx context.Context, batchID string) (*models.GetBatchResponse, error) {
	// Find logs in batch
	logs, err := bp.collections.FindLogsByBatchID(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to find logs: %w", err)
	}

	if len(logs) == 0 {
		return nil, fmt.Errorf("batch not found: %s", batchID)
	}

	// Build batch info
	logIDs := make([]string, len(logs))
	for i, log := range logs {
		logIDs[i] = log.ID
	}

	batch := models.NewMerkleBatch(batchID, logs[0].MerkleRoot, len(logs), logIDs)

	response := &models.GetBatchResponse{
		Batch:   batch,
		Logs:    logs,
		NumLogs: len(logs),
	}

	return response, nil
}

// ProcessRecordBatch batches up to batchSize pending records in a domain,
// anchors the Merkle root to Fabric, and stamps the records. Returns a nil
// result with no error when there is nothing to batch.
func (bp *BatchProcessor) ProcessRecordBatch(ctx context.Context, tenant, domain string, batchSize int) (*models.RecordBatchResult, error) {
	startTime := time.Now()
	if batchSize <= 0 {
		batchSize = bp.config.AutoBatchSize
	}

	records, err := bp.collections.FindRecordsWithoutBatch(ctx, tenant, domain, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to find records: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	batchID := fmt.Sprintf("%s-%s-%s", tenant, domain, uuid.New().String()[:8])
	merkleRoot, _ := models.CalculateRecordMerkleRoot(records)

	recordIDs := make([]string, len(records))
	for i, r := range records {
		recordIDs[i] = r.ID
	}

	if err := bp.collections.UpdateRecordBatch(ctx, tenant, domain, recordIDs, batchID, merkleRoot); err != nil {
		return nil, fmt.Errorf("failed to stamp records: %w", err)
	}

	result := &models.RecordBatchResult{
		BatchID:    batchID,
		Tenant:     tenant,
		Domain:     domain,
		MerkleRoot: merkleRoot,
		NumRecords: len(records),
	}

	if bp.fabricClient != nil {
		fabricCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		channel := bp.fabricClient.Config.ChannelForTenant(tenant)
		inv, err := bp.fabricClient.StoreMerkleBatch(fabricCtx, channel, batchID, merkleRoot, len(records), recordIDs)
		if err != nil {
			bp.incrementFailedBatch()
			return nil, fmt.Errorf("failed to anchor record batch in Fabric: %w", err)
		}
		result.TxID = inv.TxID
		result.Anchored = true
		result.Channel = channel
		bp.notifyAnchored(tenant, domain, batchID, merkleRoot, len(records), inv.TxID)
	}

	bp.updateStats(batchID, len(records), startTime)
	zlog.Info().
		Str("batch_id", batchID).
		Str("tenant", tenant).
		Str("domain", domain).
		Int("records", len(records)).
		Str("merkle_root", merkleRoot).
		Bool("anchored", result.Anchored).
		Dur("took", time.Since(startTime)).
		Msg("record batch created")

	return result, nil
}

// VerifyRecordBatch recomputes a record batch's Merkle root from current content
// and compares it with the root anchored at creation.
func (bp *BatchProcessor) VerifyRecordBatch(ctx context.Context, tenant, domain, batchID string) (*models.VerifyBatchResponse, error) {
	records, err := bp.collections.FindRecordsByBatchID(ctx, tenant, domain, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to find records: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("batch not found: %s/%s/%s", tenant, domain, batchID)
	}

	original := records[0].MerkleRoot
	recalculated, _ := models.CalculateRecordMerkleRoot(records)
	isValid := original == recalculated

	integrity := "VALID"
	message := "Batch integrity verified successfully"
	if !isValid {
		integrity = "CORRUPTED"
		message = "Batch integrity check failed - Merkle root mismatch"
	}

	return &models.VerifyBatchResponse{
		BatchID:                batchID,
		IsValid:                isValid,
		NumLogs:                len(records),
		OriginalMerkleRoot:     original,
		RecalculatedMerkleRoot: recalculated,
		Integrity:              integrity,
		Message:                message,
	}, nil
}

// ListBatches lists all batches
func (bp *BatchProcessor) ListBatches(ctx context.Context) (*models.ListBatchesResponse, error) {
	batches, err := bp.collections.AggregateBatches(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate batches: %w", err)
	}

	response := &models.ListBatchesResponse{
		Batches:      batches,
		TotalBatches: len(batches),
	}

	return response, nil
}

// GetStats returns processor statistics
func (bp *BatchProcessor) GetStats() *ProcessorStats {
	bp.statsMu.RLock()
	defer bp.statsMu.RUnlock()

	// Create a copy to avoid race conditions
	return &ProcessorStats{
		TotalBatches:     bp.stats.TotalBatches,
		TotalLogs:        bp.stats.TotalLogs,
		FailedBatches:    bp.stats.FailedBatches,
		LastBatchTime:    bp.stats.LastBatchTime,
		LastBatchSize:    bp.stats.LastBatchSize,
		LastBatchID:      bp.stats.LastBatchID,
		ProcessingErrors: bp.stats.ProcessingErrors,
	}
}

// updateStats updates processor statistics
func (bp *BatchProcessor) updateStats(batchID string, numLogs int, startTime time.Time) {
	bp.statsMu.Lock()
	defer bp.statsMu.Unlock()

	bp.stats.TotalBatches++
	bp.stats.TotalLogs += numLogs
	bp.stats.LastBatchTime = startTime
	bp.stats.LastBatchSize = numLogs
	bp.stats.LastBatchID = batchID
}

// incrementFailedBatch increments failed batch counter
func (bp *BatchProcessor) incrementFailedBatch() {
	bp.statsMu.Lock()
	defer bp.statsMu.Unlock()
	bp.stats.FailedBatches++
}

// incrementError increments error counter
func (bp *BatchProcessor) incrementError() {
	bp.statsMu.Lock()
	defer bp.statsMu.Unlock()
	bp.stats.ProcessingErrors++
}

// IsRunning returns whether the processor is running
func (bp *BatchProcessor) IsRunning() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.running
}
