package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/pricing"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// Status dan jenis stock count gudang yang dipakai rekonsiliasi (kode warehouse).
const (
	countTypeConsignment = "consignment"
	countStatusCompleted = "completed"
)

// Jenis invoice konsinyasi (ck_invoices_invoice_type).
const (
	InvoiceTypeConsignment = "consignment"
	InvoiceTypeVariance    = "variance"
)

// ReconciliationFilter adalah kriteria daftar rekonsiliasi.
type ReconciliationFilter = postgres.ReconciliationFilter

// ReconciliationView adalah rekonsiliasi beserta label rujukan untuk detail dan cetak Berita Acara.
type ReconciliationView struct {
	domain.Reconciliation
	Products              map[uuid.UUID]ProductRef
	Location              LocationRef
	ChannelName           string
	SubChannelName        string
	PartnerReportFileName string
	BAFileName            string
	InvoiceNumber         string
	VarianceInvoiceNumber string
	CreatedByName         string
	AllowedActions        []string
}

var reconciliationActionPerms = map[domain.Action][]string{
	domain.ActionEdit: {PermReconciliationWrite}, domain.ActionStart: {PermReconciliationWrite},
	domain.ActionCancel: {PermReconciliationWrite}, domain.ActionUpdate: {PermReconciliationWrite},
	domain.ActionSubmit: {PermReconciliationSubmit}, domain.ActionRevise: {PermReconciliationWrite},
	domain.ActionPrint: {PermReconciliationRead},
}

func reconciliationSnapshot(r domain.Reconciliation) map[string]any {
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = map[string]any{
			"productId": l.ProductID.String(), "soldQty": l.SoldQty.String(), "reportedQty": l.ReportedQty.String(),
			"varianceQty": l.VarianceQty.String(), "varianceCause": string(l.Cause), "borneBy": string(l.BorneBy),
		}
	}
	return map[string]any{
		"number": r.Number, "partner": r.PartnerName, "cutOffDate": r.CutOffDate.String(), "status": string(r.Status),
		"stockCount": r.StockCountNumber, "salesAmount": r.SalesAmount.String(), "billedVariance": r.BilledVariance.String(),
		"note": r.Note, "lines": lines,
	}
}

func (s *DocumentService) reconciliationView(ctx context.Context, r domain.Reconciliation) (ReconciliationView, error) {
	v := ReconciliationView{Reconciliation: r}
	ids := make([]uuid.UUID, len(r.Lines))
	for i, l := range r.Lines {
		ids[i] = l.ProductID
	}
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return ReconciliationView{}, err
	}
	locs, err := s.Locations.Locations(ctx, []uuid.UUID{r.LocationID})
	if err != nil {
		return ReconciliationView{}, err
	}
	v.Location = locs[r.LocationID]
	labels, err := s.Ledger.ChannelLabels(ctx, []uuid.UUID{r.ChannelID, r.SubChannelID})
	if err != nil {
		return ReconciliationView{}, err
	}
	v.ChannelName, v.SubChannelName = labels[r.ChannelID].Name, labels[r.SubChannelID].Name
	var files []uuid.UUID
	for _, f := range []*uuid.UUID{r.PartnerReportFileID, r.BAFileID} {
		if f != nil {
			files = append(files, *f)
		}
	}
	if len(files) > 0 {
		names, err := s.Files.Names(ctx, files)
		if err != nil {
			return ReconciliationView{}, err
		}
		if r.PartnerReportFileID != nil {
			v.PartnerReportFileName = names[*r.PartnerReportFileID]
		}
		if r.BAFileID != nil {
			v.BAFileName = names[*r.BAFileID]
		}
	}
	for _, p := range []struct {
		id  *uuid.UUID
		out *string
	}{{r.InvoiceID, &v.InvoiceNumber}, {r.VarianceInvoiceID, &v.VarianceInvoiceNumber}} {
		if p.id != nil {
			inv, err := s.Store.GetInvoice(ctx, *p.id)
			if err != nil {
				return ReconciliationView{}, err
			}
			*p.out = inv.Number
		}
	}
	names, err := s.Users.UserNames(ctx, []uuid.UUID{r.CreatedBy})
	if err != nil {
		return ReconciliationView{}, err
	}
	v.CreatedByName = names[r.CreatedBy]
	v.AllowedActions = filterActions(ctx, r.Actions(), reconciliationActionPerms)
	return v, nil
}

