package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/accounting"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// Akun default pembayaran pelanggan (COA Desain Finance & Accounting 1).
const (
	AccountReceivable = "1201"
	AccountInventory  = "1311"
	cashBankPrefix    = "11"
)

func accountingReversal(sourceType string, id uuid.UUID, rule string, date clock.BusinessDate, reason string) accounting.ReversalRequest {
	return accounting.ReversalRequest{SourceType: sourceType, SourceID: id, RuleCode: rule, BusinessDate: date, Reason: reason}
}

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

func salesDimensions(channelID, subChannelID uuid.UUID) accounting.Dimensions {
	return accounting.Dimensions{ChannelID: uuidPtr(channelID), SubChannelID: uuidPtr(subChannelID)}
}

// issueInvoice menerbitkan invoice dari pesanan dan memposting A-05 (Dr Piutang Usaha + Potongan Penjualan,
// Cr Penjualan sebelum diskon + Pendapatan Ongkos Kirim) dengan dimensi channel & sub-channel pesanan.
func (s *DocumentService) issueInvoice(ctx context.Context, o *domain.Order, date clock.BusinessDate) (domain.Invoice, error) {
	number, err := s.Numberer.Next(ctx, DocInvoice, date)
	if err != nil {
		return domain.Invoice{}, err
	}
	ids := make([]uuid.UUID, len(o.Lines))
	for i, l := range o.Lines {
		ids[i] = l.ProductID
	}
	products, err := s.Catalog.Products(ctx, uniqueIDs(ids))
	if err != nil {
		return domain.Invoice{}, err
	}
	inv, err := domain.NewInvoiceFromOrder(number, *o, date, func(id uuid.UUID) string {
		if p, ok := products[id]; ok {
			return p.Label()
		}
		return id.String()
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	if inv.ID, err = s.Store.InsertInvoice(ctx, inv, actor.UserID(ctx)); err != nil {
		return domain.Invoice{}, err
	}
	if _, err := s.Ledger.Post(ctx, accounting.PostingRequest{
		RuleCode: RuleSales, SourceType: SourceInvoice, SourceID: inv.ID, SourceLabel: inv.Number, BusinessDate: date,
		Amounts: map[string]money.Amount{
			"gross_sales": inv.Gross, "discount_value": inv.Discount, "shipping_income": inv.Shipping,
			"receivable_value": inv.Total,
		},
		Accounts:    map[accounting.Resolver]string{accounting.ResolverReceivable: AccountReceivable},
		Dimensions:  salesDimensions(inv.ChannelID, inv.SubChannelID),
		Party:       &accounting.Party{Type: inv.PartyType, ID: inv.PartyID},
		Description: fmt.Sprintf("Penjualan %s %s", o.Type.Label(), o.Number),
	}); err != nil {
		return domain.Invoice{}, err
	}
	return inv, s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionPost, EntityType: EntityInvoice, EntityID: inv.ID, EntityLabel: inv.Number,
		After: map[string]any{"salesOrder": o.Number, "totalAmount": inv.Total.String(), "dueDate": inv.DueDate.String()},
	})
}

// --- Proforma Invoice ------------------------------------------------------------------------

// ProformaFilter adalah kriteria daftar PI.
type ProformaFilter = postgres.ProformaFilter

// ProformaRowView adalah PI beserta ringkasan pesanan (daftar).
type ProformaRowView = postgres.ProformaRow

// ProformaView adalah PI beserta pesanan.
type ProformaView struct {
	postgres.ProformaRow
	Order OrderView
}

var proformaActionPerms = map[domain.Action][]string{
	domain.ActionConfirmPay: {PermInvoicePost}, domain.ActionPrint: {PermProformaRead},
}

// ProformaActions mengembalikan aksi PI yang boleh dilakukan actor.
func (s *DocumentService) ProformaActions(ctx context.Context, p domain.Proforma) []string {
	return filterActions(ctx, p.Actions(s.today()), proformaActionPerms)
}

