package http

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// Handler R1.4: aset tetap, penyusutan, tutup buku, Neraca Saldo, saldo awal.

func moneyOrZero(s *Money, field string, fields map[string]string) money.Amount {
	if s == nil {
		return money.Zero
	}
	return parseMoney(*s, field, fields)
}

func optMoney(a *money.Amount) *Money {
	if a == nil {
		return nil
	}
	s := a.String()
	return &s
}

// --- Kelas aset & aset tetap ------------------------------------------------------------------------

func toAssetClass(c domain.AssetClass) FixedAssetClass {
	return FixedAssetClass{
		Id: c.ID, Code: c.Code, Name: c.Name, UsefulLifeMonths: c.UsefulLifeMonths, AssetAccountId: c.AssetAccountID,
		AssetAccountCode: c.AssetAccountCode, AccumulatedAccountId: c.AccumulatedAccountID,
		AccumulatedAccountCode: c.AccumulatedAccountCode, IsActive: c.IsActive, Version: c.Version,
	}
}

// ListFixedAssetClasses mengembalikan kelas aset.
func (h *AccountingHandler) ListFixedAssetClasses(ctx context.Context, req ListFixedAssetClassesRequestObject) (ListFixedAssetClassesResponseObject, error) {
	list, err := h.svc.ListAssetClasses(ctx, includeInactive(req.Params.IncludeInactive))
	if err != nil {
		return nil, err
	}
	out := make([]FixedAssetClass, len(list))
	for i, c := range list {
		out[i] = toAssetClass(c)
	}
	return ListFixedAssetClasses200JSONResponse(out), nil
}

// CreateFixedAssetClass menambah kelas aset.
func (h *AccountingHandler) CreateFixedAssetClass(ctx context.Context, req CreateFixedAssetClassRequestObject) (CreateFixedAssetClassResponseObject, error) {
	b := req.Body
	c, err := h.svc.CreateAssetClass(ctx, domain.AssetClassInput{
		Code: b.Code, Name: b.Name, UsefulLifeMonths: b.UsefulLifeMonths, AssetAccountID: b.AssetAccountId,
		AccumulatedAccountID: b.AccumulatedAccountId, IsActive: b.IsActive,
	})
	if err != nil {
		return nil, err
	}
	return CreateFixedAssetClass201JSONResponse(toAssetClass(c)), nil
}

// UpdateFixedAssetClass mengubah kelas aset.
func (h *AccountingHandler) UpdateFixedAssetClass(ctx context.Context, req UpdateFixedAssetClassRequestObject) (UpdateFixedAssetClassResponseObject, error) {
	b := req.Body
	c, err := h.svc.UpdateAssetClass(ctx, req.Id, b.Version, domain.AssetClassInput{
		Name: b.Name, UsefulLifeMonths: b.UsefulLifeMonths, AssetAccountID: b.AssetAccountId,
		AccumulatedAccountID: b.AccumulatedAccountId, IsActive: b.IsActive,
	})
	if err != nil {
		return nil, err
	}
	return UpdateFixedAssetClass200JSONResponse(toAssetClass(c)), nil
}

func toFixedAsset(a domain.FixedAsset, has func(string) bool) FixedAsset {
	actions := a.AllowedActions(has)
	allowed := make([]FixedAssetAllowedActions, len(actions))
	for i, act := range actions {
		allowed[i] = FixedAssetAllowedActions(act)
	}
	out := FixedAsset{
		Id: a.ID, Code: a.Code, Name: a.Name, ClassId: a.ClassID, ClassName: a.ClassName, LocationId: a.LocationID,
		LocationLabel: a.LocationLabel, DivisionId: a.DivisionID, Source: FixedAssetSource(a.Source),
		AcquisitionDate: toDate(a.AcquisitionDate), Cost: a.Cost.String(), Salvage: a.Salvage.String(),
		UsefulLifeMonths: a.UsefulLife, DepreciationStart: toDate(a.DepreciationStart),
		OpeningAccumulated: a.OpeningAccumulated.String(), OpeningDepreciatedMonths: a.OpeningMonths,
		Accumulated: a.Accumulated.String(), BookValue: a.BookValue().String(), MonthlyDepreciation: a.MonthlyDepreciation().String(),
		PaymentAccountId: a.PaymentAccountID, Status: FixedAssetStatus(a.Status), AcquisitionJournalId: a.AcquisitionJournal,
		DisposalProceeds: optMoney(a.DisposalProceeds), DisposalAccountId: a.DisposalAccountID, DisposalReason: a.DisposalReason,
		DisposalJournalId: a.DisposalJournal, Note: a.Note, AllowedActions: allowed, UpdatedAt: a.UpdatedAt, Version: a.Version,
	}
	if a.DisposalDate != nil {
		d := toDate(*a.DisposalDate)
		out.DisposalDate = &d
	}
	return out
}

