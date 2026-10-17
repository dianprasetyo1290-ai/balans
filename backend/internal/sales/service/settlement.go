package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// Settlement marketplace lewat impor laporan settlement per marketplace (S-403, ACC-06, F1-402).

// Permission settlement marketplace (Desain Role & Approval 3.4, resource accounting.settlement).
const (
	PermSettlementRead   = "accounting.settlement.read"
	PermSettlementWrite  = "accounting.settlement.write"
	PermSettlementPost   = "accounting.settlement.post"
	PermSettlementImport = "accounting.settlement.import"

	RuleMarketplaceSettlement = "A-10s"
	SourceSettlement          = "marketplace_settlement"
	EntitySettlement          = "marketplace_settlement"
	// DefaultSettlementBank adalah akun bank default dana cair bila kolom Akun Bank kosong.
	DefaultSettlementBank = "1111"
)

// SettlementFormat adalah pemetaan kolom laporan settlement satu marketplace ke nilai settlement: beberapa kolom
// fee/promo dijumlahkan. Header mengikuti laporan unduhan seller center (dikonfirmasi dengan file asli Balans).
type SettlementFormat struct {
	Marketplace string
	Label       string
	Columns     []tabular.Column
	FeeKeys     []string
	PromoKeys   []string
}

// Kind adalah jenis data impor dataexchange, mis. `settlement.shopee`.
func (f SettlementFormat) Kind() string { return "settlement." + f.Marketplace }

func settlementColumns(orderHeader, dateHeader, refHeader, netHeader string, fees, promos [][2]string) []tabular.Column {
	cols := []tabular.Column{
		{Key: "order_no", Header: orderHeader, Kind: tabular.KindText, Required: true, Identity: true, Example: "2611020ABCDEF"},
		{Key: "payout_date", Header: dateHeader, Kind: tabular.KindDate, Required: true, Example: "2026-11-09"},
		{Key: "payout_ref", Header: refHeader, Kind: tabular.KindText, Example: "PAYOUT-20261109",
			Note: "Kosong = satu settlement per tanggal dana cair"},
		{Key: "net", Header: netHeader, Kind: tabular.KindDecimal, Required: true, Example: "185000",
			Note: "Dana yang masuk ke rekening untuk order ini"},
	}
	for _, f := range fees {
		cols = append(cols, tabular.Column{Key: f[0], Header: f[1], Kind: tabular.KindDecimal, Example: "-7500",
			Note: "Fee marketplace; tanda minus diabaikan"})
	}
	for _, p := range promos {
		cols = append(cols, tabular.Column{Key: p[0], Header: p[1], Kind: tabular.KindDecimal, Example: "-5000",
			Note: "Promo/voucher yang ditanggung penjual; tanda minus diabaikan"})
	}
	return append(cols, tabular.Column{Key: "bank_account", Header: "Akun Bank", Kind: tabular.KindText, Example: "1111",
		Note: "Kode akun Kas & Bank penerima; kosong = " + DefaultSettlementBank})
}

func keysOf(list [][2]string) []string {
	out := make([]string, len(list))
	for i, l := range list {
		out[i] = l[0]
	}
	return out
}

func newFormat(code, label, order, date, ref, net string, fees, promos [][2]string) SettlementFormat {
	return SettlementFormat{
		Marketplace: code, Label: "Settlement " + label, Columns: settlementColumns(order, date, ref, net, fees, promos),
		FeeKeys: keysOf(fees), PromoKeys: keysOf(promos),
	}
}