// ListProformas mengembalikan daftar PI.
func (s *DocumentService) ListProformas(ctx context.Context, f ProformaFilter, page paging.Page) (paging.Result[postgres.ProformaRow], error) {
	if _, err := actor.RequireAny(ctx, PermProformaRead, PermInvoiceRead); err != nil {
		return paging.Result[postgres.ProformaRow]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListProformas(ctx, f, page)
}

// GetProforma mengembalikan detail PI beserta pesanan (cetak PI).
func (s *DocumentService) GetProforma(ctx context.Context, id uuid.UUID) (ProformaView, error) {
	if _, err := actor.RequireAny(ctx, PermProformaRead, PermInvoiceRead); err != nil {
		return ProformaView{}, err
	}
	p, err := s.Store.GetProforma(ctx, id)
	if err != nil {
		return ProformaView{}, err
	}
	o, err := s.Store.GetOrder(ctx, p.OrderID, false)
	if err != nil {
		return ProformaView{}, err
	}
	v, err := s.orderView(ctx, o)
	if err != nil {
		return ProformaView{}, err
	}
	return ProformaView{ProformaRow: p, Order: v}, nil
}

// ProformaPaymentInput adalah konfirmasi pembayaran PI oleh F&A.
type ProformaPaymentInput struct {
	PaymentDate clock.BusinessDate
	Method      domain.PaymentMethod
	AccountCode string
	Reference   string
	ProofFileID *uuid.UUID
}

// ConfirmProformaPayment: F&A mengonfirmasi pembayaran PI → invoice final terbit Lunas (A-05) dan pembayaran
// tercatat dialokasikan ke invoice (A-PB, Dr Kas/Bank Cr Piutang Usaha) di transaksi yang sama; pesanan Diproses
// dan siap dikirim (BRD 5.4, D-09).
func (s *DocumentService) ConfirmProformaPayment(ctx context.Context, id uuid.UUID, ver int, in ProformaPaymentInput) (ProformaView, error) {
	if _, err := actor.Require(ctx, PermInvoicePost); err != nil {
		return ProformaView{}, err
	}
	if in.PaymentDate.IsZero() {
		in.PaymentDate = s.today()
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		pi, err := s.Store.LockProforma(ctx, id)
		if err != nil {
			return err
		}
		if pi.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		o, err := s.Store.GetOrder(ctx, pi.OrderID, true)
		if err != nil {
			return err
		}
		if pi.IsExpired(s.today()) {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Proforma Invoice %s sudah lewat masa berlaku (%s).", pi.Number, pi.ValidUntil.String()))
		}
		if o.Status != domain.StatusAwaitingPayment {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Sales Order %s berstatus %s; pembayaran Proforma tidak bisa dikonfirmasi.", o.Label(), o.Status.Label()))
		}
		date := s.today()
		inv, err := s.issueInvoice(ctx, &o, date)
		if err != nil {
			return err
		}
		if _, err := s.recordPayment(ctx, domain.PaymentInput{
			PartyType: inv.PartyType, PartyID: inv.PartyID, PaymentDate: in.PaymentDate, Amount: inv.Total,
			Method: in.Method, AccountCode: in.AccountCode, Reference: in.Reference, ProofFileID: in.ProofFileID,
			Note:        "Pembayaran Proforma " + pi.Number,
			Allocations: []domain.PaymentAllocation{{InvoiceID: inv.ID, Amount: inv.Total}},
		}); err != nil {
			return err
		}
		before := pi.Status
		if err := pi.MarkPaid(inv.ID, s.today(), s.Clock.Now()); err != nil {
			return err
		}
		if err := s.Store.SaveProforma(ctx, pi, ver, actor.UserID(ctx)); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityProforma, EntityID: pi.ID, EntityLabel: pi.Number,
			Before: map[string]any{"status": string(before)}, After: map[string]any{"status": string(pi.Status), "invoice": inv.Number},
		}); err != nil {
			return err
		}
		return s.saveOrderChange(ctx, &o, audit.ActionStatusChange, false, func(context.Context, *domain.Order) (string, error) {
			return "Pembayaran dikonfirmasi, invoice " + inv.Number, o.MarkPaid()
		})
	})
	if err != nil {
		return ProformaView{}, err
	}
	return s.GetProforma(ctx, id)
}

