package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/inventory/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func date(y int, m time.Month, day int) *clock.BusinessDate {
	b := clock.NewBusinessDate(y, m, day)
	return &b
}

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code)
	return e.Fields
}

func TestStatusAndMovementLabels(t *testing.T) {
	for _, s := range domain.Statuses {
		assert.True(t, s.Valid())
		assert.NotEmpty(t, s.Label())
	}
	assert.False(t, domain.StockStatus("lost").Valid())
	assert.Equal(t, "Karantina", domain.StatusQuarantine.Label())
	assert.Equal(t, "Keluar", domain.MovementIssue.Label())
}

func TestValidateQty(t *testing.T) {
	cases := map[string]bool{"1": true, "0.001": true, "2.5000": true, "0": false, "-1": false, "0.0001": false, "1000000000000000": false}
	for in, ok := range cases {
		assert.Equal(t, ok, domain.ValidateQty(d(in)) == "", in)
	}
	assert.Equal(t, "2,5", domain.FormatQty(d("2.500")))
}

func TestBalanceKeyOrderAndNetDeltas(t *testing.T) {
	p1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	p2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	l1 := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	b1 := uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
	mv := func(p uuid.UUID, b *uuid.UUID, s domain.StockStatus, q string) domain.Movement {
		return domain.Movement{ProductID: p, LocationID: l1, BatchID: b, Status: s, Qty: d(q)}
	}
	deltas := domain.NetDeltas([]domain.Movement{
		mv(p2, nil, domain.StatusAvailable, "-1"),
		mv(p1, &b1, domain.StatusReserved, "3"),
		mv(p1, &b1, domain.StatusAvailable, "-3"),
		mv(p1, nil, domain.StatusAvailable, "5"),
		mv(p1, &b1, domain.StatusAvailable, "-2"),
	})
	require.Len(t, deltas, 4)
	assert.Nil(t, deltas[0].Key.BatchID, "batch nil diurutkan lebih dulu")
	assert.Equal(t, domain.StatusAvailable, deltas[1].Key.Status)
	assert.True(t, deltas[1].Delta.Equal(d("-5")))
	assert.Equal(t, domain.StatusReserved, deltas[2].Key.Status)
	assert.Equal(t, p2, deltas[3].Key.ProductID)
	for i := 1; i < len(deltas); i++ {
		assert.Negative(t, deltas[i-1].Key.Compare(deltas[i].Key))
	}
	assert.True(t, domain.SameBatch(nil, nil))
	assert.False(t, domain.SameBatch(&b1, nil))
}

func TestInsufficientStockMessage(t *testing.T) {
	err := domain.InsufficientStock("MS-COK-30", "UTAMA-FG", domain.StatusReserved, d("2.5"))
	e := apperr.As(err)
	assert.Equal(t, apperr.CodeStockInsufficient, e.Code)
	assert.Equal(t, "Stok MS-COK-30 di UTAMA-FG (Dialokasikan) tidak cukup. Tersedia 2,5.", e.Message)
	assert.Equal(t, "Stok A di B tidak cukup. Tersedia 0.", apperr.As(domain.InsufficientStock("A", "B", domain.StatusAvailable, decimal.Zero)).Message)
}

func validPosting() domain.Posting {
	loc := uuid.New()
	return domain.Posting{
		Source: domain.Source{Type: "goods_receipt", ID: uuid.New(), Label: " GR/1 ", BusinessDate: clock.NewBusinessDate(2026, 10, 1)},
		Lines: []domain.Line{
			{Kind: domain.KindReceive, ProductID: uuid.New(), Qty: d("10"), To: &domain.Place{LocationID: loc, Status: domain.StatusQuarantine},
				Batch: &domain.NewBatch{Number: " b-1 "}},
			{Kind: domain.KindMove, ProductID: uuid.New(), Qty: d("1"), From: &domain.Place{LocationID: loc, Status: domain.StatusAvailable},
				To: &domain.Place{LocationID: loc, Status: domain.StatusReserved}},
			{Kind: domain.KindIssue, ProductID: uuid.New(), Qty: d("1"), From: &domain.Place{LocationID: loc, Status: domain.StatusReserved},
				Reservation: &domain.ReservationRef{SourceType: "sales_order", SourceID: uuid.New()}},
		},
	}
}

