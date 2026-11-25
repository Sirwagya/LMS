package migrations

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var sqlFiles embed.FS

func Up(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	var applied bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = '001_initial_schema')`).Scan(&applied); err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if applied {
		return nil
	}

	ddl, err := sqlFiles.ReadFile("001_initial_schema.up.sql")
	if err != nil {
		return fmt.Errorf("read up migration: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(ddl)); err != nil {
		return fmt.Errorf("exec up migration: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ('001_initial_schema')`); err != nil {
		return fmt.Errorf("record schema migration: %w", err)
	}
	return tx.Commit(ctx)
}

func Down(ctx context.Context, pool *pgxpool.Pool) error {
	ddl, err := sqlFiles.ReadFile("001_initial_schema.down.sql")
	if err != nil {
		return fmt.Errorf("read down migration: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin down migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(ddl)); err != nil {
		return fmt.Errorf("exec down migration: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = '001_initial_schema'`); err != nil {
		return fmt.Errorf("delete migration record: %w", err)
	}
	return tx.Commit(ctx)
}

func Reset(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return err
	}
	if err := Down(ctx, pool); err != nil {
		return err
	}
	return Up(ctx, pool)
}
