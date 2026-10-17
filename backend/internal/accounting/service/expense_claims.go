package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/tax"
)

// ClaimFilter adalah filter daftar pengajuan biaya.
type ClaimFilter = postgres.ClaimFilter

// ClaimOptions adalah pilihan form Pengajuan Biaya untuk semua divisi (tanpa akses master dimensi).
type ClaimOptions struct {
	Divisions    []domain.DimensionValue
	Campaigns    []domain.DimensionValue
	ExpenseTypes []domain.ExpenseType
}

// ClaimView adalah detail pengajuan beserta data bukti potong.
type ClaimView struct {
	Claim        domain.ExpenseClaim
	Withholdings []domain.WithholdingRecord
}

// canViewAllClaims: F&A (posting), approver, Direksi melihat semua pengajuan; pengaju lain hanya miliknya.
func canViewAllClaims(a actor.Actor) bool {
	return a.Has(PermExpenseClaimPost) || a.Has(PermExpenseClaimApprove)
}

func claimSnapshot(c domain.ExpenseClaim) map[string]any {
	lines := make([]map[string]any, len(c.Lines))
	for i, l := range c.Lines {
		lines[i] = map[string]any{
			"expenseTypeCode": l.ExpenseTypeCode, "description": l.Description, "amount": l.Amount.String(),
			"withholdingType": string(l.WithholdingType), "withholdingAmount": l.WithholdingAmount.String(),
		}
	}
	return map[string]any{
		"claimType": string(c.ClaimType), "divisionName": c.DivisionName, "payeeName": c.PayeeName,
		"claimDate": c.ClaimDate.String(), "description": c.Description, "total": c.Total.String(),
		"withholding": c.Withholding.String(), "payable": c.Payable.String(), "status": string(c.Status),
		"cancelReason": c.CancelReason, "lines": lines,
	}
}

// ClaimActions mengembalikan aksi pengajuan yang boleh dilakukan actor. Edit/ajukan/batal hanya oleh pengaju.
func ClaimActions(ctx context.Context, c domain.ExpenseClaim) []string {
	a, _ := actor.From(ctx)
	owner := a.UserID == c.RequesterID
	perms := map[domain.Action]string{
		domain.ActionVerify: PermExpenseClaimPost, domain.ActionPay: PermPaymentPost,
	}
	if owner {
		perms[domain.ActionEdit], perms[domain.ActionCancel] = PermExpenseClaimWrite, PermExpenseClaimWrite
		perms[domain.ActionRevise], perms[domain.ActionSubmit] = PermExpenseClaimWrite, PermExpenseClaimSubmit
	}
	return filterActions(ctx, c.Actions(), perms)
}

// ExpenseClaimOptions mengembalikan divisi, kampanye aktif, dan jenis biaya aktif untuk form pengajuan.
func (s *Service) ExpenseClaimOptions(ctx context.Context) (ClaimOptions, error) {
	if _, err := actor.Require(ctx, PermExpenseClaimRead); err != nil {
		return ClaimOptions{}, err
	}
	var out ClaimOptions
	var err error
	if out.Divisions, err = s.Store.ListDimensions(ctx, domain.KindDivision, postgres.DimensionFilter{}); err != nil {
		return ClaimOptions{}, err
	}
	campaigns, err := s.Store.ListDimensions(ctx, domain.KindCampaign, postgres.DimensionFilter{})
	if err != nil {
		return ClaimOptions{}, err
	}
	today := s.today()
	for _, c := range campaigns {
		if c.CampaignActiveOn(today) {
			out.Campaigns = append(out.Campaigns, c)
		}
	}
	if out.ExpenseTypes, err = s.Store.ListExpenseTypes(ctx, postgres.ExpenseTypeFilter{}); err != nil {
		return ClaimOptions{}, err
	}
	return out, nil
}

