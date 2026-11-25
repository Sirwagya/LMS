package uow

import "context"

// TxManager executes fn atomically. Domain and application layers never depend on pgx.Tx.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(txCtx context.Context) error) error
}