// ListReconciliations mengembalikan daftar Rekonsiliasi Konsinyasi.
func (s *DocumentService) ListReconciliations(ctx context.Context, f ReconciliationFilter, page paging.Page) (paging.Result[domain.Reconciliation], error) {
	if _, err := actor.Require(ctx, PermReconciliationRead); err != nil {
		return paging.Result[domain.Reconciliation]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListReconciliations(ctx, f, page)
}

// GetReconciliation mengembalikan detail rekonsiliasi.
func (s *DocumentService) GetReconciliation(ctx context.Context, id uuid.UUID) (ReconciliationView, error) {
	if _, err := actor.Require(ctx, PermReconciliationRead); err != nil {
		return ReconciliationView{}, err
	}
	r, err := s.Store.GetReconciliation(ctx, id, false)
	if err != nil {
		return ReconciliationView{}, err
	}
	return s.reconciliationView(ctx, r)
}

// reconciliationRefs memeriksa mitra, lokasi konsinyasi mitra, stock count (konsinyasi, Selesai, lokasi mitra,
// ±7 hari dari cut-off), dan dimensi channel konsinyasi + sub-channel mitra.
func (s *DocumentService) reconciliationRefs(ctx context.Context, r *domain.Reconciliation) error {
	if err := s.requireConsignmentDeps(); err != nil {
		return err
	}
	p, ok, err := s.Parties.Partner(ctx, r.PartnerID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Validation(map[string]string{"partnerId": "Mitra tidak ditemukan"})
	}
	r.PartnerCode, r.PartnerName, r.PartnerAddress = p.Code, p.Name, p.Address
	loc, ok, err := s.Consignment.PartnerLocation(ctx, domain.DeliveryConsignment.LocationType(), r.PartnerID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Validation(map[string]string{"partnerId": "Mitra belum punya lokasi konsinyasi"})
	}
	r.LocationID = loc.ID
	sc, err := s.Consignment.StockCount(ctx, r.StockCountID)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return apperr.Validation(map[string]string{"stockCountId": "Stock count tidak ditemukan"})
	}
	if err != nil {
		return err
	}
	switch {
	case sc.CountType != countTypeConsignment || sc.LocationID != loc.ID:
		return apperr.Validation(map[string]string{"stockCountId": "Pilih stock count konsinyasi di lokasi mitra ini"})
	case sc.Status != countStatusCompleted:
		return apperr.Validation(map[string]string{"stockCountId": "Stock count lokasi mitra belum selesai"})
	case sc.BusinessDate.Before(r.CutOffDate.AddDays(-countWindowDays)) || sc.BusinessDate.After(r.CutOffDate.AddDays(countWindowDays)):
		return apperr.Validation(map[string]string{"stockCountId": fmt.Sprintf(
			"Tanggal stock count (%s) harus dalam %d hari dari cut-off", sc.BusinessDate.String(), countWindowDays)})
	}
	r.StockCountNumber = sc.Number
	r.ChannelID, r.SubChannelID, err = s.partnerSubChannel(ctx, domain.DeliveryConsignment.ChannelCode(), r.PartnerID)
	return err
}

// CreateReconciliation membuat Rekonsiliasi Konsinyasi Draft.
func (s *DocumentService) CreateReconciliation(ctx context.Context, in domain.ReconciliationInput) (ReconciliationView, error) {
	a, err := actor.Require(ctx, PermReconciliationWrite)
	if err != nil {
		return ReconciliationView{}, err
	}
	r, err := domain.NewReconciliation(in)
	if err != nil {
		return ReconciliationView{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.reconciliationRefs(ctx, &r); err != nil {
			return err
		}
		if r, err = s.Store.InsertReconciliation(ctx, r, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityReconciliation, EntityID: r.ID, EntityLabel: r.Label(),
			After: reconciliationSnapshot(r),
		})
	})
	if err != nil {
		return ReconciliationView{}, err
	}
	return s.GetReconciliation(ctx, r.ID)
}

