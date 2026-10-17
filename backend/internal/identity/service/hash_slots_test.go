package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashSlotsLimitConcurrency(t *testing.T) {
	s := &AuthService{hashSlots: make(chan struct{}, 2)}
	var running, peak atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			require.NoError(t, s.withHashSlot(context.Background(), func() {
				n := running.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				running.Add(-1)
			}))
		})
	}
	wg.Wait()
	assert.LessOrEqual(t, peak.Load(), int32(2), "hashing argon2 dibatasi jumlah slot")

	full := &AuthService{hashSlots: make(chan struct{}, 1)}
	full.hashSlots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, full.withHashSlot(ctx, func() { t.Fatal("tidak boleh jalan") }), context.DeadlineExceeded)
}
