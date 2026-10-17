package app

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/document"
	"balans/internal/filestore"
	"balans/internal/identity"
	"balans/internal/notification"
	"balans/internal/party"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/procurement"
	"balans/internal/sales"
)

// Batas teknis file keuangan R1.4 (bukan nilai bisnis).
const (
	maxReceiptFileBytes   = 10 << 20
	maxStatementFileBytes = 10 << 20
)

// connectFinance memasang dependensi Kas & Bank, rekonsiliasi bank, pengajuan biaya, kas kecil, dan pajak
// (R1.4) ke modul accounting setelah approval, document, filestore, identity, dan party tersusun. Handler
// approval `expense_claim` dan `petty_cash_replenishment` didaftarkan setelah handler dev agar menggantikannya.
func (a *App) connectFinance(mailer *notification.Module) {
	if a.accounting == nil {
		return
	}
	c := accounting.FinanceConnections{BaseURL: a.Config.PublicBaseURL}
	if a.document != nil {
		c.DocNumbers = journalNumberer{documents: a.document}
	}
	if a.approval != nil {
		c.Approvals = financeApprovals{approval: a.approval}
		a.approval.RegisterHandler(approval.SubjectExpenseClaim, accounting.PermExpenseClaimRead, approval.Handler{
			OnApproved: func(ctx context.Context, d approval.Decided) error {
				return a.accounting.OnExpenseClaimDecided(ctx, d.Subject.ID, true, d.Note)
			},
			OnRejected: func(ctx context.Context, d approval.Decided) error {
				return a.accounting.OnExpenseClaimDecided(ctx, d.Subject.ID, false, d.Note)
			},
		})
		a.approval.RegisterHandler(approval.SubjectPettyCashReplenishment, accounting.PermPettyCashRead, approval.Handler{
			OnApproved: func(ctx context.Context, d approval.Decided) error {
				return a.accounting.OnPettyCashDecided(ctx, d.Subject.ID, true, d.Note)
			},
			OnRejected: func(ctx context.Context, d approval.Decided) error {
				return a.accounting.OnPettyCashDecided(ctx, d.Subject.ID, false, d.Note)
			},
		})
	}
	if a.filestore != nil {
		c.Files = financeFiles{files: a.filestore}
		receipts := []string{filestore.TypePDF, filestore.TypeJPEG, filestore.TypePNG, filestore.TypeWebP}
		for _, p := range []filestore.OwnerPolicy{
			{OwnerType: accounting.FileOwnerExpenseClaim, WritePermission: accounting.PermExpenseClaimWrite,
				ReadPermission: accounting.PermExpenseClaimRead, ContentTypes: receipts, MaxBytes: maxReceiptFileBytes},
			{OwnerType: accounting.FileOwnerPettyCash, WritePermission: accounting.PermPettyCashWrite,
				ReadPermission: accounting.PermPettyCashRead, ContentTypes: receipts, MaxBytes: maxReceiptFileBytes},
			{OwnerType: accounting.FileOwnerPaymentProof, WritePermission: accounting.PermPaymentWrite,
				ReadPermission: accounting.PermPaymentRead, ContentTypes: receipts, MaxBytes: maxReceiptFileBytes},
			{OwnerType: accounting.FileOwnerBankStatement, WritePermission: accounting.PermBankReconImport,
				ReadPermission: accounting.PermBankReconRead, ContentTypes: []string{filestore.TypeCSV, filestore.TypeXLSX},
				MaxBytes: maxStatementFileBytes},
			{OwnerType: accounting.FileOwnerWithholdingSlip, WritePermission: accounting.PermWithholdingExport,
				ReadPermission: accounting.PermWithholdingRead, ContentTypes: []string{filestore.TypeXLSX}, MaxBytes: maxStatementFileBytes},
		} {
			a.filestore.RegisterOwner(p)
		}
	}
	if a.identity != nil {
		c.Users = financeUsers{identity: a.identity}
	}
	if mailer != nil {
		c.Notifier = mailer
	}
	if a.party != nil {
		c.Suppliers = financeSuppliers{party: a.party}
	}
	a.accounting.ConnectFinance(c)
	if a.sales != nil {
		a.accounting.RegisterBookedPayments(salesBookedPayments{sales: a.sales})
	}
	if a.procurement != nil {
		a.accounting.RegisterPaymentCandidates(supplierPayableCandidates{procurement: a.procurement, party: a.party})
	}
	registerAuditEntities(a.audit, accounting.AuditEntities())
}

