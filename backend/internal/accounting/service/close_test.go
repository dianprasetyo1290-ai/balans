package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/platform/tabular"
)

var closePerms = append([]string{
	service.PermFixedAssetRead, service.PermFixedAssetWrite, service.PermFixedAssetPost, service.PermReportRead,
	domain.PermFiscalPeriodPost, domain.PermFiscalPeriodReopen,
}, allPerms...)

// closeCtx adalah Head F&A yang baru saja re-autentikasi (jam test tetap monday09).
func closeCtx(reauth bool) context.Context {
	set := map[string]struct{}{}
	for _, p := range closePerms {
		set[p] = struct{}{}
	}
	a := actor.Actor{UserID: uuid.New(), FullName: "Head F&A", Permissions: set}
	if reauth {
		a.ReauthAt = monday09
	}
	return actor.With(context.Background(), a)
}

func d(y int, m time.Month, day int) clock.BusinessDate { return clock.NewBusinessDate(y, m, day) }

func (h *harness) assetClass(code string) domain.AssetClass {
	h.t.Helper()
	list, err := h.svc.ListAssetClasses(closeCtx(false), true)
	require.NoError(h.t, err)
	for _, c := range list {
		if c.Code == code {
			return c
		}
	}
	h.t.Fatalf("kelas aset %s tidak ada", code)
	return domain.AssetClass{}
}

func (h *harness) entryOf(id uuid.UUID) domain.Entry {
	h.t.Helper()
	e, err := h.svc.GetJournal(closeCtx(false), id)
	require.NoError(h.t, err)
	return e
}

func lineAmounts(e domain.Entry) map[string]string {
	out := map[string]string{}
	for _, l := range e.Lines {
		if l.Debit.IsPositive() {
			out["D"+l.AccountCode] = l.Debit.String()
		} else {
			out["C"+l.AccountCode] = l.Credit.String()
		}
	}
	return out
}

