package errs

import "errors"

var (
	ErrNotFound        = errors.New("resource not found")
	ErrConflict        = errors.New("resource conflict")
	ErrUnauthorized    = errors.New("authentication required")
	ErrForbidden       = errors.New("insufficient permissions")
	ErrValidation      = errors.New("validation failed")
	ErrRateLimited     = errors.New("rate limit exceeded")
	ErrAlreadyGraded   = errors.New("graded submission is immutable")
	ErrAlreadyPaid     = errors.New("invoice already paid")
	ErrScheduleOverlap = errors.New("session overlaps an existing class")
)

type DomainError struct {
	Sentinel error
	Code     string
	Message  string
}

func (e *DomainError) Error() string { return e.Message }
func (e *DomainError) Unwrap() error { return e.Sentinel }

func New(sentinel error, code, message string) error {
	return &DomainError{
		Sentinel: sentinel,
		Code:     code,
		Message:  message,
	}
}