func toDepreciationLines(lines []domain.DepreciationLine) []DepreciationLine {
	out := make([]DepreciationLine, len(lines))
	for i, l := range lines {
		out[i] = DepreciationLine{
			Id: l.ID, RunId: l.RunID, AssetId: l.AssetID, AssetCode: l.AssetCode, AssetName: l.AssetName,
			PeriodLabel: l.Period.Label(), Amount: l.Amount.String(), Accumulated: l.Accumulated.String(),
			BookValue: l.BookValue.String(), JournalEntryId: l.JournalEntryID, JournalNumber: l.JournalNumber,
		}
	}
	return out
}

func (h *AccountingHandler) assetDetail(ctx context.Context, id uuid.UUID) (FixedAssetDetail, error) {
	d, err := h.svc.GetFixedAsset(ctx, id)
	if err != nil {
		return FixedAssetDetail{}, err
	}
	a, _ := actor.From(ctx)
	return FixedAssetDetail{Asset: toFixedAsset(d.Asset, a.Has), Depreciation: toDepreciationLines(d.Lines)}, nil
}

func assetInput(f FixedAssetFields) (domain.FixedAssetInput, error) {
	fields := map[string]string{}
	in := domain.FixedAssetInput{
		Name: f.Name, ClassID: f.ClassId, LocationID: f.LocationId, LocationLabel: str(f.LocationLabel), DivisionID: f.DivisionId,
		Source: domain.AssetSource(f.Source), AcquisitionDate: fromDate(f.AcquisitionDate), Cost: parseMoney(f.Cost, "cost", fields),
		Salvage: moneyOrZero(f.Salvage, "salvage", fields), OpeningAccumulated: moneyOrZero(f.OpeningAccumulated, "openingAccumulated", fields),
		PaymentAccountID: f.PaymentAccountId, Note: str(f.Note),
	}
	if f.UsefulLifeMonths != nil {
		in.UsefulLife = *f.UsefulLifeMonths
	}
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	return in, nil
}