func (s *DocumentService) mutateReconciliation(ctx context.Context, id uuid.UUID, ver int, perm string, action audit.Action,
	mode postgres.LineMode, fn func(ctx context.Context, r *domain.Reconciliation) (string, error),
) (ReconciliationView, error) {
	if _, err := actor.Require(ctx, perm); err != nil {
		return ReconciliationView{}, err
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		r, err := s.Store.GetReconciliation(ctx, id, true)
		if err != nil {
			return err
		}
		if r.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		return s.saveReconciliation(ctx, &r, action, mode, fn)
	})
	if err != nil {
		return ReconciliationView{}, err
	}
	return s.GetReconciliation(ctx, id)
}

func (s *DocumentService) saveReconciliation(ctx context.Context, r *domain.Reconciliation, action audit.Action,
	mode postgres.LineMode, fn func(ctx context.Context, r *domain.Reconciliation) (string, error),
) error {
	before := reconciliationSnapshot(*r)
	note, err := fn(ctx, r)
	if err != nil {
		return err
	}
	if err := s.Store.SaveReconciliation(ctx, r, r.Version, actor.UserID(ctx), mode); err != nil {
		return err
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: action, EntityType: EntityReconciliation, EntityID: r.ID, EntityLabel: r.Label(),
		Before: before, After: reconciliationSnapshot(*r), Note: note,
	})
}

// UpdateReconciliationDraft mengubah cut-off, stock count, dan catatan rekonsiliasi Draft.
func (s *DocumentService) UpdateReconciliationDraft(ctx context.Context, id uuid.UUID, ver int, in domain.ReconciliationInput) (ReconciliationView, error) {
	return s.mutateReconciliation(ctx, id, ver, PermReconciliationWrite, audit.ActionUpdate, postgres.LinesKeep,
		func(ctx context.Context, r *domain.Reconciliation) (string, error) {
			if err := r.UpdateDraft(in); err != nil {
				return "", err
			}
			return "", s.reconciliationRefs(ctx, r)
		})
}

// StartReconciliation memulai rekonsiliasi (Desain Status Dokumen 5.6): PKS berlaku pada cut-off disalin (harga,
// termin, ketentuan selisih), nomor KSN, dan per SKU: stok awal (saldo lokasi mitra sebelum periode), kiriman,
// tarikan (dari ledger inventory), sisa fisik (stock count), terjual sistem, dan harga konsinyasi (pricing).
func (s *DocumentService) StartReconciliation(ctx context.Context, id uuid.UUID, ver int) (ReconciliationView, error) {
	return s.mutateReconciliation(ctx, id, ver, PermReconciliationWrite, audit.ActionStatusChange, postgres.LinesReplace,
		func(ctx context.Context, r *domain.Reconciliation) (string, error) {
			if err := s.reconciliationRefs(ctx, r); err != nil {
				return "", err
			}
			ag, err := s.Agreements.RequireConsignment(ctx, r.PartnerID, r.CutOffDate)
			if err != nil {
				return "", err
			}
			r.AgreementID, r.AgreementNumber, r.PaymentTermDays = &ag.ID, ag.Number, ag.PaymentTermDays
			r.MarginPercent, r.Rules = ag.MarginPercent, ag.Rules.Normalize()
			last, err := s.Store.LastPostedCutOff(ctx, r.PartnerID)
			if err != nil {
				return "", err
			}
			start := domain.PeriodStartAfter(last, r.CutOffDate)
			src, err := s.reconciliationSources(ctx, r, ag, start)
			if err != nil {
				return "", err
			}
			number, err := s.Numberer.Next(ctx, DocReconciliation, r.CutOffDate)
			if err != nil {
				return "", err
			}
			return "Rekonsiliasi dimulai", r.Start(number, start, src, s.Clock.Now())
		})
}

