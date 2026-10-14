package retry_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/retry"
)

var errTransient = errors.New("transient")

func fastPolicy(attempts int) retry.Policy {
	return retry.Policy{
		MaxAttempts: attempts,
		BaseDelay:   time.Millisecond,
		MaxDelay:    2 * time.Millisecond,
		IsRetryable: func(err error) bool { return errors.Is(err, errTransient) },
	}
}

func TestDoRetriesTransientUntilSuccess(t *testing.T) {
	calls := 0
	var retried []int
	p := fastPolicy(3)
	p.OnRetry = func(attempt int, _ error) { retried = append(retried, attempt) }

	err := retry.Do(context.Background(), p, func() error {
		calls++
		if calls < 3 {
			return errTransient
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.Equal(t, []int{1, 2}, retried)
}

func TestDoStopsAtMaxAttempts(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), fastPolicy(3), func() error {
		calls++
		return errTransient
	})
	assert.ErrorIs(t, err, errTransient)
	assert.Equal(t, 3, calls)
}

func TestDoDoesNotRetryBusinessError(t *testing.T) {
	calls := 0
	business := errors.New("stok tidak cukup")
	err := retry.Do(context.Background(), fastPolicy(3), func() error {
		calls++
		return business
	})
	assert.ErrorIs(t, err, business)
	assert.Equal(t, 1, calls)
}

func TestDoWithoutClassifierNeverRetries(t *testing.T) {
	calls := 0
	_ = retry.Do(context.Background(), retry.Policy{MaxAttempts: 5}, func() error {
		calls++
		return errTransient
	})
	assert.Equal(t, 1, calls)
}

func TestDoRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := fastPolicy(5)
	p.BaseDelay = time.Second
	p.MaxDelay = time.Second
	calls := 0

	err := retry.Do(ctx, p, func() error {
		calls++
		cancel()
		return errTransient
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, errTransient)
	assert.Equal(t, 1, calls)
}

func TestBackoffStaysWithinBounds(t *testing.T) {
	p := retry.Policy{BaseDelay: 50 * time.Millisecond, MaxDelay: 200 * time.Millisecond}
	for attempt := 1; attempt <= 10; attempt++ {
		for range 50 {
			d := retry.Backoff(p, attempt)
			assert.GreaterOrEqual(t, d, 25*time.Millisecond)
			assert.LessOrEqual(t, d, 200*time.Millisecond)
		}
	}
	assert.Zero(t, retry.Backoff(retry.Policy{}, 1))
}

func TestIsTransientDB(t *testing.T) {
	assert.True(t, retry.IsTransientDB(fmt.Errorf("tx: %w", &pgconn.PgError{Code: "40001"})))
	assert.True(t, retry.IsTransientDB(&pgconn.PgError{Code: "40P01"}))
	assert.False(t, retry.IsTransientDB(&pgconn.PgError{Code: "23505"}))
	assert.False(t, retry.IsTransientDB(errTransient))
}

func TestIsTransientNetwork(t *testing.T) {
	assert.True(t, retry.IsTransientNetwork(context.DeadlineExceeded))
	assert.True(t, retry.IsTransientNetwork(&net.OpError{Op: "dial", Err: errors.New("refused")}))
	assert.False(t, retry.IsTransientNetwork(context.Canceled))
	assert.False(t, retry.IsTransientNetwork(errTransient))
}