// ListExpenseClaims mengembalikan daftar pengajuan biaya (pengaju biasa hanya miliknya).
func (s *Service) ListExpenseClaims(ctx context.Context, f ClaimFilter, page paging.Page) (paging.Result[domain.ExpenseClaim], error) {
	a, err := actor.Require(ctx, PermExpenseClaimRead)
	if err != nil {
		return paging.Result[domain.ExpenseClaim]{}, err
	}
	if !canViewAllClaims(a) {
		f.RequesterID = &a.UserID
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Finance.ListClaims(ctx, f, page)
	if err != nil {
		return res, err
	}
	ids := make([]uuid.UUID, len(res.Items))
	for i, c := range res.Items {
		ids[i] = c.RequesterID
	}
	names := s.names(ctx, ids)
	for i := range res.Items {
		res.Items[i].RequesterName = names[res.Items[i].RequesterID]
	}
	return res, nil
}

// GetExpenseClaim membaca detail pengajuan.
func (s *Service) GetExpenseClaim(ctx context.Context, id uuid.UUID) (ClaimView, error) {
	a, err := actor.Require(ctx, PermExpenseClaimRead)
	if err != nil {
		return ClaimView{}, err
	}
	c, err := s.Finance.GetClaim(ctx, id, false)
	if err != nil {
		return ClaimView{}, err
	}
	if !canViewAllClaims(a) && c.RequesterID != a.UserID {
		return ClaimView{}, apperr.New(apperr.CodeNotFound)
	}
	return s.claimView(ctx, c)
}

func (s *Service) claimView(ctx context.Context, c domain.ExpenseClaim) (ClaimView, error) {
	c.RequesterName = s.names(ctx, []uuid.UUID{c.RequesterID})[c.RequesterID]
	var fileIDs []uuid.UUID
	for _, l := range c.Lines {
		if l.ReceiptFileID != nil {
			fileIDs = append(fileIDs, *l.ReceiptFileID)
		}
	}
	files := s.fileNames(ctx, fileIDs)
	for i, l := range c.Lines {
		if l.ReceiptFileID != nil {
			c.Lines[i].ReceiptFileName = files[*l.ReceiptFileID]
		}
	}
	wh, err := s.Finance.WithholdingsBySource(ctx, domain.SourceExpenseClaim, c.ID)
	if err != nil {
		return ClaimView{}, err
	}
	return ClaimView{Claim: c, Withholdings: wh}, nil
}

// prepareClaim memeriksa divisi, kampanye, jenis biaya, dan penerima; reimbursement dibayar ke pengaju.
func (s *Service) prepareClaim(ctx context.Context, c *domain.ExpenseClaim) error {
	if err := s.CheckDivision(ctx, c.DivisionID, c.ClaimDate, "divisionId"); err != nil {
		return err
	}
	if c.CampaignID != nil {
		check, err := s.Store.CheckDimensions(ctx, domain.Dimensions{CampaignID: c.CampaignID}, c.ClaimDate)
		if err != nil {
			return err
		}
		if !check.CampaignOK {
			return apperr.Validation(map[string]string{"campaignId": "Kampanye tidak ditemukan atau tidak berlaku pada tanggal biaya"})
		}
	}
	for i, l := range c.Lines {
		t, err := s.Store.GetExpenseType(ctx, l.ExpenseTypeID, false)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return apperr.Validation(map[string]string{fmt.Sprintf("lines.%d.expenseTypeId", i): "Jenis biaya tidak ditemukan"})
		}
		if err != nil {
			return err
		}
		if msg := t.UsableError(); msg != "" {
			return apperr.Validation(map[string]string{fmt.Sprintf("lines.%d.expenseTypeId", i): msg})
		}
		c.Lines[i].ExpenseTypeCode, c.Lines[i].ExpenseTypeName, c.Lines[i].ExpenseAccountCode = t.Code, t.Name, t.ExpenseAccountCode
	}
	switch c.PayeeType {
	case domain.PayeeEmployee:
		if c.PayeeID == nil || *c.PayeeID != c.RequesterID {
			return apperr.Validation(map[string]string{"payeeId": "Reimbursement karyawan dibayarkan ke pengaju"})
		}
	case domain.PayeeSupplier:
		if s.Suppliers == nil {
			return apperr.Validation(map[string]string{"payeeType": "Penerima pemasok belum tersedia"})
		}
		name, ok, err := s.Suppliers.SupplierName(ctx, *c.PayeeID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Validation(map[string]string{"payeeId": "Pemasok tidak ditemukan"})
		}
		if c.PayeeName == "" {
			c.PayeeName = name
		}
	}
	return nil
}

func (s *Service) attachReceipts(ctx context.Context, c domain.ExpenseClaim) error {
	for _, l := range c.Lines {
		if err := s.attach(ctx, l.ReceiptFileID, FileOwnerExpenseClaim, c.ID); err != nil {
			return err
		}
	}
	return nil
}

