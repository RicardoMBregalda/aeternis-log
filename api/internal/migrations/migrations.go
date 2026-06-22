// Package migrations holds the database schema history as an ordered, versioned
// list of idempotent migrations, plus a runner that applies them and a
// read-only version assertion the API uses at boot.
//
// The design separates "change the schema" from "start the service": the
// migrate binary (cmd/migrate) or the Helm migration Job applies pending
// migrations, while the API only *asserts* the expected version at startup and
// refuses to run on a mismatch. Boot never mutates the schema, so an upgrade
// that adds an index can never silently brick or partially migrate a running
// deployment.
package migrations

import (
	"context"

	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"go.mongodb.org/mongo-driver/mongo"
)

// Migration is a single, ordered schema change. Up must be idempotent: the
// runner already guards against re-running an applied version, but a safe Up
// keeps partial-failure recovery clean.
type Migration struct {
	Version int
	Name    string
	Up      func(ctx context.Context, db *mongo.Database, cfg *config.MongoDBConfig) error
}

// registry is the schema history in version order. Append new migrations with
// the next version; never edit, reorder, or remove a released migration.
var registry = []Migration{
	migration0001RecordsIndexes,
}

// All returns the migration registry in version order.
func All() []Migration { return registry }

// TargetVersion is the highest schema version this binary knows about.
func TargetVersion() int {
	max := 0
	for _, m := range registry {
		if m.Version > max {
			max = m.Version
		}
	}
	return max
}
