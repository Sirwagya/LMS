package finance

import (
	"context"
	"time"
)

type InvoiceStatus string

const (
	StatusPending InvoiceStatus = "Pending"
	StatusOverdue InvoiceStatus = "Overdue"
	StatusPaid    InvoiceStatus = "Paid"
)

type FeeInvoice struct {
	ID               string        `json:"id"`
	UserID           string        `json:"userId,omitempty"`
	InvoiceNo        string        `json:"invoiceNo"`
	Description      string        `json:"description"`
	Semester         string        `json:"semester"`
	Amount           int64         `json:"amount"`
	DueDate          string        `json:"dueDate"`
	PaidDate         *string       `json:"paidDate,omitempty"`
	Status           InvoiceStatus `json:"status"`
	ReceiptAvailable bool          `json:"receiptAvailable"`
	Simulated        bool          `json:"simulated,omitempty"`
	Version          int64         `json:"version,omitempty"`
}

type PaymentRecord struct {
	ID             string
	UserID         string
	FeeID          string
	IdempotencyKey string
	CreatedAt      time.Time
}

type InvoiceReader interface {
	ListForStudent(ctx context.Context, userID string) ([]FeeInvoice, error)
	ListAll(ctx context.Context) ([]FeeInvoice, error)
	GetOwned(ctx context.Context, feeID, userID string) (*FeeInvoice, error)
}

type InvoiceWriter interface {
	Create(ctx context.Context, invoice *FeeInvoice) error
	MarkPaid(ctx context.Context, feeID, paidDate string) error
}

type PaymentRecorder interface {
	Record(ctx context.Context, payment *PaymentRecord) error
}