func TestPostingValidateOK(t *testing.T) {
	p, err := validPosting().Validate()
	require.NoError(t, err)
	assert.Equal(t, domain.ActionPost, p.Source.Action)
	assert.Equal(t, "GR/1", p.Source.Label)
	assert.Equal(t, "B-1", p.Lines[0].Batch.Number)
	assert.True(t, p.Lines[1].IntoReserved())
	assert.False(t, p.Lines[0].IntoReserved())
}

func TestPostingValidateErrors(t *testing.T) {
	neg := d("-1")
	cost := d("1.00001")
	loc := uuid.New()
	p := domain.Posting{
		Source: domain.Source{Type: "Bad Type", Action: string(make([]byte, 61))},
		Lines: []domain.Line{
			{Kind: domain.KindReceive, Qty: d("0"), To: &domain.Place{LocationID: loc, Status: domain.StatusReserved}, UnitCost: &neg,
				From: &domain.Place{}, Batch: &domain.NewBatch{}, BatchID: &loc},
			{Kind: domain.KindReceive, ProductID: uuid.New(), Qty: d("1"), To: &domain.Place{LocationID: loc, Status: domain.StatusAvailable}, UnitCost: &cost},
			{Kind: domain.KindMove, ProductID: uuid.New(), Qty: d("1"), From: &domain.Place{LocationID: loc, Status: domain.StatusAvailable},
				To: &domain.Place{LocationID: loc, Status: domain.StatusAvailable}, UnitCost: &cost},
			{Kind: domain.KindIssue, ProductID: uuid.New(), Qty: d("1"), From: &domain.Place{LocationID: loc, Status: domain.StatusReserved},
				To: &domain.Place{LocationID: loc, Status: domain.StatusAvailable}},
			{Kind: domain.KindIssue, ProductID: uuid.New(), Qty: d("1"), From: &domain.Place{LocationID: loc, Status: domain.StatusAvailable},
				Reservation: &domain.ReservationRef{SourceType: "so", SourceID: uuid.New()}},
			{Kind: "teleport", ProductID: uuid.New(), Qty: d("1")},
			{Kind: domain.KindIssue, ProductID: uuid.New(), Qty: d("1"), From: nil, BatchReason: string(make([]rune, 501)), Note: string(make([]rune, 501))},
			{Kind: domain.KindIssue, ProductID: uuid.New(), Qty: d("1"), From: &domain.Place{LocationID: loc, Status: "x"}},
			{Kind: domain.KindReceive, ProductID: uuid.New(), Qty: d("1"), To: &domain.Place{Status: domain.StatusAvailable},
				Batch: &domain.NewBatch{Number: "X", ProductionDate: date(2026, 5, 1), ExpiryDate: date(2026, 4, 1)}},
		},
	}
	f := fields(t, func() error { _, err := p.Validate(); return err }())
	for _, k := range []string{
		"source.type", "source.id", "source.action", "source.businessDate",
		"lines[0].productId", "lines[0].qty", "lines[0].to.status", "lines[0].unitCost", "lines[0].from", "lines[0].batch",
		"lines[1].unitCost", "lines[2].to", "lines[2].batch", "lines[3].reservation", "lines[3].to", "lines[4].reservation",
		"lines[5].kind", "lines[6].from", "lines[6].batchReason", "lines[6].note", "lines[7].from.status",
		"lines[8].to.locationId", "lines[8].batch.number",
	} {
		assert.Contains(t, f, k)
	}
	_, err := domain.Posting{Source: validPosting().Source}.Validate()
	assert.Contains(t, fields(t, err), "lines")
	many := validPosting()
	many.Lines = make([]domain.Line, domain.MaxLines+1)
	_, err = many.Validate()
	assert.Contains(t, fields(t, err), "lines")
	long := validPosting()
	long.Source.Label = string(make([]rune, 121))
	_, err = long.Validate()
	assert.Contains(t, fields(t, err), "source.label")
}