// CreateExpenseClaim menyimpan draft pengajuan biaya milik actor.
func (s *Service) CreateExpenseClaim(ctx context.Context, in domain.ClaimInput) (ClaimView, error) {
	a, err := actor.Require(ctx, PermExpenseClaimWrite)
	if err != nil {
		return ClaimView{}, err
	}
	if in.PayeeType == domain.PayeeEmployee {
		in.PayeeID = &a.UserID
		if strings.TrimSpace(in.PayeeName) == "" {
			in.PayeeName = a.FullName
		}
	}
	c, err := domain.NewExpenseClaim(in, a.UserID)
	if err != nil {
		return ClaimView{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.prepareClaim(ctx, &c); err != nil {
			return err
		}
		if c.ID, err = s.Finance.InsertClaim(ctx, c, a.UserID); err != nil {
			return err
		}
		if err := s.attachReceipts(ctx, c); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityExpenseClaim, EntityID: c.ID, EntityLabel: c.Label(), After: claimSnapshot(c),
		})
	})
	if err != nil {
		return ClaimView{}, err
	}
	return s.GetExpenseClaim(ctx, c.ID)
}

// mutateClaim mengunci pengajuan, memeriksa versi (dan pemilik bila ownerOnly), menjalankan fn, menyimpan, dan
// mencatat audit.
func (s *Service) mutateClaim(ctx context.Context, id uuid.UUID, version int, ownerOnly bool, action audit.Action,
	fn func(ctx context.Context, c *domain.ExpenseClaim) (note string, replaceLines bool, err error),
) (ClaimView, error) {
	by := actor.UserID(ctx)
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.Finance.GetClaim(ctx, id, true)
		if err != nil {
			return err
		}
		if ownerOnly && c.RequesterID != by {
			return apperr.WithMessage(apperr.CodeForbidden, "Hanya pengaju yang bisa mengubah pengajuan ini.")
		}
		if version >= 0 && c.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := claimSnapshot(c)
		note, replace, err := fn(ctx, &c)
		if err != nil {
			return err
		}
		if err := s.Finance.SaveClaim(ctx, c, c.Version, replace, by); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityExpenseClaim, EntityID: c.ID, EntityLabel: c.Label(),
			Before: before, After: claimSnapshot(c), Note: note,
		})
	})
	if err != nil {
		return ClaimView{}, err
	}
	c, err := s.Finance.GetClaim(ctx, id, false)
	if err != nil {
		return ClaimView{}, err
	}
	return s.claimView(ctx, c)
}

// UpdateExpenseClaim mengubah draft pengajuan milik actor.
func (s *Service) UpdateExpenseClaim(ctx context.Context, id uuid.UUID, version int, in domain.ClaimInput) (ClaimView, error) {
	a, err := actor.Require(ctx, PermExpenseClaimWrite)
	if err != nil {
		return ClaimView{}, err
	}
	if in.PayeeType == domain.PayeeEmployee {
		in.PayeeID = &a.UserID
		if strings.TrimSpace(in.PayeeName) == "" {
			in.PayeeName = a.FullName
		}
	}
	return s.mutateClaim(ctx, id, version, true, audit.ActionUpdate, func(ctx context.Context, c *domain.ExpenseClaim) (string, bool, error) {
		if err := c.Update(in); err != nil {
			return "", false, err
		}
		if err := s.prepareClaim(ctx, c); err != nil {
			return "", false, err
		}
		return "", true, s.attachReceipts(ctx, *c)
	})
}

