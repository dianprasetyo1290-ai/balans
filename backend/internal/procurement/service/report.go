package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/procurement/postgres"
)

// AccountGRNI adalah akun Barang Diterima Belum Ditagih (COA Desain Finance & Accounting 1).
const AccountGRNI = "2102"

// GRNIFilter adalah filter laporan Barang Diterima Belum Ditagih.
type GRNIFilter = postgres.GRNIFilter

// GRNIReport adalah laporan Barang Diterima Belum Ditagih per PO/GR beserta rekonsiliasi ke saldo akun 2102 (S-209,
// ACC-22).
type GRNIReport struct {
	AsOf     clock.BusinessDate
	Lines    []postgres.OpenReceiptLine
	Products map[uuid.UUID]ProductRef
	// FilteredTotal adalah total saldo baris yang tampil (mengikuti filter).
	FilteredTotal money.Amount
	// SubledgerTotal adalah total saldo semua GR (tanpa filter) — dibandingkan dengan saldo akun 2102.
	SubledgerTotal money.Amount
	LedgerBalance  money.Amount
}

// Difference adalah selisih saldo akun 2102 dengan total sub-ledger GR (harus nol).
func (r GRNIReport) Difference() money.Amount { return r.LedgerBalance.Sub(r.SubledgerTotal) }

// GRNIReport menyusun laporan Barang Diterima Belum Ditagih per hari ini. Hak: tagihan pemasok (baca) atau laporan
// keuangan.
func (s *Service) GRNIReport(ctx context.Context, f GRNIFilter) (GRNIReport, error) {
	if _, err := actor.RequireAny(ctx, PermBillRead, PermReportRead); err != nil {
		return GRNIReport{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	out := GRNIReport{AsOf: s.today(), FilteredTotal: money.Zero}
	var err error
	if out.Lines, err = s.Store.OpenReceiptLines(ctx, f); err != nil {
		return GRNIReport{}, err
	}
	ids := make([]uuid.UUID, 0, len(out.Lines))
	for _, l := range out.Lines {
		out.FilteredTotal = out.FilteredTotal.Add(l.OpenAmount())
		ids = append(ids, l.ProductID)
	}
	if out.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return GRNIReport{}, err
	}
	if out.SubledgerTotal, err = s.Store.TotalOpenGRNI(ctx); err != nil {
		return GRNIReport{}, err
	}
	if out.LedgerBalance, err = s.Ledger.AccountBalance(ctx, AccountGRNI, out.AsOf); err != nil {
		return GRNIReport{}, err
	}
	return out, nil
}