// SettlementFormats adalah format laporan settlement per marketplace (kode sub-channel marketplace).
var SettlementFormats = []SettlementFormat{
	newFormat("shopee", "Shopee", "No. Pesanan", "Tanggal Dana Dilepaskan", "ID Penarikan", "Total Penghasilan",
		[][2]string{{"fee_admin", "Biaya Administrasi"}, {"fee_service", "Biaya Layanan"}, {"fee_transaction", "Biaya Transaksi"}},
		[][2]string{{"promo_voucher", "Voucher Ditanggung Penjual"}, {"promo_shipping", "Promo Gratis Ongkir dari Penjual"}}),
	newFormat("tiktok_tokopedia", "TikTok Shop & Tokopedia", "ID Pesanan", "Waktu Penyelesaian", "ID Penyelesaian", "Jumlah Penyelesaian",
		[][2]string{{"fee_commission", "Biaya Komisi Platform"}, {"fee_service", "Biaya Layanan"}, {"fee_transaction", "Biaya Transaksi"}},
		[][2]string{{"promo_seller", "Diskon Penjual"}, {"promo_voucher", "Voucher Penjual"}}),
	newFormat("lazada", "Lazada", "Order No.", "Release Date", "Statement Number", "Payout Amount",
		[][2]string{{"fee_commission", "Commission"}, {"fee_payment", "Payment Fee"}, {"fee_service", "Service Fee"}},
		[][2]string{{"promo_voucher", "Seller Voucher"}, {"promo_shipping", "Shipping Fee Discount Seller"}}),
	newFormat("blibli", "Blibli", "No. Order", "Tanggal Pembayaran", "No. Pembayaran", "Total Diterima",
		[][2]string{{"fee_commission", "Biaya Komisi"}, {"fee_service", "Biaya Layanan"}},
		[][2]string{{"promo_voucher", "Voucher Seller"}}),
}

// SettlementFormatOf mencari format marketplace.
func SettlementFormatOf(code string) (SettlementFormat, bool) {
	for _, f := range SettlementFormats {
		if f.Marketplace == code {
			return f, true
		}
	}
	return SettlementFormat{}, false
}

func absSum(p *tabular.Parser, keys []string) money.Amount {
	total := decimal.Zero
	for _, k := range keys {
		if v := p.Decimal(k); v != nil {
			total = total.Add(v.Abs())
		}
	}
	return money.FromDecimal(total).Round()
}

func (s *DocumentService) marketplaceSubChannel(ctx context.Context, code string) (ChannelRef, ChannelRef, error) {
	channels, err := s.Ledger.Channels(ctx)
	if err != nil {
		return ChannelRef{}, ChannelRef{}, err
	}
	var ch, sub ChannelRef
	for _, c := range channels {
		if c.Kind == "channel" && c.Code == domain.OrderMarketplace.ChannelCode() {
			ch = c
		}
	}
	for _, c := range channels {
		if c.Kind == "sub_channel" && c.Code == code && c.ChannelID != nil && *c.ChannelID == ch.ID {
			sub = c
		}
	}
	if sub.ID == uuid.Nil {
		return ChannelRef{}, ChannelRef{}, apperr.WithMessage(apperr.CodeValidationFailed, "Marketplace "+code+" tidak aktif.")
	}
	return ch, sub, nil
}

func (s *DocumentService) checkBankAccount(ctx context.Context, code string) error {
	accs, err := s.Ledger.CashBankAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accs {
		if a.Code == code {
			return nil
		}
	}
	return apperr.Validation(map[string]string{"bank_account": "Akun " + code + " bukan akun Kas & Bank aktif"})
}

// ImportSettlementRow mencocokkan satu baris laporan settlement dengan order marketplace dan menambahkannya ke
// settlement draft (marketplace + referensi pencairan). Order harus sudah diserahkan ke kurir (piutang A-10) dan
// total settlement tidak melebihi sisa piutang. Butuh accounting.settlement.import + write.
func (s *DocumentService) ImportSettlementRow(ctx context.Context, f SettlementFormat, row tabular.Row, rowKey string) error {
	if _, err := actor.Require(ctx, PermSettlementImport); err != nil {
		return err
	}
	a, err := actor.Require(ctx, PermSettlementWrite)
	if err != nil {
		return err
	}
	p := tabular.Parse(row)
	orderNo := strings.TrimSpace(p.Text("order_no"))
	date := p.Date("payout_date")
	ref := strings.TrimSpace(p.Text("payout_ref"))
	amounts := domain.SettlementAmounts{Fee: absSum(p, f.FeeKeys), Promo: absSum(p, f.PromoKeys), Net: money.Zero}
	if v := p.Decimal("net"); v != nil {
		amounts.Net = money.FromDecimal(*v).Round()
	}
	bank := strings.TrimSpace(p.Text("bank_account"))
	if bank == "" {
		bank = DefaultSettlementBank
	}
	if err := p.Err(); err != nil {
		return err
	}
	if ref == "" && date != nil {
		ref = f.Marketplace + "-" + date.String()
	}
	if err := s.checkBankAccount(ctx, bank); err != nil {
		return err
	}
	ch, sub, err := s.marketplaceSubChannel(ctx, f.Marketplace)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, ok, err := s.Store.MarketplaceOrderForSettlement(ctx, sub.ID, orderNo)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Validation(map[string]string{"order_no": "Order " + orderNo + " tidak ditemukan di " + sub.Name})
		}
		if fields := domain.CheckSettlementLine(o, amounts); len(fields) > 0 {
			return apperr.Validation(fields)
		}
		st, found, err := s.Store.SettlementByPayout(ctx, sub.ID, ref)
		if err != nil {
			return err
		}
		if found && st.Status != domain.SettlementDraft {
			return apperr.Validation(map[string]string{"payout_ref": "Settlement " + ref + " sudah " + strings.ToLower(st.Status.Label())})
		}
		if !found {
			st, err = s.Store.InsertSettlement(ctx, domain.Settlement{
				ChannelID: ch.ID, SubChannelID: sub.ID, MarketplaceCode: f.Marketplace, PayoutReference: ref,
				SettlementDate: *date, BankAccountCode: bank,
			}, a.UserID)
			if err != nil {
				return err
			}
			if err := s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionCreate, EntityType: EntitySettlement, EntityID: st.ID, EntityLabel: st.Label(),
				After: map[string]any{"payoutReference": ref, "settlementDate": date.String()},
			}); err != nil {
				return err
			}
		}
		return s.Store.AddSettlementLine(ctx, st.ID, domain.SettlementLine{
			OrderID: o.ID, ExternalOrderID: orderNo, OrderNumber: o.Number, Receivable: o.Total,
			Fee: amounts.Fee, Promo: amounts.Promo, Net: amounts.Net, Settled: amounts.Settled(),
		}, rowKey, a.UserID)
	})
}