// ListFixedAssets mengembalikan register aset.
func (h *AccountingHandler) ListFixedAssets(ctx context.Context, req ListFixedAssetsRequestObject) (ListFixedAssetsResponseObject, error) {
	p := req.Params
	f := postgres.FixedAssetFilter{ClassID: p.ClassId, Query: strings.TrimSpace(str(p.Q))}
	if p.Status != nil {
		f.Status = domain.AssetStatus(*p.Status)
	}
	res, err := h.svc.ListFixedAssets(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	a, _ := actor.From(ctx)
	items := make([]FixedAsset, len(res.Items))
	for i, asset := range res.Items {
		items[i] = toFixedAsset(asset, a.Has)
	}
	return ListFixedAssets200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateFixedAsset menambah aset Draft.
func (h *AccountingHandler) CreateFixedAsset(ctx context.Context, req CreateFixedAssetRequestObject) (CreateFixedAssetResponseObject, error) {
	b := req.Body
	in, err := assetInput(FixedAssetFields{
		Name: b.Name, ClassId: b.ClassId, LocationId: b.LocationId, LocationLabel: b.LocationLabel, DivisionId: b.DivisionId,
		Source: b.Source, AcquisitionDate: b.AcquisitionDate, Cost: b.Cost, Salvage: b.Salvage, UsefulLifeMonths: b.UsefulLifeMonths,
		OpeningAccumulated: b.OpeningAccumulated, PaymentAccountId: b.PaymentAccountId, Note: b.Note,
	})
	if err != nil {
		return nil, err
	}
	in.Code = b.Code
	asset, err := h.svc.CreateFixedAsset(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.assetDetail(ctx, asset.ID)
	if err != nil {
		return nil, err
	}
	return CreateFixedAsset201JSONResponse(out), nil
}

// GetFixedAsset mengembalikan detail aset.
func (h *AccountingHandler) GetFixedAsset(ctx context.Context, req GetFixedAssetRequestObject) (GetFixedAssetResponseObject, error) {
	out, err := h.assetDetail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetFixedAsset200JSONResponse(out), nil
}

// UpdateFixedAsset mengubah aset Draft.
func (h *AccountingHandler) UpdateFixedAsset(ctx context.Context, req UpdateFixedAssetRequestObject) (UpdateFixedAssetResponseObject, error) {
	b := req.Body
	in, err := assetInput(FixedAssetFields{
		Name: b.Name, ClassId: b.ClassId, LocationId: b.LocationId, LocationLabel: b.LocationLabel, DivisionId: b.DivisionId,
		Source: b.Source, AcquisitionDate: b.AcquisitionDate, Cost: b.Cost, Salvage: b.Salvage, UsefulLifeMonths: b.UsefulLifeMonths,
		OpeningAccumulated: b.OpeningAccumulated, PaymentAccountId: b.PaymentAccountId, Note: b.Note,
	})
	if err != nil {
		return nil, err
	}
	if _, err := h.svc.UpdateFixedAsset(ctx, req.Id, b.Version, in); err != nil {
		return nil, err
	}
	out, err := h.assetDetail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return UpdateFixedAsset200JSONResponse(out), nil
}

// DeleteFixedAsset menghapus aset Draft.
func (h *AccountingHandler) DeleteFixedAsset(ctx context.Context, req DeleteFixedAssetRequestObject) (DeleteFixedAssetResponseObject, error) {
	if err := h.svc.DeleteFixedAsset(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteFixedAsset204Response{}, nil
}

// ActivateFixedAsset mengaktifkan aset.
func (h *AccountingHandler) ActivateFixedAsset(ctx context.Context, req ActivateFixedAssetRequestObject) (ActivateFixedAssetResponseObject, error) {
	if _, err := h.svc.ActivateFixedAsset(ctx, req.Id, req.Body.Version); err != nil {
		return nil, err
	}
	out, err := h.assetDetail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return ActivateFixedAsset200JSONResponse(out), nil
}

// DisposeFixedAsset melepas aset.
func (h *AccountingHandler) DisposeFixedAsset(ctx context.Context, req DisposeFixedAssetRequestObject) (DisposeFixedAssetResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	proceeds := parseMoney(b.Proceeds, "proceeds", fields)
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	if _, err := h.svc.DisposeFixedAsset(ctx, req.Id, b.Version, service.DisposeInput{
		Date: fromDate(b.DisposalDate), Proceeds: proceeds, AccountID: b.DisposalAccountId, Reason: b.Reason,
	}); err != nil {
		return nil, err
	}
	out, err := h.assetDetail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return DisposeFixedAsset200JSONResponse(out), nil
}

// --- Penyusutan --------------------------------------------------------------------------------------

func toRun(r domain.DepreciationRun) DepreciationRun {
	return DepreciationRun{
		Id: r.ID, PeriodId: r.PeriodID, Year: r.Period.Year, Month: int(r.Period.Month), PeriodLabel: r.Period.Label(),
		AssetCount: r.AssetCount, Total: r.Total.String(), RunCount: r.RunCount, LastRunAt: r.LastRunAt,
	}
}

func toRunDetail(d service.DepreciationRunDetail) DepreciationRunDetail {
	return DepreciationRunDetail{Run: toRun(d.Run), Lines: toDepreciationLines(d.Lines)}
}

// ListDepreciationRuns mengembalikan riwayat penyusutan.
func (h *AccountingHandler) ListDepreciationRuns(ctx context.Context, req ListDepreciationRunsRequestObject) (ListDepreciationRunsResponseObject, error) {
	res, err := h.svc.ListDepreciationRuns(ctx, paging.New(req.Params.Page, req.Params.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]DepreciationRun, len(res.Items))
	for i, r := range res.Items {
		items[i] = toRun(r)
	}
	return ListDepreciationRuns200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// RunDepreciation menjalankan penyusutan periode.
func (h *AccountingHandler) RunDepreciation(ctx context.Context, req RunDepreciationRequestObject) (RunDepreciationResponseObject, error) {
	d, err := h.svc.RunDepreciation(ctx, domain.PeriodKey{Year: req.Body.Year, Month: time.Month(req.Body.Month)})
	if err != nil {
		return nil, err
	}
	return RunDepreciation200JSONResponse(toRunDetail(d)), nil
}

// GetDepreciationRun mengembalikan detail run.
func (h *AccountingHandler) GetDepreciationRun(ctx context.Context, req GetDepreciationRunRequestObject) (GetDepreciationRunResponseObject, error) {
	d, err := h.svc.GetDepreciationRun(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetDepreciationRun200JSONResponse(toRunDetail(d)), nil
}

// --- Tutup buku ---------------------------------------------------------------------------------------

func (h *AccountingHandler) toChecklist(ctx context.Context, c service.Checklist) (CloseChecklist, error) {
	var ids []uuid.UUID
	for _, it := range c.Items {
		for _, id := range []*uuid.UUID{it.PICUserID, it.CompletedBy} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
	}
	for _, r := range c.Reopens {
		ids = append(ids, r.RequestedBy)
	}
	names, err := h.svc.UserNames(ctx, ids)
	if err != nil {
		return CloseChecklist{}, err
	}
	name := func(id *uuid.UUID) *string {
		if id == nil {
			return nil
		}
		if *id == uuid.Nil {
			n := "Sistem"
			return &n
		}
		if n, ok := names[*id]; ok {
			return &n
		}
		return nil
	}
	steps := make([]CloseChecklistStep, len(c.Items))
	for i, it := range c.Items {
		checks := make([]CloseCheckResult, len(it.Checks))
		for j, chk := range it.Checks {
			checks[j] = CloseCheckResult{Source: chk.Source, Ok: chk.OK, Message: chk.Message, Link: optStr(chk.Link)}
		}
		steps[i] = CloseChecklistStep{
			StepCode: CloseStepCode(it.Step.Code), Order: it.Step.Order, Title: it.Step.Title, PicLabel: it.Step.PICLabel,
			Automatic: it.Step.Automatic, Status: CloseStepStatus(it.Status), PicUserId: it.PICUserID, PicUserName: name(it.PICUserID),
			CompletedBy: it.CompletedBy, CompletedByName: name(it.CompletedBy), CompletedAt: it.CompletedAt, Note: it.Note,
			Checks: checks, Version: it.Version,
		}
	}
	reopens := make([]FiscalPeriodReopenRequest, len(c.Reopens))
	for i, r := range c.Reopens {
		by := r.RequestedBy
		reopens[i] = FiscalPeriodReopenRequest{
			Id: r.ID, Reason: r.Reason, Status: FiscalPeriodReopenRequestStatus(r.Status), RequestedBy: r.RequestedBy,
			RequestedByName: name(&by), RequestedAt: r.RequestedAt, DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote,
		}
	}
	actions := make([]CloseChecklistAllowedActions, len(c.AllowedActions))
	for i, a := range c.AllowedActions {
		actions[i] = CloseChecklistAllowedActions(a)
	}
	blockers := c.Blockers
	if blockers == nil {
		blockers = []string{}
	}
	return CloseChecklist{Period: toPeriod(c.Period), Steps: steps, Blockers: blockers, ReopenRequests: reopens, AllowedActions: actions}, nil
}

// GetCloseChecklist mengembalikan checklist tutup buku.
func (h *AccountingHandler) GetCloseChecklist(ctx context.Context, req GetCloseChecklistRequestObject) (GetCloseChecklistResponseObject, error) {
	c, err := h.svc.CloseChecklist(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := h.toChecklist(ctx, c)
	if err != nil {
		return nil, err
	}
	return GetCloseChecklist200JSONResponse(out), nil
}

// UpdateCloseChecklistStep mengubah satu langkah checklist.
func (h *AccountingHandler) UpdateCloseChecklistStep(ctx context.Context, req UpdateCloseChecklistStepRequestObject) (UpdateCloseChecklistStepResponseObject, error) {
	b := req.Body
	c, err := h.svc.UpdateChecklistStep(ctx, req.Id, domain.CloseStepCode(req.StepCode), b.Version, domain.ChecklistUpdate{
		Status: domain.CloseStepStatus(b.Status), PICUserID: b.PicUserId, Note: str(b.Note),
	})
	if err != nil {
		return nil, err
	}
	out, err := h.toChecklist(ctx, c)
	if err != nil {
		return nil, err
	}
	return UpdateCloseChecklistStep200JSONResponse(out), nil
}

// CloseFiscalPeriod menutup periode.
func (h *AccountingHandler) CloseFiscalPeriod(ctx context.Context, req CloseFiscalPeriodRequestObject) (CloseFiscalPeriodResponseObject, error) {
	c, err := h.svc.ClosePeriod(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	out, err := h.toChecklist(ctx, c)
	if err != nil {
		return nil, err
	}
	return CloseFiscalPeriod200JSONResponse(out), nil
}

// RequestFiscalPeriodReopen mengajukan buka kembali periode.
func (h *AccountingHandler) RequestFiscalPeriodReopen(ctx context.Context, req RequestFiscalPeriodReopenRequestObject) (RequestFiscalPeriodReopenResponseObject, error) {
	c, err := h.svc.RequestReopen(ctx, req.Id, req.Body.Version, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	out, err := h.toChecklist(ctx, c)
	if err != nil {
		return nil, err
	}
	return RequestFiscalPeriodReopen200JSONResponse(out), nil
}

// --- Neraca Saldo -------------------------------------------------------------------------------------

// GetTrialBalance mengembalikan Neraca Saldo.
func (h *AccountingHandler) GetTrialBalance(ctx context.Context, req GetTrialBalanceRequestObject) (GetTrialBalanceResponseObject, error) {
	p := req.Params
	tb, err := h.svc.TrialBalance(ctx, postgres.TrialBalanceFilter{
		DateFrom: fromDate(p.DateFrom), DateTo: fromDate(p.DateTo),
		Dimensions: domain.Dimensions{ChannelID: p.ChannelId, SubChannelID: p.SubChannelId, DivisionID: p.DivisionId, CampaignID: p.CampaignId},
	})
	if err != nil {
		return nil, err
	}
	rows := make([]TrialBalanceRow, len(tb.Rows))
	for i, r := range tb.Rows {
		od, oc := domain.SplitNet(r.OpeningNet)
		cd, cc := domain.SplitNet(r.ClosingNet())
		rows[i] = TrialBalanceRow{
			AccountId: r.AccountID, AccountCode: r.AccountCode, AccountName: r.AccountName, Group: int(r.Group),
			GroupLabel: r.Group.Label(), NormalBalance: NormalBalance(r.NormalBalance), OpeningDebit: od.String(),
			OpeningCredit: oc.String(), PeriodDebit: r.PeriodDebit.String(), PeriodCredit: r.PeriodCredit.String(),
			ClosingDebit: cd.String(), ClosingCredit: cc.String(),
		}
	}
	t := tb.Totals
	return GetTrialBalance200JSONResponse{
		DateFrom: p.DateFrom, DateTo: p.DateTo, Rows: rows, Balanced: t.Balanced(),
		Totals: TrialBalanceTotals{
			OpeningDebit: t.OpeningDebit.String(), OpeningCredit: t.OpeningCredit.String(), PeriodDebit: t.PeriodDebit.String(),
			PeriodCredit: t.PeriodCredit.String(), ClosingDebit: t.ClosingDebit.String(), ClosingCredit: t.ClosingCredit.String(),
		},
	}, nil
}

// --- Saldo awal ---------------------------------------------------------------------------------------

func toOpening(v service.OpeningView) OpeningBalance {
	lines := make([]OpeningBalanceLine, len(v.Lines))
	for i, l := range v.Lines {
		lines[i] = OpeningBalanceLine{
			Id: l.ID, AccountId: l.AccountID, AccountCode: l.AccountCode, AccountName: l.AccountName, Debit: l.Debit.String(),
			Credit: l.Credit.String(), Note: l.Note,
		}
	}
	actions := make([]OpeningBalanceAllowedActions, len(v.AllowedActions))
	for i, a := range v.AllowedActions {
		actions[i] = OpeningBalanceAllowedActions(a)
	}
	out := OpeningBalance{
		Lines: lines, TotalDebit: v.Totals.Debit.String(), TotalCredit: v.Totals.Credit.String(),
		Difference: v.Totals.Difference.String(), AllowedActions: actions,
	}
	if s := v.Set; s != nil {
		d := toDate(s.CutoverDate)
		status := OpeningBalanceStatus(s.Status)
		version := s.Version
		out.Id, out.CutoverDate, out.Status, out.JournalEntryId = &s.ID, &d, &status, s.JournalEntryID
		out.JournalNumber, out.PostedAt, out.Version = optStr(s.JournalNumber), s.PostedAt, &version
	}
	return out
}

// GetOpeningBalance mengembalikan saldo awal.
func (h *AccountingHandler) GetOpeningBalance(ctx context.Context, _ GetOpeningBalanceRequestObject) (GetOpeningBalanceResponseObject, error) {
	v, err := h.svc.GetOpening(ctx)
	if err != nil {
		return nil, err
	}
	return GetOpeningBalance200JSONResponse(toOpening(v)), nil
}

// SaveOpeningBalanceSet mengisi tanggal cut-over.
func (h *AccountingHandler) SaveOpeningBalanceSet(ctx context.Context, req SaveOpeningBalanceSetRequestObject) (SaveOpeningBalanceSetResponseObject, error) {
	v, err := h.svc.SaveCutoverDate(ctx, fromDate(req.Body.CutoverDate), req.Body.Version)
	if err != nil {
		return nil, err
	}
	return SaveOpeningBalanceSet200JSONResponse(toOpening(v)), nil
}

// SaveOpeningBalanceLine mengisi saldo awal satu akun.
func (h *AccountingHandler) SaveOpeningBalanceLine(ctx context.Context, req SaveOpeningBalanceLineRequestObject) (SaveOpeningBalanceLineResponseObject, error) {
	b := req.Body
	fields := map[string]string{}
	in := domain.OpeningLineInput{
		AccountID: b.AccountId, Debit: parseMoney(b.Debit, "debit", fields), Credit: parseMoney(b.Credit, "credit", fields), Note: str(b.Note),
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	v, err := h.svc.SaveOpeningLine(ctx, in)
	if err != nil {
		return nil, err
	}
	return SaveOpeningBalanceLine200JSONResponse(toOpening(v)), nil
}

// DeleteOpeningBalanceLine menghapus saldo awal satu akun.
func (h *AccountingHandler) DeleteOpeningBalanceLine(ctx context.Context, req DeleteOpeningBalanceLineRequestObject) (DeleteOpeningBalanceLineResponseObject, error) {
	v, err := h.svc.DeleteOpeningLine(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return DeleteOpeningBalanceLine200JSONResponse(toOpening(v)), nil
}

// PostOpeningBalance memposting saldo awal A-36.
func (h *AccountingHandler) PostOpeningBalance(ctx context.Context, req PostOpeningBalanceRequestObject) (PostOpeningBalanceResponseObject, error) {
	v, err := h.svc.PostOpening(ctx, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return PostOpeningBalance200JSONResponse(toOpening(v)), nil
}

// ListOpeningItems mengembalikan piutang/utang terbuka.
func (h *AccountingHandler) ListOpeningItems(ctx context.Context, req ListOpeningItemsRequestObject) (ListOpeningItemsResponseObject, error) {
	p := req.Params
	f := postgres.OpeningItemFilter{Query: strings.TrimSpace(str(p.Q)), OutstandingOnly: p.OutstandingOnly != nil && *p.OutstandingOnly}
	if p.ItemType != nil {
		f.Type = domain.OpeningItemType(*p.ItemType)
	}
	res, err := h.svc.ListOpeningItems(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]OpeningItem, len(res.Items))
	for i, it := range res.Items {
		items[i] = OpeningItem{
			Id: it.ID, ItemType: OpeningItemType(it.Type), PartyType: OpeningItemPartyType(it.PartyType), PartyId: it.PartyID,
			PartyCode: it.PartyCode, DocumentNumber: it.DocumentNumber, DocumentDate: toDate(it.DocumentDate), DueDate: toDate(it.DueDate),
			Amount: it.Amount.String(), OpenAmount: it.OpenAmount.String(), SettledAmount: it.Settled.String(),
			Outstanding: it.Outstanding().String(), AccountCode: it.AccountCode, ChannelId: it.ChannelID, Note: it.Note,
		}
	}
	return ListOpeningItems200JSONResponse{
		Items: items, OutstandingTotal: res.Outstanding.String(), Page: res.Page.Number, PageSize: res.Page.Size,
		TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// GetCutoverReconciliation menjalankan pengecekan rekonsiliasi cut-over.
func (h *AccountingHandler) GetCutoverReconciliation(ctx context.Context, _ GetCutoverReconciliationRequestObject) (GetCutoverReconciliationResponseObject, error) {
	r, err := h.svc.CutoverReconciliation(ctx)
	if err != nil {
		return nil, err
	}
	checks := make([]CutoverReconciliationCheck, len(r.Checks))
	for i, c := range r.Checks {
		checks[i] = CutoverReconciliationCheck{
			Code: c.Code, Label: c.Label, AccountCode: c.AccountCode, Subledger: c.Subledger.String(), Ledger: c.Ledger.String(),
			Difference: c.Difference.String(), Status: CutoverReconciliationCheckStatus(c.Status), Note: c.Note,
		}
	}
	out := CutoverReconciliation{Checks: checks, Passed: r.Passed}
	if r.CutoverDate != nil {
		d := toDate(*r.CutoverDate)
		out.CutoverDate = &d
	}
	if r.LedgerSource != "" {
		src := CutoverReconciliationLedgerSource(r.LedgerSource)
		out.LedgerSource = &src
	}
	return GetCutoverReconciliation200JSONResponse(out), nil
}