// ExpireProformas menandai PI belum dibayar yang lewat masa berlaku sebagai Kedaluwarsa, mengubah pesanan menjadi
// Kedaluwarsa, dan melepas alokasi stoknya. Idempotent; dipakai job sales.expire_proformas dan test.
func (s *DocumentService) ExpireProformas(ctx context.Context) (int, error) {
	ctx = actor.System(ctx)
	ids, err := s.Store.ExpiredProformaIDs(ctx, s.today())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
			pi, err := s.Store.LockProforma(ctx, id)
			if err != nil || !pi.IsExpired(s.today()) {
				return err
			}
			ver := pi.Version
			if err := pi.Expire(); err != nil {
				return err
			}
			if err := s.Store.SaveProforma(ctx, pi, ver, actor.UserID(ctx)); err != nil {
				return err
			}
			if err := s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionStatusChange, EntityType: EntityProforma, EntityID: pi.ID, EntityLabel: pi.Number,
				Before: map[string]any{"status": string(domain.StatusAwaitingPayment)}, After: map[string]any{"status": string(pi.Status)},
				Note: "Lewat masa berlaku " + pi.ValidUntil.String(),
			}); err != nil {
				return err
			}
			o, err := s.Store.GetOrder(ctx, pi.OrderID, true)
			if err != nil {
				return err
			}
			if o.Status != domain.StatusAwaitingPayment {
				return nil
			}
			return s.saveOrderChange(ctx, &o, audit.ActionStatusChange, false, func(ctx context.Context, o *domain.Order) (string, error) {
				if err := o.Expire(); err != nil {
					return "", err
				}
				return "Proforma " + pi.Number + " kedaluwarsa, alokasi dilepas", s.release(ctx, o, "expire")
			})
		})
		if err != nil {
			return n, fmt.Errorf("expire proforma %s: %w", id, err)
		}
		n++
	}
	return n, nil
}

// ExpireProformasArgs adalah argumen job sales.expire_proformas (Technical Guideline Lampiran B).
type ExpireProformasArgs struct{}

// Kind adalah nama job.
func (ExpireProformasArgs) Kind() string { return "sales.expire_proformas" }

// InsertOpts: antrean default, maks. 3 percobaan.
func (ExpireProformasArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueDefault, MaxAttempts: 3}
}

// ExpireProformasWorker menjalankan job sales.expire_proformas.
type ExpireProformasWorker struct {
	river.WorkerDefaults[ExpireProformasArgs]
	service *DocumentService
}

// NewExpireProformasWorker membuat worker.
func NewExpireProformasWorker(s *DocumentService) *ExpireProformasWorker {
	return &ExpireProformasWorker{service: s}
}

// Work menjalankan pelepasan Proforma kedaluwarsa.
func (w *ExpireProformasWorker) Work(ctx context.Context, _ *river.Job[ExpireProformasArgs]) error {
	n, err := w.service.ExpireProformas(ctx)
	if n > 0 && w.service.Log != nil {
		w.service.Log.InfoContext(ctx, "proforma invoices expired", "count", n)
	}
	return err
}

// --- Invoice --------------------------------------------------------------------------------

// InvoiceFilter adalah kriteria daftar invoice.
type InvoiceFilter = postgres.InvoiceFilter

// InvoiceSortFields adalah field sort daftar invoice.
var InvoiceSortFields = []string{"number", "invoiceDate", "dueDate", "totalAmount"}

// InvoiceView adalah invoice beserta pembayaran dan nama channel.
type InvoiceView struct {
	domain.Invoice
	Payments       []postgres.InvoicePayment
	ChannelName    string
	SubChannelName string
	OrderType      domain.OrderType
	ShipToAddress  string
}

var invoiceActionPerms = map[domain.Action][]string{
	domain.ActionPrint: {PermInvoiceRead, PermPaymentRead}, domain.ActionShareWhatsApp: {PermInvoiceRead},
	domain.ActionRecordPayment: {PermPaymentPost},
}

// InvoiceActions mengembalikan aksi invoice yang boleh dilakukan actor.
func InvoiceActions(ctx context.Context, inv domain.Invoice) []string {
	return filterActions(ctx, inv.Actions(), invoiceActionPerms)
}