// SettlementView adalah settlement beserta baris dan nama marketplace.
type SettlementView struct {
	domain.Settlement
	MarketplaceName string
	Lines           []domain.SettlementLine
}

var settlementActionPerms = map[domain.Action][]string{
	domain.SettlementActionPost: {PermSettlementPost}, domain.SettlementActionCancel: {PermSettlementWrite},
}

// SettlementActions mengembalikan aksi settlement yang boleh dilakukan actor.
func (s *DocumentService) SettlementActions(ctx context.Context, st domain.Settlement) []string {
	return filterActions(ctx, st.Actions(), settlementActionPerms)
}

// SettlementFilter adalah kriteria daftar settlement.
type SettlementFilter = postgres.SettlementFilter

// ListSettlements mengembalikan daftar settlement marketplace.
func (s *DocumentService) ListSettlements(ctx context.Context, f SettlementFilter, page paging.Page) (paging.Result[domain.Settlement], map[uuid.UUID]string, error) {
	if _, err := actor.Require(ctx, PermSettlementRead); err != nil {
		return paging.Result[domain.Settlement]{}, nil, err
	}
	res, err := s.Store.ListSettlements(ctx, f, page)
	if err != nil {
		return res, nil, err
	}
	ids := make([]uuid.UUID, len(res.Items))
	for i, st := range res.Items {
		ids[i] = st.SubChannelID
	}
	names, err := s.channelNames(ctx, ids)
	return res, names, err
}

func (s *DocumentService) channelNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	refs, err := s.Ledger.ChannelLabels(ctx, uniqueIDs(ids))
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(refs))
	for id, r := range refs {
		out[id] = r.Name
	}
	return out, nil
}

// GetSettlement mengembalikan detail settlement.
func (s *DocumentService) GetSettlement(ctx context.Context, id uuid.UUID) (SettlementView, error) {
	if _, err := actor.Require(ctx, PermSettlementRead); err != nil {
		return SettlementView{}, err
	}
	st, err := s.Store.GetSettlement(ctx, id, false)
	if err != nil {
		return SettlementView{}, err
	}
	return s.settlementView(ctx, st)
}

func (s *DocumentService) settlementView(ctx context.Context, st domain.Settlement) (SettlementView, error) {
	lines, err := s.Store.SettlementLines(ctx, st.ID)
	if err != nil {
		return SettlementView{}, err
	}
	names, err := s.channelNames(ctx, []uuid.UUID{st.SubChannelID})
	if err != nil {
		return SettlementView{}, err
	}
	return SettlementView{Settlement: st, MarketplaceName: names[st.SubChannelID], Lines: lines}, nil
}

