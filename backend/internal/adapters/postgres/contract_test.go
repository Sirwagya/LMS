//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vedam/lms/internal/adapters/postgres"
	"vedam/lms/internal/domain/contracttest"
	"vedam/lms/migrations"
)

func TestPostgresContracts(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://lms:lms_local_dev@127.0.0.1:5432/lms_test?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(pool.Close)

	contracttest.RunAll(t, func(t *testing.T) contracttest.Harness {
		if err := migrations.Reset(ctx, pool); err != nil {
			t.Fatalf("reset schema: %v", err)
		}
		tx := postgres.NewTxManager(pool)
		users := postgres.NewUserRepo(tx)
		cat := postgres.NewCatalogRepo(tx)
		sched := postgres.NewScheduleRepo(tx)
		att := postgres.NewAttendanceRepo(tx)
		prog := postgres.NewProgressRepo(tx)
		assign := postgres.NewAssignmentRepo(tx)
		subs := postgres.NewSubmissionRepo(tx)
		fin := postgres.NewFinanceRepo(tx)
		syncRepo := postgres.NewSyncRepo(tx)
		acad := postgres.NewAcademicRepo(tx)

		return contracttest.Harness{
			Tx:           tx,
			UsersRead:    users,
			UsersWrite:   users,
			CatRead:      cat,
			CatWrite:     cat,
			Schedule:     sched,
			Attendance:   att,
			AttRead:      att,
			Progress:     prog,
			AssignRead:   assign,
			AssignAuthor: assign,
			Submissions:  subs,
			InvoiceRead:  fin,
			InvoiceWrite: fin,
			PaymentRec:   fin,
			Outbox:       syncRepo,
			SyncReader:   syncRepo,
			Idempotency:  syncRepo,
			AcadWriter:   acad,
			EnrolRead:    acad,
			SessionRead:  acad,
			Biometrics:   acad,
			EnsureSem: func(ctx context.Context, num int) {
				if _, err := pool.Exec(ctx, `
					INSERT INTO semesters (number, label, is_current) VALUES ($1, 'Semester', $2) ON CONFLICT DO NOTHING
				`, num, num == 3); err != nil {
					t.Fatalf("ensure semester %d: %v", num, err)
				}
			},
		}
	})
}