func (s *DocumentService) reconciliationSources(ctx context.Context, r *domain.Reconciliation, ag AgreementRef, start clock.BusinessDate) ([]domain.LineSource, error) {
	if start.After(r.CutOffDate) {
		return nil, apperr.Validation(map[string]string{"cutOffDate": "Cut-off harus setelah cut-off rekonsiliasi sebelumnya"})
	}
	flows, err := s.Flows.LocationFlows(ctx, inventory.LocationFlowQuery{
		LocationID: r.LocationID, DateFrom: start, DateTo: r.CutOffDate, ExcludeSources: []string{SourceReconciliation},
	})
	if err != nil {
		return nil, err
	}
	sc, err := s.Consignment.StockCount(ctx, r.StockCountID)
	if err != nil {
		return nil, err
	}
	byProduct := map[uuid.UUID]*domain.LineSource{}
	get := func(id uuid.UUID) *domain.LineSource {
		if l, ok := byProduct[id]; ok {
			return l
		}
		l := &domain.LineSource{ProductID: id, OpeningQty: decimal.Zero, DeliveredQty: decimal.Zero, ReturnedQty: decimal.Zero, CountedQty: decimal.Zero}
		byProduct[id] = l
		return l
	}
	for _, f := range flows {
		l := get(f.ProductID)
		l.OpeningQty, l.DeliveredQty, l.ReturnedQty = f.OpeningQty, f.InQty, f.OutQty
	}
	for _, c := range sc.Lines {
		l := get(c.ProductID)
		l.CountedQty = l.CountedQty.Add(c.CountedQty)
	}
	ids := make([]uuid.UUID, 0, len(byProduct))
	for id, l := range byProduct {
		if l.OpeningQty.IsZero() && l.DeliveredQty.IsZero() && l.ReturnedQty.IsZero() && l.CountedQty.IsZero() {
			delete(byProduct, id)
			continue
		}
		ids = append(ids, id)
	}
	products, err := s.Catalog.Products(ctx, ids)
	if err != nil {
		return nil, err
	}
	sort.Slice(ids, func(i, j int) bool { return products[ids[i]].SKU < products[ids[j]].SKU })
	out := make([]domain.LineSource, 0, len(ids))
	f := map[string]string{}
	for _, id := range ids {
		l := byProduct[id]
		pid, ch, sub := r.PartnerID, r.ChannelID, r.SubChannelID
		q, err := s.Pricing.Resolve(ctx, pricing.PriceRequest{
			ProductID: id, Qty: decimal.NewFromInt(1), ChannelID: &ch, SubChannelID: &sub, PartnerID: &pid, Date: r.CutOffDate,
		})
		if code := apperr.As(err).Code; err != nil && (code == apperr.CodeNotFound || code == apperr.CodeValidationFailed) {
			f["lines"] = "Harga konsinyasi belum ada untuk " + products[id].Label()
			continue
		}
		if err != nil {
			return nil, err
		}
		fromPKS := ag.PriceListID != nil && q.PriceListID != nil && *q.PriceListID == *ag.PriceListID
		l.UnitPrice = domain.ConsignmentUnitPrice(q.UnitPrice, fromPKS, ag.MarginPercent)
		l.PriceRule, l.PriceDescription = string(q.Rule), q.Description
		if !fromPKS && ag.MarginPercent != nil && !ag.MarginPercent.IsZero() {
			l.PriceDescription = fmt.Sprintf("%s − margin mitra %s%%", q.Description, ag.MarginPercent.String())
		}
		out = append(out, *l)
	}
	if len(f) > 0 {
		return nil, apperr.Validation(f)
	}
	return out, nil
}

// ReconciliationUpdate adalah isian Sales MT pada rekonsiliasi Diproses.
type ReconciliationUpdate = domain.ReconciliationUpdate

// UpdateReconciliation mencatat laporan penjualan mitra per SKU, penyebab selisih, file laporan mitra & BA
// bertanda tangan, dan catatan.
func (s *DocumentService) UpdateReconciliation(ctx context.Context, id uuid.UUID, ver int, u ReconciliationUpdate) (ReconciliationView, error) {
	return s.mutateReconciliation(ctx, id, ver, PermReconciliationWrite, audit.ActionUpdate, postgres.LinesUpdate,
		func(ctx context.Context, r *domain.Reconciliation) (string, error) {
			oldReport, oldBA := r.PartnerReportFileID, r.BAFileID
			if err := r.Update(u); err != nil {
				return "", err
			}
			for _, f := range []struct {
				id, old *uuid.UUID
				owner   string
			}{{r.PartnerReportFileID, oldReport, FileOwnerConsignmentReport}, {r.BAFileID, oldBA, FileOwnerConsignmentBA}} {
				if f.id != nil && (f.old == nil || *f.old != *f.id) {
					if err := s.Files.Attach(ctx, *f.id, f.owner, r.ID); err != nil {
						return "", err
					}
				}
			}
			return "", nil
		})
}

