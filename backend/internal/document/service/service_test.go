package service_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/document/domain"
	"balans/internal/document/postgres"
	"balans/internal/document/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

var errRollback = errors.New("rollback")

func TestNextNumber(t *testing.T) {
	pool := dbtest.Pool(t)
	tx := db.NewTxManager(pool)
	svc := service.New(postgres.NewStore(pool), clock.NewFixed(time.Date(2027, 1, 15, 3, 0, 0, 0, time.UTC)))
	ctx := context.Background()
	jan := clock.NewBusinessDate(2027, time.January, 15)
	may := clock.NewBusinessDate(2027, time.May, 2)

	next := func(dt domain.Type, d clock.BusinessDate) string {
		var n string
		assert.NoError(t, tx.WithTx(ctx, func(ctx context.Context) error {
			var err error
			n, err = svc.Next(ctx, dt, d)
			return err
		}))
		return n
	}

	assert.Equal(t, "PR/2027/01/00001", next(domain.TypePurchaseRequest, jan))
	assert.Equal(t, "PR/2027/05/00002", next(domain.TypePurchaseRequest, may), "nomor urut per tahun, bulan dari tanggal dokumen")
	assert.Equal(t, "PO/2027/01/00001", next(domain.TypePurchaseOrder, jan), "sequence per kode")
	assert.Equal(t, "PR/2028/01/00001", next(domain.TypePurchaseRequest, clock.NewBusinessDate(2028, time.January, 1)), "reset tiap tahun")

	// Transaksi gagal mengembalikan nomor: tidak loncat.
	err := tx.WithTx(ctx, func(ctx context.Context) error {
		n, err := svc.Next(ctx, domain.TypePurchaseRequest, jan)
		require.NoError(t, err)
		assert.Equal(t, "PR/2027/01/00003", n)
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
	assert.Equal(t, "PR/2027/01/00003", next(domain.TypePurchaseRequest, jan))

	// Di luar transaksi ditolak.
	_, err = svc.Next(ctx, domain.TypePurchaseRequest, jan)
	assert.ErrorIs(t, err, service.ErrNoTransaction)
	_, err = svc.Next(ctx, "XX", jan)
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))

	// Paralel tetap unik dan berurutan.
	var (
		mu   sync.Mutex
		seen = map[string]bool{}
		wg   sync.WaitGroup
	)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := next(domain.TypeSalesOrder, jan)
			mu.Lock()
			seen[n] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	assert.Len(t, seen, 10)
	assert.True(t, seen["SO/2027/01/00010"])

	// Halaman Nomor Dokumen.
	reader := actor.With(ctx, actor.Actor{UserID: uuid.New(), Permissions: map[string]struct{}{service.PermSequenceRead: {}}})
	year, rows, err := svc.List(reader, 0)
	require.NoError(t, err)
	assert.Equal(t, 2027, year)
	assert.Len(t, rows, len(domain.Types()))
	for _, r := range rows {
		switch r.Info.Type {
		case domain.TypePurchaseRequest:
			assert.Equal(t, int64(3), r.Sequence.LastNumber)
		case domain.TypeJournal:
			assert.Equal(t, int64(0), r.Sequence.LastNumber)
		}
	}
	_, _, err = svc.List(actor.With(ctx, actor.Actor{UserID: uuid.New()}), 2027)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}