// ListInvoices mengembalikan daftar invoice (Penjualan › Invoice, Piutang Usaha).
func (s *DocumentService) ListInvoices(ctx context.Context, f InvoiceFilter, page paging.Page) (paging.Result[domain.Invoice], error) {
	if _, err := actor.RequireAny(ctx, PermInvoiceRead, PermPaymentRead); err != nil {
		return paging.Result[domain.Invoice]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListInvoices(ctx, f, page)
}

// GetInvoice mengembalikan detail invoice beserta riwayat pembayaran.
func (s *DocumentService) GetInvoice(ctx context.Context, id uuid.UUID) (InvoiceView, error) {
	if _, err := actor.RequireAny(ctx, PermInvoiceRead, PermPaymentRead); err != nil {
		return InvoiceView{}, err
	}
	inv, err := s.Store.GetInvoice(ctx, id)
	if err != nil {
		return InvoiceView{}, err
	}
	v := InvoiceView{Invoice: inv}
	if v.Payments, err = s.Store.PaymentsByInvoice(ctx, id); err != nil {
		return InvoiceView{}, err
	}
	labels, err := s.Ledger.ChannelLabels(ctx, []uuid.UUID{inv.ChannelID, inv.SubChannelID})
	if err != nil {
		return InvoiceView{}, err
	}
	v.ChannelName, v.SubChannelName = labels[inv.ChannelID].Name, labels[inv.SubChannelID].Name
	if inv.OrderID != nil {
		o, err := s.Store.GetOrder(ctx, *inv.OrderID, false)
		if err != nil {
			return InvoiceView{}, err
		}
		v.OrderType, v.ShipToAddress = o.Type, o.ShipToAddress
	}
	return v, nil
}

// InvoiceShareText menyusun teks invoice siap tempel ke WhatsApp beserta tautan cetak invoice di ERP.
func (s *DocumentService) InvoiceShareText(ctx context.Context, id uuid.UUID) (string, error) {
	v, err := s.GetInvoice(ctx, id)
	if err != nil {
		return "", err
	}
	link := ""
	if s.BaseURL != "" {
		link = strings.TrimRight(s.BaseURL, "/") + InvoicePath(id) + "/print"
	}
	return domain.ShareText(v.Invoice, link), nil
}

// --- Pembayaran pelanggan (S-315) -----------------------------------------------------------

// PaymentFilter adalah kriteria daftar pembayaran.
type PaymentFilter = postgres.PaymentFilter

// CashBankAccounts mengembalikan akun Kas & Bank (11xx) untuk form pembayaran.
func (s *DocumentService) CashBankAccounts(ctx context.Context) ([]Option, error) {
	if _, err := actor.RequireAny(ctx, PermPaymentRead, PermPaymentPost, PermInvoicePost); err != nil {
		return nil, err
	}
	return s.Ledger.CashBankAccounts(ctx)
}

func (s *DocumentService) cashAccountCodes(ctx context.Context) ([]string, error) {
	list, err := s.Ledger.CashBankAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Code
	}
	return out, nil
}

