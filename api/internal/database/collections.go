package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrDuplicateRecord is returned by InsertRecord when a record with the same
// (tenant, domain, id) already exists — including a soft-deleted one, since IDs in
// an audit trail are immutable and must not be resurrected. Callers map it to 409.
var ErrDuplicateRecord = errors.New("record already exists")

// NewFindOptions creates a new FindOptions instance
func NewFindOptions() *options.FindOptions {
	return options.Find()
}

// Collections provides easy access to MongoDB collections with type-safe methods
type Collections struct {
	Logs        *mongo.Collection
	Records     *mongo.Collection
	SyncControl *mongo.Collection
}

// NewCollections creates a new Collections instance
func NewCollections(client *MongoClient) *Collections {
	return &Collections{
		Logs:        client.GetLogsCollection(),
		Records:     client.GetRecordsCollection(),
		SyncControl: client.GetSyncControlCollection(),
	}
}

// ========================================
// LOGS COLLECTION OPERATIONS
// ========================================

// InsertLog inserts a new log into the database
func (c *Collections) InsertLog(ctx context.Context, log *models.Log) error {
	_, err := c.Logs.InsertOne(ctx, log)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}
	return nil
}

// FindLogs finds logs with optional filters and pagination
func (c *Collections) FindLogs(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]*models.Log, error) {
	cursor, err := c.Logs.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find logs: %w", err)
	}
	defer cursor.Close(ctx)

	var logs []*models.Log
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, fmt.Errorf("failed to decode logs: %w", err)
	}

	return logs, nil
}

// FindLogByID finds a single log by ID
func (c *Collections) FindLogByID(ctx context.Context, logID string) (*models.Log, error) {
	filter := bson.M{"id": logID}
	
	var log models.Log
	err := c.Logs.FindOne(ctx, filter).Decode(&log)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("log not found: %s", logID)
		}
		return nil, fmt.Errorf("failed to find log: %w", err)
	}

	return &log, nil
}

// SoftDeleteLog marks a log as deleted by setting deleted_at, without removing
// the document. This preserves the audit trail and the on-chain Merkle anchor:
// soft-deleted logs are hidden from the user-facing list/get endpoints but stay
// available for batch verification. Returns mongo.ErrNoDocuments when no
// matching, not-yet-deleted log exists.
func (c *Collections) SoftDeleteLog(ctx context.Context, logID string) error {
	filter := bson.M{"id": logID, "deleted_at": bson.M{"$exists": false}}
	update := bson.M{"$currentDate": bson.M{"deleted_at": true}}

	result, err := c.Logs.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to soft delete log: %w", err)
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// UpdateLogBatch updates logs with batch information
func (c *Collections) UpdateLogBatch(ctx context.Context, logIDs []string, batchID, merkleRoot string) error {
	filter := bson.M{"id": bson.M{"$in": logIDs}}
	update := bson.M{
		"$set": bson.M{
			"batch_id":    batchID,
			"merkle_root": merkleRoot,
			// RFC3339 string: decodes into both Log.BatchedAt (FlexTime) and the
			// BatchInfo.BatchedAt string used by the batches aggregation.
			"batched_at": time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := c.Logs.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update logs with batch: %w", err)
	}

	if result.ModifiedCount != int64(len(logIDs)) {
		return fmt.Errorf("expected to update %d logs, but updated %d", len(logIDs), result.ModifiedCount)
	}

	return nil
}

// FindLogsByBatchID finds all logs in a specific batch.
//
// The sort must be deterministic (created_at + id tiebreaker): the Merkle root
// depends on log ordering, and many logs can share the same created_at. Without
// the id tiebreaker, this query and FindLogsWithoutBatch can order ties
// differently, producing a different Merkle root on verification and a false
// "CORRUPTED" result.
func (c *Collections) FindLogsByBatchID(ctx context.Context, batchID string) ([]*models.Log, error) {
	filter := bson.M{"batch_id": batchID}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "id", Value: 1}})

	return c.FindLogs(ctx, filter, opts)
}

// CountLogs returns the total number of logs
func (c *Collections) CountLogs(ctx context.Context, filter bson.M) (int64, error) {
	count, err := c.Logs.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count logs: %w", err)
	}
	return count, nil
}

