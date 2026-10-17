package http

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
	"balans/internal/platform/softdelete"
	"balans/internal/tax"
)

// --- Konversi umum ----------------------------------------------------------------------------

func optDatePtr(d *clock.BusinessDate) *BusinessDate {
	if d == nil || d.IsZero() {
		return nil
	}
	v := toDate(*d)
	return &v
}

func moneyStr(a money.Amount) Money { return a.String() }

func optMoneyStr(a *money.Amount) *Money {
	if a == nil {
		return nil
	}
	v := a.String()
	return &v
}

func pct(d decimal.Decimal) TaxPercent {
	return d.Round(4).String()
}

func parsePct(p *TaxPercent, field string, fields map[string]string) decimal.Decimal {
	if p == nil || strings.TrimSpace(*p) == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(strings.TrimSpace(*p))
	if err != nil {
		fields[field] = "Persentase tidak valid"
	}
	return d
}

func actionsOf(list []string) []FinanceAction {
	out := make([]FinanceAction, len(list))
	for i, a := range list {
		out[i] = FinanceAction(a)
	}
	return out
}

func validationOrNil(fields map[string]string) error {
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func bodyRequired[T any](b *T) error {
	if b == nil {
		return apperr.Validation(map[string]string{"body": "Isi permintaan wajib diisi"})
	}
	return nil
}

func listMeta(p paging.Page, total int64) (int, int, int64, int) {
	return p.Number, p.Size, total, p.TotalPages(total)
}

// --- Rekening ---------------------------------------------------------------------------------

func toBankAccount(ctx context.Context, b domain.BankAccount, full bool) CashBankAccount {
	out := CashBankAccount{
		Id: b.ID, Code: b.Code, Name: b.Name, Kind: CashBankAccountKind(b.Kind), BankName: b.BankName,
		AccountNumberMasked: b.MaskedNumber(), AccountHolder: b.AccountHolder, AccountId: b.AccountID, AccountCode: b.AccountCode,
		AccountName: b.AccountName, IsActive: b.IsActive, Description: b.Description, DeletedAt: b.DeletedAt,
		UpdatedAt: b.UpdatedAt, Version: b.Version, AllowedActions: []FinanceAction{},
	}
	if full && b.AccountNumber != "" {
		n := b.AccountNumber
		out.AccountNumber = &n
	}
	a, _ := actor.From(ctx)
	if a.Has(service.PermPaymentWrite) {
		if b.DeletedAt == nil {
			out.AllowedActions = []FinanceAction{"edit", "delete"}
		} else if softdelete.CanRestore(a, service.PermPaymentWrite) {
			out.AllowedActions = []FinanceAction{"restore"}
		}
	}
	return out
}

func bankInput(code, name string, kind CashBankAccountKind, bankName, number, holder *string, accountID uuid.UUID, active bool, desc *string) domain.BankAccountInput {
	return domain.BankAccountInput{
		Code: code, Name: name, Kind: domain.BankAccountKind(kind), BankName: str(bankName), AccountNumber: str(number),
		AccountHolder: str(holder), AccountID: accountID, IsActive: active, Description: str(desc),
	}
}

// ListCashBankAccounts mengembalikan rekening kas & bank.
func (h *AccountingHandler) ListCashBankAccounts(ctx context.Context, req ListCashBankAccountsRequestObject) (ListCashBankAccountsResponseObject, error) {
	list, err := h.svc.ListBankAccounts(ctx, includeInactive(req.Params.IncludeInactive), includeInactive(req.Params.Deleted))
	if err != nil {
		return nil, err
	}
	out := make([]CashBankAccount, len(list))
	for i, b := range list {
		out[i] = toBankAccount(ctx, b, false)
	}
	return ListCashBankAccounts200JSONResponse(out), nil
}

// CreateCashBankAccount menambah rekening.
func (h *AccountingHandler) CreateCashBankAccount(ctx context.Context, req CreateCashBankAccountRequestObject) (CreateCashBankAccountResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	acc, err := h.svc.CreateBankAccount(ctx, bankInput(b.Code, b.Name, b.Kind, b.BankName, b.AccountNumber, b.AccountHolder, b.AccountId, b.IsActive, b.Description))
	if err != nil {
		return nil, err
	}
	return CreateCashBankAccount201JSONResponse(toBankAccount(ctx, acc, true)), nil
}

// GetCashBankAccount membaca detail rekening.
func (h *AccountingHandler) GetCashBankAccount(ctx context.Context, req GetCashBankAccountRequestObject) (GetCashBankAccountResponseObject, error) {
	acc, err := h.svc.GetBankAccount(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetCashBankAccount200JSONResponse(toBankAccount(ctx, acc, true)), nil
}

// UpdateCashBankAccount mengubah rekening.
func (h *AccountingHandler) UpdateCashBankAccount(ctx context.Context, req UpdateCashBankAccountRequestObject) (UpdateCashBankAccountResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	acc, err := h.svc.UpdateBankAccount(ctx, req.Id, b.Version, bankInput("", b.Name, b.Kind, b.BankName, b.AccountNumber, b.AccountHolder, b.AccountId, b.IsActive, b.Description))
	if err != nil {
		return nil, err
	}
	return UpdateCashBankAccount200JSONResponse(toBankAccount(ctx, acc, true)), nil
}

// DeleteCashBankAccount menghapus rekening.
func (h *AccountingHandler) DeleteCashBankAccount(ctx context.Context, req DeleteCashBankAccountRequestObject) (DeleteCashBankAccountResponseObject, error) {
	if err := h.svc.DeleteBankAccount(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteCashBankAccount204Response{}, nil
}

// RestoreCashBankAccount memulihkan rekening.
func (h *AccountingHandler) RestoreCashBankAccount(ctx context.Context, req RestoreCashBankAccountRequestObject) (RestoreCashBankAccountResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	if err := h.svc.RestoreBankAccount(ctx, req.Id, req.Body.Version); err != nil {
		return nil, err
	}
	return RestoreCashBankAccount204Response{}, nil
}

// ListCashBankMutations mengembalikan mutasi rekening dengan saldo berjalan.
func (h *AccountingHandler) ListCashBankMutations(ctx context.Context, req ListCashBankMutationsRequestObject) (ListCashBankMutationsResponseObject, error) {
	p := req.Params
	res, err := h.svc.BankMutations(ctx, req.Id, fromDate(p.DateFrom), fromDate(p.DateTo), paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]BankMutation, len(res.Items))
	for i, m := range res.Items {
		items[i] = BankMutation{
			JournalLineId: m.JournalLineID, JournalEntryId: m.JournalEntryID, JournalNumber: m.JournalNumber,
			BusinessDate: toDate(m.BusinessDate), Description: m.Description, SourceType: m.SourceType, SourceLabel: m.SourceLabel,
			Memo: m.Memo, Amount: moneyStr(m.Amount), Balance: moneyStr(m.Balance), Reconciled: m.Reconciled,
		}
	}
	page, size, total, pages := listMeta(res.Page, res.Total)
	return ListCashBankMutations200JSONResponse{
		Account: toBankAccount(ctx, res.Account, false), OpeningBalance: moneyStr(res.Opening), Items: items,
		Page: page, PageSize: size, TotalItems: total, TotalPages: pages,
	}, nil
}

// --- Pembayaran -------------------------------------------------------------------------------

func toPayment(ctx context.Context, p domain.Payment) CashPayment {
	out := CashPayment{
		Id: p.ID, Number: optStr(p.Number), Direction: PaymentDirection(p.Direction), BankAccountId: p.BankAccountID,
		BankAccountCode: p.BankAccountCode, BankAccountName: p.BankAccountName, PaymentDate: toDate(p.PaymentDate),
		Amount: moneyStr(p.Amount), CounterAccountId: p.CounterAccountID, CounterAccountCode: p.CounterCode,
		CounterAccountName: p.CounterName, PartyType: optStr(p.PartyType), PartyId: p.PartyID, PartyName: p.PartyName,
		Reference: p.Reference, Description: p.Description, ProofFileId: p.ProofFileID, SourceType: p.SourceType,
		SourceId: p.SourceID, SourceLabel: p.SourceLabel, Status: CashPaymentStatus(p.Status), JournalEntryId: p.JournalEntryID,
		JournalNumber: optStr(p.JournalNumber), PostedAt: p.PostedAt, CancelReason: p.CancelReason, Reconciled: p.Reconciled,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version,
		AllowedActions: actionsOf(service.PaymentActions(ctx, p)),
	}
	return out
}

func paymentInput(dir PaymentDirection, bank uuid.UUID, date BusinessDate, amount Money, counter uuid.UUID, party, ref, desc *string,
	proof *uuid.UUID,
) (domain.PaymentInput, error) {
	fields := map[string]string{}
	in := domain.PaymentInput{
		Direction: domain.PaymentDirection(dir), BankAccountID: bank, PaymentDate: fromDate(date),
		Amount: parseMoney(amount, "amount", fields), CounterAccountID: counter, PartyName: str(party), Reference: str(ref),
		Description: str(desc), ProofFileID: proof,
	}
	return in, validationOrNil(fields)
}

// ListCashPayments mengembalikan pembayaran.
func (h *AccountingHandler) ListCashPayments(ctx context.Context, req ListCashPaymentsRequestObject) (ListCashPaymentsResponseObject, error) {
	p := req.Params
	f := service.PaymentFilter{BankAccountID: p.BankAccountId, DateFrom: optDate(p.DateFrom), DateTo: optDate(p.DateTo), Search: str(p.Q)}
	if p.Direction != nil {
		f.Direction = string(*p.Direction)
	}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	res, err := h.svc.ListPayments(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]CashPayment, len(res.Items))
	for i, x := range res.Items {
		items[i] = toPayment(ctx, x)
	}
	page, size, total, pages := listMeta(res.Page, res.Total)
	return ListCashPayments200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateCashPayment menyimpan draft pembayaran manual.
func (h *AccountingHandler) CreateCashPayment(ctx context.Context, req CreateCashPaymentRequestObject) (CreateCashPaymentResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	in, err := paymentInput(b.Direction, b.BankAccountId, b.PaymentDate, b.Amount, b.CounterAccountId, b.PartyName, b.Reference, b.Description, b.ProofFileId)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.CreatePayment(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateCashPayment201JSONResponse(toPayment(ctx, p)), nil
}

// GetCashPayment membaca pembayaran.
func (h *AccountingHandler) GetCashPayment(ctx context.Context, req GetCashPaymentRequestObject) (GetCashPaymentResponseObject, error) {
	p, err := h.svc.GetPayment(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetCashPayment200JSONResponse(toPayment(ctx, p)), nil
}

// UpdateCashPayment mengubah draft pembayaran.
func (h *AccountingHandler) UpdateCashPayment(ctx context.Context, req UpdateCashPaymentRequestObject) (UpdateCashPaymentResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	in, err := paymentInput(b.Direction, b.BankAccountId, b.PaymentDate, b.Amount, b.CounterAccountId, b.PartyName, b.Reference, b.Description, b.ProofFileId)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.UpdatePayment(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateCashPayment200JSONResponse(toPayment(ctx, p)), nil
}

// PostCashPayment memposting pembayaran.
func (h *AccountingHandler) PostCashPayment(ctx context.Context, req PostCashPaymentRequestObject) (PostCashPaymentResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	p, err := h.svc.PostPayment(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return PostCashPayment200JSONResponse(toPayment(ctx, p)), nil
}

// CancelCashPayment membatalkan pembayaran.
func (h *AccountingHandler) CancelCashPayment(ctx context.Context, req CancelCashPaymentRequestObject) (CancelCashPaymentResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	p, err := h.svc.CancelPayment(ctx, req.Id, req.Body.Version, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return CancelCashPayment200JSONResponse(toPayment(ctx, p)), nil
}

// --- Rekonsiliasi bank ------------------------------------------------------------------------

func toStatement(s domain.StatementSummary) BankStatement {
	return BankStatement{
		Id: s.ID, BankAccountId: s.BankAccountID, BankAccountCode: s.BankAccountCode, BankAccountName: s.BankAccountName,
		FileId: s.FileID, FileName: s.FileName, PeriodFrom: toDate(s.PeriodFrom), PeriodTo: toDate(s.PeriodTo),
		LineCount: s.LineCount, TotalIn: moneyStr(s.TotalIn), TotalOut: moneyStr(s.TotalOut), MatchedCount: s.MatchedCount,
		SuggestedCount: s.SuggestedCount, CreatedAt: s.CreatedAt,
	}
}

func lineActions(ctx context.Context, l domain.StatementLine) []BankLineAction {
	a, _ := actor.From(ctx)
	if !a.Has(service.PermBankReconPost) {
		return []BankLineAction{}
	}
	switch l.Status {
	case domain.LineSuggested:
		return []BankLineAction{"confirm", "match", "create_entry"}
	case domain.LineUnmatched:
		return []BankLineAction{"match", "create_entry"}
	}
	return []BankLineAction{"unmatch"}
}

func toStatementLine(ctx context.Context, l domain.StatementLine) BankStatementLine {
	out := BankStatementLine{
		Id: l.ID, StatementId: l.StatementID, LineNo: l.LineNo, TransactionDate: toDate(l.TransactionDate),
		Description: l.Description, Reference: l.Reference, Amount: moneyStr(l.Amount), Balance: optMoneyStr(l.Balance),
		Status: BankLineStatus(l.Status), JournalLineId: l.JournalLineID, JournalEntryId: l.JournalEntryID,
		JournalNumber: optStr(l.JournalNumber), CandidateSource: optStr(l.CandidateSource), CandidateDocumentId: l.CandidateDocID,
		CandidateLabel: l.CandidateLabel, PaymentId: l.PaymentID, MatchedAt: l.MatchedAt, Version: l.Version,
		AllowedActions: lineActions(ctx, l),
	}
	if l.MatchKind != "" {
		k := BankMatchKind(l.MatchKind)
		out.MatchKind = &k
	}
	if l.MatchRule != "" {
		r := BankMatchRule(l.MatchRule)
		out.MatchRule = &r
	}
	return out
}

func toStatementDetail(ctx context.Context, d service.StatementDetail) BankStatementDetail {
	s := toStatement(d.Summary)
	lines := make([]BankStatementLine, len(d.Lines))
	matched, suggested := 0, 0
	for i, l := range d.Lines {
		lines[i] = toStatementLine(ctx, l)
		switch l.Status {
		case domain.LineMatched:
			matched++
		case domain.LineSuggested:
			suggested++
		}
	}
	return BankStatementDetail{
		Id: s.Id, BankAccountId: s.BankAccountId, BankAccountCode: s.BankAccountCode, BankAccountName: s.BankAccountName,
		FileId: s.FileId, FileName: s.FileName, PeriodFrom: s.PeriodFrom, PeriodTo: s.PeriodTo, LineCount: s.LineCount,
		TotalIn: s.TotalIn, TotalOut: s.TotalOut, MatchedCount: matched, SuggestedCount: suggested, CreatedAt: s.CreatedAt,
		Lines: lines,
	}
}

// ListBankStatements mengembalikan file mutasi.
func (h *AccountingHandler) ListBankStatements(ctx context.Context, req ListBankStatementsRequestObject) (ListBankStatementsResponseObject, error) {
	p := req.Params
	res, err := h.svc.ListStatements(ctx, p.BankAccountId, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]BankStatement, len(res.Items))
	for i, s := range res.Items {
		items[i] = toStatement(s)
	}
	page, size, total, pages := listMeta(res.Page, res.Total)
	return ListBankStatements200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// ImportBankStatement mengimpor file mutasi.
func (h *AccountingHandler) ImportBankStatement(ctx context.Context, req ImportBankStatementRequestObject) (ImportBankStatementResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	d, err := h.svc.ImportStatement(ctx, req.Body.BankAccountId, req.Body.FileId)
	if err != nil {
		return nil, err
	}
	return ImportBankStatement201JSONResponse(toStatementDetail(ctx, d)), nil
}

// GetBankStatement membaca detail file mutasi.
func (h *AccountingHandler) GetBankStatement(ctx context.Context, req GetBankStatementRequestObject) (GetBankStatementResponseObject, error) {
	d, err := h.svc.GetStatement(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetBankStatement200JSONResponse(toStatementDetail(ctx, d)), nil
}

// AutoMatchBankStatement menjalankan ulang pencocokan otomatis.
func (h *AccountingHandler) AutoMatchBankStatement(ctx context.Context, req AutoMatchBankStatementRequestObject) (AutoMatchBankStatementResponseObject, error) {
	d, err := h.svc.AutoMatch(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return AutoMatchBankStatement200JSONResponse(toStatementDetail(ctx, d)), nil
}

// ConfirmBankStatementSuggestions mengonfirmasi semua usulan.
func (h *AccountingHandler) ConfirmBankStatementSuggestions(ctx context.Context, req ConfirmBankStatementSuggestionsRequestObject) (ConfirmBankStatementSuggestionsResponseObject, error) {
	n, err := h.svc.ConfirmSuggestions(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	d, err := h.svc.GetStatement(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return ConfirmBankStatementSuggestions200JSONResponse{Confirmed: n, Statement: toStatementDetail(ctx, d)}, nil
}

// ListBankStatementLineCandidates mengembalikan calon pasangan manual.
func (h *AccountingHandler) ListBankStatementLineCandidates(ctx context.Context, req ListBankStatementLineCandidatesRequestObject) (ListBankStatementLineCandidatesResponseObject, error) {
	list, err := h.svc.LineCandidates(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out := make([]BankMatchCandidate, len(list))
	for i, c := range list {
		out[i] = BankMatchCandidate{Kind: BankMatchKind(c.Kind), Label: c.Label, Date: toDate(c.Date), Amount: moneyStr(c.Amount)}
		if c.Kind == domain.MatchJournalLine {
			jl, je := c.JournalLineID, c.JournalEntryID
			out[i].JournalLineId, out[i].JournalEntryId = &jl, &je
		} else {
			src, doc := c.Source, c.DocumentID
			out[i].Source, out[i].DocumentId = &src, &doc
		}
	}
	return ListBankStatementLineCandidates200JSONResponse(out), nil
}

// ConfirmBankStatementLine mengonfirmasi usulan satu baris.
func (h *AccountingHandler) ConfirmBankStatementLine(ctx context.Context, req ConfirmBankStatementLineRequestObject) (ConfirmBankStatementLineResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	l, err := h.svc.ConfirmLine(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return ConfirmBankStatementLine200JSONResponse(toStatementLine(ctx, l)), nil
}

// MatchBankStatementLine mencocokkan manual.
func (h *AccountingHandler) MatchBankStatementLine(ctx context.Context, req MatchBankStatementLineRequestObject) (MatchBankStatementLineResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	l, err := h.svc.MatchLine(ctx, req.Id, b.Version, service.MatchTarget{JournalLineID: b.JournalLineId, Source: str(b.Source), DocumentID: b.DocumentId})
	if err != nil {
		return nil, err
	}
	return MatchBankStatementLine200JSONResponse(toStatementLine(ctx, l)), nil
}

// CreateBankStatementLineEntry membuat jurnal dari baris tak cocok.
func (h *AccountingHandler) CreateBankStatementLineEntry(ctx context.Context, req CreateBankStatementLineEntryRequestObject) (CreateBankStatementLineEntryResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	l, err := h.svc.CreateEntryFromLine(ctx, req.Id, b.Version, service.NewEntryInput{CounterAccountID: b.CounterAccountId, Description: str(b.Description)})
	if err != nil {
		return nil, err
	}
	return CreateBankStatementLineEntry200JSONResponse(toStatementLine(ctx, l)), nil
}

// UnmatchBankStatementLine melepas pencocokan.
func (h *AccountingHandler) UnmatchBankStatementLine(ctx context.Context, req UnmatchBankStatementLineRequestObject) (UnmatchBankStatementLineResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	l, err := h.svc.UnmatchLine(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return UnmatchBankStatementLine200JSONResponse(toStatementLine(ctx, l)), nil
}

// --- Pajak (dipakai beberapa bagian) ----------------------------------------------------------

func toSlip(w domain.WithholdingRecord) WithholdingSlip {
	out := WithholdingSlip{
		SourceType: w.SourceType, SourceId: w.SourceID, SourceLabel: w.SourceLabel, BusinessDate: toDate(w.BusinessDate),
		TaxYear: w.Period.Year, TaxMonth: int(w.Period.Month), TaxType: TaxType(w.TaxType), ObjectCode: w.ObjectCode,
		ObjectName: w.ObjectName, TaxpayerName: w.TaxpayerName, TaxpayerTaxIdMasked: pii.Mask(w.TaxpayerTaxID),
		HasTaxId: w.HasTaxID, GrossAmount: moneyStr(w.Gross), TaxableBase: moneyStr(w.TaxableBase), RatePercent: pct(w.RatePercent),
		TaxAmount: moneyStr(w.TaxAmount), Status: WithholdingSlipStatus(w.Status), DepositId: w.DepositID,
		DepositNumber: optStr(w.DepositNumber), SlipNumber: w.SlipNumber, Version: w.Version,
	}
	if w.ID != uuid.Nil {
		id := w.ID
		out.Id = &id
	}
	if w.Status == "" {
		out.Status = WithholdingSlipStatus(domain.WithholdingRecorded)
	}
	return out
}

// --- Pengajuan biaya --------------------------------------------------------------------------

func toClaimSummary(c domain.ExpenseClaim) ExpenseClaimSummary {
	return ExpenseClaimSummary{
		Id: c.ID, Number: optStr(c.Number), ClaimType: ExpenseClaimType(c.ClaimType), RequesterId: c.RequesterID,
		RequesterName: c.RequesterName, DivisionId: c.DivisionID, DivisionName: c.DivisionName, CampaignId: c.CampaignID,
		CampaignName: optStr(c.CampaignName), PayeeType: ExpenseClaimPayeeType(c.PayeeType), PayeeName: c.PayeeName,
		ClaimDate: toDate(c.ClaimDate), Description: c.Description, TotalAmount: moneyStr(c.Total),
		WithholdingAmount: moneyStr(c.Withholding), PayableAmount: moneyStr(c.Payable), Status: ExpenseClaimStatus(c.Status),
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: c.Version,
	}
}

func toClaimDetail(ctx context.Context, v service.ClaimView) ExpenseClaimDetail {
	c := v.Claim
	s := toClaimSummary(c)
	lines := make([]ExpenseClaimLine, len(c.Lines))
	for i, l := range c.Lines {
		lines[i] = ExpenseClaimLine{
			Id: l.ID, LineNo: l.LineNo, ExpenseTypeId: l.ExpenseTypeID, ExpenseTypeCode: l.ExpenseTypeCode,
			ExpenseTypeName: l.ExpenseTypeName, ExpenseAccountCode: l.ExpenseAccountCode, Description: l.Description,
			Amount: moneyStr(l.Amount), ReceiptFileId: l.ReceiptFileID, ReceiptFileName: optStr(l.ReceiptFileName),
			WithholdingType: WithholdingType(l.WithholdingType), ObjectCode: l.ObjectCode, WithholdingRate: pct(l.WithholdingRate),
			WithholdingAmount: moneyStr(l.WithholdingAmount),
		}
	}
	slips := make([]WithholdingSlip, len(v.Withholdings))
	for i, w := range v.Withholdings {
		slips[i] = toSlip(w)
	}
	out := ExpenseClaimDetail{
		Id: s.Id, Number: s.Number, ClaimType: s.ClaimType, RequesterId: s.RequesterId, RequesterName: s.RequesterName,
		DivisionId: s.DivisionId, DivisionName: s.DivisionName, CampaignId: s.CampaignId, CampaignName: s.CampaignName,
		PayeeType: s.PayeeType, PayeeName: s.PayeeName, ClaimDate: s.ClaimDate, Description: s.Description,
		TotalAmount: s.TotalAmount, WithholdingAmount: s.WithholdingAmount, PayableAmount: s.PayableAmount, Status: s.Status,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Version: s.Version,
		PayeeId: c.PayeeID, PayeeHasTaxId: c.PayeeTaxID != "", Lines: lines, SubmittedAt: c.SubmittedAt, DecidedAt: c.DecidedAt,
		DecisionNote: c.DecisionNote, VerifiedAt: c.VerifiedAt, VerifyDate: optDatePtr(c.VerifyDate), JournalEntryId: c.JournalEntryID,
		JournalNumber: optStr(c.JournalNumber), PaymentId: c.PaymentID, PaymentNumber: optStr(c.PaymentNumber), PaidAt: c.PaidAt,
		CancelReason: c.CancelReason, Withholdings: slips, AllowedActions: actionsOf(service.ClaimActions(ctx, c)),
	}
	if c.PayeeTaxID != "" {
		m := pii.Mask(c.PayeeTaxID)
		out.PayeeTaxIdMasked = &m
	}
	return out
}

func claimInput(t ExpenseClaimType, division uuid.UUID, campaign *uuid.UUID, payeeType ExpenseClaimPayeeType, payeeID *uuid.UUID,
	payeeName, payeeTaxID *string, date BusinessDate, desc *string, lines []ExpenseClaimLineInput,
) (domain.ClaimInput, error) {
	fields := map[string]string{}
	in := domain.ClaimInput{
		ClaimType: domain.ClaimType(t), DivisionID: division, CampaignID: campaign, PayeeType: domain.PayeeType(payeeType),
		PayeeID: payeeID, PayeeName: str(payeeName), PayeeTaxID: str(payeeTaxID), ClaimDate: fromDate(date), Description: str(desc),
		Lines: make([]domain.ClaimLineInput, len(lines)),
	}
	for i, l := range lines {
		in.Lines[i] = domain.ClaimLineInput{
			ExpenseTypeID: l.ExpenseTypeId, Description: str(l.Description), ReceiptFileID: l.ReceiptFileId,
			Amount: parseMoney(l.Amount, "lines."+strconv.Itoa(i)+".amount", fields),
		}
	}
	return in, validationOrNil(fields)
}

// GetExpenseClaimOptions mengembalikan pilihan form pengajuan.
func (h *AccountingHandler) GetExpenseClaimOptions(ctx context.Context, _ GetExpenseClaimOptionsRequestObject) (GetExpenseClaimOptionsResponseObject, error) {
	o, err := h.svc.ExpenseClaimOptions(ctx)
	if err != nil {
		return nil, err
	}
	opt := func(list []domain.DimensionValue) []ExpenseClaimOption {
		out := make([]ExpenseClaimOption, len(list))
		for i, d := range list {
			out[i] = ExpenseClaimOption{Id: d.ID, Code: d.Code, Name: d.Name}
		}
		return out
	}
	types := make([]ExpenseClaimTypeOption, len(o.ExpenseTypes))
	for i, t := range o.ExpenseTypes {
		types[i] = ExpenseClaimTypeOption{
			Id: t.ID, Code: t.Code, Name: t.Name, WithholdingType: WithholdingType(t.WithholdingType), RequiresReceipt: t.RequiresReceipt,
			RequiresCampaign: t.RequiresCampaign, ExpenseAccountCode: t.ExpenseAccountCode,
		}
	}
	return GetExpenseClaimOptions200JSONResponse{Divisions: opt(o.Divisions), Campaigns: opt(o.Campaigns), ExpenseTypes: types}, nil
}

// ListExpenseClaims mengembalikan pengajuan biaya.
func (h *AccountingHandler) ListExpenseClaims(ctx context.Context, req ListExpenseClaimsRequestObject) (ListExpenseClaimsResponseObject, error) {
	p := req.Params
	f := service.ClaimFilter{DivisionID: p.DivisionId, DateFrom: optDate(p.DateFrom), DateTo: optDate(p.DateTo), Search: str(p.Q)}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	res, err := h.svc.ListExpenseClaims(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]ExpenseClaimSummary, len(res.Items))
	for i, c := range res.Items {
		items[i] = toClaimSummary(c)
	}
	page, size, total, pages := listMeta(res.Page, res.Total)
	return ListExpenseClaims200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreateExpenseClaim menyimpan draft pengajuan.
func (h *AccountingHandler) CreateExpenseClaim(ctx context.Context, req CreateExpenseClaimRequestObject) (CreateExpenseClaimResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	in, err := claimInput(b.ClaimType, b.DivisionId, b.CampaignId, b.PayeeType, b.PayeeId, b.PayeeName, b.PayeeTaxId, b.ClaimDate, b.Description, b.Lines)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.CreateExpenseClaim(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateExpenseClaim201JSONResponse(toClaimDetail(ctx, v)), nil
}

// GetExpenseClaim membaca detail pengajuan.
func (h *AccountingHandler) GetExpenseClaim(ctx context.Context, req GetExpenseClaimRequestObject) (GetExpenseClaimResponseObject, error) {
	v, err := h.svc.GetExpenseClaim(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetExpenseClaim200JSONResponse(toClaimDetail(ctx, v)), nil
}

// UpdateExpenseClaim mengubah draft pengajuan.
func (h *AccountingHandler) UpdateExpenseClaim(ctx context.Context, req UpdateExpenseClaimRequestObject) (UpdateExpenseClaimResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	in, err := claimInput(b.ClaimType, b.DivisionId, b.CampaignId, b.PayeeType, b.PayeeId, b.PayeeName, b.PayeeTaxId, b.ClaimDate, b.Description, b.Lines)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.UpdateExpenseClaim(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateExpenseClaim200JSONResponse(toClaimDetail(ctx, v)), nil
}

// SubmitExpenseClaim mengajukan pengajuan.
func (h *AccountingHandler) SubmitExpenseClaim(ctx context.Context, req SubmitExpenseClaimRequestObject) (SubmitExpenseClaimResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	v, err := h.svc.SubmitExpenseClaim(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return SubmitExpenseClaim200JSONResponse(toClaimDetail(ctx, v)), nil
}

// ReviseExpenseClaim merevisi pengajuan ditolak.
func (h *AccountingHandler) ReviseExpenseClaim(ctx context.Context, req ReviseExpenseClaimRequestObject) (ReviseExpenseClaimResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	v, err := h.svc.ReviseExpenseClaim(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return ReviseExpenseClaim200JSONResponse(toClaimDetail(ctx, v)), nil
}

// CancelExpenseClaim membatalkan pengajuan.
func (h *AccountingHandler) CancelExpenseClaim(ctx context.Context, req CancelExpenseClaimRequestObject) (CancelExpenseClaimResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	v, err := h.svc.CancelExpenseClaim(ctx, req.Id, req.Body.Version, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return CancelExpenseClaim200JSONResponse(toClaimDetail(ctx, v)), nil
}

func verifyInput(b ExpenseClaimVerifyInput) service.VerifyInput {
	in := service.VerifyInput{PayeeTaxID: b.PayeeTaxId, Lines: make([]domain.LineTaxChoice, len(b.Lines))}
	for i, l := range b.Lines {
		in.Lines[i] = domain.LineTaxChoice{LineID: l.LineId, Type: domain.WithholdingType(l.WithholdingType), ObjectCode: str(l.ObjectCode)}
	}
	return in
}

// PreviewExpenseClaimVerification menghitung PPh tanpa menyimpan.
func (h *AccountingHandler) PreviewExpenseClaimVerification(ctx context.Context, req PreviewExpenseClaimVerificationRequestObject) (PreviewExpenseClaimVerificationResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	records, total, err := h.svc.PreviewExpenseClaimVerification(ctx, req.Id, verifyInput(*req.Body))
	if err != nil {
		return nil, err
	}
	v, err := h.svc.GetExpenseClaim(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	slips := make([]WithholdingSlip, len(records))
	for i, r := range records {
		slips[i] = toSlip(r)
	}
	return PreviewExpenseClaimVerification200JSONResponse{
		Withholdings: slips, TotalWithholding: moneyStr(total), PayableAmount: moneyStr(v.Claim.Total.Sub(total)),
	}, nil
}

// VerifyExpenseClaim memverifikasi pengajuan.
func (h *AccountingHandler) VerifyExpenseClaim(ctx context.Context, req VerifyExpenseClaimRequestObject) (VerifyExpenseClaimResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	v, err := h.svc.VerifyExpenseClaim(ctx, req.Id, req.Body.Version, verifyInput(*req.Body))
	if err != nil {
		return nil, err
	}
	return VerifyExpenseClaim200JSONResponse(toClaimDetail(ctx, v)), nil
}

// PayExpenseClaim membayar pengajuan.
func (h *AccountingHandler) PayExpenseClaim(ctx context.Context, req PayExpenseClaimRequestObject) (PayExpenseClaimResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	v, err := h.svc.PayExpenseClaim(ctx, req.Id, b.Version, service.PayInput{
		BankAccountID: b.BankAccountId, PaymentDate: fromDate(b.PaymentDate), Reference: str(b.Reference), ProofFileID: b.ProofFileId,
	})
	if err != nil {
		return nil, err
	}
	return PayExpenseClaim200JSONResponse(toClaimDetail(ctx, v)), nil
}

// --- Kas kecil --------------------------------------------------------------------------------

func toFund(f domain.PettyCashFund) PettyCashFund {
	return PettyCashFund{
		Id: f.ID, Code: f.Code, Name: f.Name, HolderUserId: f.HolderUserID, HolderName: f.HolderName, DivisionId: f.DivisionID,
		DivisionName: f.DivisionName, CeilingAmount: moneyStr(f.Ceiling), BalanceAmount: moneyStr(f.Balance),
		ShortfallAmount: moneyStr(f.Shortfall()), IsActive: f.IsActive, Description: f.Description, UpdatedAt: f.UpdatedAt,
		Version: f.Version,
	}
}

func toPettyCash(ctx context.Context, t domain.PettyCashTxn) PettyCashTransaction {
	return PettyCashTransaction{
		Id: t.ID, Number: optStr(t.Number), FundId: t.FundID, FundCode: t.FundCode, FundName: t.FundName, Type: PettyCashType(t.Type),
		BusinessDate: toDate(t.BusinessDate), ExpenseTypeId: t.ExpenseTypeID, ExpenseTypeCode: t.ExpenseTypeCode,
		ExpenseTypeName: t.ExpenseTypeName, DivisionId: t.DivisionID, DivisionName: t.DivisionName, CampaignId: t.CampaignID,
		CampaignName: t.CampaignName, Description: t.Description, Amount: moneyStr(t.Amount), ReceiptFileId: t.ReceiptFileID,
		ReceiptFileName: optStr(t.ReceiptFileName), BankAccountId: t.BankAccountID, BankAccountName: optStr(t.BankAccountName),
		Status: PettyCashStatus(t.Status), DecisionNote: t.DecisionNote, JournalEntryId: t.JournalEntryID,
		JournalNumber: optStr(t.JournalNumber), PostedAt: t.PostedAt, CancelReason: t.CancelReason, CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt, Version: t.Version, AllowedActions: actionsOf(service.PettyCashActions(ctx, t)),
	}
}

// ListPettyCashFunds mengembalikan dana kas kecil.
func (h *AccountingHandler) ListPettyCashFunds(ctx context.Context, req ListPettyCashFundsRequestObject) (ListPettyCashFundsResponseObject, error) {
	list, err := h.svc.ListPettyCashFunds(ctx, includeInactive(req.Params.IncludeInactive))
	if err != nil {
		return nil, err
	}
	items := make([]PettyCashFund, len(list))
	for i, f := range list {
		items[i] = toFund(f)
	}
	lim := service.CurrentPettyCashLimits()
	return ListPettyCashFunds200JSONResponse{
		Items: items, MaxPerTransaction: moneyStr(lim.MaxPerTransaction), DefaultCeiling: moneyStr(lim.DefaultCeiling),
	}, nil
}

// CreatePettyCashFund menambah dana.
func (h *AccountingHandler) CreatePettyCashFund(ctx context.Context, req CreatePettyCashFundRequestObject) (CreatePettyCashFundResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	fields := map[string]string{}
	in := domain.FundInput{
		Code: b.Code, Name: b.Name, HolderUserID: b.HolderUserId, DivisionID: b.DivisionId, IsActive: b.IsActive, Description: str(b.Description),
	}
	if c := parseOptMoney(b.CeilingAmount, "ceilingAmount", fields); c != nil {
		in.Ceiling = *c
	}
	if err := validationOrNil(fields); err != nil {
		return nil, err
	}
	f, err := h.svc.CreatePettyCashFund(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreatePettyCashFund201JSONResponse(toFund(f)), nil
}

// UpdatePettyCashFund mengubah dana.
func (h *AccountingHandler) UpdatePettyCashFund(ctx context.Context, req UpdatePettyCashFundRequestObject) (UpdatePettyCashFundResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	fields := map[string]string{}
	in := domain.FundInput{
		Name: b.Name, HolderUserID: b.HolderUserId, DivisionID: b.DivisionId, IsActive: b.IsActive, Description: str(b.Description),
		Ceiling: parseMoney(b.CeilingAmount, "ceilingAmount", fields),
	}
	if err := validationOrNil(fields); err != nil {
		return nil, err
	}
	f, err := h.svc.UpdatePettyCashFund(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdatePettyCashFund200JSONResponse(toFund(f)), nil
}

// ListPettyCashTransactions mengembalikan transaksi kas kecil.
func (h *AccountingHandler) ListPettyCashTransactions(ctx context.Context, req ListPettyCashTransactionsRequestObject) (ListPettyCashTransactionsResponseObject, error) {
	p := req.Params
	f := service.PettyCashFilter{FundID: p.FundId, DateFrom: optDate(p.DateFrom), DateTo: optDate(p.DateTo)}
	if p.Type != nil {
		f.Type = string(*p.Type)
	}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	res, err := h.svc.ListPettyCash(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]PettyCashTransaction, len(res.Items))
	for i, t := range res.Items {
		items[i] = toPettyCash(ctx, t)
	}
	page, size, total, pages := listMeta(res.Page, res.Total)
	return ListPettyCashTransactions200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// CreatePettyCashTransaction menyimpan draft transaksi kas kecil.
func (h *AccountingHandler) CreatePettyCashTransaction(ctx context.Context, req CreatePettyCashTransactionRequestObject) (CreatePettyCashTransactionResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	fields := map[string]string{}
	in := domain.PettyCashInput{
		Type: domain.PettyCashType(b.Type), FundID: b.FundId, BusinessDate: fromDate(b.BusinessDate), ExpenseTypeID: b.ExpenseTypeId,
		CampaignID: b.CampaignId, Description: str(b.Description), ReceiptFileID: b.ReceiptFileId,
	}
	if b.DivisionId != nil {
		in.DivisionID = *b.DivisionId
	}
	if a := parseOptMoney(b.Amount, "amount", fields); a != nil {
		in.Amount = *a
	}
	if err := validationOrNil(fields); err != nil {
		return nil, err
	}
	t, err := h.svc.CreatePettyCash(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreatePettyCashTransaction201JSONResponse(toPettyCash(ctx, t)), nil
}

// GetPettyCashTransaction membaca transaksi kas kecil.
func (h *AccountingHandler) GetPettyCashTransaction(ctx context.Context, req GetPettyCashTransactionRequestObject) (GetPettyCashTransactionResponseObject, error) {
	t, err := h.svc.GetPettyCash(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetPettyCashTransaction200JSONResponse(toPettyCash(ctx, t)), nil
}

// UpdatePettyCashTransaction mengubah draft transaksi kas kecil.
func (h *AccountingHandler) UpdatePettyCashTransaction(ctx context.Context, req UpdatePettyCashTransactionRequestObject) (UpdatePettyCashTransactionResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	fields := map[string]string{}
	in := domain.PettyCashInput{
		BusinessDate: fromDate(b.BusinessDate), ExpenseTypeID: b.ExpenseTypeId, CampaignID: b.CampaignId, Description: str(b.Description),
		ReceiptFileID: b.ReceiptFileId, Amount: parseMoney(b.Amount, "amount", fields),
	}
	if b.DivisionId != nil {
		in.DivisionID = *b.DivisionId
	}
	if err := validationOrNil(fields); err != nil {
		return nil, err
	}
	t, err := h.svc.UpdatePettyCash(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdatePettyCashTransaction200JSONResponse(toPettyCash(ctx, t)), nil
}

// SubmitPettyCashTransaction mengajukan pengisian kembali.
func (h *AccountingHandler) SubmitPettyCashTransaction(ctx context.Context, req SubmitPettyCashTransactionRequestObject) (SubmitPettyCashTransactionResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	t, err := h.svc.SubmitPettyCash(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return SubmitPettyCashTransaction200JSONResponse(toPettyCash(ctx, t)), nil
}

// RevisePettyCashTransaction merevisi pengisian ditolak.
func (h *AccountingHandler) RevisePettyCashTransaction(ctx context.Context, req RevisePettyCashTransactionRequestObject) (RevisePettyCashTransactionResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	t, err := h.svc.RevisePettyCash(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return RevisePettyCashTransaction200JSONResponse(toPettyCash(ctx, t)), nil
}

// CancelPettyCashTransaction membatalkan transaksi kas kecil.
func (h *AccountingHandler) CancelPettyCashTransaction(ctx context.Context, req CancelPettyCashTransactionRequestObject) (CancelPettyCashTransactionResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	t, err := h.svc.CancelPettyCash(ctx, req.Id, req.Body.Version, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return CancelPettyCashTransaction200JSONResponse(toPettyCash(ctx, t)), nil
}

// PostPettyCashTransaction memposting transaksi kas kecil.
func (h *AccountingHandler) PostPettyCashTransaction(ctx context.Context, req PostPettyCashTransactionRequestObject) (PostPettyCashTransactionResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	t, err := h.svc.PostPettyCash(ctx, req.Id, req.Body.Version, service.PostPettyCashInput{BankAccountID: req.Body.BankAccountId})
	if err != nil {
		return nil, err
	}
	return PostPettyCashTransaction200JSONResponse(toPettyCash(ctx, t)), nil
}

// --- Pajak & Bukti Potong ---------------------------------------------------------------------

func toTaxRate(r domain.RateVersion) TaxRate {
	brackets := make([]TaxRateBracket, len(r.Brackets))
	for i, b := range r.Brackets {
		brackets[i] = TaxRateBracket{RatePercent: pct(b.RatePercent)}
		if !b.UpTo.IsZero() {
			v := b.UpTo.String()
			brackets[i].UpTo = &v
		}
	}
	base := r.BasePercent
	if base.IsZero() {
		base = decimal.NewFromInt(100)
	}
	return TaxRate{
		Id: r.ID, TaxType: TaxType(r.Type), ObjectCode: r.ObjectCode, Name: r.Name, RatePercent: pct(r.RatePercent),
		Brackets: brackets, BasePercent: pct(base), NoTaxIdSurchargePercent: pct(r.NoTaxIDSurchargePercent), IsDefault: r.IsDefault,
		ValidFrom: toDate(r.ValidFrom), ValidTo: optDatePtr(r.ValidTo), Note: r.Note, Version: r.Version,
	}
}

func taxTypeParam(t *TaxType) string {
	if t == nil {
		return ""
	}
	return string(*t)
}

// ListTaxRates mengembalikan versi tarif.
func (h *AccountingHandler) ListTaxRates(ctx context.Context, req ListTaxRatesRequestObject) (ListTaxRatesResponseObject, error) {
	list, err := h.svc.ListTaxRates(ctx, taxTypeParam(req.Params.TaxType))
	if err != nil {
		return nil, err
	}
	out := make([]TaxRate, len(list))
	for i, r := range list {
		out[i] = toTaxRate(r)
	}
	return ListTaxRates200JSONResponse(out), nil
}

// CreateTaxRate menambah versi tarif.
func (h *AccountingHandler) CreateTaxRate(ctx context.Context, req CreateTaxRateRequestObject) (CreateTaxRateResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	fields := map[string]string{}
	r := tax.Rate{
		Type: tax.Type(b.TaxType), ObjectCode: b.ObjectCode, Name: b.Name, RatePercent: parsePct(b.RatePercent, "ratePercent", fields),
		BasePercent: parsePct(b.BasePercent, "basePercent", fields), ValidFrom: fromDate(b.ValidFrom),
		NoTaxIDSurchargePercent: parsePct(b.NoTaxIdSurchargePercent, "noTaxIdSurchargePercent", fields),
	}
	if b.Brackets != nil {
		for _, br := range *b.Brackets {
			rate := br.RatePercent
			nb := tax.Bracket{RatePercent: parsePct(&rate, "brackets", fields), UpTo: money.Zero}
			if a := parseOptMoney(br.UpTo, "brackets", fields); a != nil {
				nb.UpTo = *a
			}
			r.Brackets = append(r.Brackets, nb)
		}
	}
	if err := validationOrNil(fields); err != nil {
		return nil, err
	}
	v, err := h.svc.CreateTaxRate(ctx, r, b.IsDefault, str(b.Note))
	if err != nil {
		return nil, err
	}
	return CreateTaxRate201JSONResponse(toTaxRate(v)), nil
}

// ListWithholdingSlips mengembalikan rekap bukti potong.
func (h *AccountingHandler) ListWithholdingSlips(ctx context.Context, req ListWithholdingSlipsRequestObject) (ListWithholdingSlipsResponseObject, error) {
	p := req.Params
	res, err := h.svc.ListWithholdings(ctx, service.WithholdingFilter{
		Period: tax.Period{Year: p.Year, Month: time.Month(p.Month)}, TaxType: taxTypeParam(p.TaxType),
		IncludeCancelled: includeInactive(p.IncludeCancelled),
	}, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]WithholdingSlip, len(res.Items))
	for i, w := range res.Items {
		items[i] = toSlip(w)
	}
	page, size, total, pages := listMeta(res.Page, res.Total)
	return ListWithholdingSlips200JSONResponse{Items: items, Page: page, PageSize: size, TotalItems: total, TotalPages: pages}, nil
}

// ListWithholdingSummary meringkas utang PPh potong setahun.
func (h *AccountingHandler) ListWithholdingSummary(ctx context.Context, req ListWithholdingSummaryRequestObject) (ListWithholdingSummaryResponseObject, error) {
	list, err := h.svc.WithholdingSummary(ctx, req.Params.Year)
	if err != nil {
		return nil, err
	}
	out := make([]WithholdingSummaryRow, len(list))
	for i, m := range list {
		out[i] = WithholdingSummaryRow{
			TaxType: TaxType(m.TaxType), TaxYear: m.Period.Year, TaxMonth: int(m.Period.Month), SlipCount: int(m.SlipCount),
			OpenAmount: moneyStr(m.Open), DepositedAmount: moneyStr(m.Deposited),
		}
	}
	return ListWithholdingSummary200JSONResponse(out), nil
}

// SetWithholdingSlipNumber mencatat nomor bukti potong.
func (h *AccountingHandler) SetWithholdingSlipNumber(ctx context.Context, req SetWithholdingSlipNumberRequestObject) (SetWithholdingSlipNumberResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	w, err := h.svc.SetSlipNumber(ctx, req.Id, req.Body.Version, req.Body.SlipNumber)
	if err != nil {
		return nil, err
	}
	return SetWithholdingSlipNumber200JSONResponse(toSlip(w)), nil
}

// ExportWithholdingRecap mengekspor rekap bukti potong.
func (h *AccountingHandler) ExportWithholdingRecap(ctx context.Context, req ExportWithholdingRecapRequestObject) (ExportWithholdingRecapResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	link, err := h.svc.ExportWithholdingRecap(ctx, tax.Period{Year: b.Year, Month: time.Month(b.Month)}, taxTypeParam(b.TaxType))
	if err != nil {
		return nil, err
	}
	return ExportWithholdingRecap200JSONResponse{FileId: link.FileID, Url: link.URL, ExpiresAt: link.ExpiresAt}, nil
}

func toDeposit(d domain.TaxDeposit) TaxDeposit {
	return TaxDeposit{
		Id: d.ID, Number: d.Number, TaxType: TaxType(d.TaxType), TaxYear: d.Period.Year, TaxMonth: int(d.Period.Month),
		BankAccountId: d.BankAccountID, BankAccountName: d.BankAccountName, DepositDate: toDate(d.DepositDate),
		Amount: moneyStr(d.Amount), BillingCode: d.BillingCode, Ntpn: d.NTPN, JournalEntryId: d.JournalEntryID,
		JournalNumber: d.JournalNumber, CreatedAt: d.CreatedAt,
	}
}

// ListTaxDeposits mengembalikan penyetoran pajak.
func (h *AccountingHandler) ListTaxDeposits(ctx context.Context, req ListTaxDepositsRequestObject) (ListTaxDepositsResponseObject, error) {
	list, err := h.svc.ListTaxDeposits(ctx, req.Params.Year)
	if err != nil {
		return nil, err
	}
	out := make([]TaxDeposit, len(list))
	for i, d := range list {
		out[i] = toDeposit(d)
	}
	return ListTaxDeposits200JSONResponse(out), nil
}

// CreateTaxDeposit menyetor pajak.
func (h *AccountingHandler) CreateTaxDeposit(ctx context.Context, req CreateTaxDepositRequestObject) (CreateTaxDepositResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	d, err := h.svc.CreateTaxDeposit(ctx, domain.TaxDepositInput{
		TaxType: tax.Type(b.TaxType), Period: tax.Period{Year: b.TaxYear, Month: time.Month(b.TaxMonth)}, BankAccountID: b.BankAccountId,
		DepositDate: fromDate(b.DepositDate), BillingCode: str(b.BillingCode), NTPN: str(b.Ntpn),
	})
	if err != nil {
		return nil, err
	}
	return CreateTaxDeposit201JSONResponse(toDeposit(d)), nil
}

func toFinalSetting(s domain.FinalTaxSetting) FinalTaxSetting {
	return FinalTaxSetting{
		Id: s.ID, ValidFrom: toDate(s.ValidFrom), IsEnabled: s.IsEnabled, OpeningYear: s.OpeningYear,
		OpeningTurnover: moneyStr(s.OpeningTurnover), Note: s.Note, CreatedAt: s.CreatedAt,
	}
}

func toFinalRun(r domain.FinalTaxRun) FinalTaxRun {
	return FinalTaxRun{
		Id: r.ID, TaxYear: r.Period.Year, TaxMonth: int(r.Period.Month), GrossRevenue: moneyStr(r.GrossRevenue),
		RatePercent: pct(r.RatePercent), TaxAmount: moneyStr(r.TaxAmount), JournalEntryId: r.JournalEntryID,
		JournalNumber: optStr(r.JournalNumber), DepositId: r.DepositID, CreatedAt: r.CreatedAt,
	}
}

// GetFinalTaxSummary mengembalikan pemantauan PPh Final UMKM.
func (h *AccountingHandler) GetFinalTaxSummary(ctx context.Context, req GetFinalTaxSummaryRequestObject) (GetFinalTaxSummaryResponseObject, error) {
	s, err := h.svc.FinalTaxSummary(ctx, req.Params.Year)
	if err != nil {
		return nil, err
	}
	out := FinalTaxSummary{
		Year: s.Year, Enabled: s.Setting != nil && s.Setting.IsEnabled, OpeningTurnover: moneyStr(s.Opening),
		Settings: make([]FinalTaxSetting, len(s.Settings)), Months: make([]FinalTaxMonth, len(s.Months)),
		Threshold: FinalTaxThreshold{
			Cumulative: moneyStr(s.Threshold.Cumulative), Limit: moneyStr(s.Threshold.Limit), Remaining: moneyStr(s.Threshold.Remaining),
			UsedPercent: s.Threshold.UsedPercent.String(), Status: FinalTaxThresholdStatus(s.Threshold.Status),
		},
	}
	if s.Setting != nil {
		st := toFinalSetting(*s.Setting)
		out.Setting = &st
	}
	if s.Rate != nil {
		p := pct(s.Rate.RatePercent)
		out.RatePercent = &p
	}
	for i, st := range s.Settings {
		out.Settings[i] = toFinalSetting(st)
	}
	for i, m := range s.Months {
		out.Months[i] = FinalTaxMonth{
			TaxYear: m.Period.Year, TaxMonth: int(m.Period.Month), GrossRevenue: moneyStr(m.GrossRevenue), Cumulative: moneyStr(m.Cumulative),
		}
		if m.Run != nil {
			r := toFinalRun(*m.Run)
			out.Months[i].Run = &r
		}
	}
	return GetFinalTaxSummary200JSONResponse(out), nil
}

// CreateFinalTaxSetting mengaktifkan/menonaktifkan PPh Final UMKM.
func (h *AccountingHandler) CreateFinalTaxSetting(ctx context.Context, req CreateFinalTaxSettingRequestObject) (CreateFinalTaxSettingResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	b := req.Body
	fields := map[string]string{}
	st := domain.FinalTaxSetting{
		ValidFrom: fromDate(b.ValidFrom), IsEnabled: b.IsEnabled, OpeningYear: b.OpeningYear, Note: str(b.Note), OpeningTurnover: money.Zero,
	}
	if a := parseOptMoney(b.OpeningTurnover, "openingTurnover", fields); a != nil {
		st.OpeningTurnover = *a
	}
	if err := validationOrNil(fields); err != nil {
		return nil, err
	}
	out, err := h.svc.SetFinalTaxSetting(ctx, st)
	if err != nil {
		return nil, err
	}
	return CreateFinalTaxSetting201JSONResponse(toFinalSetting(out)), nil
}

// RunFinalTax menghitung PPh Final satu masa.
func (h *AccountingHandler) RunFinalTax(ctx context.Context, req RunFinalTaxRequestObject) (RunFinalTaxResponseObject, error) {
	if err := bodyRequired(req.Body); err != nil {
		return nil, err
	}
	r, err := h.svc.RunFinalTax(ctx, tax.Period{Year: req.Body.Year, Month: time.Month(req.Body.Month)})
	if err != nil {
		return nil, err
	}
	return RunFinalTax201JSONResponse(toFinalRun(r)), nil
}
