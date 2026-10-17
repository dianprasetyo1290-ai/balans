package http

import (
	"context"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
)

// Laporan keuangan, laba rugi per dimensi, rekap pajak, dashboard eksekutif, dan status read model.

func toPLSummary(p domain.ProfitLoss) ProfitLossSummary {
	return ProfitLossSummary{
		Revenue: p.Revenue.String(), Cogs: p.COGS.String(), GrossProfit: p.GrossProfit.String(),
		OperatingExpense: p.OperatingExp.String(), OperatingProfit: p.OperatingProfit.String(),
		OtherNet: p.OtherNet.String(), NetProfit: p.NetProfit.String(),
		GrossMarginPercent: domain.MarginPercent(p.GrossProfit, p.Revenue).StringFixed(2),
		NetMarginPercent:   domain.MarginPercent(p.NetProfit, p.Revenue).StringFixed(2),
	}
}

func optTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

// GetFinancialStatement mengembalikan Neraca, Laba Rugi, atau Arus Kas.
func (h *AccountingHandler) GetFinancialStatement(ctx context.Context, req GetFinancialStatementRequestObject) (GetFinancialStatementResponseObject, error) {
	p := req.Params
	f := service.StatementFilter{
		Kind: domain.StatementKind(p.Kind), DateTo: fromDate(p.DateTo),
		Dimensions: domain.Dimensions{ChannelID: p.ChannelId, SubChannelID: p.SubChannelId, DivisionID: p.DivisionId, CampaignID: p.CampaignId},
	}
	if p.DateFrom != nil {
		f.DateFrom = fromDate(*p.DateFrom)
	}
	st, err := h.reports.Statement(ctx, f)
	if err != nil {
		return nil, err
	}
	return GetFinancialStatement200JSONResponse(toFinancialStatement(st)), nil
}

func toFinancialStatement(st service.StatementView) FinancialStatement {
	rows := make([]StatementRow, len(st.Rows))
	for i, r := range st.Rows {
		rows[i] = StatementRow{Kind: ReportRowKind(r.Kind), Code: optStr(r.Code), Label: r.Label, Level: r.Level, Amount: r.Amount.String()}
	}
	return FinancialStatement{
		Kind: StatementKind(st.Kind), Title: st.Kind.Label(), DateFrom: toDate(st.DateFrom), DateTo: toDate(st.DateTo),
		Rows: rows, Balanced: st.Balanced, Summary: toPLSummary(st.Summary), RefreshedAt: optTime(st.RefreshedAt),
	}
}

func dimensionName(kind domain.DimensionKind) string {
	return "Tanpa " + kind.Label()
}

// GetProfitLossByDimension mengembalikan laba rugi per dimensi.
func (h *AccountingHandler) GetProfitLossByDimension(ctx context.Context, req GetProfitLossByDimensionRequestObject) (GetProfitLossByDimensionResponseObject, error) {
	p := req.Params
	r, err := h.reports.DimensionProfitLoss(ctx, service.DimensionReportFilter{
		Kind: domain.DimensionKind(p.Dimension), DateFrom: fromDate(p.DateFrom), DateTo: fromDate(p.DateTo), ChannelID: p.ChannelId,
	})
	if err != nil {
		return nil, err
	}
	return GetProfitLossByDimension200JSONResponse(toDimensionProfitLoss(r)), nil
}

func toDimensionProfitLoss(r service.DimensionReport) DimensionProfitLoss {
	cols := make([]DimensionColumn, len(r.Columns))
	for i, c := range r.Columns {
		col := DimensionColumn{Code: "", Name: dimensionName(r.Kind), Summary: toPLSummary(c.Summary)}
		if c.ID != uuid.Nil {
			id := c.ID
			l := r.Labels[c.ID]
			col.Id, col.Code, col.Name = &id, l.Code, l.Name
		}
		cols[i] = col
	}
	accs := make([]DimensionAccountRow, len(r.Accounts))
	for i, a := range r.Accounts {
		amounts := make([]Money, len(a.Amounts))
		for j, v := range a.Amounts {
			amounts[j] = v.String()
		}
		accs[i] = DimensionAccountRow{
			AccountCode: a.Account.Code, AccountName: a.Account.Name, Group: int(a.Account.Group), Amounts: amounts, Total: a.Total.String(),
		}
	}
	return DimensionProfitLoss{
		Dimension: DimensionKind(r.Kind), DateFrom: toDate(r.DateFrom), DateTo: toDate(r.DateTo), Columns: cols,
		Accounts: accs, Total: toPLSummary(r.Total), RefreshedAt: optTime(r.RefreshedAt),
	}
}