// recordPayment mengunci invoice, memvalidasi alokasi, memberi nomor BYR, memposting A-PB, dan menambah nominal
// terbayar invoice di transaksi aktif.
func (s *DocumentService) recordPayment(ctx context.Context, in domain.PaymentInput) (domain.CustomerPayment, error) {
	invoices := map[uuid.UUID]domain.Invoice{}
	for _, a := range in.Allocations {
		if _, ok := invoices[a.InvoiceID]; ok {
			continue
		}
		inv, err := s.Store.LockInvoice(ctx, a.InvoiceID)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return domain.CustomerPayment{}, err
		}
		invoices[inv.ID] = inv
	}
	accounts, err := s.cashAccountCodes(ctx)
	if err != nil {
		return domain.CustomerPayment{}, err
	}
	p, err := domain.NewPayment(in, invoices, accounts)
	if err != nil {
		return domain.CustomerPayment{}, err
	}
	for _, inv := range invoices {
		p.PartyName = inv.PartyName
	}
	if p.Number, err = s.Numberer.Next(ctx, DocPayment, p.PaymentDate); err != nil {
		return domain.CustomerPayment{}, err
	}
	if p.ID, err = s.Store.InsertPayment(ctx, p, actor.UserID(ctx)); err != nil {
		return domain.CustomerPayment{}, err
	}
	if p.ProofFileID != nil {
		if err := s.Files.Attach(ctx, *p.ProofFileID, FileOwnerCustomerPaymentProof, p.ID); err != nil {
			return domain.CustomerPayment{}, err
		}
	}
	entry, err := s.Ledger.Post(ctx, accounting.PostingRequest{
		RuleCode: RulePayment, SourceType: SourcePayment, SourceID: p.ID, SourceLabel: p.Number, BusinessDate: p.PaymentDate,
		Amounts: map[string]money.Amount{"incoming_value": p.Amount},
		Accounts: map[accounting.Resolver]string{
			accounting.ResolverBank: p.AccountCode, accounting.ResolverReceivable: AccountReceivable,
		},
		Party:       &accounting.Party{Type: p.PartyType, ID: p.PartyID},
		Description: "Pembayaran pelanggan " + p.PartyName,
	})
	if err != nil {
		return domain.CustomerPayment{}, err
	}
	p.JournalEntryID = &entry.ID
	if err := s.Store.SavePayment(ctx, p, 1, actor.UserID(ctx)); err != nil {
		return domain.CustomerPayment{}, err
	}
	p.Version = 2
	for _, a := range p.Allocations {
		inv := invoices[a.InvoiceID]
		ver := inv.Version
		if err := inv.ApplyPayment(a.Amount); err != nil {
			return domain.CustomerPayment{}, err
		}
		if err := s.Store.SaveInvoice(ctx, inv, ver, actor.UserID(ctx)); err != nil {
			return domain.CustomerPayment{}, err
		}
		inv.Version = ver + 1
		invoices[a.InvoiceID] = inv
	}
	return p, s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionPost, EntityType: EntityCustomerPayment, EntityID: p.ID, EntityLabel: p.Number,
		After: paymentSnapshot(p),
	})
}

func paymentSnapshot(p domain.CustomerPayment) map[string]any {
	allocs := make([]map[string]any, len(p.Allocations))
	for i, a := range p.Allocations {
		allocs[i] = map[string]any{"invoice": a.InvoiceNumber, "amount": a.Amount.String()}
	}
	return map[string]any{
		"party": p.PartyName, "paymentDate": p.PaymentDate.String(), "amount": p.Amount.String(),
		"method": string(p.Method), "accountCode": p.AccountCode, "reference": p.Reference, "status": string(p.Status),
		"cancelReason": p.CancelReason, "allocations": allocs,
	}
}

// CreatePayment mencatat dan memposting pembayaran pelanggan yang dialokasikan ke satu atau banyak invoice
// (pembayaran sebagian diperbolehkan). Aksi ★: butuh re-autentikasi 5 menit.
func (s *DocumentService) CreatePayment(ctx context.Context, in domain.PaymentInput) (domain.CustomerPayment, error) {
	if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermPaymentPost); err != nil {
		return domain.CustomerPayment{}, err
	}
	var p domain.CustomerPayment
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		p, err = s.recordPayment(ctx, in)
		return err
	})
	if err != nil {
		return domain.CustomerPayment{}, err
	}
	return s.Store.GetPayment(ctx, p.ID, false)
}

// CancelPayment membatalkan pembayaran: jurnal A-PB dibalik dan sisa piutang invoice dikembalikan.
func (s *DocumentService) CancelPayment(ctx context.Context, id uuid.UUID, ver int, reason string) (domain.CustomerPayment, error) {
	if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermPaymentPost); err != nil {
		return domain.CustomerPayment{}, err
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		p, err := s.Store.GetPayment(ctx, id, true)
		if err != nil {
			return err
		}
		if p.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := paymentSnapshot(p)
		if err := p.Cancel(reason); err != nil {
			return err
		}
		if err := s.Store.SavePayment(ctx, p, ver, actor.UserID(ctx)); err != nil {
			return err
		}
		if _, err := s.Ledger.Reverse(ctx, accountingReversal(SourcePayment, p.ID, RulePayment, s.today(), p.CancelReason)); err != nil {
			return err
		}
		for _, a := range p.Allocations {
			inv, err := s.Store.LockInvoice(ctx, a.InvoiceID)
			if err != nil {
				return err
			}
			iv := inv.Version
			if err := inv.ApplyPayment(a.Amount.Neg()); err != nil {
				return err
			}
			if err := s.Store.SaveInvoice(ctx, inv, iv, actor.UserID(ctx)); err != nil {
				return err
			}
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCancel, EntityType: EntityCustomerPayment, EntityID: p.ID, EntityLabel: p.Number,
			Before: before, After: paymentSnapshot(p), Note: p.CancelReason,
		})
	})
	if err != nil {
		return domain.CustomerPayment{}, err
	}
	return s.Store.GetPayment(ctx, id, false)
}