// SubmitReconciliation mengajukan Berita Acara Rekonsiliasi ke approval `consignment_reconciliation`
// (Sales MT → Head Finance & Accounting) dengan nominal tagihan.
func (s *DocumentService) SubmitReconciliation(ctx context.Context, id uuid.UUID, ver int) (ReconciliationView, error) {
	return s.mutateReconciliation(ctx, id, ver, PermReconciliationSubmit, audit.ActionSubmit, postgres.LinesKeep,
		func(ctx context.Context, r *domain.Reconciliation) (string, error) {
			if err := r.Submit(s.Clock.Now()); err != nil {
				return "", err
			}
			summary := fmt.Sprintf("%s · cut-off %s · penjualan %s", r.PartnerName, r.CutOffDate.String(), r.SalesAmount.Rupiah())
			if r.HasVariance() {
				summary += " · selisih ditagih " + r.BilledVariance.Rupiah()
			}
			_, err := s.Approvals.Request(ctx, approval.Subject{
				Type: approval.SubjectConsignmentReconciliation, ID: r.ID, Label: r.Number, Link: ReconciliationPath(r.ID),
				Summary: summary, Amount: r.ApprovalAmount(),
			})
			return "", err
		})
}

// ReviseReconciliation membuka kembali rekonsiliasi yang ditolak.
func (s *DocumentService) ReviseReconciliation(ctx context.Context, id uuid.UUID, ver int) (ReconciliationView, error) {
	return s.mutateReconciliation(ctx, id, ver, PermReconciliationWrite, audit.ActionStatusChange, postgres.LinesKeep,
		func(_ context.Context, r *domain.Reconciliation) (string, error) { return "Direvisi", r.Revise() })
}

// CancelReconciliation membatalkan rekonsiliasi Draft/Diproses.
func (s *DocumentService) CancelReconciliation(ctx context.Context, id uuid.UUID, ver int, reason string) (ReconciliationView, error) {
	return s.mutateReconciliation(ctx, id, ver, PermReconciliationWrite, audit.ActionCancel, postgres.LinesKeep,
		func(_ context.Context, r *domain.Reconciliation) (string, error) {
			return strings.TrimSpace(reason), r.Cancel(reason)
		})
}

