// Package clock adalah satu-satunya sumber waktu di kode domain agar bisa diuji (Technical Guideline 6.5).
package clock

import (
	"sync"
	"time"
)

// Clock memberi waktu sekarang dalam UTC.
type Clock interface {
	Now() time.Time
}

// System memakai jam sistem.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed adalah jam yang bisa diatur, untuk test.
type Fixed struct {
	mu  sync.Mutex
	now time.Time
}

// NewFixed membuat jam tetap pada waktu t.
func NewFixed(t time.Time) *Fixed { return &Fixed{now: t.UTC()} }

func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance memajukan jam sebesar d.
func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