// ListPayments mengembalikan daftar pembayaran pelanggan.
func (s *DocumentService) ListPayments(ctx context.Context, f PaymentFilter, page paging.Page) (paging.Result[domain.CustomerPayment], error) {
	if _, err := actor.Require(ctx, PermPaymentRead); err != nil {
		return paging.Result[domain.CustomerPayment]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListPayments(ctx, f, page)
}

// GetPayment mengembalikan detail pembayaran.
func (s *DocumentService) GetPayment(ctx context.Context, id uuid.UUID) (domain.CustomerPayment, error) {
	if _, err := actor.Require(ctx, PermPaymentRead); err != nil {
		return domain.CustomerPayment{}, err
	}
	return s.Store.GetPayment(ctx, id, false)
}

// PaymentActions mengembalikan aksi pembayaran yang boleh dilakukan actor.
func PaymentActions(ctx context.Context, p domain.CustomerPayment) []string {
	if p.Status != domain.StatusPosted {
		return []string{}
	}
	return filterActions(ctx, []domain.Action{domain.ActionCancel}, map[domain.Action][]string{domain.ActionCancel: {PermPaymentPost}})
}

// AgingReport mengelompokkan sisa piutang invoice terbuka per pelanggan/mitra pada tanggal asOf (Laporan Piutang).
func (s *DocumentService) AgingReport(ctx context.Context, asOf clock.BusinessDate, partyType string) ([]domain.AgingRow, error) {
	if _, err := actor.RequireAny(ctx, PermPaymentRead, PermInvoiceRead); err != nil {
		return nil, err
	}
	if asOf.IsZero() {
		asOf = s.today()
	}
	list, err := s.Store.OpenInvoices(ctx, asOf, partyType)
	if err != nil {
		return nil, err
	}
	return domain.AgeReceivables(list, asOf), nil
}

// --- API rekonsiliasi bank (R1.4) ------------------------------------------------------------

// PaymentQuery adalah kriteria pembayaran masuk untuk pencocokan mutasi bank.
type PaymentQuery struct {
	AccountCode string
	DateFrom    clock.BusinessDate
	DateTo      clock.BusinessDate
	// IncludeCancelled ikut menampilkan pembayaran yang dibatalkan.
	IncludeCancelled bool
}

// IncomingPayments mengembalikan pembayaran pelanggan Diposting di rekening dan rentang tanggal (maks. 500), tanpa
// cek permission (dipanggil modul rekonsiliasi bank yang memeriksa aksi user-nya).
func (s *DocumentService) IncomingPayments(ctx context.Context, q PaymentQuery) ([]domain.CustomerPayment, error) {
	f := PaymentFilter{AccountCode: q.AccountCode, DateFrom: &q.DateFrom, DateTo: &q.DateTo}
	if !q.IncludeCancelled {
		f.Status = domain.StatusPosted
	}
	size := 200
	var out []domain.CustomerPayment
	for n := 1; n <= 3; n++ {
		res, err := s.Store.ListPayments(ctx, f, paging.New(&n, &size))
		if err != nil {
			return nil, err
		}
		out = append(out, res.Items...)
		if int64(len(out)) >= res.Total || len(res.Items) == 0 {
			break
		}
	}
	return out, nil
}

// Payment membaca satu pembayaran pelanggan tanpa cek permission (rekonsiliasi bank).
func (s *DocumentService) Payment(ctx context.Context, id uuid.UUID) (domain.CustomerPayment, error) {
	return s.Store.GetPayment(ctx, id, false)
}
