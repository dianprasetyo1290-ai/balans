// Package retry mengulang operasi idempotent yang gagal karena error sementara (Technical Guideline 7.4).
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

// Policy mengatur jumlah percobaan dan backoff eksponensial dengan jitter.
type Policy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// IsRetryable menentukan error yang boleh diulang. Nil berarti tidak ada yang diulang.
	IsRetryable func(error) bool
	// OnRetry dipanggil sebelum menunggu percobaan berikutnya, untuk logging.
	OnRetry func(attempt int, err error)
}

// TransientDBError adalah kebijakan untuk TxManager: maks. 3 percobaan, 50–200 ms + jitter.
var TransientDBError = Policy{
	MaxAttempts: 3,
	BaseDelay:   50 * time.Millisecond,
	MaxDelay:    200 * time.Millisecond,
	IsRetryable: IsTransientDB,
}

// Do menjalankan fn dan mengulanginya sesuai policy. Retry berhenti jika context selesai.
func Do(ctx context.Context, p Policy, fn func() error) error {
	attempts := max(p.MaxAttempts, 1)
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if attempt == attempts || p.IsRetryable == nil || !p.IsRetryable(err) {
			return err
		}
		if p.OnRetry != nil {
			p.OnRetry(attempt, err)
		}
		timer := time.NewTimer(Backoff(p, attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("retry stopped after attempt %d: %w", attempt, errors.Join(err, ctx.Err()))
		case <-timer.C:
		}
	}
	return err
}

// Backoff menghitung jeda sebelum percobaan berikutnya: eksponensial dengan full jitter di separuh atas.
func Backoff(p Policy, attempt int) time.Duration {
	if p.BaseDelay <= 0 {
		return 0
	}
	delay := p.BaseDelay << (attempt - 1)
	if p.MaxDelay > 0 && (delay > p.MaxDelay || delay <= 0) {
		delay = p.MaxDelay
	}
	half := delay / 2
	return half + rand.N(half+1) //nolint:gosec // jitter tidak butuh random kriptografis
}
