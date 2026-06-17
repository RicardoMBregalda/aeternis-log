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
	Records *mongo.Collection
}

// NewCollections creates a new Collections instance
func NewCollections(client *MongoClient) *Collections {
	return &Collections{
		Records: client.GetRecordsCollection(),
	}
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

// ClaimRecordsForBatch atomically claims up to limit pending records of a
// (tenant, domain) into batchID (anchor_status=pending) and returns the claimed
// set in deterministic order. The claim is atomic: concurrent batchers cannot
// claim the same record, because the update guards on batch_id not existing.
func (c *Collections) ClaimRecordsForBatch(ctx context.Context, tenant, domain, batchID string, limit int) ([]*models.Record, error) {
	// 1) Pick candidate pending ids (bounded by limit), in deterministic order.
	pending, err := c.FindRecordsWithoutBatch(ctx, tenant, domain, limit)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return nil, nil
	}
	ids := make([]string, len(pending))
	for i, r := range pending {
		ids[i] = r.ID
	}

	// 2) Claim only those still unbatched. The "batch_id does not exist" guard is
	// what makes this race-safe: each document is updated atomically, so a record
	// another batcher already claimed will not match and is never double-claimed.
	filter := bson.M{
		"tenant":     tenant,
		"domain":     domain,
		"id":         bson.M{"$in": ids},
		"batch_id":   bson.M{"$exists": false},
		"deleted_at": bson.M{"$exists": false},
	}
	update := bson.M{"$set": bson.M{
		"batch_id":      batchID,
		"anchor_status": models.RecordAnchorPending,
		"batched_at":    time.Now().UTC().Format(time.RFC3339),
	}}
	if _, err := c.Records.UpdateMany(ctx, filter, update); err != nil {
		return nil, fmt.Errorf("failed to claim records: %w", err)
	}

	// 3) Return exactly the records this call won (read back by batch id).
	return c.FindRecordsByBatchID(ctx, tenant, domain, batchID)
}

// SetRecordBatchMerkleRoot stamps the Merkle root on a batch's records. Used when
// no ledger is configured, so the batch keeps its pending status.
func (c *Collections) SetRecordBatchMerkleRoot(ctx context.Context, tenant, domain, batchID, merkleRoot string) error {
	filter := bson.M{"tenant": tenant, "domain": domain, "batch_id": batchID}
	if _, err := c.Records.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"merkle_root": merkleRoot}}); err != nil {
		return fmt.Errorf("failed to set batch merkle root: %w", err)
	}
	return nil
}

// SetRecordBatchAnchored marks a batch anchored: it records the Merkle root, the
// Fabric tx id and anchor_status=anchored on every record of the batch.
func (c *Collections) SetRecordBatchAnchored(ctx context.Context, tenant, domain, batchID, merkleRoot, txID string) error {
	filter := bson.M{"tenant": tenant, "domain": domain, "batch_id": batchID}
	update := bson.M{"$set": bson.M{
		"merkle_root":   merkleRoot,
		"tx_id":         txID,
		"anchor_status": models.RecordAnchorAnchored,
	}}
	if _, err := c.Records.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("failed to mark batch anchored: %w", err)
	}
	return nil
}

// MarkRecordBatchFailed records the Merkle root and anchor_status=failed on a
// batch whose anchor attempt failed. The records keep their batch_id (so they
// never return to the pending pool) and the reconciler re-drives them.
func (c *Collections) MarkRecordBatchFailed(ctx context.Context, tenant, domain, batchID, merkleRoot string) error {
	filter := bson.M{"tenant": tenant, "domain": domain, "batch_id": batchID}
	update := bson.M{"$set": bson.M{
		"merkle_root":   merkleRoot,
		"anchor_status": models.RecordAnchorFailed,
	}}
	if _, err := c.Records.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("failed to mark batch failed: %w", err)
	}
	return nil
}

// UnanchoredBatch identifies a batch that was claimed but is not yet anchored.
type UnanchoredBatch struct {
	Tenant  string
	Domain  string
	BatchID string
}

// FindUnanchoredBatchScopes returns the batches still awaiting anchoring
// (anchor_status pending or failed), so the reconciler can re-drive them.
func (c *Collections) FindUnanchoredBatchScopes(ctx context.Context) ([]UnanchoredBatch, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "anchor_status", Value: bson.D{{Key: "$in", Value: bson.A{models.RecordAnchorPending, models.RecordAnchorFailed}}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "tenant", Value: "$tenant"},
				{Key: "domain", Value: "$domain"},
				{Key: "batch_id", Value: "$batch_id"},
			}},
		}}},
	}

	cursor, err := c.Records.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to list unanchored batches: %w", err)
	}
	defer cursor.Close(ctx)

	var rows []struct {
		ID struct {
			Tenant  string `bson:"tenant"`
			Domain  string `bson:"domain"`
			BatchID string `bson:"batch_id"`
		} `bson:"_id"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("failed to decode unanchored batches: %w", err)
	}

	out := make([]UnanchoredBatch, 0, len(rows))
	for _, r := range rows {
		if r.ID.BatchID != "" {
			out = append(out, UnanchoredBatch{Tenant: r.ID.Tenant, Domain: r.ID.Domain, BatchID: r.ID.BatchID})
		}
	}
	return out, nil
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