// SubmitExpenseClaim memberi nomor EXP dan mengajukan approval `expense_claim` (berjenjang per nominal dan
// divisi pengaju, Desain Role & Approval 4 #3).
func (s *Service) SubmitExpenseClaim(ctx context.Context, id uuid.UUID, version int) (ClaimView, error) {
	if _, err := actor.Require(ctx, PermExpenseClaimSubmit); err != nil {
		return ClaimView{}, err
	}
	return s.mutateClaim(ctx, id, version, true, audit.ActionSubmit, func(ctx context.Context, c *domain.ExpenseClaim) (string, bool, error) {
		types := map[uuid.UUID]domain.ExpenseType{}
		for _, l := range c.Lines {
			if _, ok := types[l.ExpenseTypeID]; ok {
				continue
			}
			t, err := s.Store.GetExpenseType(ctx, l.ExpenseTypeID, false)
			if err != nil && !apperr.HasCode(err, apperr.CodeNotFound) {
				return "", false, err
			}
			if err == nil {
				types[l.ExpenseTypeID] = t
			}
		}
		if err := c.CheckSubmit(types); err != nil {
			return "", false, err
		}
		number, err := s.DocNumbers.NextDocNumber(ctx, DocExpenseClaim, c.ClaimDate)
		if err != nil {
			return "", false, err
		}
		if err := c.Submit(number, s.Clock.Now()); err != nil {
			return "", false, err
		}
		division := c.DivisionID
		summary := c.PayeeName
		if c.Description != "" {
			summary = c.Description + " · " + c.PayeeName
		}
		return "", false, s.Approvals.RequestApproval(ctx, ApprovalSubject{
			Type: domain.ApprovalSubjectExpenseClaim, ID: c.ID, Label: number, Link: "/finance/expense-claims/" + c.ID.String(),
			Summary: truncateText(summary, 200), Amount: c.Total, Division: &division,
		})
	})
}

// ReviseExpenseClaim mengembalikan pengajuan ditolak menjadi draft.
func (s *Service) ReviseExpenseClaim(ctx context.Context, id uuid.UUID, version int) (ClaimView, error) {
	if _, err := actor.Require(ctx, PermExpenseClaimWrite); err != nil {
		return ClaimView{}, err
	}
	return s.mutateClaim(ctx, id, version, true, audit.ActionUpdate, func(_ context.Context, c *domain.ExpenseClaim) (string, bool, error) {
		return "Revisi setelah ditolak", false, c.Revise()
	})
}

// CancelExpenseClaim membatalkan draft/pengajuan ditolak.
func (s *Service) CancelExpenseClaim(ctx context.Context, id uuid.UUID, version int, reason string) (ClaimView, error) {
	if _, err := actor.Require(ctx, PermExpenseClaimWrite); err != nil {
		return ClaimView{}, err
	}
	return s.mutateClaim(ctx, id, version, true, audit.ActionCancel, func(_ context.Context, c *domain.ExpenseClaim) (string, bool, error) {
		return reason, false, c.Cancel(reason)
	})
}

// OnExpenseClaimDecided dipanggil handler approval `expense_claim` di transaksi keputusan.
func (s *Service) OnExpenseClaimDecided(ctx context.Context, id uuid.UUID, approved bool, note string) error {
	var requester uuid.UUID
	var label string
	action := audit.ActionApprove
	if !approved {
		action = audit.ActionReject
	}
	_, err := s.mutateClaim(ctx, id, -1, false, action, func(_ context.Context, c *domain.ExpenseClaim) (string, bool, error) {
		requester, label = c.RequesterID, c.Label()
		if approved {
			return note, false, c.Approve(s.Clock.Now(), note)
		}
		return note, false, c.Reject(s.Clock.Now(), note)
	})
	if err != nil {
		return err
	}
	title, body := "Pengajuan biaya disetujui", label+" disetujui dan menunggu verifikasi Finance & Accounting."
	if !approved {
		title, body = "Pengajuan biaya ditolak", label+" ditolak. "+note
	}
	s.notify(ctx, []uuid.UUID{requester}, "expense_claim.decided", title, strings.TrimSpace(body), "/finance/expense-claims/"+id.String())
	return nil
}

// VerifyInput adalah isian verifikasi F&A: NPWP/NIK penerima (untuk bukti potong) dan pilihan PPh per baris.
type VerifyInput struct {
	PayeeTaxID *string
	Lines      []domain.LineTaxChoice
}

