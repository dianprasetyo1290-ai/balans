package db_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
)

func setupTable(t *testing.T) *db.TxManager {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_probe (name text PRIMARY KEY)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `TRUNCATE tx_probe`)
	require.NoError(t, err)
	return db.NewTxManager(pool)
}

func countProbe(t *testing.T, ctx context.Context, conn db.DBTX) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM tx_probe`).Scan(&n))
	return n
}

func TestWithTxCommitsAndNestedSharesTransaction(t *testing.T) {
	tm := setupTable(t)
	pool := dbtest.Pool(t)
	ctx := context.Background()

	err := tm.WithTx(ctx, func(ctx context.Context) error {
		outer, ok := db.Tx(ctx)
		require.True(t, ok)
		if _, err := db.Conn(ctx, pool).Exec(ctx, `INSERT INTO tx_probe VALUES ('a')`); err != nil {
			return err
		}
		return tm.WithTx(ctx, func(ctx context.Context) error {
			inner, _ := db.Tx(ctx)
			assert.Same(t, outer, inner, "transaksi bersarang memakai transaksi luar")
			_, err := db.Conn(ctx, pool).Exec(ctx, `INSERT INTO tx_probe VALUES ('b')`)
			return err
		})
	})
	require.NoError(t, err)
	assert.Equal(t, 2, countProbe(t, ctx, pool))
}

func TestWithTxRollsBackOnError(t *testing.T) {
	tm := setupTable(t)
	pool := dbtest.Pool(t)
	ctx := context.Background()
	businessErr := errors.New("stok tidak cukup")

	err := tm.WithTx(ctx, func(ctx context.Context) error {
		if _, err := db.Conn(ctx, pool).Exec(ctx, `INSERT INTO tx_probe VALUES ('a')`); err != nil {
			return err
		}
		return businessErr
	})

	require.ErrorIs(t, err, businessErr)
	assert.Equal(t, 0, countProbe(t, ctx, pool))
}

func TestWithTxRetriesSerializationFailure(t *testing.T) {
	tm := setupTable(t)
	pool := dbtest.Pool(t)
	ctx := context.Background()
	var attempts atomic.Int32

	err := tm.WithTx(ctx, func(ctx context.Context) error {
		if attempts.Add(1) == 1 {
			return &pgconn.PgError{Code: "40001"}
		}
		_, err := db.Conn(ctx, pool).Exec(ctx, `INSERT INTO tx_probe VALUES ('a')`)
		return err
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
	assert.Equal(t, 1, countProbe(t, ctx, pool))
}

func TestConnWithoutTxUsesPool(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	_, ok := db.Tx(ctx)
	assert.False(t, ok)
	assert.Same(t, pool, db.Conn(ctx, pool))
}

func TestMigratorStatusIsUpToDate(t *testing.T) {
	pool := dbtest.Pool(t)
	m, err := db.NewMigrator(pool)
	require.NoError(t, err)
	defer m.Close() //nolint:errcheck

	status, err := m.Status(context.Background())
	require.NoError(t, err)
	assert.True(t, status.UpToDate())
	assert.Positive(t, status.Latest)
}

func TestConnectRejectsInvalidURL(t *testing.T) {
	_, err := db.Connect(context.Background(), "::not a url::")
	assert.Error(t, err)
}
