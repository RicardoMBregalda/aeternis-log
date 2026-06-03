package database

import (
	"context"
	"fmt"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// NewFindOptions creates a new FindOptions instance
func NewFindOptions() *options.FindOptions {
	return options.Find()
}

// Collections provides easy access to MongoDB collections with type-safe methods
type Collections struct {
	Logs        *mongo.Collection
	SyncControl *mongo.Collection
}

// NewCollections creates a new Collections instance
func NewCollections(client *MongoClient) *Collections {
	return &Collections{
		Logs:        client.GetLogsCollection(),
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

// UpdateLogBatch updates logs with batch information
func (c *Collections) UpdateLogBatch(ctx context.Context, logIDs []string, batchID, merkleRoot string) error {
	filter := bson.M{"id": bson.M{"$in": logIDs}}
	update := bson.M{
		"$set": bson.M{
			"batch_id":    batchID,
			"merkle_root": merkleRoot,
			"batched_at":  bson.M{"$currentDate": true},
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

// FindLogsByBatchID finds all logs in a specific batch
func (c *Collections) FindLogsByBatchID(ctx context.Context, batchID string) ([]*models.Log, error) {
	filter := bson.M{"batch_id": batchID}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

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
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}}).
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
			"synced_at":   bson.M{"$currentDate": true},
		},
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
