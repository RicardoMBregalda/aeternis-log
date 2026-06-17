package merkle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/database"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/fabric"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/metrics"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/webhook"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"github.com/google/uuid"
	zlog "github.com/rs/zerolog/log"
)

// Anchorer is the ledger seam the batch processor depends on: it stores and
// reads Merkle batch roots. *fabric.FabricClient satisfies it; a fake makes the
// anchor-failure and reconciliation paths testable without a live Fabric.
type Anchorer interface {
	Enabled() bool
	ChannelForTenant(tenant string) string
	StoreMerkleBatch(ctx context.Context, channel, batchID, merkleRoot string, numRecords int, recordIDs []string) (*fabric.InvokeResponse, error)
	VerifyMerkleBatch(ctx context.Context, channel, batchID string) (*fabric.QueryResponse, error)
}

// BatchProcessor batches pending records, builds their Merkle tree and anchors
// the root on Fabric — automatically (auto-batch ticker) or on demand.
type BatchProcessor struct {
	collections *database.Collections
	anchorer    Anchorer
	config      *config.BatchingConfig
	notifier    *webhook.Notifier

	stopChan chan struct{}
	wg       sync.WaitGroup
	running  bool
	mu       sync.RWMutex

	stats   *ProcessorStats
	statsMu sync.RWMutex
}

