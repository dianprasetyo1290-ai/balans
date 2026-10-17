package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/apperr"
	"balans/internal/warehouse/domain"
)

func ids(o *domain.Outbound) {
	for i := range o.Lines {
		if o.Lines[i].ID == uuid.Nil {
			o.Lines[i].ID = uuid.New()
		}
	}
}

func newOutbound(t *testing.T) domain.Outbound {
	t.Helper()
	b1, b2 := uuid.New(), uuid.New()
	alloc := uuid.New()
	o, err := domain.NewOutbound(domain.OutboundInput{
		SourceType: domain.OutboundSourceDeliveryOrder, SourceID: uuid.New(), SourceLabel: "DO/2026/11/00001",
		LocationID: uuid.New(), BusinessDate: day, RecipientName: " Toko Sehat ", Note: "pagi",
		Lines: []domain.OutboundLineInput{
			{SourceLineID: &alloc, ProductID: uuid.New(), BatchID: &b1, Qty: d("6")},
			{SourceLineID: &alloc, ProductID: uuid.New(), BatchID: &b2, Qty: d("4")},
		},
	})
	require.NoError(t, err)
	ids(&o)
	o.ID = uuid.New()
	return o
}

var okChecks = domain.Checklist{Batch: true, Expiry: true, Condition: true, Seal: true}

func TestNewOutboundValidation(t *testing.T) {
	_, err := domain.NewOutbound(domain.OutboundInput{SourceType: "X", Lines: []domain.OutboundLineInput{{Qty: d("0")}}})
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code)
	for _, f := range []string{"source.type", "source.id", "locationId", "businessDate", "lines[0].productId", "lines[0].qty"} {
		assert.Contains(t, e.Fields, f)
	}
	_, err = domain.NewOutbound(domain.OutboundInput{SourceType: "delivery_order", SourceID: uuid.New(), LocationID: uuid.New(), BusinessDate: day})
	assert.Contains(t, apperr.As(err).Fields, "lines")

	o := newOutbound(t)
	assert.Equal(t, "Toko Sehat", o.RecipientName)
	assert.Equal(t, domain.StatusInProgress, o.Status)
	assert.Equal(t, []string{domain.ActionPick}, o.Actions())
	assert.Equal(t, "Delivery Order", domain.OutboundSourceLabel(o.SourceType))
	assert.Equal(t, "lain", domain.OutboundSourceLabel("lain"))
}

