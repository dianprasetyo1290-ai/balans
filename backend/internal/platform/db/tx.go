package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/platform/retry"
)

// ErrNoTransaction dikembalikan Savepoint di luar transaksi.
var ErrNoTransaction = errors.New("savepoint membutuhkan transaksi aktif")

// Savepoint menjalankan fn di savepoint transaksi aktif. Jika fn gagal, hanya perubahan fn yang dibatalkan
// dan transaksi luar tetap bisa dipakai (mis. validasi per baris impor Excel). Di luar transaksi ditolak.
func Savepoint(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, ok := Tx(ctx)
	if !ok {
		return ErrNoTransaction
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin savepoint: %w", err)
	}
	if err := fn(withTx(ctx, sp)); err != nil {
		if rerr := sp.Rollback(ctx); rerr != nil {
			return errors.Join(err, fmt.Errorf("rollback savepoint: %w", rerr))
		}
		return err
	}
	if err := sp.Commit(ctx); err != nil {
		return fmt.Errorf("release savepoint: %w", err)
	}
	return nil
}

// WithoutTx melepas transaksi aktif dari context sehingga query berikutnya memakai pool, mis. untuk menulis
// progress job yang harus tetap tersimpan walau transaksi utama nanti dibatalkan.
func WithoutTx(ctx context.Context) context.Context {
	return context.WithValue(ctx, txKey{}, nil)
}

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