// VerifyExpenseClaim memverifikasi pengajuan Disetujui (bukti + PPh): PPh dihitung kalkulator tax dengan tarif
// effective-dated per tanggal verifikasi, jurnal A-15 (Dr beban / Cr 2103 + utang PPh) diposting, dan data bukti
// potong dicatat di transaksi yang sama. Status menjadi Menunggu Pembayaran.
func (s *Service) VerifyExpenseClaim(ctx context.Context, id uuid.UUID, version int, in VerifyInput) (ClaimView, error) {
	if _, err := actor.Require(ctx, PermExpenseClaimPost); err != nil {
		return ClaimView{}, err
	}
	return s.mutateClaim(ctx, id, version, false, audit.ActionPost, func(ctx context.Context, c *domain.ExpenseClaim) (string, bool, error) {
		date := s.today()
		results, records, err := s.computeClaimTax(ctx, c, in, date)
		if err != nil {
			return "", false, err
		}
		if err := c.Verify(results, date, s.Clock.Now(), actor.UserID(ctx)); err != nil {
			return "", false, err
		}
		splits, amounts := c.PostingSplits()
		division := c.DivisionID
		entry, err := s.Post(ctx, domain.PostingRequest{
			RuleCode: domain.RuleExpenseClaim, SourceType: domain.SourceExpenseClaim, SourceID: c.ID, SourceLabel: c.Number,
			BusinessDate: date, Amounts: amounts, Splits: splits,
			Dimensions:  domain.Dimensions{DivisionID: &division, CampaignID: c.CampaignID},
			Description: truncateText(strings.TrimSpace("Biaya operasional "+c.Number+" "+c.PayeeName), 300),
		})
		if err != nil {
			return "", false, err
		}
		c.JournalEntryID = &entry.ID
		for _, rec := range records {
			if rec.TaxAmount.IsZero() {
				continue
			}
			if _, err := s.Finance.InsertWithholding(ctx, rec, actor.UserID(ctx)); err != nil {
				return "", false, err
			}
		}
		return "Verifikasi F&A, jurnal " + entry.Number, false, nil
	})
}

// computeClaimTax menerapkan NPWP/NIK penerima dan menghitung PPh per kelompok jenis + objek pajak.
func (s *Service) computeClaimTax(ctx context.Context, c *domain.ExpenseClaim, in VerifyInput, date clock.BusinessDate) ([]domain.TaxResult, []domain.WithholdingRecord, error) {
	if in.PayeeTaxID != nil {
		c.PayeeTaxID = strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, *in.PayeeTaxID)
		if c.PayeeTaxID != "" && len(c.PayeeTaxID) != 15 && len(c.PayeeTaxID) != 16 {
			return nil, nil, apperr.Validation(map[string]string{"payeeTaxId": "NPWP/NIK 15 atau 16 angka"})
		}
	}
	defaults := map[uuid.UUID]domain.WithholdingType{}
	for _, l := range c.Lines {
		t, err := s.Store.GetExpenseType(ctx, l.ExpenseTypeID, false)
		if err != nil && !apperr.HasCode(err, apperr.CodeNotFound) {
			return nil, nil, err
		}
		defaults[l.ExpenseTypeID] = t.WithholdingType
	}
	groups, err := c.TaxGroups(in.Lines, defaults)
	if err != nil {
		return nil, nil, err
	}
	rates, err := s.Finance.ListRates(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	subject := domain.WithholdingSubject{
		SourceType: domain.SourceExpenseClaim, SourceID: c.ID, SourceLabel: c.Number, BusinessDate: date,
		TaxpayerType: string(c.PayeeType), TaxpayerID: c.PayeeID, TaxpayerName: c.PayeeName, TaxpayerTaxID: c.PayeeTaxID,
	}
	results := make([]domain.TaxResult, 0, len(groups))
	records := make([]domain.WithholdingRecord, 0, len(groups))
	for _, g := range groups {
		rec, err := s.calculateWithholding(rates, subject, domain.TaxTypeOf(g.Type), g.ObjectCode, g.Gross)
		if err != nil {
			return nil, nil, err
		}
		results = append(results, domain.TaxResult{Group: g, ObjectCode: rec.ObjectCode, RatePercent: rec.RatePercent, Amount: rec.TaxAmount})
		records = append(records, rec)
	}
	return results, records, nil
}

// PreviewExpenseClaimVerification menghitung PPh verifikasi tanpa menyimpan (dialog Verifikasi F&A).
func (s *Service) PreviewExpenseClaimVerification(ctx context.Context, id uuid.UUID, in VerifyInput) ([]domain.WithholdingRecord, money.Amount, error) {
	if _, err := actor.Require(ctx, PermExpenseClaimPost); err != nil {
		return nil, money.Zero, err
	}
	c, err := s.Finance.GetClaim(ctx, id, false)
	if err != nil {
		return nil, money.Zero, err
	}
	_, records, err := s.computeClaimTax(ctx, &c, in, s.today())
	if err != nil {
		return nil, money.Zero, err
	}
	total := money.Zero
	for _, r := range records {
		total = total.Add(r.TaxAmount)
	}
	return records, total, nil
}

