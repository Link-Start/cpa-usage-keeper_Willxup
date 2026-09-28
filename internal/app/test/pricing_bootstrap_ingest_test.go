package test

import (
	"context"
	"path/filepath"
	"testing"

	"cpa-usage-keeper/internal/app"
	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/repository"
)

func TestPricingBootstrapIngestConstructionDoesNotInitializeBusinessWorkers(t *testing.T) {
	cfg := config.Config{SQLitePath: filepath.Join(t.TempDir(), "bootstrap.db")}
	db, reader, err := repository.OpenUnmigratedDatabasePools(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if reader != db {
			if sqlDB, err := reader.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if _, err := repository.BootstrapPricingInitialization(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsurePricingBootstrapInbox(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if runner := app.NewPricingBootstrapIngestRunner(cfg, db); runner == nil {
		t.Fatal("missing isolated ingest runner")
	}
	for _, table := range []string{"usage_events", "model_price_settings", "usage_aggregation_checkpoints"} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("bootstrap ingest unexpectedly initialized business table %s", table)
		}
	}
}