// FindLogsWithoutBatch finds logs that haven't been batched yet
func (c *Collections) FindLogsWithoutBatch(ctx context.Context, limit int) ([]*models.Log, error) {
	filter := bson.M{"batch_id": bson.M{"$exists": false}}
	// created_at + id: deterministic ordering so the Merkle root computed here at
	// batch creation matches the one recomputed by FindLogsByBatchID on verify.
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "id", Value: 1}}).
		SetLimit(int64(limit))

	return c.FindLogs(ctx, filter, opts)
}

// AggregateBatches aggregates batch information
func (c *Collections) AggregateBatches(ctx context.Context) ([]*models.BatchInfo, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "batch_id", Value: bson.D{{Key: "$exists", Value: true}}}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$batch_id"},
			{Key: "merkle_root", Value: bson.D{{Key: "$first", Value: "$merkle_root"}}},
			{Key: "num_logs", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "batched_at", Value: bson.D{{Key: "$first", Value: "$batched_at"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "batched_at", Value: -1}}}},
	}

	cursor, err := c.Logs.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate batches: %w", err)
	}
	defer cursor.Close(ctx)

	var batches []*models.BatchInfo
	if err := cursor.All(ctx, &batches); err != nil {
		return nil, fmt.Errorf("failed to decode batches: %w", err)
	}

	return batches, nil
}

// ========================================
// RECORDS COLLECTION OPERATIONS (generic, domain-scoped)
// ========================================

// InsertRecord inserts a new generic record.
func (c *Collections) InsertRecord(ctx context.Context, record *models.Record) error {
	if _, err := c.Records.InsertOne(ctx, record); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateRecord
		}
		return fmt.Errorf("failed to insert record: %w", err)
	}
	return nil
}

// FindRecords finds records with the given filter and options.
func (c *Collections) FindRecords(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]*models.Record, error) {
	cursor, err := c.Records.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find records: %w", err)
	}
	defer cursor.Close(ctx)

	var records []*models.Record
	if err := cursor.All(ctx, &records); err != nil {
		return nil, fmt.Errorf("failed to decode records: %w", err)
	}
	return records, nil
}

// FindRecordByID finds a single record by tenant, domain and id.
func (c *Collections) FindRecordByID(ctx context.Context, tenant, domain, id string) (*models.Record, error) {
	filter := bson.M{"tenant": tenant, "domain": domain, "id": id}

	var record models.Record
	err := c.Records.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("record not found: %s/%s/%s", tenant, domain, id)
		}
		return nil, fmt.Errorf("failed to find record: %w", err)
	}
	return &record, nil
}

// CountRecords returns the number of records matching the filter.
func (c *Collections) CountRecords(ctx context.Context, filter bson.M) (int64, error) {
	count, err := c.Records.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count records: %w", err)
	}
	return count, nil
}

// SoftDeleteRecord marks a record as deleted (sets deleted_at) without removing
// it, preserving the audit trail. Returns mongo.ErrNoDocuments when no matching,
// not-yet-deleted record exists.
func (c *Collections) SoftDeleteRecord(ctx context.Context, tenant, domain, id string) error {
	filter := bson.M{"tenant": tenant, "domain": domain, "id": id, "deleted_at": bson.M{"$exists": false}}
	update := bson.M{"$currentDate": bson.M{"deleted_at": true}}

	result, err := c.Records.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to soft delete record: %w", err)
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// FindRecordsWithoutBatch finds not-yet-batched, non-deleted records in a domain,
// in deterministic (created_at, id) order for a stable Merkle root.
func (c *Collections) FindRecordsWithoutBatch(ctx context.Context, tenant, domain string, limit int) ([]*models.Record, error) {
	filter := bson.M{
		"tenant":     tenant,
		"domain":     domain,
		"batch_id":   bson.M{"$exists": false},
		"deleted_at": bson.M{"$exists": false},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "id", Value: 1}}).
		SetLimit(int64(limit))

	return c.FindRecords(ctx, filter, opts)
}

