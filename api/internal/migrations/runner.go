package migrations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SchemaCollection records which migrations have been applied (one document per
// version). It is the single source of truth for the database schema version.
const SchemaCollection = "schema_migrations"

type appliedRecord struct {
	Version   int       `bson:"version"`
	Name      string    `bson:"name"`
	AppliedAt time.Time `bson:"applied_at"`
}

// Runner applies the migration registry against one database.
type Runner struct {
	db   *mongo.Database
	cfg  *config.MongoDBConfig
	migs []Migration
}

// NewRunner builds a Runner for the given database and Mongo configuration.
func NewRunner(db *mongo.Database, cfg *config.MongoDBConfig) *Runner {
	return &Runner{db: db, cfg: cfg, migs: All()}
}

// CurrentVersion is the highest applied migration version, or 0 if none have run.
func CurrentVersion(ctx context.Context, db *mongo.Database) (int, error) {
	opts := options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}})
	var rec appliedRecord
	err := db.Collection(SchemaCollection).FindOne(ctx, bson.D{}, opts).Decode(&rec)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return rec.Version, nil
}

// ensureVersionIndex enforces one record per schema version, so two runners
// racing to apply the same migration cannot both record it.
func (r *Runner) ensureVersionIndex(ctx context.Context) error {
	_, err := r.db.Collection(SchemaCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("ensure %s index: %w", SchemaCollection, err)
	}
	return nil
}

// Apply runs every pending migration in version order and records each one. It
// is idempotent: a second run with nothing pending applies nothing and returns
// 0. It returns the number of migrations applied. If another runner records the
// same version concurrently, the duplicate insert fails loudly rather than
// silently double-applying.
func (r *Runner) Apply(ctx context.Context) (int, error) {
	if err := r.ensureVersionIndex(ctx); err != nil {
		return 0, err
	}

	current, err := CurrentVersion(ctx, r.db)
	if err != nil {
		return 0, err
	}

	pending := make([]Migration, 0, len(r.migs))
	for _, m := range r.migs {
		if m.Version > current {
			pending = append(pending, m)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Version < pending[j].Version })

	applied := 0
	for _, m := range pending {
		if err := m.Up(ctx, r.db, r.cfg); err != nil {
			return applied, fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
		_, err := r.db.Collection(SchemaCollection).InsertOne(ctx, appliedRecord{
			Version:   m.Version,
			Name:      m.Name,
			AppliedAt: time.Now().UTC(),
		})
		if mongo.IsDuplicateKeyError(err) {
			return applied, fmt.Errorf("migration %d (%s) was recorded concurrently by another runner; aborting", m.Version, m.Name)
		}
		if err != nil {
			return applied, fmt.Errorf("record migration %d (%s): %w", m.Version, m.Name, err)
		}
		applied++
	}
	return applied, nil
}

// AssertVersion returns an actionable error when the database is not exactly at
// the expected schema version. It never mutates — the API calls this at boot so
// an out-of-date (or ahead-of-binary) deployment fails loudly instead of
// silently migrating.
func AssertVersion(ctx context.Context, db *mongo.Database, expected int) error {
	current, err := CurrentVersion(ctx, db)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf(
			"schema version mismatch: database at v%d, binary expects v%d — run the migration job (cmd/migrate) before starting the API",
			current, expected,
		)
	}
	return nil
}
