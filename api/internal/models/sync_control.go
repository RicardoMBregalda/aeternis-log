package models

import (
	"time"
)

// SyncStatus represents the synchronization status with Fabric
type SyncStatus string

const (
	SyncStatusPending      SyncStatus = "pending"
	SyncStatusPendingBatch SyncStatus = "pending_batch"
	SyncStatusSynced       SyncStatus = "synced"
	SyncStatusFailed       SyncStatus = "failed"
)

// SyncControl tracks the synchronization status of logs with Fabric blockchain
type SyncControl struct {
	LogID      string     `json:"log_id" bson:"log_id" binding:"required"`
	SyncStatus SyncStatus `json:"sync_status" bson:"sync_status"`
	BatchID    string     `json:"batch_id,omitempty" bson:"batch_id,omitempty"`
	FabricTxID string     `json:"fabric_tx_id,omitempty" bson:"fabric_tx_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
	SyncedAt   *time.Time `json:"synced_at,omitempty" bson:"synced_at,omitempty"`
	FailedAt   *time.Time `json:"failed_at,omitempty" bson:"failed_at,omitempty"`
	Error      string     `json:"error,omitempty" bson:"error,omitempty"`
}

// NewSyncControl creates a new SyncControl with default values
func NewSyncControl(logID string, status SyncStatus) *SyncControl {
	return &SyncControl{
		LogID:      logID,
		SyncStatus: status,
		CreatedAt:  time.Now().UTC(),
	}
}

// MarkAsSynced updates the sync control to synced status
func (sc *SyncControl) MarkAsSynced(fabricTxID, batchID string) {
	now := time.Now().UTC()
	sc.SyncStatus = SyncStatusSynced
	sc.FabricTxID = fabricTxID
	sc.BatchID = batchID
	sc.SyncedAt = &now
	sc.Error = ""
}

// MarkAsFailed updates the sync control to failed status
func (sc *SyncControl) MarkAsFailed(err error) {
	now := time.Now().UTC()
	sc.SyncStatus = SyncStatusFailed
	sc.FailedAt = &now
	if err != nil {
		sc.Error = err.Error()
	}
}

// MarkAsPendingBatch updates the sync control to pending_batch status
func (sc *SyncControl) MarkAsPendingBatch() {
	sc.SyncStatus = SyncStatusPendingBatch
}

// IsValid checks if the sync status is valid
func (s SyncStatus) IsValid() bool {
	switch s {
	case SyncStatusPending, SyncStatusPendingBatch, SyncStatusSynced, SyncStatusFailed:
		return true
	default:
		return false
	}
}

// String returns the string representation of SyncStatus
func (s SyncStatus) String() string {
	return string(s)
}

// SyncStats represents aggregated sync statistics
type SyncStats struct {
	Pending      int `json:"pending" bson:"pending"`
	PendingBatch int `json:"pending_batch" bson:"pending_batch"`
	Synced       int `json:"synced" bson:"synced"`
	Failed       int `json:"failed" bson:"failed"`
	Total        int `json:"total" bson:"total"`
}

// CalculateTotal calculates the total count
func (ss *SyncStats) CalculateTotal() {
	ss.Total = ss.Pending + ss.PendingBatch + ss.Synced + ss.Failed
}

// BatchInfo represents batch aggregation info (used for database queries)
type BatchInfo struct {
	BatchID    string `json:"batch_id" bson:"_id"`
	MerkleRoot string `json:"merkle_root" bson:"merkle_root"`
	NumLogs    int    `json:"num_logs" bson:"num_logs"`
	BatchedAt  string `json:"batched_at" bson:"batched_at"`
}
