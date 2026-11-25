package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	cryptoadapter "vedam/lms/internal/adapters/crypto"
	"vedam/lms/internal/seed"
	"vedam/lms/migrations"
)

func main() {
	reset := flag.Bool("reset", false, "drop and recreate schema before seeding")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://lms:lms_local_dev@127.0.0.1:5432/lms_dev?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
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