// FindRecordsByBatchID returns the records of a batch in the same deterministic
// order used at batch creation (so the Merkle root recomputes identically).
func (c *Collections) FindRecordsByBatchID(ctx context.Context, tenant, domain, batchID string) ([]*models.Record, error) {
	filter := bson.M{"tenant": tenant, "domain": domain, "batch_id": batchID}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "id", Value: 1}})

	return c.FindRecords(ctx, filter, opts)
}

// UpdateRecordBatch stamps a set of records with their batch id and Merkle root.
func (c *Collections) UpdateRecordBatch(ctx context.Context, tenant, domain string, recordIDs []string, batchID, merkleRoot string) error {
	filter := bson.M{"tenant": tenant, "domain": domain, "id": bson.M{"$in": recordIDs}}
	update := bson.M{
		"$set": bson.M{
			"batch_id":    batchID,
			"merkle_root": merkleRoot,
			"batched_at":  time.Now().UTC().Format(time.RFC3339),
		},
	}

	if _, err := c.Records.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("failed to update records with batch: %w", err)
	}
	return nil
}

// SetRecordBatchTxID records the Fabric transaction id on every record of a batch,
// after the batch has been anchored (the tx id is unknown at stamp time).
func (c *Collections) SetRecordBatchTxID(ctx context.Context, tenant, domain, batchID, txID string) error {
	filter := bson.M{"tenant": tenant, "domain": domain, "batch_id": batchID}
	if _, err := c.Records.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"tx_id": txID}}); err != nil {
		return fmt.Errorf("failed to set batch tx id: %w", err)
	}
	return nil
}

// AggregateRecordBatches summarizes the anchored batches of a (tenant, domain) whose
// batched_at falls in [from, to] (RFC3339 strings; empty bounds are open), newest first.
func (c *Collections) AggregateRecordBatches(ctx context.Context, tenant, domain, from, to string) ([]models.AuditBatch, error) {
	match := bson.D{
		{Key: "tenant", Value: tenant},
		{Key: "domain", Value: domain},
		{Key: "batch_id", Value: bson.D{{Key: "$exists", Value: true}}},
	}
	if from != "" || to != "" {
		rng := bson.D{}
		if from != "" {
			rng = append(rng, bson.E{Key: "$gte", Value: from})
		}
		if to != "" {
			rng = append(rng, bson.E{Key: "$lte", Value: to})
		}
		match = append(match, bson.E{Key: "batched_at", Value: rng})
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$batch_id"},
			{Key: "merkle_root", Value: bson.D{{Key: "$first", Value: "$merkle_root"}}},
			{Key: "tx_id", Value: bson.D{{Key: "$first", Value: "$tx_id"}}},
			{Key: "batched_at", Value: bson.D{{Key: "$first", Value: "$batched_at"}}},
			{Key: "num_records", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "batched_at", Value: -1}}}},
	}

	cursor, err := c.Records.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate record batches: %w", err)
	}
	defer cursor.Close(ctx)

	batches := []models.AuditBatch{}
	if err := cursor.All(ctx, &batches); err != nil {
		return nil, fmt.Errorf("failed to decode record batches: %w", err)
	}
	return batches, nil
}

// RecordScope identifies a (tenant, domain) partition of records.
type RecordScope struct {
	Tenant string
	Domain string
}