// GetTaxSummary mengembalikan rekap pajak tahunan.
func (h *AccountingHandler) GetTaxSummary(ctx context.Context, req GetTaxSummaryRequestObject) (GetTaxSummaryResponseObject, error) {
	month := 0
	if req.Params.Month != nil {
		month = *req.Params.Month
	}
	r, err := h.reports.TaxReport(ctx, req.Params.Year, month)
	if err != nil {
		return nil, err
	}
	months := make([]TaxMonth, len(r.Months))
	for i, m := range r.Months {
		months[i] = TaxMonth{
			Month: toDate(m.Month), Pph21Withheld: m.PPh21Withheld.String(), Pph23Withheld: m.PPh23Withheld.String(),
			Pph42Withheld: m.PPh42Withheld.String(), WithholdingPaid: m.WithholdingPaid.String(), FinalTax: m.FinalTax.String(),
			FinalTaxPaid: m.FinalTaxPaid.String(), Turnover: m.Turnover.String(), CumulativeTurnover: m.Cumulative.String(),
		}
	}
	lines := make([]TaxJournalLine, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = toTaxLine(l)
	}
	return GetTaxSummary200JSONResponse{
		Year: r.Year, Months: months, Turnover: r.Turnover.String(), Limit: r.Limit.String(),
		UsedPercent: r.UsedPercent.StringFixed(2), Warning: r.WarningExceeded, LimitExceeded: r.LimitExceeded,
		Lines: lines, RefreshedAt: optTime(r.RefreshedAt),
	}, nil
}

func toTaxLine(l postgres.TaxLine) TaxJournalLine {
	return TaxJournalLine{
		EntryId: l.EntryID, BusinessDate: toDate(l.BusinessDate), Number: l.Number, SourceType: l.SourceType,
		SourceLabel: l.SourceLabel, Description: l.Description, AccountCode: l.AccountCode, AccountName: l.AccountName,
		Debit: l.Debit.String(), Credit: l.Credit.String(),
	}
}

// GetExecutiveDashboard mengembalikan dashboard eksekutif.
func (h *AccountingHandler) GetExecutiveDashboard(ctx context.Context, req GetExecutiveDashboardRequestObject) (GetExecutiveDashboardResponseObject, error) {
	d, err := h.reports.ExecutiveDashboard(ctx, fromDate(req.Params.DateFrom), fromDate(req.Params.DateTo))
	if err != nil {
		return nil, err
	}
	trend := make([]DashboardTrendPoint, len(d.Trend))
	for i, t := range d.Trend {
		trend[i] = DashboardTrendPoint{Month: toDate(t.Month), Revenue: t.Revenue.String(), GrossProfit: t.GrossProfit.String(), NetProfit: t.NetProfit.String()}
	}
	channels := make([]DashboardChannel, len(d.Channels))
	for i, c := range d.Channels {
		ch := DashboardChannel{Code: c.Code, Name: c.Name, Revenue: c.Revenue.String(), GrossProfit: c.GrossProfit.String(), MarginPercent: c.MarginPct.StringFixed(2)}
		if c.ChannelID != uuid.Nil {
			id := c.ChannelID
			ch.Id = &id
		}
		channels[i] = ch
	}
	return GetExecutiveDashboard200JSONResponse{
		DateFrom: toDate(d.DateFrom), DateTo: toDate(d.DateTo), Period: toPLSummary(d.Period), Previous: toPLSummary(d.Previous),
		Cash: d.Cash.String(), Receivables: d.Receivables.String(), Inventory: d.Inventory.String(), Payables: d.Payables.String(),
		Trend: trend, Channels: channels, RefreshedAt: optTime(d.RefreshedAt),
	}, nil
}

func toReadModelStatus(list []postgres.ReadModelRefresh) ReadModelStatus {
	items := make([]ReadModelRefresh, len(list))
	for i, r := range list {
		items[i] = ReadModelRefresh{Name: r.Name, RefreshedAt: r.RefreshedAt, DurationMs: r.DurationMs}
	}
	return ReadModelStatus{Items: items}
}

// ListReadModels mengembalikan waktu refresh read model.
func (h *AccountingHandler) ListReadModels(ctx context.Context, _ ListReadModelsRequestObject) (ListReadModelsResponseObject, error) {
	list, err := h.reports.ReadModelStatus(ctx)
	if err != nil {
		return nil, err
	}
	return ListReadModels200JSONResponse(toReadModelStatus(list)), nil
}

// RefreshReadModels me-refresh read model sekarang.
func (h *AccountingHandler) RefreshReadModels(ctx context.Context, _ RefreshReadModelsRequestObject) (RefreshReadModelsResponseObject, error) {
	list, err := h.reports.RefreshNow(ctx)
	if err != nil {
		return nil, err
	}
	return RefreshReadModels200JSONResponse(toReadModelStatus(list)), nil
}
