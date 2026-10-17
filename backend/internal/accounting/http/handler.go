package http

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/accounting.yaml ../../../api/openapi.bundle.yaml

// AccountingHandler mengimplementasikan StrictServerInterface hasil generate.
type AccountingHandler struct {
	svc *service.Service
}

var _ StrictServerInterface = (*AccountingHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service) *AccountingHandler { return &AccountingHandler{svc: svc} }

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *AccountingHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// --- Konversi ---------------------------------------------------------------------------------

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func fromDate(d BusinessDate) clock.BusinessDate {
	if d.IsZero() {
		return clock.BusinessDate{}
	}
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func optDate(d *BusinessDate) *clock.BusinessDate {
	if d == nil {
		return nil
	}
	v := fromDate(*d)
	return &v
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseMoney(s Money, field string, fields map[string]string) money.Amount {
	if strings.TrimSpace(s) == "" {
		return money.Zero
	}
	a, err := money.Parse(s)
	if err != nil {
		fields[field] = "Nominal tidak valid"
	}
	return a
}

func parseOptMoney(s *Money, field string, fields map[string]string) *money.Amount {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	a := parseMoney(*s, field, fields)
	return &a
}

func toAccount(a domain.Account) Account {
	return Account{
		Id: a.ID, Code: a.Code, Name: a.Name, Group: int(a.Group), GroupLabel: a.Group.Label(),
		NormalBalance: NormalBalance(a.NormalBalance), ParentId: a.ParentID, IsPostable: a.IsPostable,
		AllowManualPosting: a.AllowManualPosting, IsActive: a.IsActive, Description: a.Description,
		UpdatedAt: a.UpdatedAt, Version: a.Version,
	}
}

func accountInput(f AccountFields) domain.AccountInput {
	return domain.AccountInput{
		Name: f.Name, NormalBalance: domain.NormalBalance(f.NormalBalance), ParentID: f.ParentId, IsPostable: f.IsPostable,
		AllowManualPosting: f.AllowManualPosting, IsActive: f.IsActive, Description: str(f.Description),
	}
}

func toDimension(v domain.DimensionValue) Dimension {
	out := Dimension{
		Id: v.ID, Kind: DimensionKind(v.Kind), Code: v.Code, Name: v.Name, IsActive: v.IsActive,
		UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
	switch v.Kind {
	case domain.KindSubChannel:
		out.ChannelId, out.PartyType, out.PartyId = v.ChannelID, optStr(v.PartyType), v.PartyID
	case domain.KindCampaign:
		from, to := toDate(v.ValidFrom), toDate(v.ValidTo)
		out.ValidFrom, out.ValidTo, out.Description = &from, &to, &v.Description
		if v.Budget != nil {
			b := v.Budget.String()
			out.BudgetAmount = &b
		}
	case domain.KindChannel, domain.KindDivision:
	}
	return out
}

func toDimensions(list []domain.DimensionValue) []Dimension {
	out := make([]Dimension, len(list))
	for i, v := range list {
		out[i] = toDimension(v)
	}
	return out
}

func toPeriod(p domain.Period) FiscalPeriod {
	return FiscalPeriod{
		Id: p.ID, Year: p.Year, Month: int(p.Month), Label: p.Label(), Status: FiscalPeriodStatus(p.Status),
		StartDate: toDate(p.Start()), EndDate: toDate(p.End()), ClosedAt: p.ClosedAt, ReopenedCount: p.ReopenedCount, Version: p.Version,
	}
}

// dimensionRefs memuat kode & nama dimensi yang dirujuk baris.
type dimensionRefs map[uuid.UUID]postgres.DimensionName

func (h *AccountingHandler) loadRefs(ctx context.Context, dims []domain.Dimensions) (dimensionRefs, error) {
	var ids []uuid.UUID
	for _, d := range dims {
		for _, id := range []*uuid.UUID{d.ChannelID, d.SubChannelID, d.DivisionID, d.CampaignID} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
	}
	names, err := h.svc.DimensionNames(ctx, ids)
	return dimensionRefs(names), err
}

func (r dimensionRefs) ref(id *uuid.UUID) *DimensionRef {
	if id == nil {
		return nil
	}
	n, ok := r[*id]
	if !ok {
		return &DimensionRef{Id: *id}
	}
	return &DimensionRef{Id: *id, Code: n.Code, Name: n.Name}
}

func toSummary(e domain.Entry) JournalEntrySummary {
	return JournalEntrySummary{
		Id: e.ID, Number: optStr(e.Number), BusinessDate: toDate(e.BusinessDate), EntryType: JournalEntryType(e.EntryType),
		SourceType: e.SourceType, SourceId: e.SourceID, SourceLabel: e.SourceLabel, PostingRuleCode: optStr(e.PostingRuleCode),
		Description: e.Description, Status: JournalStatus(e.Status), TotalAmount: e.TotalAmount.String(),
		ReversalOfId: e.ReversalOfID, ReversalOfNumber: optStr(e.ReversalOfNumber), ReversedById: e.ReversedByID,
		ReversedByNumber: optStr(e.ReversedByNumber), PostedAt: e.PostedAt, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		Version: e.Version,
	}
}

func (h *AccountingHandler) toEntry(ctx context.Context, e domain.Entry) (JournalEntry, error) {
	dims := make([]domain.Dimensions, len(e.Lines))
	for i, l := range e.Lines {
		dims[i] = l.Dimensions
	}
	refs, err := h.loadRefs(ctx, dims)
	if err != nil {
		return JournalEntry{}, err
	}
	lines := make([]JournalLine, len(e.Lines))
	for i, l := range e.Lines {
		lines[i] = JournalLine{
			LineNo: l.LineNo, AccountId: l.AccountID, AccountCode: l.AccountCode, AccountName: l.AccountName,
			Debit: l.Debit.String(), Credit: l.Credit.String(), Channel: refs.ref(l.Dimensions.ChannelID),
			SubChannel: refs.ref(l.Dimensions.SubChannelID), Division: refs.ref(l.Dimensions.DivisionID),
			Campaign: refs.ref(l.Dimensions.CampaignID), AmountKey: optStr(l.AmountKey), Memo: l.Memo,
		}
		if l.Party != nil {
			lines[i].PartyType, lines[i].PartyId = &l.Party.Type, &l.Party.ID
		}
	}
	a, _ := actor.From(ctx)
	actions := e.AllowedActions(a.Has)
	allowed := make([]JournalEntryAllowedActions, len(actions))
	for i, act := range actions {
		allowed[i] = JournalEntryAllowedActions(act)
	}
	debit, credit := e.Totals()
	s := toSummary(e)
	return JournalEntry{
		Id: s.Id, Number: s.Number, BusinessDate: s.BusinessDate, EntryType: s.EntryType, SourceType: s.SourceType,
		SourceId: s.SourceId, SourceLabel: s.SourceLabel, PostingRuleCode: s.PostingRuleCode, Description: s.Description,
		Status: s.Status, TotalAmount: s.TotalAmount, ReversalOfId: s.ReversalOfId, ReversalOfNumber: s.ReversalOfNumber,
		ReversedById: s.ReversedById, ReversedByNumber: s.ReversedByNumber, PostedAt: s.PostedAt, CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt, Version: s.Version, Lines: lines, TotalDebit: debit.String(), TotalCredit: credit.String(),
		AllowedActions: allowed,
	}, nil
}

func manualInput(date BusinessDate, description string, in []ManualJournalLineInput) (domain.ManualInput, error) {
	fields := map[string]string{}
	lines := make([]domain.Line, len(in))
	for i, l := range in {
		p := "lines." + strconv.Itoa(i) + "."
		lines[i] = domain.Line{
			AccountID: l.AccountId, Debit: parseMoney(l.Debit, p+"debit", fields), Credit: parseMoney(l.Credit, p+"credit", fields),
			Dimensions: domain.Dimensions{ChannelID: l.ChannelId, SubChannelID: l.SubChannelId, DivisionID: l.DivisionId, CampaignID: l.CampaignId},
			Memo:       str(l.Memo),
		}
	}
	if len(fields) > 0 {
		return domain.ManualInput{}, apperr.Validation(fields)
	}
	return domain.ManualInput{BusinessDate: fromDate(date), Description: description, Lines: lines}, nil
}

// --- Akun -------------------------------------------------------------------------------------

// ListAccounts mengembalikan COA.
func (h *AccountingHandler) ListAccounts(ctx context.Context, req ListAccountsRequestObject) (ListAccountsResponseObject, error) {
	list, err := h.svc.ListAccounts(ctx, req.Params.IncludeInactive != nil && *req.Params.IncludeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(list))
	for i, a := range list {
		out[i] = toAccount(a)
	}
	return ListAccounts200JSONResponse(out), nil
}

// CreateAccount menambah akun.
func (h *AccountingHandler) CreateAccount(ctx context.Context, req CreateAccountRequestObject) (CreateAccountResponseObject, error) {
	b := req.Body
	in := accountInput(AccountFields{
		Name: b.Name, NormalBalance: b.NormalBalance, ParentId: b.ParentId, IsPostable: b.IsPostable,
		AllowManualPosting: b.AllowManualPosting, IsActive: b.IsActive, Description: b.Description,
	})
	in.Code = b.Code
	a, err := h.svc.CreateAccount(ctx, in)
	if err != nil {
		return nil, err
	}
	return CreateAccount201JSONResponse(toAccount(a)), nil
}

// GetAccount mengembalikan satu akun.
func (h *AccountingHandler) GetAccount(ctx context.Context, req GetAccountRequestObject) (GetAccountResponseObject, error) {
	a, err := h.svc.GetAccount(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetAccount200JSONResponse(toAccount(a)), nil
}

// UpdateAccount mengubah akun.
func (h *AccountingHandler) UpdateAccount(ctx context.Context, req UpdateAccountRequestObject) (UpdateAccountResponseObject, error) {
	b := req.Body
	in := accountInput(AccountFields{
		Name: b.Name, NormalBalance: b.NormalBalance, ParentId: b.ParentId, IsPostable: b.IsPostable,
		AllowManualPosting: b.AllowManualPosting, IsActive: b.IsActive, Description: b.Description,
	})
	a, err := h.svc.UpdateAccount(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateAccount200JSONResponse(toAccount(a)), nil
}

// DeleteAccount menghapus akun.
func (h *AccountingHandler) DeleteAccount(ctx context.Context, req DeleteAccountRequestObject) (DeleteAccountResponseObject, error) {
	if err := h.svc.DeleteAccount(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteAccount204Response{}, nil
}

// --- Dimensi ----------------------------------------------------------------------------------

func includeInactive(b *bool) bool { return b != nil && *b }

// ListChannels mengembalikan channel.
func (h *AccountingHandler) ListChannels(ctx context.Context, req ListChannelsRequestObject) (ListChannelsResponseObject, error) {
	list, err := h.svc.ListDimensions(ctx, domain.KindChannel, postgres.DimensionFilter{IncludeInactive: includeInactive(req.Params.IncludeInactive)})
	if err != nil {
		return nil, err
	}
	return ListChannels200JSONResponse(toDimensions(list)), nil
}

// CreateChannel menambah channel.
func (h *AccountingHandler) CreateChannel(ctx context.Context, req CreateChannelRequestObject) (CreateChannelResponseObject, error) {
	v, err := h.svc.CreateDimension(ctx, domain.KindChannel, domain.DimensionInput{Code: req.Body.Code, Name: req.Body.Name, IsActive: req.Body.IsActive})
	if err != nil {
		return nil, err
	}
	return CreateChannel201JSONResponse(toDimension(v)), nil
}

// UpdateChannel mengubah channel.
func (h *AccountingHandler) UpdateChannel(ctx context.Context, req UpdateChannelRequestObject) (UpdateChannelResponseObject, error) {
	v, err := h.svc.UpdateDimension(ctx, domain.KindChannel, req.Id, req.Body.Version, domain.DimensionInput{Name: req.Body.Name, IsActive: req.Body.IsActive})
	if err != nil {
		return nil, err
	}
	return UpdateChannel200JSONResponse(toDimension(v)), nil
}

// DeleteChannel menghapus channel.
func (h *AccountingHandler) DeleteChannel(ctx context.Context, req DeleteChannelRequestObject) (DeleteChannelResponseObject, error) {
	if err := h.svc.DeleteDimension(ctx, domain.KindChannel, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteChannel204Response{}, nil
}

// ListSubChannels mengembalikan sub-channel.
func (h *AccountingHandler) ListSubChannels(ctx context.Context, req ListSubChannelsRequestObject) (ListSubChannelsResponseObject, error) {
	list, err := h.svc.ListDimensions(ctx, domain.KindSubChannel, postgres.DimensionFilter{
		IncludeInactive: includeInactive(req.Params.IncludeInactive), ChannelID: req.Params.ChannelId,
	})
	if err != nil {
		return nil, err
	}
	return ListSubChannels200JSONResponse(toDimensions(list)), nil
}

// CreateSubChannel menambah sub-channel.
func (h *AccountingHandler) CreateSubChannel(ctx context.Context, req CreateSubChannelRequestObject) (CreateSubChannelResponseObject, error) {
	b := req.Body
	ch := b.ChannelId
	v, err := h.svc.CreateDimension(ctx, domain.KindSubChannel, domain.DimensionInput{
		Code: b.Code, Name: b.Name, ChannelID: &ch, PartyType: str(b.PartyType), PartyID: b.PartyId, IsActive: b.IsActive,
	})
	if err != nil {
		return nil, err
	}
	return CreateSubChannel201JSONResponse(toDimension(v)), nil
}

// UpdateSubChannel mengubah sub-channel.
func (h *AccountingHandler) UpdateSubChannel(ctx context.Context, req UpdateSubChannelRequestObject) (UpdateSubChannelResponseObject, error) {
	b := req.Body
	v, err := h.svc.UpdateDimension(ctx, domain.KindSubChannel, req.Id, b.Version, domain.DimensionInput{
		Name: b.Name, PartyType: str(b.PartyType), PartyID: b.PartyId, IsActive: b.IsActive,
	})
	if err != nil {
		return nil, err
	}
	return UpdateSubChannel200JSONResponse(toDimension(v)), nil
}

// DeleteSubChannel menghapus sub-channel.
func (h *AccountingHandler) DeleteSubChannel(ctx context.Context, req DeleteSubChannelRequestObject) (DeleteSubChannelResponseObject, error) {
	if err := h.svc.DeleteDimension(ctx, domain.KindSubChannel, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteSubChannel204Response{}, nil
}

// ListDivisions mengembalikan divisi.
func (h *AccountingHandler) ListDivisions(ctx context.Context, req ListDivisionsRequestObject) (ListDivisionsResponseObject, error) {
	list, err := h.svc.ListDimensions(ctx, domain.KindDivision, postgres.DimensionFilter{IncludeInactive: includeInactive(req.Params.IncludeInactive)})
	if err != nil {
		return nil, err
	}
	return ListDivisions200JSONResponse(toDimensions(list)), nil
}

// CreateDivision menambah divisi.
func (h *AccountingHandler) CreateDivision(ctx context.Context, req CreateDivisionRequestObject) (CreateDivisionResponseObject, error) {
	v, err := h.svc.CreateDimension(ctx, domain.KindDivision, domain.DimensionInput{Code: req.Body.Code, Name: req.Body.Name, IsActive: req.Body.IsActive})
	if err != nil {
		return nil, err
	}
	return CreateDivision201JSONResponse(toDimension(v)), nil
}

// UpdateDivision mengubah divisi.
func (h *AccountingHandler) UpdateDivision(ctx context.Context, req UpdateDivisionRequestObject) (UpdateDivisionResponseObject, error) {
	v, err := h.svc.UpdateDimension(ctx, domain.KindDivision, req.Id, req.Body.Version, domain.DimensionInput{Name: req.Body.Name, IsActive: req.Body.IsActive})
	if err != nil {
		return nil, err
	}
	return UpdateDivision200JSONResponse(toDimension(v)), nil
}

// DeleteDivision menghapus divisi.
func (h *AccountingHandler) DeleteDivision(ctx context.Context, req DeleteDivisionRequestObject) (DeleteDivisionResponseObject, error) {
	if err := h.svc.DeleteDimension(ctx, domain.KindDivision, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteDivision204Response{}, nil
}

func campaignInput(f CampaignFields) (domain.DimensionInput, error) {
	fields := map[string]string{}
	in := domain.DimensionInput{
		Name: f.Name, ValidFrom: fromDate(f.ValidFrom), ValidTo: fromDate(f.ValidTo),
		Budget: parseOptMoney(f.BudgetAmount, "budgetAmount", fields), Description: str(f.Description), IsActive: f.IsActive,
	}
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	return in, nil
}

// ListCampaigns mengembalikan kampanye.
func (h *AccountingHandler) ListCampaigns(ctx context.Context, req ListCampaignsRequestObject) (ListCampaignsResponseObject, error) {
	list, err := h.svc.ListDimensions(ctx, domain.KindCampaign, postgres.DimensionFilter{
		IncludeInactive: includeInactive(req.Params.IncludeInactive), Query: strings.TrimSpace(str(req.Params.Q)),
	})
	if err != nil {
		return nil, err
	}
	return ListCampaigns200JSONResponse(toDimensions(list)), nil
}

// CreateCampaign menambah kampanye.
func (h *AccountingHandler) CreateCampaign(ctx context.Context, req CreateCampaignRequestObject) (CreateCampaignResponseObject, error) {
	b := req.Body
	in, err := campaignInput(CampaignFields{
		Name: b.Name, ValidFrom: b.ValidFrom, ValidTo: b.ValidTo, BudgetAmount: b.BudgetAmount, Description: b.Description, IsActive: b.IsActive,
	})
	if err != nil {
		return nil, err
	}
	in.Code = b.Code
	v, err := h.svc.CreateDimension(ctx, domain.KindCampaign, in)
	if err != nil {
		return nil, err
	}
	return CreateCampaign201JSONResponse(toDimension(v)), nil
}

// UpdateCampaign mengubah kampanye.
func (h *AccountingHandler) UpdateCampaign(ctx context.Context, req UpdateCampaignRequestObject) (UpdateCampaignResponseObject, error) {
	b := req.Body
	in, err := campaignInput(CampaignFields{
		Name: b.Name, ValidFrom: b.ValidFrom, ValidTo: b.ValidTo, BudgetAmount: b.BudgetAmount, Description: b.Description, IsActive: b.IsActive,
	})
	if err != nil {
		return nil, err
	}
	v, err := h.svc.UpdateDimension(ctx, domain.KindCampaign, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	return UpdateCampaign200JSONResponse(toDimension(v)), nil
}

// DeleteCampaign menghapus kampanye.
func (h *AccountingHandler) DeleteCampaign(ctx context.Context, req DeleteCampaignRequestObject) (DeleteCampaignResponseObject, error) {
	if err := h.svc.DeleteDimension(ctx, domain.KindCampaign, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteCampaign204Response{}, nil
}

// --- Periode & posting rule -------------------------------------------------------------------

// ListFiscalPeriods mengembalikan periode akuntansi.
func (h *AccountingHandler) ListFiscalPeriods(ctx context.Context, req ListFiscalPeriodsRequestObject) (ListFiscalPeriodsResponseObject, error) {
	year := 0
	if req.Params.Year != nil {
		year = *req.Params.Year
	}
	list, err := h.svc.ListPeriods(ctx, year)
	if err != nil {
		return nil, err
	}
	out := make([]FiscalPeriod, len(list))
	for i, p := range list {
		out[i] = toPeriod(p)
	}
	return ListFiscalPeriods200JSONResponse(out), nil
}

// EnsureFiscalPeriods membuat periode bulan berjalan dan berikutnya.
func (h *AccountingHandler) EnsureFiscalPeriods(ctx context.Context, _ EnsureFiscalPeriodsRequestObject) (EnsureFiscalPeriodsResponseObject, error) {
	n, err := h.svc.CreateUpcomingPeriods(ctx)
	if err != nil {
		return nil, err
	}
	return EnsureFiscalPeriods200JSONResponse{Created: n}, nil
}

// ListPostingRules mengembalikan posting rule.
func (h *AccountingHandler) ListPostingRules(ctx context.Context, _ ListPostingRulesRequestObject) (ListPostingRulesResponseObject, error) {
	rules, views, err := h.svc.PostingRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PostingRule, len(rules))
	for i, r := range rules {
		dims := make([]DimensionKind, len(r.RequiredDimensions))
		for j, d := range r.RequiredDimensions {
			dims[j] = DimensionKind(d)
		}
		accounts := make([]PostingRuleAccount, 0, len(views[r.Code]))
		for _, v := range views[r.Code] {
			accounts = append(accounts, PostingRuleAccount{
				AmountKey: v.AmountKey, Side: NormalBalance(v.Side), Resolver: string(v.Resolver), AccountCode: v.AccountCode, AccountName: v.AccountName,
			})
		}
		out[i] = PostingRule{
			Code: r.Code, Name: r.Name, Description: r.Description, RequiredDimensions: dims, Release: r.Release,
			IsActive: r.IsActive, Accounts: accounts,
		}
	}
	return ListPostingRules200JSONResponse(out), nil
}

// --- Jurnal -----------------------------------------------------------------------------------

// ListJournalEntries mengembalikan daftar jurnal.
func (h *AccountingHandler) ListJournalEntries(ctx context.Context, req ListJournalEntriesRequestObject) (ListJournalEntriesResponseObject, error) {
	p := req.Params
	f := postgres.EntryFilter{DateFrom: optDate(p.DateFrom), DateTo: optDate(p.DateTo), Query: strings.TrimSpace(str(p.Q))}
	if p.Status != nil {
		f.Status = domain.EntryStatus(*p.Status)
	}
	if p.EntryType != nil {
		f.EntryType = domain.EntryType(*p.EntryType)
	}
	res, err := h.svc.ListJournals(ctx, f, paging.New(p.Page, p.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]JournalEntrySummary, len(res.Items))
	for i, e := range res.Items {
		items[i] = toSummary(e)
	}
	return ListJournalEntries200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total, TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// CreateJournalEntry menyimpan draft Jurnal Umum.
func (h *AccountingHandler) CreateJournalEntry(ctx context.Context, req CreateJournalEntryRequestObject) (CreateJournalEntryResponseObject, error) {
	in, err := manualInput(req.Body.BusinessDate, req.Body.Description, req.Body.Lines)
	if err != nil {
		return nil, err
	}
	e, err := h.svc.CreateManualJournal(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.toEntry(ctx, e)
	if err != nil {
		return nil, err
	}
	return CreateJournalEntry201JSONResponse(out), nil
}

// GetJournalEntry mengembalikan detail jurnal.
func (h *AccountingHandler) GetJournalEntry(ctx context.Context, req GetJournalEntryRequestObject) (GetJournalEntryResponseObject, error) {
	e, err := h.svc.GetJournal(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := h.toEntry(ctx, e)
	if err != nil {
		return nil, err
	}
	return GetJournalEntry200JSONResponse(out), nil
}

// UpdateJournalEntry mengubah draft Jurnal Umum.
func (h *AccountingHandler) UpdateJournalEntry(ctx context.Context, req UpdateJournalEntryRequestObject) (UpdateJournalEntryResponseObject, error) {
	in, err := manualInput(req.Body.BusinessDate, req.Body.Description, req.Body.Lines)
	if err != nil {
		return nil, err
	}
	e, err := h.svc.UpdateManualJournal(ctx, req.Id, req.Body.Version, in)
	if err != nil {
		return nil, err
	}
	out, err := h.toEntry(ctx, e)
	if err != nil {
		return nil, err
	}
	return UpdateJournalEntry200JSONResponse(out), nil
}

// DeleteJournalEntry menghapus draft Jurnal Umum.
func (h *AccountingHandler) DeleteJournalEntry(ctx context.Context, req DeleteJournalEntryRequestObject) (DeleteJournalEntryResponseObject, error) {
	if err := h.svc.DeleteManualJournal(ctx, req.Id, req.Params.Version); err != nil {
		return nil, err
	}
	return DeleteJournalEntry204Response{}, nil
}

// PostJournalEntry memposting draft Jurnal Umum.
func (h *AccountingHandler) PostJournalEntry(ctx context.Context, req PostJournalEntryRequestObject) (PostJournalEntryResponseObject, error) {
	e, err := h.svc.PostManualJournal(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	out, err := h.toEntry(ctx, e)
	if err != nil {
		return nil, err
	}
	return PostJournalEntry200JSONResponse(out), nil
}

// ReverseJournalEntry menjalankan Balik Jurnal.
func (h *AccountingHandler) ReverseJournalEntry(ctx context.Context, req ReverseJournalEntryRequestObject) (ReverseJournalEntryResponseObject, error) {
	e, err := h.svc.ReverseManualJournal(ctx, req.Id, req.Body.Version, fromDate(req.Body.BusinessDate), req.Body.Reason)
	if err != nil {
		return nil, err
	}
	out, err := h.toEntry(ctx, e)
	if err != nil {
		return nil, err
	}
	return ReverseJournalEntry201JSONResponse(out), nil
}

// GetGeneralLedger mengembalikan Buku Besar satu akun.
func (h *AccountingHandler) GetGeneralLedger(ctx context.Context, req GetGeneralLedgerRequestObject) (GetGeneralLedgerResponseObject, error) {
	p := req.Params
	f := postgres.LedgerFilter{
		AccountID: p.AccountId, DateFrom: fromDate(p.DateFrom), DateTo: fromDate(p.DateTo),
		Dimensions: domain.Dimensions{ChannelID: p.ChannelId, SubChannelID: p.SubChannelId, DivisionID: p.DivisionId, CampaignID: p.CampaignId},
	}
	page := paging.New(p.Page, p.PageSize)
	l, err := h.svc.GeneralLedger(ctx, f, page)
	if err != nil {
		return nil, err
	}
	dims := make([]domain.Dimensions, len(l.Lines))
	for i, line := range l.Lines {
		dims[i] = line.Dimensions
	}
	refs, err := h.loadRefs(ctx, dims)
	if err != nil {
		return nil, err
	}
	items := make([]GeneralLedgerLine, len(l.Lines))
	for i, line := range l.Lines {
		items[i] = GeneralLedgerLine{
			EntryId: line.EntryID, Number: optStr(line.Number), BusinessDate: toDate(line.BusinessDate),
			EntryType: JournalEntryType(line.EntryType), SourceType: line.SourceType, SourceLabel: line.SourceLabel,
			Description: line.Description, LineNo: line.LineNo, Debit: line.Debit.String(), Credit: line.Credit.String(),
			Balance: line.Balance.String(), Memo: line.Memo, Channel: refs.ref(line.Dimensions.ChannelID),
			SubChannel: refs.ref(line.Dimensions.SubChannelID), Division: refs.ref(line.Dimensions.DivisionID),
			Campaign: refs.ref(line.Dimensions.CampaignID),
		}
	}
	return GetGeneralLedger200JSONResponse{
		Account: toAccount(l.Account), DateFrom: p.DateFrom, DateTo: p.DateTo, OpeningBalance: l.OpeningBalance.String(),
		PeriodDebit: l.PeriodDebit.String(), PeriodCredit: l.PeriodCredit.String(), ClosingBalance: l.ClosingBalance.String(),
		Items: items, Page: l.Page.Number, PageSize: l.Page.Size, TotalItems: l.Total, TotalPages: l.Page.TotalPages(l.Total),
	}, nil
}
