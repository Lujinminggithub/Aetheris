package main

import (
	"context"
	"log"

	"github.com/aetheris-dev/aetheris/server/internal/config"
	"github.com/aetheris-dev/aetheris/server/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.ApplyMigrations(context.Background(), pool, cfg.MigrationDir); err != nil {
		log.Fatal(err)
	}
}
