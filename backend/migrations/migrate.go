package migrations

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var sqlFiles embed.FS

func run(ctx context.Context, pool *pgxpool.Pool, down bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(24851926)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
   CREATE TABLE IF NOT EXISTS schema_migrations (
     version TEXT PRIMARY KEY,
     applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
   )
 `); err != nil {
		return fmt.Errorf("ensure schema migrations: %w", err)
	}
	entries, err := sqlFiles.ReadDir(".")
	if err != nil {
		return err
	}
	versions := []string{}
	for _, entry := range entries {
		if version, ok := strings.CutSuffix(entry.Name(), ".up.sql"); ok {
			versions = append(versions, version)
		}
	}
	for i := range versions {
		index := i
		if down {
			index = len(versions) - 1 - i
		}
		if err := apply(ctx, tx, versions[index], down); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func apply(ctx context.Context, tx pgx.Tx, version string, down bool) error {
	var applied bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&applied); err != nil {
		return err
	}
	if applied != down {
		return nil
	}
	suffix, statement := ".up.sql", `INSERT INTO schema_migrations(version) VALUES ($1)`
	if down {
		suffix, statement = ".down.sql", `DELETE FROM schema_migrations WHERE version=$1`
	}
	ddl, err := sqlFiles.ReadFile(version + suffix)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, string(ddl)); err != nil {
		return fmt.Errorf("migration %s%s: %w", version, suffix, err)
	}
	_, err = tx.Exec(ctx, statement, version)
	return err
}

func Up(ctx context.Context, pool *pgxpool.Pool) error   { return run(ctx, pool, false) }
func Down(ctx context.Context, pool *pgxpool.Pool) error { return run(ctx, pool, true) }
func Reset(ctx context.Context, pool *pgxpool.Pool) error {
	if err := Down(ctx, pool); err != nil {
		return err
	}
	return Up(ctx, pool)
}
