package main

import (
	"context"
	"errors"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestStorageStartupHonorsCancellationBeforeOpeningClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, autoMigrate := range []bool{false, true} {
		store, err := openDatabases(ctx, config.Config{Database: config.DatabaseConfig{AutoMigrate: autoMigrate}})
		if store != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled startup (auto migration=%t) = %v, %v", autoMigrate, store, err)
		}
	}
}
