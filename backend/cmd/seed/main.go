package main

import (
	"context"
	"flag"
	"log"
	"net"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	cryptoadapter "vedam/lms/internal/adapters/crypto"
	"vedam/lms/internal/runtime"
	"vedam/lms/internal/seed"
	"vedam/lms/migrations"
)

func main() {
	reset := flag.Bool("reset", false, "drop and recreate schema before seeding")
	flag.Parse()

	cfg, err := runtime.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Production {
		log.Fatal("Demo seeding is disabled in production.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Invalid demo database URL.")
	}
	local := func(host string) bool {
		return filepath.IsAbs(host) || host == "localhost" || net.ParseIP(host).IsLoopback()
	}
	if !local(db.ConnConfig.Host) {
		log.Fatal("Demo seeding requires a local PostgreSQL instance.")
	}
	for _, fallback := range db.ConnConfig.Fallbacks {
		if !local(fallback.Host) {
			log.Fatal("Demo database fallback must also be local.")
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, db)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if *reset {
		if err := migrations.Reset(ctx, pool); err != nil {
			log.Fatalf("reset migrations: %v", err)
		}
	} else {
		if err := migrations.Up(ctx, pool); err != nil {
			log.Fatalf("run migrations: %v", err)
		}
	}

	if err := seed.Run(ctx, pool, cryptoadapter.NewDualPasswordHasher()); err != nil {
		log.Fatalf("seed database: %v", err)
	}
	log.Println("Migrations and seed completed successfully.")
}