// NextDocNumber mengambil nomor EXP, KK, atau BYR dari modul document.
func (n journalNumberer) NextDocNumber(ctx context.Context, docType string, businessDate clock.BusinessDate) (string, error) {
	switch t := document.Type(docType); t {
	case document.TypeExpenseClaim, document.TypePettyCash, document.TypePayment:
		return n.documents.Next(ctx, t, businessDate)
	default:
		return "", fmt.Errorf("jenis nomor dokumen keuangan %q tidak dikenal", docType)
	}
}

// financeApprovals memetakan ApprovalSubject accounting ke approval.Subject.
type financeApprovals struct{ approval *approval.Module }

func (f financeApprovals) RequestApproval(ctx context.Context, s accounting.ApprovalSubject) error {
	_, err := f.approval.Request(ctx, approval.Subject{
		Type: approval.SubjectType(s.Type), ID: s.ID, Label: s.Label, Link: s.Link, Summary: s.Summary, Amount: s.Amount,
		Division: s.Division,
	})
	return err
}

// financeFiles menyesuaikan modul filestore untuk bukti, file mutasi, dan ekspor rekap bukti potong.
type financeFiles struct{ files *filestore.Module }

func (f financeFiles) Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error {
	return f.files.Attach(ctx, fileID, ownerType, ownerID)
}

func (f financeFiles) Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return f.files.Names(ctx, ids)
}

// Read membaca file yang diunggah actor untuk jenis pemilik tertentu (belum ditautkan ke data lain).
func (f financeFiles) Read(ctx context.Context, fileID uuid.UUID, ownerType string) (accounting.FileContent, error) {
	info, err := f.files.Info(ctx, fileID)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return accounting.FileContent{}, apperr.Validation(map[string]string{"fileId": "File tidak ditemukan"})
	}
	if err != nil {
		return accounting.FileContent{}, err
	}
	switch {
	case info.OwnerType != ownerType:
		return accounting.FileContent{}, apperr.Validation(map[string]string{"fileId": "File tidak bisa dipakai untuk data ini"})
	case info.OwnerID != nil:
		return accounting.FileContent{}, apperr.Validation(map[string]string{"fileId": "File sudah dipakai data lain"})
	case info.CreatedBy != actor.UserID(ctx):
		return accounting.FileContent{}, apperr.Validation(map[string]string{"fileId": "File diunggah pengguna lain"})
	}
	rc, err := f.files.OpenContent(ctx, fileID)
	if err != nil {
		return accounting.FileContent{}, apperr.Validation(map[string]string{"fileId": "File belum selesai diunggah"})
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, maxStatementFileBytes+1))
	if err != nil {
		return accounting.FileContent{}, fmt.Errorf("baca file %s: %w", fileID, err)
	}
	return accounting.FileContent{Name: info.OriginalName, ContentType: info.ContentType, Data: data}, nil
}

func (f financeFiles) SaveExport(ctx context.Context, ownerType string, ownerID uuid.UUID, name, contentType string, data []byte) (accounting.DownloadLink, error) {
	file, err := f.files.Save(ctx, ownerType, ownerID, name, contentType, data)
	if err != nil {
		return accounting.DownloadLink{}, err
	}
	url, exp, err := f.files.SignedDownloadURL(ctx, file.ID)
	if err != nil {
		return accounting.DownloadLink{}, err
	}
	return accounting.DownloadLink{FileID: file.ID, URL: url, ExpiresAt: exp}, nil
}

// financeUsers membaca nama pengguna (pengaju biaya, pemegang kas kecil).
type financeUsers struct{ identity *identity.Module }

func (u financeUsers) UserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return procurementUsers(u).UserNames(ctx, ids)
}

