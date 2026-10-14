// Package db menyediakan pool koneksi dan TxManager (Technical Guideline 5.2).
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX adalah antarmuka yang dipakai kode hasil generate sqlc. Dipenuhi oleh *pgxpool.Pool dan pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Connect membuka pool dan memastikan database bisa dihubungi.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	// Server dan DB berjalan dalam UTC (Technical Guideline 6.5).
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

type txKey struct{}

func withTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// Tx mengembalikan transaksi aktif di context, misalnya untuk river InsertTx.
func Tx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// Conn mengembalikan transaksi aktif jika ada, jika tidak pool.
func Conn(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := Tx(ctx); ok {
		return tx
	}
	return pool
}