func TestOutboundPickPackQCComplete(t *testing.T) {
	o := newOutbound(t)
	staff, qc := uuid.New(), uuid.New()

	// Packing sebelum picking ditolak.
	isTransition(t, o.Pack([]domain.PackageInput{{}}, staff, "Staf", now))
	// QC sebelum packing ditolak.
	isTransition(t, o.RequireChecker(qc))

	require.NoError(t, o.Pick([]uuid.UUID{o.Lines[0].ID}, staff, "Staf"))
	isTransition(t, o.Pick([]uuid.UUID{o.Lines[0].ID}, staff, "Staf"))
	require.NoError(t, o.Pick(nil, staff, "Staf"))
	isTransition(t, o.Pick(nil, staff, "Staf"))
	assert.Equal(t, []string{domain.ActionPack}, o.Actions())
	assert.True(t, o.Lines[1].PickedQty.Equal(d("4")))

	// Validasi koli.
	err := o.Pack(nil, staff, "Staf", now)
	assert.Contains(t, apperr.As(err).Fields, "packages")
	w := -1
	err = o.Pack([]domain.PackageInput{{WeightGrams: &w, Items: []domain.PackageItemInput{{LineID: uuid.New(), Qty: d("1")}, {LineID: o.Lines[0].ID, Qty: d("0")}}}, {}}, staff, "Staf", now)
	f := apperr.As(err).Fields
	assert.Contains(t, f, "packages[0].weightGrams")
	assert.Contains(t, f, "packages[0].items[0].lineId")
	assert.Contains(t, f, "packages[0].items[1].qty")
	assert.Contains(t, f, "packages[1].items")
	err = o.Pack([]domain.PackageInput{{Items: []domain.PackageItemInput{{LineID: o.Lines[0].ID, Qty: d("6")}}}}, staff, "Staf", now)
	assert.Contains(t, apperr.As(err).Fields["packages"], "baris 2", "baris 2 belum masuk koli")

	require.NoError(t, o.Pack([]domain.PackageInput{
		{Items: []domain.PackageItemInput{{LineID: o.Lines[0].ID, Qty: d("6")}}},
		{Items: []domain.PackageItemInput{{LineID: o.Lines[1].ID, Qty: d("4")}}},
	}, staff, "Staf", now))
	require.Len(t, o.Packages, 2)
	assert.Equal(t, 2, o.Packages[1].Count)
	assert.Equal(t, 2, o.Packages[1].Number)
	assert.False(t, o.NeedsRepack())

	// Penyiap barang tidak boleh QC (D-23/D-24).
	err = o.RequireChecker(staff)
	assert.Equal(t, apperr.CodeForbidden, apperr.As(err).Code)
	require.NoError(t, o.RequireChecker(qc))
	assert.Equal(t, []string{domain.ActionQC}, o.LineActions(o.Lines[0]))

	// Validasi QC.
	l := &o.Lines[0]
	_, err = l.ApplyQC(domain.OutboundQCInput{PassedQty: d("5"), RejectedQty: d("0"), Checks: okChecks}, qc, "QC", now)
	assert.Contains(t, apperr.As(err).Fields, "passedQty")
	_, err = l.ApplyQC(domain.OutboundQCInput{PassedQty: d("4"), RejectedQty: d("2"), Checks: okChecks}, qc, "QC", now)
	assert.Contains(t, apperr.As(err).Fields, "rejectReason")
	_, err = l.ApplyQC(domain.OutboundQCInput{PassedQty: d("6"), RejectedQty: d("0")}, qc, "QC", now)
	assert.Contains(t, apperr.As(err).Fields, "checks")
	_, err = l.ApplyQC(domain.OutboundQCInput{PassedQty: d("-1"), RejectedQty: d("0.0001"), PhotoFileIDs: make([]uuid.UUID, 6)}, qc, "QC", now)
	f = apperr.As(err).Fields
	assert.Contains(t, f, "passedQty")
	assert.Contains(t, f, "rejectedQty")
	assert.Contains(t, f, "photoFileIds")

	in, err := l.ApplyQC(domain.OutboundQCInput{PassedQty: d("4"), RejectedQty: d("2"), Checks: okChecks, RejectReason: "Segel rusak", Note: "foto"}, qc, "QC", now)
	require.NoError(t, err)
	assert.Equal(t, domain.OutboundPassed, l.Status)
	checks := domain.OutboundQualityChecks(o, *l, in)
	require.Len(t, checks, 2)
	assert.Equal(t, "Segel rusak — foto", checks[1].Note)
	assert.Equal(t, "outbound", checks[0].DocumentType)
	_, err = l.ApplyQC(in, qc, "QC", now)
	isTransition(t, err)

	// Stok pengganti: 1 dari batch lain, kurang 1 → baris ditahan; packing harus diulang.
	b3 := uuid.New()
	o.AddReplacement(*l, []domain.ReplacementBatch{{BatchID: &b3, Qty: d("1")}, {Qty: d("0")}}, d("1"))
	ids(&o)
	require.Len(t, o.Lines, 4)
	assert.Equal(t, domain.OutboundPending, o.Lines[2].Status)
	assert.Equal(t, domain.OutboundHeld, o.Lines[3].Status)
	assert.Equal(t, o.Lines[0].ID, *o.Lines[3].ReplacementOfLineID)
	assert.Nil(t, o.PackedAt)
	assert.ElementsMatch(t, []string{domain.ActionReallocate, domain.ActionDrop}, o.LineActions(o.Lines[3]))
	assert.Empty(t, o.LineActions(o.Lines[2]))

	// Baris 2 seluruhnya ditolak.
	_, err = o.Lines[1].ApplyQC(domain.OutboundQCInput{PassedQty: d("0"), RejectedQty: d("4"), RejectReason: "Penyok"}, qc, "QC", now)
	require.NoError(t, err)
	assert.Equal(t, domain.OutboundRejected, o.Lines[1].Status)

	assert.False(t, o.TryComplete(qc, "QC", now), "masih ada baris menunggu")
	require.NoError(t, o.Pick(nil, staff, "Staf"))

	// Refill baris ditahan: 2 batch (0,5 + 0,3) → sisa 0,2 tetap ditahan.
	b4, b5 := uuid.New(), uuid.New()
	err = o.Refill(o.Lines[3].ID, []domain.ReplacementBatch{{BatchID: &b4, Qty: d("0.5")}, {BatchID: &b5, Qty: d("0.3")}})
	require.NoError(t, err)
	ids(&o)
	require.Len(t, o.Lines, 6)
	assert.Equal(t, domain.OutboundPending, o.Lines[3].Status)
	assert.Equal(t, b4, *o.Lines[3].BatchID)
	assert.True(t, o.Lines[5].AllocatedQty.Equal(d("0.2")))
	assert.Equal(t, domain.OutboundHeld, o.Lines[5].Status)
	require.NoError(t, o.Refill(o.Lines[5].ID, nil), "tanpa stok tidak berubah")
	isTransition(t, o.Refill(o.Lines[3].ID, nil))
	assert.Equal(t, apperr.CodeNotFound, apperr.As(o.Refill(uuid.New(), nil)).Code)

	// Kirim sebagian untuk sisa yang ditahan.
	assert.Contains(t, apperr.As(o.Drop(o.Lines[5].ID, " ")).Fields, "reason")
	isTransition(t, o.Drop(o.Lines[3].ID, "x"))
	assert.Equal(t, apperr.CodeNotFound, apperr.As(o.Drop(uuid.New(), "x")).Code)
	require.NoError(t, o.Drop(o.Lines[5].ID, "Pelanggan setuju kirim sebagian"))
	assert.Equal(t, domain.OutboundDropped, o.Lines[5].Status)

	require.NoError(t, o.Pick(nil, staff, "Staf"))
	assert.True(t, o.NeedsRepack())
	pack := []domain.PackageInput{{Items: []domain.PackageItemInput{
		{LineID: o.Lines[0].ID, Qty: d("4")}, {LineID: o.Lines[2].ID, Qty: d("1")}, {LineID: o.Lines[3].ID, Qty: d("0.5")}, {LineID: o.Lines[4].ID, Qty: d("0.3")},
	}}}
	require.NoError(t, o.Pack(pack, staff, "Staf", now))
	for _, i := range []int{2, 3, 4} {
		_, err := o.Lines[i].ApplyQC(domain.OutboundQCInput{PassedQty: o.Lines[i].PickedQty, RejectedQty: decimal.Zero, Checks: okChecks}, qc, "QC", now)
		require.NoError(t, err)
	}
	assert.True(t, o.TryComplete(qc, "QC", now))
	assert.Equal(t, domain.StatusCompleted, o.Status)
	assert.False(t, o.TryComplete(qc, "QC", now))
	assert.Empty(t, o.Actions())
	assert.Empty(t, o.LineActions(o.Lines[0]))
	isTransition(t, o.Cancel("x", now))
	isTransition(t, o.Pick(nil, staff, "Staf"))
}

func TestOutboundCancel(t *testing.T) {
	o := newOutbound(t)
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'a'
	}
	assert.Contains(t, apperr.As(o.Cancel(string(long), now)).Fields, "reason")
	require.NoError(t, o.Cancel("DO dibatalkan", now))
	assert.Equal(t, domain.StatusCancelled, o.Status)
	assert.NotNil(t, o.CancelledAt)
	assert.False(t, o.TryComplete(uuid.New(), "QC", now))
	_, ok := o.Line(uuid.New())
	assert.False(t, ok)
}
