package database

import (
	"context"
	"fmt"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	zlog "github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/bson"
)

// MongoClient wraps the MongoDB client with configuration
type MongoClient struct {
	Client     *mongo.Client
	Database   *mongo.Database
	Config     *config.MongoDBConfig
}

// NewMongoClient creates a new MongoDB client with connection pooling
func NewMongoClient(cfg *config.MongoDBConfig) (*MongoClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	// Configure client options with connection pooling
	clientOpts := options.Client().
		ApplyURI(cfg.URL).
		SetMinPoolSize(uint64(cfg.MinPoolSize)).
		SetMaxPoolSize(uint64(cfg.MaxPoolSize)).
		SetMaxConnIdleTime(time.Duration(cfg.MaxIdleTimeMS) * time.Millisecond).
		SetServerSelectionTimeout(time.Duration(cfg.ServerSelectionTimeoutMS) * time.Millisecond).
		SetSocketTimeout(cfg.SocketTimeout).
		SetConnectTimeout(cfg.ConnectTimeout)

	// Create client
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Get database
	database := client.Database(cfg.Database)

	mongoClient := &MongoClient{
		Client:   client,
		Database: database,
		Config:   cfg,
	}

	// Create indexes
	if err := mongoClient.CreateIndexes(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to create indexes: %w", err)
	}

	return mongoClient, nil
}

// CreateIndexes creates optimized compound indexes for the collections
func (mc *MongoClient) CreateIndexes(ctx context.Context) error {
	logsCollection := mc.Database.Collection(mc.Config.Collection)
	syncControlCollection := mc.Database.Collection(mc.Config.SyncControlCollection)

	// Logs collection indexes
	logsIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "timestamp", Value: -1}},
		},
		{
			Keys: bson.D{
				{Key: "source", Value: 1},
				{Key: "timestamp", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "level", Value: 1},
				{Key: "timestamp", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "source", Value: 1},
				{Key: "level", Value: 1},
				{Key: "timestamp", Value: -1},
			},
		},
		{
			// Compound index supporting created_at sorting and (created_at, id)
			// keyset/cursor pagination (the id prefix covers created_at-only use).
			Keys: bson.D{
				{Key: "created_at", Value: -1},
				{Key: "id", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "batch_id", Value: 1}},
		},
	}

	// Create logs indexes
	if _, err := logsCollection.Indexes().CreateMany(ctx, logsIndexes); err != nil {
		return fmt.Errorf("failed to create logs indexes: %w", err)
	}

	// Sync control collection indexes
	syncIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "log_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "sync_status", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: 1}},
		},
	}

	// Create sync control indexes
	if _, err := syncControlCollection.Indexes().CreateMany(ctx, syncIndexes); err != nil {
		return fmt.Errorf("failed to create sync control indexes: %w", err)
	}

	// Records collection indexes (generic domain-scoped records)
	recordsCollection := mc.Database.Collection(mc.Config.RecordsCollection)
	recordsIndexes := []mongo.IndexModel{
		{
			// Unique per (tenant, domain): the same id can exist across tenants/domains.
			Keys:    bson.D{{Key: "tenant", Value: 1}, {Key: "domain", Value: 1}, {Key: "id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// Tenant+domain-scoped listing and (created_at, id) keyset pagination.
			Keys: bson.D{
				{Key: "tenant", Value: 1},
				{Key: "domain", Value: 1},
				{Key: "created_at", Value: -1},
				{Key: "id", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "tenant", Value: 1}, {Key: "domain", Value: 1}, {Key: "batch_id", Value: 1}},
		},
	}
	if _, err := recordsCollection.Indexes().CreateMany(ctx, recordsIndexes); err != nil {
		return fmt.Errorf("failed to create records indexes: %w", err)
	}

	return nil
}

// Close closes the MongoDB connection
func (mc *MongoClient) Close(ctx context.Context) error {
	return mc.Client.Disconnect(ctx)
}

// Ping checks if the connection is alive
func (mc *MongoClient) Ping(ctx context.Context) error {
	return mc.Client.Ping(ctx, readpref.Primary())
}

// GetLogsCollection returns the logs collection
func (mc *MongoClient) GetLogsCollection() *mongo.Collection {
	return mc.Database.Collection(mc.Config.Collection)
}

// GetSyncControlCollection returns the sync control collection
func (mc *MongoClient) GetSyncControlCollection() *mongo.Collection {
	return mc.Database.Collection(mc.Config.SyncControlCollection)
}

// GetRecordsCollection returns the generic records collection
func (mc *MongoClient) GetRecordsCollection() *mongo.Collection {
	return mc.Database.Collection(mc.Config.RecordsCollection)
}

// HealthCheck performs a health check on the MongoDB connection
func (mc *MongoClient) HealthCheck(ctx context.Context) error {
	// Ping with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := mc.Ping(pingCtx); err != nil {
		return fmt.Errorf("mongodb health check failed: %w", err)
	}

	return nil
}

// GetStats returns MongoDB connection statistics
func (mc *MongoClient) GetStats(ctx context.Context) (map[string]interface{}, error) {
	var result bson.M
	
	err := mc.Database.RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&result)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"connected": true,
		"database":  mc.Config.Database,
		"pool_size": fmt.Sprintf("%d-%d", mc.Config.MinPoolSize, mc.Config.MaxPoolSize),
	}

	if connections, ok := result["connections"].(bson.M); ok {
		stats["current_connections"] = connections["current"]
		stats["available_connections"] = connections["available"]
	}

	return stats, nil
}

// ConnectWithRetry attempts to connect to MongoDB with exponential backoff retry
func ConnectWithRetry(cfg *config.MongoDBConfig, maxRetries int) (*MongoClient, error) {
	var client *MongoClient
	var err error

	for i := 0; i < maxRetries; i++ {
		client, err = NewMongoClient(cfg)
		if err == nil {
			return client, nil
		}

		// Exponential backoff: 1s, 2s, 4s, 8s, 16s
		waitTime := time.Duration(1<<uint(i)) * time.Second
		if waitTime > 30*time.Second {
			waitTime = 30 * time.Second
		}

		zlog.Warn().
			Err(err).
			Int("attempt", i+1).
			Int("max_retries", maxRetries).
			Dur("retry_in", waitTime).
			Msg("MongoDB connection failed, retrying")
		time.Sleep(waitTime)
	}

	return nil, fmt.Errorf("failed to connect to MongoDB after %d attempts: %w", maxRetries, err)
}
