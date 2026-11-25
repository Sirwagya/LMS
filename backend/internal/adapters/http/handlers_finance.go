package httpadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"vedam/lms/internal/domain/finance"
)

func (a *api) pay(w http.ResponseWriter, r *http.Request) error {
	f := read(w, r)
	if f.err != nil {
		return f.err
	}
	id := f.id("feeId")
	session := actor(r)
	key, _ := r.Context().Value(mutationKey).(string)
	if key == "" {
		key = strings.TrimSpace(f.optional("idempotencyKey"))
	}
	if len(key) > 200 {
		return invalid("Idempotency-Key must be at most 200 characters.")
	}
	data, err := a.once(r.Context(), session.UserID, "payment", key, id, func(ctx context.Context) (any, error) {
		err := a.Payments.Pay(ctx, session.UserID, string(session.Role), id, "")
		return map[string]bool{"ok": true}, missing(err, "Invoice not found.")
	})
	if err != nil {
		return err
	}
	writeJSON(w, 200, json.RawMessage(data))
	return nil
}
func (a *api) receipt(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	user, err := a.Users.GetByID(ctx, actor(r).UserID)
	if err != nil {
		return err
	}
	fee, err := a.Invoices.GetOwned(ctx, r.PathValue("id"), user.ID)
	if err != nil {
		return missing(err, "Receipt not found.")
	}
	if fee.Status != finance.StatusPaid {
		return problem(409, "Invoice is unpaid.")
	}
	number := fee.InvoiceNo
	if number == "" {
		number = fee.ID
	}
	paid := ""
	if fee.PaidDate != nil {
		paid = *fee.PaidDate
	}
	textFile(w, r, number+"-sample-receipt.txt", fmt.Sprintf("VEDAM LOCAL LMS — SAMPLE RECEIPT\nNot an official payment receipt. No real money collected.\n\nStudent: %s\nInvoice: %s\nDescription: %s\nAmount: INR %d\nPaid: %s\n", user.Name, fee.InvoiceNo, fee.Description, fee.Amount, paid))
	return nil
}
