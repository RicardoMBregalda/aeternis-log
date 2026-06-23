package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/migrations"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// testDB connects to a real MongoDB (the plan requires migrations be exercised
// against the real driver/index machinery, not a fake) and hands back a clean
// database. It skips when no MongoDB is reachable so unit runs stay portable.
func testDB(t *testing.T) (*mongo.Database, *config.MongoDBConfig) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		t.Skipf("MongoDB not available: %v", err)
	}

	cfg := &config.MongoDBConfig{
		Database:          "logdb_migrations_test",
		RecordsCollection: "records_mig_test",
	}
	db := client.Database(cfg.Database)
	drop := func(c context.Context) {
		_ = db.Collection(migrations.SchemaCollection).Drop(c)
		_ = db.Collection(cfg.RecordsCollection).Drop(c)
	}
	drop(ctx)
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		drop(cctx)
		_ = client.Disconnect(cctx)
	})
	return db, cfg
}

// TestApplyThenAssert proves the boot contract: an un-migrated database reports
// version 0 and AssertVersion refuses it; after Apply the version reaches the
// binary's target and AssertVersion passes.
func TestApplyThenAssert(t *testing.T) {
	db, cfg := testDB(t)
	ctx := context.Background()
	r := migrations.NewRunner(db, cfg)

	if v, err := migrations.CurrentVersion(ctx, db); err != nil || v != 0 {
		t.Fatalf("fresh CurrentVersion = %d, %v; want 0, nil", v, err)
	}
	if err := migrations.AssertVersion(ctx, db, migrations.TargetVersion()); err == nil {
		t.Fatal("AssertVersion on un-migrated DB: want a clear error, got nil")
	}

	n, err := r.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := len(migrations.All()); n != want {
		t.Fatalf("applied %d migrations, want %d", n, want)
	}
	if v, _ := migrations.CurrentVersion(ctx, db); v != migrations.TargetVersion() {
		t.Fatalf("CurrentVersion = %d, want %d", v, migrations.TargetVersion())
	}
	if err := migrations.AssertVersion(ctx, db, migrations.TargetVersion()); err != nil {
		t.Fatalf("AssertVersion after migrate: %v", err)
	}
}

// TestApplyIsIdempotent proves a re-run (a restarted Job, a second replica)
// applies nothing and never mutates an already-current schema.
func TestApplyIsIdempotent(t *testing.T) {
	db, cfg := testDB(t)
	ctx := context.Background()
	r := migrations.NewRunner(db, cfg)

	if _, err := r.Apply(ctx); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	n, err := r.Apply(ctx)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if n != 0 {
		t.Fatalf("second Apply applied %d migrations, want 0", n)
	}
}

// TestMigration0001CreatesUniqueRecordsIndex drives the real index: after the
// migration, a duplicate (tenant, domain, id) must be rejected by Mongo.
func TestMigration0001CreatesUniqueRecordsIndex(t *testing.T) {
	db, cfg := testDB(t)
	ctx := context.Background()
	if _, err := migrations.NewRunner(db, cfg).Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	coll := db.Collection(cfg.RecordsCollection)
	doc := bson.D{{Key: "tenant", Value: "acme"}, {Key: "domain", Value: "contracts"}, {Key: "id", Value: "r-1"}}
	if _, err := coll.InsertOne(ctx, doc); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := coll.InsertOne(ctx, doc); err == nil || !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate insert: want duplicate-key error, got %v", err)
	}
}

// TestSchemaVersionIsUnique proves the runner enforces one record per version, so
// two runners racing to apply the same migration can't both record it (the loser
// fails loudly instead of silently double-applying).
func TestSchemaVersionIsUnique(t *testing.T) {
	db, cfg := testDB(t)
	ctx := context.Background()
	if _, err := migrations.NewRunner(db, cfg).Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// A second record for an applied version must be rejected by Mongo.
	dup := bson.D{{Key: "version", Value: 1}, {Key: "name", Value: "dup"}, {Key: "applied_at", Value: time.Now()}}
	if _, err := db.Collection(migrations.SchemaCollection).InsertOne(ctx, dup); err == nil || !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate version insert: want duplicate-key error, got %v", err)
	}
}