func TestFEFOAllocation(t *testing.T) {
	today := clock.NewBusinessDate(2026, 10, 17)
	b := func(n string, exp *clock.BusinessDate, q string) domain.Candidate {
		id := uuid.New()
		return domain.Candidate{BatchID: &id, BatchNumber: n, ExpiryDate: exp, Qty: d(q)}
	}
	expired := b("OLD", date(2026, 10, 16), "50")
	todayExp := b("TODAY", date(2026, 10, 17), "3")
	near := b("NEAR", date(2026, 12, 1), "5")
	far := b("FAR", date(2027, 6, 1), "10")
	none := b("NONE", nil, "100")
	empty := b("EMPTY", date(2026, 11, 1), "0")
	cands := []domain.Candidate{none, far, expired, near, empty, todayExp}

	el := domain.Eligible(cands, today)
	names := []string{}
	for _, c := range el {
		names = append(names, c.BatchNumber)
	}
	assert.Equal(t, []string{"TODAY", "NEAR", "FAR", "NONE"}, names)
	assert.True(t, expired.Expired(today))
	assert.False(t, todayExp.Expired(today))

	allocs, short := domain.FEFO{}.Allocate(cands, d("10"), today)
	require.Len(t, allocs, 3)
	assert.True(t, short.IsZero())
	assert.Equal(t, "TODAY", allocs[0].Candidate.BatchNumber)
	assert.True(t, allocs[2].Qty.Equal(d("2")))

	_, short = domain.FEFO{}.Allocate(cands, d("200"), today)
	assert.True(t, short.Equal(d("82")))

	assert.True(t, domain.IsFEFOChoice(domain.FEFO{}, cands, todayExp.BatchID, d("1"), today))
	assert.True(t, domain.IsFEFOChoice(domain.FEFO{}, cands, near.BatchID, d("4"), today), "qty melebihi batch pertama")
	assert.False(t, domain.IsFEFOChoice(domain.FEFO{}, cands, far.BatchID, d("4"), today))
	assert.False(t, domain.IsFEFOChoice(domain.FEFO{}, cands, expired.BatchID, d("1"), today))

	prodA, prodB := date(2026, 1, 1), date(2026, 2, 1)
	same := []domain.Candidate{
		{BatchNumber: "B", ExpiryDate: near.ExpiryDate, ProductionDate: prodB},
		{BatchNumber: "C", ExpiryDate: near.ExpiryDate},
		{BatchNumber: "A", ExpiryDate: near.ExpiryDate, ProductionDate: prodA},
		{BatchNumber: "D", ExpiryDate: near.ExpiryDate, ProductionDate: prodA},
	}
	domain.SortFEFO(same)
	assert.Equal(t, "A", same[0].BatchNumber)
	assert.Equal(t, "D", same[1].BatchNumber)
	assert.Equal(t, "B", same[2].BatchNumber)
	assert.Equal(t, "C", same[3].BatchNumber)
}