// OnReconciliationRejected menandai rekonsiliasi Ditolak (bisa direvisi).
func (s *DocumentService) OnReconciliationRejected(ctx context.Context, d approval.Decided) error {
	r, err := s.Store.GetReconciliation(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	return s.saveReconciliation(ctx, &r, audit.ActionReject, postgres.LinesKeep, func(_ context.Context, r *domain.Reconciliation) (string, error) {
		return d.Note, r.Reject(d.Note, s.Clock.Now())
	})
}

// OnReconciliationApproved memposting rekonsiliasi yang disetujui level terakhir (S-305), di transaksi keputusan:
//   - stok terjual (laporan mitra) dan selisih keluar dari lokasi konsinyasi (HPP FIFO), bertanggal cut-off;
//   - invoice konsinyasi (termin PKS) + A-08: Dr 1201 / Cr 4101 sebesar harga konsinyasi, Dr 5101 / Cr 1312 HPP;
//   - selisih ditagih ke mitra: invoice selisih + A-23 seperti A-08; ditanggung Balans: A-23 Dr 6404 / Cr 1312.
//
// Jurnal bertanggal cut-off dengan dimensi channel konsinyasi + sub-channel mitra (pengakuan penjualan konsinyasi
// saat rekonsiliasi, U-25); invoice bertanggal hari persetujuan dan jatuh tempo + termin PKS.
func (s *DocumentService) OnReconciliationApproved(ctx context.Context, d approval.Decided) error {
	r, err := s.Store.GetReconciliation(ctx, d.Subject.ID, true)
	if err != nil {
		return err
	}
	return s.saveReconciliation(ctx, &r, audit.ActionApprove, postgres.LinesUpdate, func(ctx context.Context, r *domain.Reconciliation) (string, error) {
		costs, err := s.issueReconciliation(ctx, r)
		if err != nil {
			return "", err
		}
		byLine := map[uuid.UUID]domain.LineCost{}
		for _, c := range costs {
			byLine[c.LineID] = c
		}
		soldCost, billedCost, absorbedCost := money.Zero, money.Zero, money.Zero
		for _, l := range r.Lines {
			c := byLine[l.ID]
			soldCost = soldCost.Add(c.Cost)
			if l.BorneBy == domain.BorneByBalans {
				absorbedCost = absorbedCost.Add(c.VarianceCost)
			} else {
				billedCost = billedCost.Add(c.VarianceCost)
			}
		}
		products, err := s.reconciliationProducts(ctx, r)
		if err != nil {
			return "", err
		}
		invoiceID, err := s.reconciliationInvoice(ctx, r, InvoiceTypeConsignment, products)
		if err != nil {
			return "", err
		}
		varianceID, err := s.reconciliationInvoice(ctx, r, InvoiceTypeVariance, products)
		if err != nil {
			return "", err
		}
		dims := salesDimensions(r.ChannelID, r.SubChannelID)
		party := &accounting.Party{Type: domain.PartyPartner, ID: r.PartnerID}
		if r.SalesAmount.IsPositive() || soldCost.IsPositive() {
			if _, err := s.Ledger.Post(ctx, accounting.PostingRequest{
				RuleCode: RuleConsignmentSales, SourceType: SourceReconciliation, SourceID: r.ID, SourceLabel: r.Number,
				BusinessDate: r.CutOffDate, Dimensions: dims, Party: party,
				Amounts: map[string]money.Amount{
					"receivable_value": r.SalesAmount, "gross_sales": r.SalesAmount, "cogs_value": soldCost,
				},
				Description: fmt.Sprintf("Penjualan konsinyasi %s %s", r.PartnerName, r.Number),
			}); err != nil {
				return "", err
			}
		}
		if r.BilledVariance.IsPositive() || billedCost.IsPositive() || absorbedCost.IsPositive() {
			if _, err := s.Ledger.Post(ctx, accounting.PostingRequest{
				RuleCode: RuleConsignmentVariance, SourceType: SourceReconciliation, SourceID: r.ID, SourceLabel: r.Number,
				BusinessDate: r.CutOffDate, Dimensions: dims, Party: party,
				Amounts: map[string]money.Amount{
					"billed_receivable": r.BilledVariance, "billed_sales": r.BilledVariance, "billed_cogs": billedCost,
					"absorbed_value": absorbedCost,
				},
				Description: fmt.Sprintf("Selisih konsinyasi %s %s", r.PartnerName, r.Number),
			}); err != nil {
				return "", err
			}
		}
		return d.Note, r.MarkPosted(costs, invoiceID, varianceID, d.Note, s.Clock.Now())
	})
}

func (s *DocumentService) reconciliationProducts(ctx context.Context, r *domain.Reconciliation) (map[uuid.UUID]ProductRef, error) {
	ids := make([]uuid.UUID, len(r.Lines))
	for i, l := range r.Lines {
		ids[i] = l.ProductID
	}
	return s.Catalog.Products(ctx, uniqueIDs(ids))
}

// issueReconciliation mengeluarkan qty terjual dan selisih dari lokasi konsinyasi (FEFO, HPP FIFO).
func (s *DocumentService) issueReconciliation(ctx context.Context, r *domain.Reconciliation) ([]domain.LineCost, error) {
	type ref struct {
		line     uuid.UUID
		variance bool
	}
	var (
		lines []inventory.Line
		refs  []ref
	)
	place := &inventory.Place{LocationID: r.LocationID, Status: inventory.StatusAvailable}
	for _, l := range r.Lines {
		lineID := l.ID
		if l.ReportedQty.IsPositive() {
			lines = append(lines, inventory.Line{Kind: inventory.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.ReportedQty, From: place, Note: "Terjual"})
			refs = append(refs, ref{line: l.ID})
		}
		if l.VarianceQty.IsPositive() {
			lines = append(lines, inventory.Line{Kind: inventory.KindIssue, SourceLineID: &lineID, ProductID: l.ProductID, Qty: l.VarianceQty, From: place,
				Note: "Selisih: " + l.Cause.Label()})
			refs = append(refs, ref{line: l.ID, variance: true})
		}
	}
	costs := map[uuid.UUID]*domain.LineCost{}
	out := make([]domain.LineCost, 0, len(r.Lines))
	for _, l := range r.Lines {
		costs[l.ID] = &domain.LineCost{LineID: l.ID, Cost: money.Zero, VarianceCost: money.Zero}
	}
	if len(lines) > 0 {
		res, err := s.Stock.Post(ctx, inventory.Posting{
			Source: inventory.Source{Type: SourceReconciliation, ID: r.ID, Action: "post", Label: r.Number, BusinessDate: r.CutOffDate},
			Lines:  lines,
		})
		if err != nil {
			return nil, err
		}
		for i, lr := range res.Lines {
			c := costs[refs[i].line]
			if refs[i].variance {
				c.VarianceCost = c.VarianceCost.Add(money.FromDecimal(lr.CostAmount))
			} else {
				c.Cost = c.Cost.Add(money.FromDecimal(lr.CostAmount))
			}
		}
	}
	for _, l := range r.Lines {
		out = append(out, *costs[l.ID])
	}
	return out, nil
}

// reconciliationInvoice menerbitkan invoice konsinyasi (barang terjual) atau invoice selisih (ditagih ke mitra)
// NON-PKP dengan jatuh tempo sesuai termin PKS; nil bila nilainya nol. Jurnalnya diposting OnReconciliationApproved.
func (s *DocumentService) reconciliationInvoice(ctx context.Context, r *domain.Reconciliation, invoiceType string, products map[uuid.UUID]ProductRef) (*uuid.UUID, error) {
	total := r.SalesAmount
	if invoiceType == InvoiceTypeVariance {
		total = r.BilledVariance
	}
	if !total.IsPositive() {
		return nil, nil
	}
	date := s.today()
	number, err := s.Numberer.Next(ctx, DocInvoice, date)
	if err != nil {
		return nil, err
	}
	inv := domain.Invoice{
		Number: number, Type: invoiceType, PartyType: domain.PartyPartner, PartyID: r.PartnerID, PartyCode: r.PartnerCode,
		PartyName: r.PartnerName, ChannelID: r.ChannelID, SubChannelID: r.SubChannelID, InvoiceDate: date,
		DueDate: date.AddDays(r.PaymentTermDays), Gross: total, Discount: money.Zero, Shipping: money.Zero, Total: total,
		Paid: money.Zero, Status: domain.StatusAwaitingPayment,
	}
	for _, l := range r.Lines {
		pid := l.ProductID
		label := products[l.ProductID].Label()
		qty, amt := l.ReportedQty, l.SalesAmount
		if invoiceType == InvoiceTypeVariance {
			qty, amt = l.VarianceQty, l.VarianceAmount
			label = fmt.Sprintf("%s (selisih: %s)", label, l.Cause.Label())
		}
		if !amt.IsPositive() {
			continue
		}
		inv.Lines = append(inv.Lines, domain.InvoiceLine{
			LineNo: len(inv.Lines) + 1, ProductID: &pid, Description: label, Qty: qty, UnitPrice: l.UnitPrice,
			Discount: money.Zero, Total: amt,
		})
	}
	id, err := s.Store.InsertInvoice(ctx, inv, actor.UserID(ctx))
	if err != nil {
		return nil, err
	}
	return &id, s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionPost, EntityType: EntityInvoice, EntityID: id, EntityLabel: number,
		After: map[string]any{"consignmentReconciliation": r.Number, "invoiceType": invoiceType, "totalAmount": total.String(),
			"dueDate": inv.DueDate.String()},
	})
}
