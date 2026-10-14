package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/platform/retry"
)

// TxManager membuka transaksi di layer service. Repository mengambil koneksi lewat Conn,
// sehingga beberapa repository dan modul berjalan dalam satu transaksi.
type TxManager struct {
	pool   *pgxpool.Pool
	policy retry.Policy
}

// NewTxManager membuat TxManager yang me-retry deadlock dan serialization failure.
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool, policy: retry.TransientDBError}
}

// WithTx menjalankan fn dalam transaksi. Transaksi bersarang memakai transaksi yang sudah ada,
// dan hanya transaksi terluar yang di-retry.
func (m *TxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := Tx(ctx); ok {
		return fn(ctx)
	}
	return retry.Do(ctx, m.policy, func() error {
		tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck // no-op setelah commit

		if err := fn(withTx(ctx, tx)); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		return nil
	})
}