func TestFixedAssetLifecycleAndDepreciation(t *testing.T) {
	h := setup(t)
	ctx := closeCtx(false)
	class := h.assetClass("office_equipment")
	assert.Equal(t, 48, class.UsefulLifeMonths)
	assert.Equal(t, "1502", class.AssetAccountCode)
	bank := h.account("1111")
	code := "AST-T-" + uuid.NewString()[:8]

	asset, err := h.svc.CreateFixedAsset(ctx, domain.FixedAssetInput{
		Code: code, Name: "Laptop Finance", ClassID: class.ID, DivisionID: h.division, Source: domain.AssetPurchase,
		AcquisitionDate: d(2031, time.January, 15), Cost: money.FromInt(12_000_000), PaymentAccountID: &bank.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.AssetDraft, asset.Status)

	_, err = h.svc.ActivateFixedAsset(as(service.PermFixedAssetRead), asset.ID, asset.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))

	asset, err = h.svc.ActivateFixedAsset(ctx, asset.ID, asset.Version)
	require.NoError(t, err)
	require.NotNil(t, asset.AcquisitionJournal)
	acq := h.entryOf(*asset.AcquisitionJournal)
	assert.Equal(t, map[string]string{"D1502": "12000000.00", "C1111": "12000000.00"}, lineAmounts(acq), "A-31 perolehan")

	run, err := h.svc.RunDepreciation(ctx, domain.PeriodKey{Year: 2031, Month: time.January})
	require.NoError(t, err)
	var line domain.DepreciationLine
	for _, l := range run.Lines {
		if l.AssetID == asset.ID {
			line = l
		}
	}
	require.NotEqual(t, uuid.Nil, line.ID)
	assert.Equal(t, "250000.00", line.Amount.String())
	dep := h.entryOf(line.JournalEntryID)
	assert.Equal(t, map[string]string{"D6305": "250000.00", "C1592": "250000.00"}, lineAmounts(dep), "A-31 penyusutan")
	assert.Equal(t, d(2031, time.January, 31), dep.BusinessDate)

	// Idempotent: job diulang tidak menyusutkan ulang.
	again, err := h.svc.Depreciate(context.Background(), domain.PeriodKey{Year: 2031, Month: time.January})
	require.NoError(t, err)
	assert.Equal(t, run.Run.ID, again.Run.ID)
	assert.Equal(t, 2, again.Run.RunCount)
	detail, err := h.svc.GetFixedAsset(ctx, asset.ID)
	require.NoError(t, err)
	assert.Len(t, detail.Lines, 1)
	assert.Equal(t, "250000.00", detail.Asset.Accumulated.String())

	// Langkah 6 checklist selesai otomatis.
	p, err := h.svc.OpenPeriod(context.Background(), d(2031, time.January, 1))
	require.NoError(t, err)
	cl, err := h.svc.CloseChecklist(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StepDone, cl.Items[5].Status)
	assert.True(t, cl.Items[5].ChecksPassed())

	runs, err := h.svc.ListDepreciationRuns(ctx, paging.New(nil, nil))
	require.NoError(t, err)
	assert.NotEmpty(t, runs.Items)

	_, err = h.svc.UpdateFixedAsset(ctx, asset.ID, asset.Version, domain.FixedAssetInput{})
	assert.Error(t, err, "aset aktif tidak bisa diubah")
	assert.Error(t, h.svc.DeleteFixedAsset(ctx, asset.ID, detail.Asset.Version))

	// Pelepasan: nilai buku 11.750.000, dijual 12.000.000 → laba 250.000.
	disposed, err := h.svc.DisposeFixedAsset(ctx, asset.ID, detail.Asset.Version, service.DisposeInput{
		Date: d(2031, time.February, 10), Proceeds: money.FromInt(12_000_000), AccountID: &bank.ID, Reason: "Dijual ke karyawan",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.AssetDisposed, disposed.Status)
	disp := h.entryOf(*disposed.DisposalJournal)
	assert.Equal(t, map[string]string{
		"D1592": "250000.00", "D1111": "12000000.00", "C1502": "12000000.00", "C7101": "250000.00",
	}, lineAmounts(disp))

	feb, err := h.svc.Depreciate(context.Background(), domain.PeriodKey{Year: 2031, Month: time.February})
	require.NoError(t, err)
	for _, l := range feb.Lines {
		assert.NotEqual(t, asset.ID, l.AssetID, "aset dilepas tidak disusutkan")
	}

	// Draft bisa diubah dan dihapus.
	draft, err := h.svc.CreateFixedAsset(ctx, domain.FixedAssetInput{
		Code: code + "B", Name: "Printer", ClassID: class.ID, DivisionID: h.division, Source: domain.AssetPurchase,
		AcquisitionDate: d(2031, time.January, 20), Cost: money.FromInt(2_400_000), PaymentAccountID: &bank.ID,
	})
	require.NoError(t, err)
	_, err = h.svc.CreateFixedAsset(ctx, domain.FixedAssetInput{
		Code: code + "B", Name: "Printer", ClassID: class.ID, DivisionID: h.division, Source: domain.AssetPurchase,
		AcquisitionDate: d(2031, time.January, 20), Cost: money.FromInt(2_400_000), PaymentAccountID: &bank.ID,
	})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode))
	draft, err = h.svc.UpdateFixedAsset(ctx, draft.ID, draft.Version, domain.FixedAssetInput{
		Name: "Printer Laser", ClassID: class.ID, DivisionID: h.division, Source: domain.AssetPurchase,
		AcquisitionDate: d(2031, time.January, 20), Cost: money.FromInt(2_400_000), PaymentAccountID: &bank.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "Printer Laser", draft.Name)
	require.NoError(t, h.svc.DeleteFixedAsset(ctx, draft.ID, draft.Version))
	list, err := h.svc.ListFixedAssets(ctx, postgres.FixedAssetFilter{Query: code}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, list.Items, 1, "draft terhapus tidak tampil")
}

