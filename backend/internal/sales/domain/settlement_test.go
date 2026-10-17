package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/money"
)

func TestCheckSettlementLine(t *testing.T) {
	order := MarketplaceOrderRef{Total: money.FromInt(290_000), Status: StatusInTransit, HandedOver: true, AlreadySettled: money.Zero}
	amounts := SettlementAmounts{Net: money.FromInt(270_000), Fee: money.FromInt(12_000), Promo: money.FromInt(8_000)}
	assert.Equal(t, "290000.00", amounts.Settled().String())
	assert.Empty(t, CheckSettlementLine(order, amounts))

	cases := map[string]struct {
		order   MarketplaceOrderRef
		amounts SettlementAmounts
		field   string
	}{
		"belum serah kurir": {MarketplaceOrderRef{Total: money.FromInt(1), HandedOver: false}, amounts, "order_no"},
		"dibatalkan":        {MarketplaceOrderRef{Total: money.FromInt(1), HandedOver: true, Status: StatusCancelled}, amounts, "order_no"},
		"negatif":           {order, SettlementAmounts{Net: money.FromInt(-1), Fee: money.Zero, Promo: money.Zero}, "net"},
		"nol":               {order, SettlementAmounts{Net: money.Zero, Fee: money.Zero, Promo: money.Zero}, "net"},
		"melebihi sisa": {
			MarketplaceOrderRef{Total: money.FromInt(290_000), HandedOver: true, Status: StatusCompleted, AlreadySettled: money.FromInt(1)},
			amounts, "net",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, CheckSettlementLine(c.order, c.amounts), c.field)
		})
	}
}

func TestSettlementTransitions(t *testing.T) {
	s := Settlement{MarketplaceCode: "shopee", PayoutReference: "P1", Status: SettlementDraft}
	assert.Equal(t, "Settlement shopee P1", s.Label())
	assert.Equal(t, []Action{SettlementActionCancel}, s.Actions())
	require.Error(t, s.CanPost(), "settlement kosong tidak bisa diposting")
	s.LineCount, s.Settled = 1, money.FromInt(10)
	assert.Equal(t, []Action{SettlementActionPost, SettlementActionCancel}, s.Actions())
	require.NoError(t, s.CanPost())
	require.Error(t, s.CanCancel(" "))
	require.NoError(t, s.CanCancel("salah impor"))
	s.Status = SettlementPosted
	assert.Nil(t, s.Actions())
	require.Error(t, s.CanPost())
	require.Error(t, s.CanCancel("x"))
	assert.Equal(t, "Diposting", SettlementPosted.Label())
	assert.Equal(t, "Draft", SettlementDraft.Label())
	assert.Equal(t, "Dibatalkan", SettlementCancelled.Label())
	assert.Equal(t, "x", SettlementStatus("x").Label())
	l := SettlementLine{Receivable: money.FromInt(100), PostedSettled: money.FromInt(40)}
	assert.Equal(t, "60.00", l.Outstanding().String())
}