// DistinctPendingRecordScopes returns the (tenant, domain) partitions that have
// records awaiting batching, so auto-batching can process each independently.
func (c *Collections) DistinctPendingRecordScopes(ctx context.Context) ([]RecordScope, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "batch_id", Value: bson.D{{Key: "$exists", Value: false}}},
			{Key: "deleted_at", Value: bson.D{{Key: "$exists", Value: false}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "tenant", Value: "$tenant"}, {Key: "domain", Value: "$domain"}}},
		}}},
	}

	cursor, err := c.Records.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending record scopes: %w", err)
	}
	defer cursor.Close(ctx)

	var rows []struct {
		ID struct {
			Tenant string `bson:"tenant"`
			Domain string `bson:"domain"`
		} `bson:"_id"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("failed to decode pending record scopes: %w", err)
	}

	scopes := make([]RecordScope, 0, len(rows))
	for _, r := range rows {
		if r.ID.Domain != "" {
			scopes = append(scopes, RecordScope{Tenant: r.ID.Tenant, Domain: r.ID.Domain})
		}
	}
	return scopes, nil
}

// ========================================
// SYNC CONTROL COLLECTION OPERATIONS
// ========================================

// InsertSyncControl inserts a new sync control record
func (c *Collections) InsertSyncControl(ctx context.Context, syncControl *models.SyncControl) error {
	_, err := c.SyncControl.InsertOne(ctx, syncControl)
	if err != nil {
		return fmt.Errorf("failed to insert sync control: %w", err)
	}
	return nil
}

// UpsertSyncControl inserts or updates sync control record
func (c *Collections) UpsertSyncControl(ctx context.Context, syncControl *models.SyncControl) error {
	filter := bson.M{"log_id": syncControl.LogID}
	update := bson.M{"$set": syncControl}
	opts := options.Update().SetUpsert(true)

	_, err := c.SyncControl.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert sync control: %w", err)
	}
	return nil
}

// UpdateSyncStatus updates the sync status of a log
func (c *Collections) UpdateSyncStatus(ctx context.Context, logID string, status models.SyncStatus) error {
	filter := bson.M{"log_id": logID}
	update := bson.M{
		"$set": bson.M{
			"sync_status": status,
		},
	}

	_, err := c.SyncControl.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update sync status: %w", err)
	}
	return nil
}

// UpdateSyncStatusBatch updates sync status for multiple logs
func (c *Collections) UpdateSyncStatusBatch(ctx context.Context, logIDs []string, status models.SyncStatus, batchID string) error {
	filter := bson.M{"log_id": bson.M{"$in": logIDs}}
	update := bson.M{
		"$set": bson.M{
			"sync_status": status,
			"batch_id":    batchID,
		},
		// $currentDate must be a top-level update operator; inside $set it would
		// be stored as the literal object {$currentDate: true}.
		"$currentDate": bson.M{"synced_at": true},
	}

	_, err := c.SyncControl.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update sync status batch: %w", err)
	}
	return nil
}

// FindSyncControlByLogID finds sync control by log ID
func (c *Collections) FindSyncControlByLogID(ctx context.Context, logID string) (*models.SyncControl, error) {
	filter := bson.M{"log_id": logID}
	
	var syncControl models.SyncControl
	err := c.SyncControl.FindOne(ctx, filter).Decode(&syncControl)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found, return nil without error
		}
		return nil, fmt.Errorf("failed to find sync control: %w", err)
	}

	return &syncControl, nil
}

// FindPendingBatchLogs finds logs pending for batching
func (c *Collections) FindPendingBatchLogs(ctx context.Context, limit int) ([]*models.SyncControl, error) {
	filter := bson.M{"sync_status": models.SyncStatusPendingBatch}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := c.SyncControl.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find pending batch logs: %w", err)
	}
	defer cursor.Close(ctx)

	var syncControls []*models.SyncControl
	if err := cursor.All(ctx, &syncControls); err != nil {
		return nil, fmt.Errorf("failed to decode sync controls: %w", err)
	}

	return syncControls, nil
}

// CountSyncByStatus counts sync records by status
func (c *Collections) CountSyncByStatus(ctx context.Context, status models.SyncStatus) (int64, error) {
	filter := bson.M{"sync_status": status}
	count, err := c.SyncControl.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count sync by status: %w", err)
	}
	return count, nil
}

// AggregateSyncStats aggregates sync statistics by status
func (c *Collections) AggregateSyncStats(ctx context.Context) (*models.SyncStats, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$sync_status"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}

	cursor, err := c.SyncControl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate sync stats: %w", err)
	}
	defer cursor.Close(ctx)

	stats := &models.SyncStats{}
	
	for cursor.Next(ctx) {
		var result struct {
			ID    models.SyncStatus `bson:"_id"`
			Count int               `bson:"count"`
		}
		
		if err := cursor.Decode(&result); err != nil {
			continue
		}

		switch result.ID {
		case models.SyncStatusPending:
			stats.Pending = result.Count
		case models.SyncStatusPendingBatch:
			stats.PendingBatch = result.Count
		case models.SyncStatusSynced:
			stats.Synced = result.Count
		case models.SyncStatusFailed:
			stats.Failed = result.Count
		}
	}

	stats.CalculateTotal()
	return stats, nil
}
