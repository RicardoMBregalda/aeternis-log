package migrations

import (
	"context"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// migration0001RecordsIndexes creates the records collection indexes. These were
// previously built imperatively on every boot (database.NewMongoClient); they
// now live here so the schema has a single, versioned source of truth.
//
// CreateMany is idempotent for identical specs (Mongo no-ops when the index
// already exists), so re-running this migration is safe.
var migration0001RecordsIndexes = Migration{
	Version: 1,
	Name:    "records_indexes",
	Up: func(ctx context.Context, db *mongo.Database, cfg *config.MongoDBConfig) error {
		records := db.Collection(cfg.RecordsCollection)
		_, err := records.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{
				// Unique per (tenant, domain): the same id can exist across
				// tenants/domains, but not twice within one.
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
		})
		return err
	},
}