func TestLayerSetFIFO(t *testing.T) {
	l1, l2, l3 := uuid.New(), uuid.New(), uuid.New()
	set := domain.LayerSet{Layers: []domain.Layer{
		{ID: l1, Remaining: d("5"), UnitCost: d("100")},
		{ID: l2, Remaining: d("0"), UnitCost: d("999")},
		{ID: l3, Remaining: d("10"), UnitCost: d("120.3333")},
	}}
	before := map[uuid.UUID]decimal.Decimal{l1: d("5"), l2: d("0"), l3: d("10")}
	assert.True(t, set.Available().Equal(d("15")))

	peek, ok := set.Peek(d("7"))
	require.True(t, ok)
	assert.Len(t, peek, 2)
	assert.True(t, set.Available().Equal(d("15")), "peek tidak mengubah layer")

	cons, ok := set.Consume(d("7"))
	require.True(t, ok)
	require.Len(t, cons, 2)
	assert.Equal(t, l1, cons[0].LayerID)
	assert.True(t, cons[1].Qty.Equal(d("2")))
	// 5 × 100 + 2 × 120,3333 = 740,6666 → 740,67
	assert.Equal(t, "740.67", domain.CostAmount(cons).StringFixed(2))
	assert.Equal(t, "105.8095", domain.AverageUnitCost(cons).StringFixed(4))
	assert.Equal(t, "100", domain.AverageUnitCost(cons[:1]).String())
	assert.True(t, domain.AverageUnitCost(nil).IsZero())

	changed := set.Changed(before)
	assert.Len(t, changed, 2)

	_, ok = set.Consume(d("9"))
	assert.False(t, ok)
	assert.True(t, set.Available().Equal(d("8")), "konsumsi gagal tidak mengubah layer")

	set.Add(domain.Layer{ID: uuid.New(), Remaining: d("1"), UnitCost: d("1")})
	assert.True(t, set.Available().Equal(d("9")))
	assert.Equal(t, "0.01", domain.LineAmount(d("0.005"), d("1")).StringFixed(2))
}

func TestBatchExpiry(t *testing.T) {
	today := clock.NewBusinessDate(2026, 10, 17)
	b := domain.Batch{Number: "B1", ExpiryDate: date(2026, 11, 16)}
	days, ok := b.DaysToExpiry(today)
	require.True(t, ok)
	assert.Equal(t, 30, days)
	assert.Equal(t, domain.ExpiryNear, b.State(today, 90))
	assert.Equal(t, domain.ExpiryOK, b.State(today, 10))
	assert.Equal(t, domain.ExpiryExpired, domain.Batch{ExpiryDate: date(2026, 10, 16)}.State(today, 90))
	assert.Equal(t, domain.ExpiryNone, domain.Batch{}.State(today, 90))
	c := b.Candidate()
	assert.Equal(t, "B1", c.BatchNumber)
}

func TestResolveNewBatch(t *testing.T) {
	pid := uuid.New()
	shelf := 365
	b, err := domain.ResolveNewBatch(pid, domain.NewBatch{Number: " x1 ", ProductionDate: date(2026, 1, 1)}, &shelf, "lines[0].batch")
	require.NoError(t, err)
	assert.Equal(t, "X1", b.Number)
	assert.Equal(t, "2027-01-01", b.ExpiryDate.String())

	noExp, err := domain.ResolveNewBatch(pid, domain.NewBatch{Number: "X"}, &shelf, "batch")
	require.NoError(t, err)
	assert.Contains(t, fields(t, domain.RequireExpiry(noExp, &shelf, "batch")), "batch.expiryDate")
	assert.NoError(t, domain.RequireExpiry(noExp, nil, "batch"))
	_, err = domain.ResolveNewBatch(pid, domain.NewBatch{Number: ""}, nil, "batch")
	assert.Contains(t, fields(t, err), "batch.number")
	_, err = domain.ResolveNewBatch(pid, domain.NewBatch{Number: string(make([]rune, 41))}, nil, "batch")
	assert.Contains(t, fields(t, err), "batch.number")
	nb, err := domain.ResolveNewBatch(pid, domain.NewBatch{Number: "PKG"}, nil, "batch")
	require.NoError(t, err)
	assert.Nil(t, nb.ExpiryDate)

	existing := domain.Batch{Number: "X1", ExpiryDate: date(2027, 1, 1), ProductionDate: date(2026, 1, 1)}
	assert.NoError(t, existing.Matches(domain.Batch{ExpiryDate: date(2027, 1, 1)}, "b"))
	assert.NoError(t, existing.Matches(domain.Batch{}, "b"))
	f := fields(t, existing.Matches(domain.Batch{ExpiryDate: date(2027, 2, 1), ProductionDate: date(2026, 2, 1)}, "b"))
	assert.Contains(t, f, "b.expiryDate")
	assert.Contains(t, f, "b.productionDate")
}