// financeSuppliers membaca nama pemasok penerima pengajuan biaya.
type financeSuppliers struct{ party *party.Module }

func (s financeSuppliers) SupplierName(ctx context.Context, id uuid.UUID) (string, bool, error) {
	sup, err := s.party.SupplierByID(ctx, id)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return sup.Name, true, nil
}

// salesBookedPayments memperkaya kandidat rekonsiliasi dengan nomor & referensi pembayaran pelanggan sales (BYR
// yang sudah dijurnal A-PB oleh modul sales).
type salesBookedPayments struct{ sales *sales.Module }

func (s salesBookedPayments) BookedPayments(ctx context.Context, accountCode string, from, to clock.BusinessDate) ([]accounting.BookedPayment, error) {
	list, err := s.sales.IncomingPayments(ctx, sales.PaymentQuery{AccountCode: accountCode, DateFrom: from, DateTo: to})
	if err != nil {
		return nil, err
	}
	out := make([]accounting.BookedPayment, 0, len(list))
	for _, p := range list {
		if p.JournalEntryID == nil {
			continue
		}
		out = append(out, accounting.BookedPayment{
			JournalEntryID: *p.JournalEntryID, Number: p.Number, Reference: p.Reference, PartyName: p.PartyName,
		})
	}
	return out, nil
}

// SourceSupplierPayables adalah nama penyedia kandidat utang pemasok untuk rekonsiliasi bank.
const SourceSupplierPayables = "procurement.supplier_payable"

// supplierPayableCandidates menyediakan tagihan pemasok terbuka sebagai kandidat mutasi keluar. Konfirmasi
// membuat pembayaran BYR (Dr 2101 / Cr bank) lalu SettlePayable mengurangi sisa utang.
type supplierPayableCandidates struct {
	procurement *procurement.Module
	party       *party.Module
}

func (p supplierPayableCandidates) Source() string { return SourceSupplierPayables }

func (p supplierPayableCandidates) list(ctx context.Context) ([]accounting.PaymentCandidate, error) {
	payables, err := p.procurement.OpenPayables(ctx, uuid.Nil)
	if err != nil {
		return nil, err
	}
	names := map[uuid.UUID]string{}
	out := make([]accounting.PaymentCandidate, 0, len(payables))
	for _, py := range payables {
		if !py.OpenAmount.IsPositive() {
			continue // nota debit dikompensasi lewat tagihan, bukan mutasi bank
		}
		name, ok := names[py.SupplierID]
		if !ok && p.party != nil {
			if sup, err := p.party.SupplierByID(ctx, py.SupplierID); err == nil {
				name = sup.Name
			}
			names[py.SupplierID] = name
		}
		supplier := py.SupplierID
		out = append(out, accounting.PaymentCandidate{
			Source: SourceSupplierPayables, DocumentID: py.ID, DocumentLabel: py.DocumentNumber,
			Direction: accounting.PaymentOutgoing, PartyType: "supplier", PartyID: &supplier, PartyName: name,
			OpenAmount: py.OpenAmount, DocumentDate: py.DueDate, References: []string{py.DocumentNumber},
			CounterAccountCode: accounting.TradePayableAccount,
		})
	}
	return out, nil
}

func (p supplierPayableCandidates) PaymentCandidates(ctx context.Context, q accounting.CandidateQuery) ([]accounting.PaymentCandidate, error) {
	if q.Direction != accounting.PaymentOutgoing {
		return nil, nil
	}
	return p.list(ctx)
}

func (p supplierPayableCandidates) PaymentCandidate(ctx context.Context, id uuid.UUID) (accounting.PaymentCandidate, bool, error) {
	list, err := p.list(ctx)
	if err != nil {
		return accounting.PaymentCandidate{}, false, err
	}
	for _, c := range list {
		if c.DocumentID == id {
			return c, true, nil
		}
	}
	return accounting.PaymentCandidate{}, false, nil
}

func (p supplierPayableCandidates) SettlePayment(ctx context.Context, c accounting.PaymentCandidate, pay accounting.Payment) error {
	_, err := p.procurement.SettlePayable(ctx, c.DocumentID, pay.Amount)
	return err
}
