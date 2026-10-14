package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/system/service"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

type fakeSchema struct {
	status db.SchemaStatus
	err    error
}

func (f fakeSchema) Status(context.Context) (db.SchemaStatus, error) { return f.status, f.err }

var now = time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC)

func newService(p fakePinger, s fakeSchema) *service.SystemService {
	return service.NewSystemService(service.Build{Version: "v1.0.0", Environment: "staging"}, p, s, clock.NewFixed(now))
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name      string
		ping      error
		schema    fakeSchema
		wantReady bool
		failed    string
	}{
		{"siap", nil, fakeSchema{status: db.SchemaStatus{Current: 5, Latest: 5}}, true, ""},
		{"db mati", errors.New("refused"), fakeSchema{}, false, "database"},
		{"migrasi tertinggal", nil, fakeSchema{status: db.SchemaStatus{Current: 4, Latest: 5}}, false, "migration"},
		{"versi tak terbaca", nil, fakeSchema{err: errors.New("no table")}, false, "migration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newService(fakePinger{tt.ping}, tt.schema).Readiness(context.Background())
			assert.Equal(t, tt.wantReady, r.Ready)
			for _, c := range r.Checks {
				if c.Name == tt.failed {
					assert.False(t, c.OK)
					assert.NotEmpty(t, c.Detail)
				}
			}
		})
	}
}

func TestInfo(t *testing.T) {
	info := newService(fakePinger{}, fakeSchema{status: db.SchemaStatus{Current: 5, Latest: 5}}).Info(context.Background())

	assert.Equal(t, "v1.0.0", info.Build.Version)
	assert.Equal(t, now, info.ServerTime)
	assert.True(t, info.Database.OK)
	assert.Equal(t, int64(5), info.Database.SchemaVersion)

	down := newService(fakePinger{errors.New("x")}, fakeSchema{}).Info(context.Background())
	assert.False(t, down.Database.OK)
}
