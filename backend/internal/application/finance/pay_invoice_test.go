package finance_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	clockadapter "vedam/lms/internal/adapters/clock"
	"vedam/lms/internal/adapters/memory"
	"vedam/lms/internal/application/finance"
	"vedam/lms/internal/domain/errs"
	domain "vedam/lms/internal/domain/finance"
	"vedam/lms/internal/domain/iam"
)

func TestPayInvoiceService_IdempotentAndAtomic(t *testing.T) {
	st := memory.NewStore()
	ctx := context.Background()
	_ = st.Users().Create(ctx, &iam.User{
		ID: "stu-1", Email: "stu@vedam.local", Name: "Student", Role: iam.RoleStudent, PasswordHash: "x",
	})
	_ = st.Invoices().Create(ctx, &domain.FeeInvoice{
		ID: "fee-1", UserID: "stu-1", InvoiceNo: "INV-1", Description: "Tuition", Semester: "S3",
		Amount: 10000, DueDate: "2026-11-01", Status: domain.StatusPending,
	})

	idSeq := 0
	svc := finance.NewPayInvoiceService(
		st,
		st.Invoices(),
		st.Invoices(),
		st.Payments(),
		st.Sync(),
		st.Sync(),
		clockadapter.FixedClock{Fixed: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)},
		func() string { idSeq++; return fmt.Sprintf("id-%d", idSeq) },
	)

	if err := svc.Pay(ctx, "stu-1", "teacher", "fee-1", "k1"); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-student, got %v", err)
	}

	if err := svc.Pay(ctx, "stu-1", "student", "fee-1", "idem-abc"); err != nil {
		t.Fatalf("first pay: %v", err)
	}
	// Replay with same idempotency key succeeds cleanly without duplicating outbox
	if err := svc.Pay(ctx, "stu-1", "student", "fee-1", "idem-abc"); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	// Paying same already-paid invoice with new key returns ErrAlreadyPaid
	if err := svc.Pay(ctx, "stu-1", "student", "fee-1", "idem-other"); !errors.Is(err, errs.ErrAlreadyPaid) {
		t.Fatalf("expected ErrAlreadyPaid on duplicate pay, got %v", err)
	}

	inv, _ := st.Invoices().GetOwned(ctx, "fee-1", "stu-1")
	if inv.Status != domain.StatusPaid || !inv.ReceiptAvailable || inv.PaidDate == nil || *inv.PaidDate != "2026-11-25" {
		t.Fatalf("unexpected paid invoice state: %+v", inv)
	}

	events, _ := st.Sync().ClaimUnpublished(ctx, 10)
	if len(events) != 1 || events[0].Topic != "cache.invalidate.bootstrap" {
		t.Fatalf("expected exactly 1 outbox event, got %+v", events)
	}
}