func TestAssetClassValidation(t *testing.T) {
	h := setup(t)
	ctx := closeCtx(false)
	asset, accum, expense := h.account("1503"), h.account("1593"), h.account("6301")
	code := "test_" + uuid.NewString()[:6]
	c, err := h.svc.CreateAssetClass(ctx, domain.AssetClassInput{Code: code, Name: "Kelas Test", UsefulLifeMonths: 24,
		AssetAccountID: asset.ID, AccumulatedAccountID: accum.ID, IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "1593", c.AccumulatedAccountCode)
	_, err = h.svc.UpdateAssetClass(ctx, c.ID, c.Version, domain.AssetClassInput{Name: "Kelas Test", UsefulLifeMonths: 24,
		AssetAccountID: expense.ID, AccumulatedAccountID: asset.ID, IsActive: true})
	f := fieldsOf(err)
	assert.Contains(t, f, "assetAccountId", "akun aset harus 1xxx")
	assert.Contains(t, f, "accumulatedAccountId", "akun akumulasi bersaldo kredit")
	c, err = h.svc.UpdateAssetClass(ctx, c.ID, c.Version, domain.AssetClassInput{Name: "Kelas Test 2", UsefulLifeMonths: 36,
		AssetAccountID: asset.ID, AccumulatedAccountID: accum.ID, IsActive: false})
	require.NoError(t, err)
	assert.Equal(t, 36, c.UsefulLifeMonths)
}

type fakeApprovals struct{ subjects []service.ApprovalSubject }

func (f *fakeApprovals) RequestApproval(_ context.Context, s service.ApprovalSubject) error {
	f.subjects = append(f.subjects, s)
	return nil
}

func TestClosePeriodChecklistReauthAndReopen(t *testing.T) {
	h := setup(t)
	approvals := &fakeApprovals{}
	h.svc.SetApprovals(approvals)
	var hooked bool
	h.svc.RegisterCloseCheck(domain.StepInventoryReview, func(_ context.Context, from, to clock.BusinessDate) ([]domain.CloseCheck, error) {
		hooked = true
		return []domain.CloseCheck{{Source: "Gudang", OK: true, Message: "Tidak ada selisih stock count " + from.String() + " s.d. " + to.String()}}, nil
	})
	ctx := closeCtx(true)
	// Periode target Mar 2032; semua periode sebelumnya ditutup agar tutup buku berurutan.
	p, err := h.svc.OpenPeriod(context.Background(), d(2032, time.March, 1))
	require.NoError(t, err)
	_, err = h.pool.Exec(context.Background(),
		`UPDATE fiscal_periods SET status = 'closed', closed_at = now() WHERE year * 12 + month < 2032 * 12 + 3`)
	require.NoError(t, err)

	cash, rent := h.account("1111"), h.account("6301")
	draft := h.draft(d(2032, time.March, 10),
		line(rent, "500000", "0", domain.Dimensions{DivisionID: ptr(h.division)}),
		line(cash, "0", "500000", domain.Dimensions{}))

	cl, err := h.svc.CloseChecklist(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, cl.Items, 9)
	assert.True(t, hooked, "hook modul lain dijalankan")
	assert.False(t, cl.Items[0].ChecksPassed(), "jurnal draft menahan langkah 1")
	assert.Contains(t, cl.AllowedActions, domain.PeriodActionClose)
	assert.NotEmpty(t, cl.Blockers)

	_, err = h.svc.UpdateChecklistStep(ctx, p.ID, domain.StepOperationalDocuments, 0, domain.ChecklistUpdate{Status: domain.StepDone})
	assert.Contains(t, fieldsOf(err), "status", "pemeriksaan otomatis belum lolos")
	_, err = h.svc.PostManualJournal(h.ctx(), draft.ID, draft.Version)
	require.NoError(t, err)

	pic := uuid.New()
	for _, st := range domain.CloseSteps[:8] {
		u := domain.ChecklistUpdate{Status: domain.StepDone, PICUserID: &pic}
		if st.Code == domain.StepPayroll {
			u = domain.ChecklistUpdate{Status: domain.StepSkipped, Note: "Payroll mulai Fase 5"}
		}
		cl, err = h.svc.UpdateChecklistStep(ctx, p.ID, st.Code, 0, u)
		require.NoError(t, err, st.Code)
	}
	assert.Empty(t, cl.Blockers)
	assert.Equal(t, &pic, cl.Items[0].PICUserID)

	_, err = h.svc.ClosePeriod(closeCtx(false), p.ID, cl.Period.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthReauthRequired), "tutup periode butuh re-autentikasi")
	_, err = h.svc.ClosePeriod(ctx, p.ID, cl.Period.Version+1)
	assert.True(t, apperr.HasCode(err, apperr.CodeVersionConflict))

	cl, err = h.svc.ClosePeriod(ctx, p.ID, cl.Period.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodClosed, cl.Period.Status)
	assert.Equal(t, domain.StepDone, cl.Items[8].Status, "langkah 9 selesai saat tutup periode")
	assert.Equal(t, []domain.PeriodAction{domain.PeriodActionRequestReopen}, cl.AllowedActions)

	// Periode tertutup menolak posting end-to-end: Poster, Jurnal Umum, pemeriksaan modul lain.
	_, err = h.svc.Post(context.Background(), domain.PostingRequest{
		RuleCode: "A-30", SourceType: "final_tax_run", SourceID: uuid.New(), BusinessDate: d(2032, time.March, 31),
		Amounts: map[string]money.Amount{"tax_value": money.FromInt(1000)},
	})
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))
	_, err = h.svc.CreateManualJournal(h.ctx(), domain.ManualInput{BusinessDate: d(2032, time.March, 5), Description: "x",
		Lines: []domain.Line{line(cash, "1", "0", domain.Dimensions{}), line(cash, "0", "1", domain.Dimensions{})}})
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))
	assert.True(t, apperr.HasCode(h.svc.CheckPeriodOpen(context.Background(), d(2032, time.March, 20)), apperr.CodePeriodLocked))
	_, err = h.svc.UpdateChecklistStep(ctx, p.ID, domain.StepTaxRecap, 0, domain.ChecklistUpdate{Status: domain.StepPending})
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))

	// Buka kembali: ★ re-auth, menunggu approval (tetap terkunci), lalu Head F&A → Direksi menyetujui.
	_, err = h.svc.RequestReopen(closeCtx(false), p.ID, cl.Period.Version, "Koreksi HPP")
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthReauthRequired))
	_, err = h.svc.RequestReopen(ctx, p.ID, cl.Period.Version, " ")
	assert.Contains(t, fieldsOf(err), "reason")
	cl, err = h.svc.RequestReopen(ctx, p.ID, cl.Period.Version, "Koreksi HPP backdate")
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodPendingApproval, cl.Period.Status)
	require.Len(t, approvals.subjects, 1)
	subj := approvals.subjects[0]
	assert.Equal(t, domain.ApprovalSubjectFiscalPeriodReopen, subj.Type)
	assert.Equal(t, "Buka Kembali Periode Mar 2032", subj.Label)
	require.Len(t, cl.Reopens, 1)
	assert.True(t, apperr.HasCode(h.svc.CheckPeriodOpen(context.Background(), d(2032, time.March, 20)), apperr.CodePeriodLocked))

	require.NoError(t, h.svc.DecideReopen(context.Background(), subj.ID, false, uuid.New(), "Tidak perlu"))
	cl, err = h.svc.CloseChecklist(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodClosed, cl.Period.Status, "ditolak kembali Ditutup")
	assert.Error(t, h.svc.DecideReopen(context.Background(), subj.ID, true, uuid.New(), ""), "keputusan kedua ditolak")

	cl, err = h.svc.RequestReopen(ctx, p.ID, cl.Period.Version, "Koreksi HPP backdate")
	require.NoError(t, err)
	require.NoError(t, h.svc.DecideReopen(context.Background(), approvals.subjects[1].ID, true, uuid.New(), "OK"))
	cl, err = h.svc.CloseChecklist(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodOpen, cl.Period.Status)
	assert.Equal(t, 1, cl.Period.ReopenedCount)
	assert.Equal(t, domain.StepPending, cl.Items[8].Status, "langkah 9 kembali Belum")
	require.NoError(t, h.svc.CheckPeriodOpen(context.Background(), d(2032, time.March, 20)))
}