// ProcessorStats holds processor statistics
type ProcessorStats struct {
	TotalBatches     int       `json:"total_batches"`
	TotalRecords     int       `json:"total_records"`
	FailedBatches    int       `json:"failed_batches"`
	LastBatchTime    time.Time `json:"last_batch_time"`
	LastBatchSize    int       `json:"last_batch_size"`
	LastBatchID      string    `json:"last_batch_id"`
	ProcessingErrors int       `json:"processing_errors"`
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(collections *database.Collections, anchorer Anchorer, cfg *config.BatchingConfig) *BatchProcessor {
	return &BatchProcessor{
		collections: collections,
		anchorer:    anchorer,
		config:      cfg,
		stopChan:    make(chan struct{}),
		stats:       &ProcessorStats{},
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
		return fmt.Errorf("timeout waiting for batch processor to stop")
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
			// First re-drive any batch claimed but not yet anchored (F03).
			if err := bp.ReconcileBatches(ctx); err != nil {
				zlog.Error().Err(err).Msg("batch reconcile error")
			}
			// Then auto-batch newly pending records per (tenant, domain).
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

// anchoredRoot fetches the Merkle root anchored on-chain for a batch and reports
// where it came from. It returns (root, AnchorAnchored) when the ledger holds the
// batch, ("", AnchorUnanchored) when the ledger has no such batch, and
// ("", AnchorUnknown) when Fabric is disabled or unreachable (so the caller can
// fall back to a local check and disclose that the anchor was not consulted).
func (bp *BatchProcessor) anchoredRoot(ctx context.Context, channel, batchID string) (string, string) {
	if bp.anchorer == nil || !bp.anchorer.Enabled() {
		return "", models.AnchorUnknown
	}
	resp, err := bp.anchorer.VerifyMerkleBatch(ctx, channel, batchID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return "", models.AnchorUnanchored
		}
		zlog.Warn().Err(err).Str("batch_id", batchID).Str("channel", channel).
			Msg("on-chain anchor lookup failed; verifying against local state only")
		return "", models.AnchorUnknown
	}
	root, _ := resp.Data["merkle_root"].(string)
	if root == "" {
		return "", models.AnchorUnknown
	}
	return root, models.AnchorAnchored
}

// buildVerifyResponse decides integrity from the authoritative source. When the
// batch is anchored, the on-chain root is the source of truth and the locally
// stored root is irrelevant (this is what makes tampering both the content AND
// the stored root detectable). When the ledger cannot be consulted, it falls
// back to local consistency and says so via AnchorStatus.
func buildVerifyResponse(batchID string, n int, storedRoot, recalculated, onChainRoot, anchorStatus string) *models.VerifyBatchResponse {
	resp := &models.VerifyBatchResponse{
		BatchID:                batchID,
		NumLogs:                n,
		OriginalMerkleRoot:     storedRoot,
		RecalculatedMerkleRoot: recalculated,
		OnChainMerkleRoot:      onChainRoot,
		AnchorStatus:           anchorStatus,
	}

	switch anchorStatus {
	case models.AnchorAnchored:
		resp.IsValid = recalculated == onChainRoot
		if resp.IsValid {
			resp.Integrity = models.IntegrityValid
			resp.Message = "Batch integrity verified against the on-chain anchor"
		} else {
			resp.Integrity = models.IntegrityCorrupted
			resp.Message = "Recomputed Merkle root does not match the on-chain anchor"
		}
	case models.AnchorUnanchored:
		resp.IsValid = false
		resp.Integrity = models.IntegrityUnanchored
		resp.Message = "Batch is not anchored on the ledger; integrity cannot be proven"
	default: // AnchorUnknown
		resp.IsValid = storedRoot == recalculated
		if resp.IsValid {
			resp.Integrity = models.IntegrityValid
		} else {
			resp.Integrity = models.IntegrityCorrupted
		}
		resp.Message = "Ledger not consulted (anchor status unknown); local consistency only"
	}
	return resp
}

// ProcessRecordBatch batches up to batchSize pending records in a domain,
// anchors the Merkle root to Fabric, and stamps the records. Returns a nil
// result with no error when there is nothing to batch.
func (bp *BatchProcessor) ProcessRecordBatch(ctx context.Context, tenant, domain string, batchSize int) (*models.RecordBatchResult, error) {
	startTime := time.Now()
	if batchSize <= 0 {
		batchSize = bp.config.AutoBatchSize
	}

	// Atomically claim pending records into a fresh batch id (F03). The claim is
	// race-safe, so concurrent batchers never grab the same records.
	batchID := fmt.Sprintf("%s-%s-%s", tenant, domain, uuid.New().String()[:8])
	records, err := bp.collections.ClaimRecordsForBatch(ctx, tenant, domain, batchID, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to claim records: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	merkleRoot, _ := models.CalculateRecordMerkleRoot(records)
	recordIDs := make([]string, len(records))
	for i, r := range records {
		recordIDs[i] = r.ID
	}

	result := &models.RecordBatchResult{
		BatchID:    batchID,
		Tenant:     tenant,
		Domain:     domain,
		MerkleRoot: merkleRoot,
		NumRecords: len(records),
	}

	// No ledger configured: persist the root, leave the batch pending.
	if bp.anchorer == nil || !bp.anchorer.Enabled() {
		if err := bp.collections.SetRecordBatchMerkleRoot(ctx, tenant, domain, batchID, merkleRoot); err != nil {
			return result, err
		}
		bp.updateStats(batchID, len(records), startTime)
		return result, nil
	}

	// Anchor AFTER claiming (stamp-after-anchor). On failure the records are
	// marked failed — kept, never excluded forever — and the reconciler re-drives
	// them; we never commit a "batched but unanchored" state we can't recover.
	fabricCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	channel := bp.anchorer.ChannelForTenant(tenant)
	result.Channel = channel

	inv, err := bp.anchorer.StoreMerkleBatch(fabricCtx, channel, batchID, merkleRoot, len(records), recordIDs)
	if err != nil {
		bp.incrementFailedBatch()
		if mErr := bp.collections.MarkRecordBatchFailed(ctx, tenant, domain, batchID, merkleRoot); mErr != nil {
			zlog.Error().Err(mErr).Str("batch_id", batchID).Msg("failed to mark batch failed")
		}
		return result, fmt.Errorf("failed to anchor record batch in Fabric: %w", err)
	}

	if err := bp.collections.SetRecordBatchAnchored(ctx, tenant, domain, batchID, merkleRoot, inv.TxID); err != nil {
		zlog.Warn().Err(err).Str("batch_id", batchID).Msg("anchored but failed to persist status/tx id")
	}
	result.TxID = inv.TxID
	result.Anchored = true
	bp.notifyAnchored(tenant, domain, batchID, merkleRoot, len(records), inv.TxID)
	metrics.RecordAnchoredBatch(tenant, domain, len(records))

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

	storedRoot := records[0].MerkleRoot
	recalculated, _ := models.CalculateRecordMerkleRoot(records)

	channel := ""
	if bp.anchorer != nil {
		channel = bp.anchorer.ChannelForTenant(tenant)
	}
	onChainRoot, anchorStatus := bp.anchoredRoot(ctx, channel, batchID)

	resp := buildVerifyResponse(batchID, len(records), storedRoot, recalculated, onChainRoot, anchorStatus)
	metrics.RecordVerification(domain, resp.IsValid)
	return resp, nil
}

// ReconcileBatches re-drives batches that were claimed but not anchored (status
// pending or failed) by re-submitting each to the ledger. Thanks to the
// write-once anchor (F02), a batch that is in fact already on-chain returns an
// "already anchored" error, which the reconciler treats as success and stops
// retrying. Genuine failures are left for the next cycle.
func (bp *BatchProcessor) ReconcileBatches(ctx context.Context) error {
	if bp.anchorer == nil || !bp.anchorer.Enabled() {
		return nil
	}
	scopes, err := bp.collections.FindUnanchoredBatchScopes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list unanchored batches: %w", err)
	}
	for _, s := range scopes {
		records, err := bp.collections.FindRecordsByBatchID(ctx, s.Tenant, s.Domain, s.BatchID)
		if err != nil || len(records) == 0 {
			continue
		}
		// Re-submit the ORIGINAL stored root (not a recompute), so the on-chain
		// root matches what this batch committed at creation.
		root := records[0].MerkleRoot
		ids := make([]string, len(records))
		for i, r := range records {
			ids[i] = r.ID
		}
		channel := bp.anchorer.ChannelForTenant(s.Tenant)

		inv, err := bp.anchorer.StoreMerkleBatch(ctx, channel, s.BatchID, root, len(records), ids)
		if err != nil {
			if isAlreadyAnchored(err) {
				if sErr := bp.collections.SetRecordBatchAnchored(ctx, s.Tenant, s.Domain, s.BatchID, root, records[0].TxID); sErr != nil {
					zlog.Error().Err(sErr).Str("batch_id", s.BatchID).Msg("reconcile: failed to sync already-anchored status")
				}
				continue
			}
			zlog.Warn().Err(err).Str("batch_id", s.BatchID).Msg("reconcile: re-anchor failed, will retry")
			continue
		}
		if err := bp.collections.SetRecordBatchAnchored(ctx, s.Tenant, s.Domain, s.BatchID, root, inv.TxID); err != nil {
			zlog.Error().Err(err).Str("batch_id", s.BatchID).Msg("reconcile: failed to persist anchored status")
			continue
		}
		bp.notifyAnchored(s.Tenant, s.Domain, s.BatchID, root, len(records), inv.TxID)
		metrics.RecordAnchoredBatch(s.Tenant, s.Domain, len(records))
		zlog.Info().Str("batch_id", s.BatchID).Str("tx_id", inv.TxID).Msg("reconcile: batch anchored")
	}
	return nil
}

// isAlreadyAnchored reports whether an anchor error is the write-once guard (F02),
// meaning the batch is already committed on-chain.
func isAlreadyAnchored(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "already anchored")
}

// GetStats returns processor statistics
func (bp *BatchProcessor) GetStats() *ProcessorStats {
	bp.statsMu.RLock()
	defer bp.statsMu.RUnlock()

	// Create a copy to avoid race conditions
	return &ProcessorStats{
		TotalBatches:     bp.stats.TotalBatches,
		TotalRecords:     bp.stats.TotalRecords,
		FailedBatches:    bp.stats.FailedBatches,
		LastBatchTime:    bp.stats.LastBatchTime,
		LastBatchSize:    bp.stats.LastBatchSize,
		LastBatchID:      bp.stats.LastBatchID,
		ProcessingErrors: bp.stats.ProcessingErrors,
	}
}

// updateStats updates processor statistics
func (bp *BatchProcessor) updateStats(batchID string, numRecords int, startTime time.Time) {
	bp.statsMu.Lock()
	defer bp.statsMu.Unlock()

	bp.stats.TotalBatches++
	bp.stats.TotalRecords += numRecords
	bp.stats.LastBatchTime = startTime
	bp.stats.LastBatchSize = numRecords
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
