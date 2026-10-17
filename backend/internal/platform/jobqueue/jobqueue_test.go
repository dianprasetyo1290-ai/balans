package jobqueue_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/jobqueue"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type probeArgs struct {
	Marker string `json:"marker"`
}

func (probeArgs) Kind() string { return "test.probe" }

func countJobs(t *testing.T, marker string) int {
	t.Helper()
	var n int
	err := dbtest.Pool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'test.probe' AND args->>'marker' = $1`, marker).Scan(&n)
	require.NoError(t, err)
	return n
}

func TestEnqueueRequiresTransaction(t *testing.T) {
	ins, err := jobqueue.NewInserter(dbtest.Pool(t))
	require.NoError(t, err)

	err = ins.Enqueue(context.Background(), probeArgs{Marker: "no-tx"}, nil)

	assert.ErrorIs(t, err, jobqueue.ErrNoTransaction)
}

func TestEnqueueFollowsTransactionOutcome(t *testing.T) {
	pool := dbtest.Pool(t)
	ins, err := jobqueue.NewInserter(pool)
	require.NoError(t, err)
	tm := db.NewTxManager(pool)
	ctx := context.Background()

	require.NoError(t, tm.WithTx(ctx, func(ctx context.Context) error {
		return ins.Enqueue(ctx, probeArgs{Marker: "committed"}, &river.InsertOpts{Queue: jobqueue.QueueDefault})
	}))
	_ = tm.WithTx(ctx, func(ctx context.Context) error {
		require.NoError(t, ins.Enqueue(ctx, probeArgs{Marker: "rolled-back"}, nil))
		return assert.AnError
	})

	assert.Equal(t, 1, countJobs(t, "committed"))
	assert.Equal(t, 0, countJobs(t, "rolled-back"), "job tidak ada jika transaksi gagal")
}

func TestDailyAtUsesJakartaTime(t *testing.T) {
	sched := jobqueue.DailyAt{Hour: 2, Minute: 0}
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"sebelum jam 02.00 WIB", time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC), time.Date(2026, 10, 13, 19, 0, 0, 0, time.UTC)},
		{"tepat jam 02.00 WIB pindah ke besok", time.Date(2026, 10, 13, 19, 0, 0, 0, time.UTC), time.Date(2026, 10, 14, 19, 0, 0, 0, time.UTC)},
		{"siang WIB", time.Date(2026, 10, 14, 5, 0, 0, 0, time.UTC), time.Date(2026, 10, 14, 19, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sched.Next(tt.now))
		})
	}
	assert.Len(t, jobqueue.QueueConfig(), 3)
}

func TestMonthlyAtUsesJakartaTime(t *testing.T) {
	sched := jobqueue.MonthlyAt{Day: 1, Hour: 1, Minute: 0}
	// 1 Nov 2026 01.00 WIB = 31 Okt 2026 18.00 UTC.
	first := time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC)
	assert.Equal(t, first, sched.Next(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)))
	assert.Equal(t, time.Date(2026, 11, 30, 18, 0, 0, 0, time.UTC), sched.Next(first), "tepat jadwal pindah ke bulan berikutnya")
	// 1 Jan 2027 01.00 WIB = 31 Des 2026 18.00 UTC.
	assert.Equal(t, time.Date(2026, 12, 31, 18, 0, 0, 0, time.UTC), sched.Next(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)))
}
