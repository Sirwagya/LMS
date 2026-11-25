package memory_test

import (
	"context"
	"testing"

	"vedam/lms/internal/adapters/memory"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/contracttest"
)

func TestMemoryContracts(t *testing.T) {
	contracttest.RunAll(t, func(t *testing.T) contracttest.Harness {
		st := memory.NewStore()
		return contracttest.Harness{
			Tx:           st,
			UsersRead:    st.Users(),
			UsersWrite:   st.Users(),
			CatRead:      st.Catalog(),
			CatWrite:     st.Catalog(),
			Schedule:     st.Schedule(),
			Attendance:   st.Attendance(),
			AttRead:      st.Attendance(),
			Progress:     st.Progress(),
			AssignRead:   st.Assignments(),
			AssignAuthor: st.Assignments(),
			Submissions:  st.Submissions(),
			InvoiceRead:  st.Invoices(),
			InvoiceWrite: st.Invoices(),
			PaymentRec:   st.Payments(),
			Outbox:       st.Sync(),
			SyncReader:   st.Sync(),
			Idempotency:  st.Sync(),
			AcadWriter:   st.Academic(),
			EnrolRead:    st.Academic(),
			SessionRead:  st.Academic(),
			Biometrics:   st.Academic(),
			EnsureSem: func(ctx context.Context, num int) {
				st.Catalog().EnsureSemester(ctx, catalog.Semester{Number: num, Label: "Semester", IsCurrent: num == 3})
			},
		}
	})
}