func TestCloseRequiresEarlierPeriodsClosed(t *testing.T) {
	h := setup(t)
	p, err := h.svc.OpenPeriod(context.Background(), d(2032, time.June, 1))
	require.NoError(t, err)
	_, err = h.svc.OpenPeriod(context.Background(), d(2032, time.May, 1))
	require.NoError(t, err)
	_, err = h.svc.ClosePeriod(closeCtx(true), p.ID, p.Version)
	require.Error(t, err)
	assert.Contains(t, apperr.As(err).Message, "berurutan")
}

func TestTrialBalanceAndExports(t *testing.T) {
	h := setup(t)
	ctx := closeCtx(false)
	cash, rent := h.account("1111"), h.account("6301")
	e := h.draft(d(2033, time.April, 3),
		line(rent, "750000", "0", domain.Dimensions{DivisionID: ptr(h.division)}),
		line(cash, "0", "750000", domain.Dimensions{}))
	_, err := h.svc.PostManualJournal(h.ctx(), e.ID, e.Version)
	require.NoError(t, err)

	tb, err := h.svc.TrialBalance(ctx, postgres.TrialBalanceFilter{DateFrom: d(2033, time.April, 1), DateTo: d(2033, time.April, 30)})
	require.NoError(t, err)
	assert.True(t, tb.Totals.Balanced())
	rows := map[string]domain.TrialBalanceRow{}
	for _, r := range tb.Rows {
		rows[r.AccountCode] = r
	}
	assert.True(t, rows["6301"].PeriodDebit.Cmp(money.FromInt(750_000)) >= 0)

	byDiv, err := h.svc.TrialBalance(ctx, postgres.TrialBalanceFilter{DateFrom: d(2033, time.April, 1), DateTo: d(2033, time.April, 30),
		Dimensions: domain.Dimensions{DivisionID: ptr(h.division)}})
	require.NoError(t, err)
	for _, r := range byDiv.Rows {
		assert.NotEqual(t, "1111", r.AccountCode, "baris kas tanpa divisi tidak ikut filter divisi")
	}
	_, err = h.svc.TrialBalance(ctx, postgres.TrialBalanceFilter{DateFrom: d(2033, time.April, 30), DateTo: d(2033, time.April, 1)})
	assert.Contains(t, fieldsOf(err), "dateTo")
	_, err = h.svc.TrialBalance(as(service.PermAccountRead), postgres.TrialBalanceFilter{DateFrom: d(2033, time.April, 1), DateTo: d(2033, time.April, 30)})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))

	var exported []tabular.Row
	require.NoError(t, h.svc.ExportTrialBalance(ctx, map[string]string{"year": "2033", "month": "4"}, func(r tabular.Row) error {
		exported = append(exported, r)
		return nil
	}))
	require.NotEmpty(t, exported)
	assert.Contains(t, exported[len(exported)-1].Get("accountName"), "TOTAL 2033-04-01 s.d. 2033-04-30")
	assert.Error(t, h.svc.ExportTrialBalance(ctx, map[string]string{"year": "2033", "month": "13"}, func(tabular.Row) error { return nil }))

	var ledger []tabular.Row
	require.NoError(t, h.svc.ExportLedger(ctx, map[string]string{"dateFrom": "2033-04-01", "dateTo": "2033-04-30", "accountId": rent.ID.String()},
		func(r tabular.Row) error {
			ledger = append(ledger, r)
			return nil
		}))
	require.NotEmpty(t, ledger)
	assert.Equal(t, "6301", ledger[0].Get("accountCode"))
	assert.Equal(t, "FAT", ledger[0].Get("division"))
	assert.Error(t, h.svc.ExportLedger(ctx, map[string]string{"accountId": "x"}, func(tabular.Row) error { return nil }))
}

