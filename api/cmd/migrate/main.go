// Command migrate applies pending database schema migrations and exits. It is
// the deployment's schema-change step: run it as a Kubernetes/Helm pre-upgrade
// Job (or before the API container in development). The API itself never
// mutates the schema — it only asserts the version at boot (see cmd/api).
//
// Running it is idempotent: with nothing pending it applies nothing and exits 0.
package main

import (
	"context"
	"os"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/database"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/migrations"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	zlog.Logger = zlog.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to load configuration")
	}

	client, err := database.ConnectWithRetry(&cfg.MongoDB, 5)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to connect to MongoDB")
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	before, err := migrations.CurrentVersion(ctx, client.Database)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to read current schema version")
	}

	applied, err := migrations.NewRunner(client.Database, &cfg.MongoDB).Apply(ctx)
	if err != nil {
		zlog.Fatal().Err(err).Msg("migration failed")
	}

	after, err := migrations.CurrentVersion(ctx, client.Database)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to read schema version after migrating")
	}

	zlog.Info().
		Int("from_version", before).
		Int("to_version", after).
		Int("target_version", migrations.TargetVersion()).
		Int("applied", applied).
		Msg("migrations complete")
}
