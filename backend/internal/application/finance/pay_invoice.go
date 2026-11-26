package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"vedam/lms/internal/domain/clock"
	"vedam/lms/internal/domain/errs"
	domain "vedam/lms/internal/domain/finance"
	syncdomain "vedam/lms/internal/domain/sync"
	"vedam/lms/internal/domain/uow"
)

type PayInvoiceService struct {
	tx      uow.TxManager
	invRead domain.InvoiceReader
	invWrit domain.InvoiceWriter
	payRec  domain.PaymentRecorder
	outbox  syncdomain.OutboxWriter
	idem    syncdomain.IdempotencyStore
	clk     clock.Clock
	newID   func() string
}

func NewPayInvoiceService(
	tx uow.TxManager,
	invRead domain.InvoiceReader,
	invWrit domain.InvoiceWriter,
	payRec domain.PaymentRecorder,
	outbox syncdomain.OutboxWriter,
	idem syncdomain.IdempotencyStore,
	clk clock.Clock,
	newID func() string,
) *PayInvoiceService {
	return &PayInvoiceService{
		tx:      tx,
		invRead: invRead,
		invWrit: invWrit,
		payRec:  payRec,
		outbox:  outbox,
		idem:    idem,
		clk:     clk,
		newID:   newID,
	}
}

func (s *PayInvoiceService) Pay(ctx context.Context, userID, role, feeID, idempotencyKey string) error {
	if role != "student" {
		return errs.New(errs.ErrForbidden, "STUDENT_ONLY", "Student access required.")
	}
	if feeID == "" {
		return errs.New(errs.ErrNotFound, "INVOICE_NOT_FOUND", "Invoice not found.")
	}

	if idempotencyKey != "" {
		encoded, _ := json.Marshal([2]string{userID, idempotencyKey})
		digest := sha256.Sum256(encoded)
		idempotencyKey = "payment:" + hex.EncodeToString(digest[:])
	}
	return s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		if idempotencyKey != "" && s.idem != nil {
			if result, found, err := s.idem.LookupMutation(txCtx, idempotencyKey); err != nil {
				return err
			} else if found {
				var paid struct {
					FeeID string `json:"feeId"`
				}
				if json.Unmarshal(result, &paid) != nil || paid.FeeID != feeID {
					return errs.New(errs.ErrConflict, "PAYMENT_KEY_REUSED", "Payment key already used for another invoice.")
				}
				return nil
			}
		}

		inv, err := s.invRead.GetOwned(txCtx, feeID, userID)
		if err != nil {
			return err
		}
		if inv.Status == domain.StatusPaid {
			return errs.New(errs.ErrAlreadyPaid, "INVOICE_PAID", "Invoice already paid.")
		}

		now := s.clk.Now().UTC()
		paidDate := now.Format("2006-01-02")
		idemKey := idempotencyKey
		if idemKey == "" {
			idemKey = "pay:" + feeID + ":" + s.newID()
		}

		if err := s.payRec.Record(txCtx, &domain.PaymentRecord{
			ID:             s.newID(),
			UserID:         userID,
			FeeID:          feeID,
			IdempotencyKey: idemKey,
			CreatedAt:      now,
		}); err != nil {
			if errors.Is(err, errs.ErrConflict) {
				return errs.New(errs.ErrAlreadyPaid, "INVOICE_PAID", "Invoice already paid.")
			}
			return err
		}

		if err := s.invWrit.MarkPaid(txCtx, feeID, paidDate); err != nil {
			return err
		}

		payload, _ := json.Marshal(map[string]any{
			"feeId":    feeID,
			"userId":   userID,
			"paidDate": paidDate,
			"status":   domain.StatusPaid,
		})
		if err := s.outbox.EnqueueEvent(txCtx, syncdomain.OutboxEvent{
			ID:          s.newID(),
			AggregateID: feeID,
			Topic:       "cache.invalidate.bootstrap",
			Payload:     payload,
			CreatedAt:   now,
		}); err != nil {
			return err
		}

		targetUser := userID
		if _, err := s.outbox.AppendChangelog(txCtx, "user:"+userID, &targetUser, "fee_invoice", feeID, "upsert", payload); err != nil {
			return err
		}

		if idempotencyKey != "" && s.idem != nil {
			response, _ := json.Marshal(map[string]string{"feeId": feeID})
			if err := s.idem.SaveMutation(txCtx, idempotencyKey, userID, response); err != nil {
				return err
			}
		}
		return nil
	})
}