// calculateWithholding memilih tarif berlaku (objek default bila kosong) dan menghitung potongan.
func (s *Service) calculateWithholding(rates []domain.RateVersion, subj domain.WithholdingSubject, t tax.Type, object string,
	gross money.Amount,
) (domain.WithholdingRecord, error) {
	if !t.IsWithholding() {
		return domain.WithholdingRecord{}, apperr.Validation(map[string]string{"taxType": "Jenis PPh tidak dikenal"})
	}
	if err := subj.Validate(); err != nil {
		return domain.WithholdingRecord{}, err
	}
	if strings.TrimSpace(object) == "" {
		code, ok := domain.DefaultObject(rates, t, subj.BusinessDate)
		if !ok {
			return domain.WithholdingRecord{}, apperr.Validation(map[string]string{
				"objectCode": "Tarif " + t.Label() + " belum ada untuk tanggal " + subj.BusinessDate.String(),
			})
		}
		object = code
	}
	rate, err := domain.FindRate(rates, t, object, subj.BusinessDate)
	if err != nil {
		return domain.WithholdingRecord{}, err
	}
	w, err := rate.Withhold(tax.WithholdingInput{Gross: gross, HasTaxID: strings.TrimSpace(subj.TaxpayerTaxID) != ""})
	if err != nil {
		return domain.WithholdingRecord{}, err
	}
	return domain.NewWithholdingRecord(subj, rate, w), nil
}

// PayInput adalah isian Bayar pengajuan biaya.
type PayInput struct {
	BankAccountID uuid.UUID
	PaymentDate   clock.BusinessDate
	Reference     string
	ProofFileID   *uuid.UUID
}

// PayExpenseClaim membayar pengajuan Menunggu Pembayaran: pembayaran keluar BYR (Dr 2103 / Cr bank, A-PB) dengan
// re-autentikasi (★), lalu pengajuan menjadi Lunas.
func (s *Service) PayExpenseClaim(ctx context.Context, id uuid.UUID, version int, in PayInput) (ClaimView, error) {
	if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermPaymentPost); err != nil {
		return ClaimView{}, err
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.Finance.GetClaim(ctx, id, true)
		if err != nil {
			return err
		}
		if c.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		if c.Status != domain.StatusAwaitingPayment {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Pengajuan tidak menunggu pembayaran.")
		}
		if in.PaymentDate.IsZero() {
			return apperr.Validation(map[string]string{"paymentDate": "Tanggal bayar wajib diisi"})
		}
		req := PaymentRequest{
			Direction: domain.PaymentOutgoing, BankAccountID: in.BankAccountID, PaymentDate: in.PaymentDate, Amount: c.Payable,
			CounterAccountCode: domain.ExpensePayableCode, PartyName: c.PayeeName, Reference: strings.TrimSpace(in.Reference),
			Description: "Pembayaran " + c.Number, ProofFileID: in.ProofFileID, SourceType: domain.SourceExpenseClaim,
			SourceID: c.ID, SourceLabel: c.Number,
		}
		if c.PayeeType == domain.PayeeSupplier {
			req.PartyType, req.PartyID = string(domain.PayeeSupplier), c.PayeeID
		}
		p, err := s.recordPayment(ctx, req)
		if err != nil {
			return err
		}
		return s.markClaimPaid(ctx, c.ID, p)
	})
	if err != nil {
		return ClaimView{}, err
	}
	c, err := s.Finance.GetClaim(ctx, id, false)
	if err != nil {
		return ClaimView{}, err
	}
	return s.claimView(ctx, c)
}

// markClaimPaid menandai pengajuan Lunas oleh pembayaran (dari Bayar atau rekonsiliasi bank).
func (s *Service) markClaimPaid(ctx context.Context, id uuid.UUID, p domain.Payment) error {
	var requester uuid.UUID
	var label string
	_, err := s.mutateClaim(ctx, id, -1, false, audit.ActionUpdate, func(_ context.Context, c *domain.ExpenseClaim) (string, bool, error) {
		requester, label = c.RequesterID, c.Label()
		return "Dibayar " + p.Number, false, c.MarkPaid(p.ID, p.Number, s.Clock.Now())
	})
	if err != nil {
		return err
	}
	s.notify(ctx, []uuid.UUID{requester}, "expense_claim.paid", "Pengajuan biaya dibayar",
		label+" sudah dibayar ("+p.Number+").", "/finance/expense-claims/"+id.String())
	return nil
}