func row(n int, kv ...string) tabular.Row {
	r := tabular.NewRow(n)
	for i := 0; i+1 < len(kv); i += 2 {
		r.Set(kv[i], kv[i+1])
	}
	return r
}

func TestJournalImportGroupsByReference(t *testing.T) {
	h := setup(t)
	ctx := h.ctx()
	key := uuid.NewString() + ":2"
	require.NoError(t, h.svc.ImportJournalRow(ctx, key, row(2, "reference", "JU-1", "businessDate", "2033-05-31",
		"description", "Koreksi sewa", "accountCode", "6301", "debit", "1500000", "divisionCode", "FAT")))
	require.NoError(t, h.svc.ImportJournalRow(ctx, key, row(3, "reference", "JU-1", "businessDate", "2033-05-31",
		"description", "Koreksi sewa", "accountCode", "1111", "credit", "1500000")))
	err := h.svc.ImportJournalRow(ctx, key, row(4, "reference", "JU-1", "businessDate", "2033-05-30",
		"description", "Koreksi sewa", "accountCode", "1111", "credit", "1"))
	assert.Contains(t, fieldsOf(err), "reference", "tanggal satu referensi harus sama")
	err = h.svc.ImportJournalRow(ctx, key, row(5, "reference", "JU-2", "businessDate", "2033-05-31",
		"description", "x", "accountCode", "6301", "debit", "1", "divisionCode", "NOPE"))
	assert.Contains(t, fieldsOf(err), "divisionCode")
	err = h.svc.ImportJournalRow(ctx, key, row(5, "reference", "JU-2", "businessDate", "2033-05-31",
		"description", "x", "accountCode", "1201", "debit", "1"))
	assert.Contains(t, fieldsOf(err), "accountCode", "akun kontrol tidak boleh Jurnal Umum")
	err = h.svc.ImportJournalRow(ctx, key, row(6, "reference", "JU-3", "businessDate", "2033-05-31",
		"description", "x", "accountCode", "9999", "debit", "1", "credit", "1"))
	assert.Contains(t, fieldsOf(err), "accountCode")

	list, err := h.svc.ListJournals(ctx, postgres.EntryFilter{Query: "Impor JU-1"}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	e, err := h.svc.GetJournal(ctx, list.Items[0].ID)
	require.NoError(t, err)
	require.Len(t, e.Lines, 2)
	assert.Equal(t, domain.EntryDraft, e.Status)
	posted, err := h.svc.PostManualJournal(ctx, e.ID, e.Version)
	require.NoError(t, err, "draft impor balance bisa diposting")
	assert.Equal(t, domain.EntryPosted, posted.Status)
}

type fakeParties struct{}

func (fakeParties) PartyByCode(_ context.Context, t domain.OpeningItemType, code string) (string, uuid.UUID, bool, error) {
	if code == "MISSING" {
		return "", uuid.Nil, false, nil
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(code))
	if t == domain.OpeningPayable {
		return domain.PartySupplier, id, true, nil
	}
	return domain.PartyCustomer, id, true, nil
}

type fakeStock map[string]money.Amount

func (f fakeStock) InventoryValueByAccount(context.Context, clock.BusinessDate) (map[string]money.Amount, error) {
	return f, nil
}

func TestOpeningBalancesImportReconcileAndPost(t *testing.T) {
	h := setup(t)
	bg := context.Background()
	for _, q := range []string{
		`DELETE FROM opening_item_settlements`, `DELETE FROM opening_items`, `DELETE FROM opening_balance_lines`,
		`DELETE FROM opening_balance_sets`,
	} {
		_, err := h.pool.Exec(bg, q)
		require.NoError(t, err)
	}
	h.svc.SetParties(fakeParties{})
	h.svc.SetStockValuer(fakeStock{"1311": money.FromInt(3_000_000)})
	ctx := closeCtx(false)

	v, err := h.svc.GetOpening(ctx)
	require.NoError(t, err)
	assert.Nil(t, v.Set)
	assert.Error(t, h.svc.ImportOpeningBalanceRow(ctx, row(2, "accountCode", "1111", "debit", "1")), "tanggal cut-over belum diisi")
	_, err = h.svc.SaveCutoverDate(ctx, d(2033, time.December, 30), 0)
	assert.Contains(t, fieldsOf(err), "cutoverDate", "harus akhir bulan")
	v, err = h.svc.SaveCutoverDate(ctx, d(2033, time.December, 31), 0)
	require.NoError(t, err)
	require.NotNil(t, v.Set)
	require.NoError(t, h.svc.EnsureCutoverDate(ctx, d(2033, time.December, 31)))
	assert.Error(t, h.svc.EnsureCutoverDate(ctx, d(2033, time.November, 30)))

	// Neraca Saldo Awal: impor + entri.
	for i, r := range [][]string{
		{"accountCode", "1111", "debit", "10000000"},
		{"accountCode", "1201", "debit", "4500000"},
		{"accountCode", "1311", "debit", "3000000"},
		{"accountCode", "1502", "debit", "12000000"},
		{"accountCode", "1592", "credit", "3000000"},
		{"accountCode", "2101", "credit", "2000000"},
		{"accountCode", "3101", "credit", "20000000"},
	} {
		require.NoError(t, h.svc.ImportOpeningBalanceRow(ctx, row(i+2, r...)), r[1])
	}
	err = h.svc.ImportOpeningBalanceRow(ctx, row(9, "accountCode", "6301", "debit", "1"))
	assert.Contains(t, fieldsOf(err), "accountCode", "hanya akun neraca")
	assert.Contains(t, fieldsOf(h.svc.ImportOpeningBalanceRow(ctx, row(10, "accountCode", "0000", "debit", "1"))), "accountCode")

	// Piutang & utang terbuka.
	require.NoError(t, h.svc.ImportOpeningReceivableRow(ctx, row(2, "documentNumber", "INV-LAMA-1", "documentDate", "2033-12-15",
		"dueDate", "2034-01-14", "partyCode", "PLG-1", "channel", "B2B", "amount", "5000000", "openAmount", "4500000")))
	err = h.svc.ImportOpeningReceivableRow(ctx, row(3, "documentNumber", "INV-LAMA-2", "documentDate", "2033-12-15",
		"dueDate", "2034-01-14", "partyCode", "MISSING", "amount", "1", "openAmount", "2"))
	assert.Contains(t, fieldsOf(err), "partyCode")
	require.NoError(t, h.svc.ImportOpeningPayableRow(ctx, row(2, "documentNumber", "BILL-1", "partyCode", "SUP-1",
		"documentDate", "2033-12-20", "dueDate", "2034-01-19", "amount", "2000000", "openAmount", "2000000")))

	// Aset tetap saldo awal.
	require.NoError(t, h.svc.ImportOpeningAssetRow(ctx, row(2, "code", "AST-OB-1", "name", "Laptop lama",
		"className", "Peralatan Kantor & Komputer", "acquisitionDate", "2032-01-01", "cost", "12000000", "usefulLife", "48",
		"openingAccumulated", "3000000")))
	err = h.svc.ImportOpeningAssetRow(ctx, row(3, "code", "AST-OB-2", "name", "X", "className", "Pesawat",
		"acquisitionDate", "2032-01-01", "cost", "1", "usefulLife", "12", "openingAccumulated", "0", "divisionCode", "NOPE"))
	assert.Contains(t, fieldsOf(err), "className")
	assert.Contains(t, fieldsOf(err), "divisionCode")

	items, err := h.svc.ListOpeningItems(ctx, postgres.OpeningItemFilter{Type: domain.OpeningReceivable}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, items.Items, 1)
	assert.Equal(t, "4500000.00", items.Outstanding.String())
	assert.NotNil(t, items.Items[0].ChannelID)

	rec, err := h.svc.CutoverReconciliation(ctx)
	require.NoError(t, err)
	assert.Equal(t, "draft", rec.LedgerSource)
	got := map[string]domain.ReconciliationStatus{}
	for _, c := range rec.Checks {
		got[c.Code+c.AccountCode] = c.Status
	}
	assert.Equal(t, domain.ReconFailed, got["trial_balance"], "debit 29,5 jt ≠ kredit 25 jt")
	assert.Equal(t, domain.ReconOK, got["inventory1311"])
	assert.Equal(t, domain.ReconOK, got["receivable1201"])
	assert.Equal(t, domain.ReconOK, got["payable2101"])
	assert.Equal(t, domain.ReconOK, got["fixed_asset_cost1502"])
	assert.Equal(t, domain.ReconOK, got["fixed_asset_accumulated1592"])
	assert.False(t, rec.Passed)

	// Koreksi ekuitas lewat entri manual → balance.
	equity := h.account("3101")
	v, err = h.svc.SaveOpeningLine(ctx, domain.OpeningLineInput{AccountID: equity.ID, Credit: money.FromInt(24_500_000), Debit: money.Zero})
	require.NoError(t, err)
	assert.True(t, v.Totals.Difference.IsZero())
	rec, err = h.svc.CutoverReconciliation(ctx)
	require.NoError(t, err)
	assert.True(t, rec.Passed, "%+v", rec.Checks)

	// Hapus baris lalu entri ulang.
	var cashLine domain.OpeningLine
	for _, l := range v.Lines {
		if l.AccountCode == "1111" {
			cashLine = l
		}
	}
	v, err = h.svc.DeleteOpeningLine(ctx, cashLine.ID)
	require.NoError(t, err)
	assert.Len(t, v.Lines, 6)
	cash := h.account("1111")
	v, err = h.svc.SaveOpeningLine(ctx, domain.OpeningLineInput{AccountID: cash.ID, Debit: money.FromInt(10_000_000), Credit: money.Zero})
	require.NoError(t, err)
	assert.Contains(t, v.AllowedActions, service.OpeningActionPost)

	var exported []tabular.Row
	require.NoError(t, h.svc.ExportOpeningBalances(ctx, func(r tabular.Row) error {
		exported = append(exported, r)
		return nil
	}))
	assert.Len(t, exported, 7)

	_, err = h.svc.PostOpening(ctx, v.Set.Version+1)
	assert.True(t, apperr.HasCode(err, apperr.CodeVersionConflict))
	v, err = h.svc.PostOpening(ctx, v.Set.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.OpeningPosted, v.Set.Status)
	require.NotNil(t, v.Set.JournalEntryID)
	e := h.entryOf(*v.Set.JournalEntryID)
	assert.Equal(t, "A-36", e.PostingRuleCode)
	assert.Equal(t, d(2033, time.December, 31), e.BusinessDate)
	assert.Len(t, e.Lines, 7)
	assert.Empty(t, v.AllowedActions)

	rec, err = h.svc.CutoverReconciliation(ctx)
	require.NoError(t, err)
	assert.Equal(t, "posted", rec.LedgerSource)

	_, err = h.svc.SaveOpeningLine(ctx, domain.OpeningLineInput{AccountID: cash.ID, Debit: money.FromInt(1), Credit: money.Zero})
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "saldo awal terkunci setelah diposting")

	// Pelunasan saldo terbuka oleh modul pembayaran (API publik), idempotent per dokumen sumber.
	pay := service.OpeningSettlement{ItemID: items.Items[0].ID, SourceType: "payment", SourceID: uuid.New(),
		BusinessDate: d(2034, time.January, 5), Amount: money.FromInt(1_500_000)}
	it, err := h.svc.SettleOpeningItem(bg, pay)
	require.NoError(t, err)
	assert.Equal(t, "3000000.00", it.Outstanding().String())
	it, err = h.svc.SettleOpeningItem(bg, pay)
	require.NoError(t, err)
	assert.Equal(t, "3000000.00", it.Outstanding().String(), "sumber sama tidak dihitung dua kali")
	_, err = h.svc.SettleOpeningItem(bg, service.OpeningSettlement{ItemID: it.ID, SourceType: "payment", SourceID: uuid.New(),
		BusinessDate: d(2034, time.January, 6), Amount: money.FromInt(9_000_000)})
	assert.Error(t, err)
	outstanding, err := h.svc.OpeningItems(bg, postgres.OpeningItemFilter{OutstandingOnly: true, Type: domain.OpeningReceivable}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Equal(t, "3000000.00", outstanding.Outstanding.String())
}