// PostSettlement memposting jurnal A-10s (Dr Bank dana cair, Dr 6105 fee, Dr 6107 promo, Cr Piutang Marketplace
// 121x) dengan dimensi channel marketplace + sub-channel, bertanggal dana cair; sisa piutang order berkurang.
func (s *DocumentService) PostSettlement(ctx context.Context, id uuid.UUID, version int) (SettlementView, error) {
	a, err := actor.Require(ctx, PermSettlementPost)
	if err != nil {
		return SettlementView{}, err
	}
	var out SettlementView
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		st, err := s.Store.GetSettlement(ctx, id, true)
		if err != nil {
			return err
		}
		if st.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		if err := st.CanPost(); err != nil {
			return err
		}
		accounts := map[accounting.Resolver]string{accounting.ResolverBank: st.BankAccountCode}
		if acct := domain.MarketplaceReceivableAccount(st.MarketplaceCode); acct != "" {
			accounts[accounting.ResolverReceivable] = acct
		}
		entry, err := s.Ledger.Post(ctx, accounting.PostingRequest{
			RuleCode: RuleMarketplaceSettlement, SourceType: SourceSettlement, SourceID: st.ID, SourceLabel: st.Label(),
			BusinessDate: st.SettlementDate, Dimensions: salesDimensions(st.ChannelID, st.SubChannelID),
			Amounts: map[string]money.Amount{
				"net_received": st.Net, "marketplace_fee": st.Fee, "marketplace_promo": st.Promo, "receivable_value": st.Settled,
			},
			Accounts:    accounts,
			Description: fmt.Sprintf("Dana cair %s (%d order)", st.Label(), st.LineCount),
		})
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		before := st.Status
		st.Status = domain.SettlementPosted
		if err := s.Store.SetSettlementStatus(ctx, st, &entry.ID, &now, a.UserID); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: EntitySettlement, EntityID: st.ID, EntityLabel: st.Label(),
			Before: map[string]any{"status": before}, After: map[string]any{"status": st.Status, "journal": entry.Number, "settledAmount": st.Settled.String()},
		}); err != nil {
			return err
		}
		st, err = s.Store.GetSettlement(ctx, id, false)
		if err != nil {
			return err
		}
		out, err = s.settlementView(ctx, st)
		return err
	})
	return out, err
}

// CancelSettlement membatalkan settlement draft (mis. salah impor) sehingga order bisa diimpor ulang.
func (s *DocumentService) CancelSettlement(ctx context.Context, id uuid.UUID, version int, reason string) (SettlementView, error) {
	a, err := actor.Require(ctx, PermSettlementWrite)
	if err != nil {
		return SettlementView{}, err
	}
	var out SettlementView
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		st, err := s.Store.GetSettlement(ctx, id, true)
		if err != nil {
			return err
		}
		if st.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		if err := st.CanCancel(reason); err != nil {
			return err
		}
		st.Status, st.CancelReason = domain.SettlementCancelled, strings.TrimSpace(reason)
		if err := s.Store.SetSettlementStatus(ctx, st, nil, nil, a.UserID); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCancel, EntityType: EntitySettlement, EntityID: st.ID, EntityLabel: st.Label(),
			After: map[string]any{"status": st.Status, "reason": st.CancelReason},
		}); err != nil {
			return err
		}
		st, err = s.Store.GetSettlement(ctx, id, false)
		if err != nil {
			return err
		}
		out, err = s.settlementView(ctx, st)
		return err
	})
	return out, err
}

// MarketplaceReceivableFilter adalah kriteria sisa piutang marketplace.
type MarketplaceReceivableFilter = postgres.MarketplaceReceivableFilter

// MarketplaceReceivables mengembalikan sisa piutang marketplace per order (F1-402).
func (s *DocumentService) MarketplaceReceivables(ctx context.Context, f MarketplaceReceivableFilter, page paging.Page) ([]postgres.MarketplaceReceivable, postgres.MarketplaceReceivableTotals, map[uuid.UUID]string, error) {
	if _, err := actor.RequireAny(ctx, PermSettlementRead, PermMarketplaceRead); err != nil {
		return nil, postgres.MarketplaceReceivableTotals{}, nil, err
	}
	f.Search = strings.TrimSpace(f.Search)
	rows, totals, err := s.Store.ListMarketplaceReceivables(ctx, f, page)
	if err != nil {
		return nil, totals, nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.SubChannelID
	}
	names, err := s.channelNames(ctx, ids)
	return rows, totals, names, err
}
