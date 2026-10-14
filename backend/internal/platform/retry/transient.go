package retry

import (
	"context"
	"errors"
	"net"

	"github.com/jackc/pgx/v5/pgconn"
)

// Kode SQLSTATE Postgres yang aman diulang.
const (
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
)

// IsTransientDB bernilai true untuk serialization failure dan deadlock Postgres.
func IsTransientDB(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == sqlStateSerializationFailure || pgErr.Code == sqlStateDeadlockDetected
	}
	return false
}

// IsTransientNetwork bernilai true untuk timeout dan koneksi terputus, tetapi bukan pembatalan context.
func IsTransientNetwork(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
