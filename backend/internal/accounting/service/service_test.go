package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/accounting/service"
	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type numberer struct{ docs *document.Module }

func (n numberer) NextJournalNumber(ctx context.Context, d clock.BusinessDate) (string, error) {
	return n.docs.Next(ctx, document.TypeJournal, d)
}

type harness struct {
	t     *testing.T
	pool  *pgxpool.Pool
	svc   *service.Service
	clock *clock.Fixed

	channel, marketplace, shopee, lazada, division uuid.UUID
}

// monday09 adalah Senin 19 Okt 2026 09.00 WIB.
var (
	monday09 = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	oct19    = clock.NewBusinessDate(2026, time.October, 19)
)

var allPerms = []string{
	service.PermAccountRead, service.PermAccountWrite, service.PermPostingRuleRead, service.PermDimensionRead,
	service.PermDimensionWrite, service.PermFiscalPeriodRead, service.PermFiscalPeriodWrite, service.PermJournalRead,
	service.PermJournalWrite, service.PermJournalPost, service.PermJournalReverse,
}

func as(perms ...string) context.Context {
	set := map[string]struct{}{}
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: uuid.New(), FullName: "Finance", Permissions: set})
}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	// Periode dibuka kembali agar test lain tidak terpengaruh test periode tertutup.
	_, err := pool.Exec(ctx, `UPDATE fiscal_periods SET status = 'open', closed_at = NULL`)
	require.NoError(t, err)
	tx := db.NewTxManager(pool)
	fixed := clock.NewFixed(monday09)
	svc := service.New(service.Deps{
		Store: postgres.NewStore(pool), Tx: tx, Audit: audit.New(audit.Deps{Pool: pool}), Clock: fixed,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Numberer: numberer{docs: document.New(document.Deps{Pool: pool, Clock: fixed})},
	})
	h := &harness{t: t, pool: pool, svc: svc, clock: fixed}
	h.channel = h.dimID(domain.KindChannel, "internal")
	h.marketplace = h.dimID(domain.KindChannel, "marketplace")
	h.shopee = h.dimID(domain.KindSubChannel, "shopee")
	h.lazada = h.dimID(domain.KindSubChannel, "lazada")
	h.division = h.dimID(domain.KindDivision, "FAT")
	return h
}

func (h *harness) ctx() context.Context { return as(allPerms...) }

func (h *harness) dimID(kind domain.DimensionKind, code string) uuid.UUID {
	h.t.Helper()
	list, err := h.svc.ListDimensions(h.ctx(), kind, postgres.DimensionFilter{IncludeInactive: true})
	require.NoError(h.t, err)
	for _, v := range list {
		if v.Code == code {
			return v.ID
		}
	}
	h.t.Fatalf("dimensi %s %s tidak ada", kind, code)
	return uuid.Nil
}

func (h *harness) account(code string) domain.Account {
	h.t.Helper()
	list, err := h.svc.ListAccounts(h.ctx(), true)
	require.NoError(h.t, err)
	for _, a := range list {
		if a.Code == code {
			return a
		}
	}
	h.t.Fatalf("akun %s tidak ada", code)
	return domain.Account{}
}

func ptr(id uuid.UUID) *uuid.UUID { return &id }

func line(acc domain.Account, debit, credit string, dims domain.Dimensions) domain.Line {
	return domain.Line{AccountID: acc.ID, Debit: money.MustParse(debit), Credit: money.MustParse(credit), Dimensions: dims}
}

func (h *harness) draft(date clock.BusinessDate, lines ...domain.Line) domain.Entry {
	h.t.Helper()
	e, err := h.svc.CreateManualJournal(h.ctx(), domain.ManualInput{BusinessDate: date, Description: "Jurnal test", Lines: lines})
	require.NoError(h.t, err)
	return e
}

func (h *harness) closePeriod(year int, month time.Month) {
	h.t.Helper()
	_, err := h.pool.Exec(context.Background(), `
		INSERT INTO fiscal_periods (year, month, created_by, updated_by) VALUES ($1, $2, $3, $3) ON CONFLICT DO NOTHING`,
		year, int(month), uuid.Nil)
	require.NoError(h.t, err)
	_, err = h.pool.Exec(context.Background(), `UPDATE fiscal_periods SET status = 'closed', closed_at = now() WHERE year = $1 AND month = $2`, year, int(month))
	require.NoError(h.t, err)
}

func fieldsOf(err error) map[string]string { return apperr.As(err).Fields }

// --- COA (S-126) ------------------------------------------------------------------------------

func TestSeededChartOfAccounts(t *testing.T) {
	h := setup(t)
	list, err := h.svc.ListAccounts(h.ctx(), true)
	require.NoError(t, err)
	byCode := map[string]domain.Account{}
	for _, a := range list {
		byCode[a.Code] = a
	}
	assert.GreaterOrEqual(t, len(list), 86)
	assert.False(t, byCode["1100"].IsPostable, "akun induk tidak bisa diposting")
	assert.Equal(t, byCode["1000"].ID, *byCode["1100"].ParentID)
	assert.Equal(t, byCode["1100"].ID, *byCode["1111"].ParentID)
	assert.Equal(t, byCode["2000"].ID, *byCode["2101"].ParentID)
	assert.Equal(t, byCode["1500"].ID, *byCode["1591"].ParentID)
	assert.Nil(t, byCode["7000"].ParentID)
	assert.Equal(t, domain.NormalCredit, byCode["1591"].NormalBalance, "akun kontra")
	assert.True(t, byCode["1111"].AllowManualPosting)
	assert.False(t, byCode["1201"].AllowManualPosting, "akun kontrol hanya jurnal otomatis")
	assert.Equal(t, domain.GroupOther, byCode["7301"].Group)
}

func TestAccountLifecycle(t *testing.T) {
	h := setup(t)
	ctx := h.ctx()
	parent := h.account("1100")
	in := domain.AccountInput{Code: "1190", Name: "Bank — rekening test", NormalBalance: domain.NormalDebit, ParentID: &parent.ID,
		IsPostable: true, AllowManualPosting: true, IsActive: true}
	acc, err := h.svc.CreateAccount(ctx, in)
	require.NoError(t, err)

	_, err = h.svc.CreateAccount(ctx, in)
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode), "kode unik permanen")

	postable := h.account("1111")
	bad := in
	bad.Code, bad.ParentID = "1199", &postable.ID
	_, err = h.svc.CreateAccount(ctx, bad)
	assert.Contains(t, fieldsOf(err), "parentId", "induk harus akun judul")

	other := h.account("2000")
	bad.ParentID = &other.ID
	_, err = h.svc.CreateAccount(ctx, bad)
	assert.Contains(t, fieldsOf(err), "parentId", "induk satu kelompok")

	_, err = h.svc.CreateAccount(as(service.PermAccountRead), in)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))

	upd := in
	upd.Name = "Bank — rekening test (ubah)"
	acc, err = h.svc.UpdateAccount(ctx, acc.ID, acc.Version, upd)
	require.NoError(t, err)
	assert.Equal(t, 2, acc.Version)
	_, err = h.svc.UpdateAccount(ctx, acc.ID, 1, upd)
	assert.True(t, apperr.HasCode(err, apperr.CodeVersionConflict))

	// Akun yang sudah dipakai jurnal tidak bisa dihapus.
	h.post(h.draft(oct19, line(acc, "100", "0", domain.Dimensions{}), line(h.account("3101"), "0", "100", domain.Dimensions{})))
	err = h.svc.DeleteAccount(ctx, acc.ID, acc.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
	upd.NormalBalance = domain.NormalCredit
	_, err = h.svc.UpdateAccount(ctx, acc.ID, acc.Version, upd)
	assert.Contains(t, fieldsOf(err), "normalBalance")

	// Akun induk dengan sub-akun tidak bisa dihapus; akun belum terpakai bisa dihapus.
	assert.True(t, apperr.HasCode(h.svc.DeleteAccount(ctx, parent.ID, parent.Version), apperr.CodeInvalidStateTransition))
	fresh, err := h.svc.CreateAccount(ctx, domain.AccountInput{Code: "6399", Name: "Beban test", NormalBalance: domain.NormalDebit,
		ParentID: ptr(h.account("6000").ID), IsPostable: true, IsActive: true})
	if apperr.HasCode(err, apperr.CodeDuplicateCode) {
		return // dijalankan ulang dengan DB yang sama
	}
	require.NoError(t, err)
	require.NoError(t, h.svc.DeleteAccount(ctx, fresh.ID, fresh.Version))
	_, err = h.svc.GetAccount(ctx, fresh.ID)
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound))
	_, err = h.svc.CreateAccount(ctx, domain.AccountInput{Code: "6399", Name: "Pakai ulang", NormalBalance: domain.NormalDebit, IsPostable: true})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode), "kode akun terhapus tidak dipakai ulang")
}

// --- Dimensi (S-129) --------------------------------------------------------------------------

func TestDimensions(t *testing.T) {
	h := setup(t)
	ctx := h.ctx()
	suffix := strings.ToUpper(uuid.NewString()[:6])
	jan1, jan31 := clock.NewBusinessDate(2026, time.October, 1), clock.NewBusinessDate(2026, time.October, 31)
	c, err := h.svc.CreateDimension(ctx, domain.KindCampaign, domain.DimensionInput{
		Code: "KMP-202610-T" + suffix, Name: "Kampanye test", ValidFrom: jan1, ValidTo: jan31, IsActive: true,
	})
	require.NoError(t, err)
	_, err = h.svc.CreateDimension(ctx, domain.KindCampaign, domain.DimensionInput{
		Code: c.Code, Name: "x", ValidFrom: jan1, ValidTo: jan31,
	})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode))

	sub, err := h.svc.CreateDimension(ctx, domain.KindSubChannel, domain.DimensionInput{
		Code: "mt-" + strings.ToLower(suffix), Name: "Partner MT", ChannelID: ptr(h.dimID(domain.KindChannel, "modern_trade")), IsActive: true,
	})
	require.NoError(t, err)
	sub, err = h.svc.UpdateDimension(ctx, domain.KindSubChannel, sub.ID, sub.Version, domain.DimensionInput{Name: "Partner MT 2", IsActive: false})
	require.NoError(t, err)
	assert.False(t, sub.IsActive)
	active, err := h.svc.ListDimensions(ctx, domain.KindSubChannel, postgres.DimensionFilter{})
	require.NoError(t, err)
	for _, v := range active {
		assert.NotEqual(t, sub.ID, v.ID, "nonaktif tidak tampil di dropdown")
	}

	// Channel dengan sub-channel tidak bisa dihapus.
	mp, err := h.svc.ListDimensions(ctx, domain.KindChannel, postgres.DimensionFilter{})
	require.NoError(t, err)
	for _, v := range mp {
		if v.ID == h.marketplace {
			assert.True(t, apperr.HasCode(h.svc.DeleteDimension(ctx, domain.KindChannel, v.ID, v.Version), apperr.CodeInvalidStateTransition))
		}
	}
	// Kampanye belum dipakai bisa dihapus; setelah dihapus tidak tampil.
	require.NoError(t, h.svc.DeleteDimension(ctx, domain.KindCampaign, c.ID, c.Version))
	list, err := h.svc.ListDimensions(ctx, domain.KindCampaign, postgres.DimensionFilter{IncludeInactive: true, Query: suffix})
	require.NoError(t, err)
	assert.Empty(t, list)

	_, err = h.svc.CreateDimension(as(service.PermDimensionRead), domain.KindDivision, domain.DimensionInput{Code: "OPS", Name: "x"})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}

// --- Posting rule & Poster (S-130) ------------------------------------------------------------

func (h *harness) request(rule string, amounts map[string]string, mutate ...func(*domain.PostingRequest)) domain.PostingRequest {
	r := domain.PostingRequest{
		RuleCode: rule, SourceType: "test_document", SourceID: uuid.New(), SourceLabel: "TEST/2026/10/00001",
		BusinessDate: oct19, Amounts: map[string]money.Amount{},
		Dimensions: domain.Dimensions{DivisionID: ptr(h.division)},
	}
	for k, v := range amounts {
		r.Amounts[k] = money.MustParse(v)
	}
	for _, m := range mutate {
		m(&r)
	}
	return r
}

func TestPoster(t *testing.T) {
	h := setup(t)
	reader := h.ctx()
	ctx := actor.System(context.Background())
	marketplaceDims := func(r *domain.PostingRequest) {
		r.Dimensions = domain.Dimensions{ChannelID: ptr(h.marketplace), SubChannelID: ptr(h.shopee)}
	}
	tests := []struct {
		name    string
		req     domain.PostingRequest
		lines   map[string][2]string // kode akun → {debit, kredit}
		errCode apperr.Code
		errKey  string
	}{
		{
			name:  "A-01 penerimaan barang ke persediaan FG gudang",
			req:   h.request("A-01", map[string]string{"inventory_value": "1500000"}),
			lines: map[string][2]string{"1311": {"1500000.00", "0.00"}, "2102": {"0.00", "1500000.00"}},
		},
		{
			name: "A-01 packaging memilih akun 1301",
			req: h.request("A-01", map[string]string{"inventory_value": "250000"}, func(r *domain.PostingRequest) {
				r.Accounts = map[domain.Resolver]string{domain.ResolverLocationInventory: "1301"}
			}),
			lines: map[string][2]string{"1301": {"250000.00", "0.00"}, "2102": {"0.00", "250000.00"}},
		},
		{
			name: "A-10 marketplace dengan voucher penjual dan HPP",
			req: h.request("A-10", map[string]string{"receivable_value": "90000", "seller_voucher": "10000", "gross_sales": "100000", "cogs_value": "40000"},
				marketplaceDims),
			lines: map[string][2]string{
				"1211": {"90000.00", "0.00"}, "4102": {"10000.00", "0.00"}, "4101": {"0.00", "100000.00"},
				"5101": {"40000.00", "0.00"}, "1311": {"0.00", "40000.00"},
			},
		},
		{
			name: "tidak balance ditolak", req: h.request("A-05", map[string]string{"receivable_value": "90", "gross_sales": "100"}, marketplaceDims),
			errCode: apperr.CodeValidationFailed, errKey: "amounts",
		},
		{
			name: "dimensi wajib kosong ditolak", req: h.request("A-05", map[string]string{"receivable_value": "100", "gross_sales": "100"}),
			errCode: apperr.CodeValidationFailed, errKey: "dimensions.channelId",
		},
		{
			name: "sub-channel bukan milik channel ditolak",
			req: h.request("A-05", map[string]string{"receivable_value": "100", "gross_sales": "100"}, func(r *domain.PostingRequest) {
				r.Dimensions = domain.Dimensions{ChannelID: ptr(h.channel), SubChannelID: ptr(h.lazada)}
			}),
			errCode: apperr.CodeValidationFailed, errKey: "dimensions.subChannelId",
		},
		{
			name: "akun induk ditolak",
			req: h.request("A-01", map[string]string{"inventory_value": "10"}, func(r *domain.PostingRequest) {
				r.Accounts = map[domain.Resolver]string{domain.ResolverLocationInventory: "1300"}
			}),
			errCode: apperr.CodeValidationFailed, errKey: "accounts.inventory_value",
		},
		{
			name:    "rule tidak dikenal ditolak",
			req:     h.request("A-99", map[string]string{"x": "10"}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name:    "rule tanpa jurnal (A-20) ditolak",
			req:     h.request("A-20", map[string]string{}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := h.svc.Post(ctx, tt.req)
			if tt.errCode != "" {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, apperr.As(err).Code)
				assert.Contains(t, fieldsOf(err), tt.errKey, fieldsOf(err))
				return
			}
			require.NoError(t, err)
			assert.True(t, e.IsPosted())
			assert.True(t, strings.HasPrefix(e.Number, "JV/2026/10/"), e.Number)
			assert.Equal(t, domain.EntryAutomatic, e.EntryType)
			stored, err := h.svc.GetJournal(reader, e.ID)
			require.NoError(t, err)
			got := map[string][2]string{}
			for _, l := range stored.Lines {
				got[l.AccountCode] = [2]string{l.Debit.String(), l.Credit.String()}
				assert.Equal(t, tt.req.Dimensions, l.Dimensions, "dimensi dibawa setiap baris")
			}
			assert.Equal(t, tt.lines, got)
			require.NoError(t, stored.CheckBalanced())
		})
	}
}

func TestPosterUniqueAndPeriods(t *testing.T) {
	h := setup(t)
	ctx := actor.System(context.Background())
	req := h.request("A-12", map[string]string{"usage_value": "5000"})
	_, err := h.svc.Post(ctx, req)
	require.NoError(t, err)

	// Satu dokumen sumber hanya sekali per rule (uq_journal_entries_source).
	_, err = h.svc.Post(ctx, req)
	assert.True(t, errors.Is(err, postgres.ErrAlreadyPosted))
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
	// Constraint DB tetap menjaga meski cek service terlewati.
	_, err = h.pool.Exec(context.Background(), `
		INSERT INTO journal_entries (business_date, entry_type, source_type, source_id, posting_rule_code, created_by, updated_by)
		VALUES ('2026-10-19', 'automatic', $1, $2, 'A-12', $3, $3)`, req.SourceType, req.SourceID, uuid.Nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uq_journal_entries_source")

	// Rule sama, dokumen lain → boleh; rule lain, dokumen sama → boleh.
	other := req
	other.SourceID = uuid.New()
	_, err = h.svc.Post(ctx, other)
	require.NoError(t, err)

	// Periode tertutup → PERIOD_LOCKED.
	h.closePeriod(2026, time.August)
	locked := h.request("A-12", map[string]string{"usage_value": "1000"}, func(r *domain.PostingRequest) {
		r.BusinessDate = clock.NewBusinessDate(2026, time.August, 31)
	})
	_, err = h.svc.Post(ctx, locked)
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))
	assert.Contains(t, apperr.As(err).Message, "Agu 2026")
	assert.True(t, apperr.HasCode(h.svc.CheckPeriodOpen(ctx, locked.BusinessDate), apperr.CodePeriodLocked))
	// Bulan tanpa periode sebelum periode tertutup tidak dibuat otomatis.
	locked.BusinessDate = clock.NewBusinessDate(2026, time.March, 2)
	_, err = h.svc.Post(ctx, locked)
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))
	assert.True(t, apperr.HasCode(h.svc.CheckPeriodOpen(ctx, locked.BusinessDate), apperr.CodePeriodLocked))
	assert.NoError(t, h.svc.CheckPeriodOpen(ctx, oct19))

	// Pembatalan dokumen sumber membalik jurnal otomatis.
	r, err := h.svc.Reverse(ctx, service.ReversalRequest{SourceType: req.SourceType, SourceID: req.SourceID, RuleCode: "A-12", BusinessDate: oct19, Reason: "Dokumen dibatalkan"})
	require.NoError(t, err)
	assert.Equal(t, domain.EntryReversal, r.EntryType)
	_, err = h.svc.Reverse(ctx, service.ReversalRequest{SourceType: req.SourceType, SourceID: req.SourceID, RuleCode: "A-12", BusinessDate: oct19, Reason: "lagi"})
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
	_, err = h.svc.Reverse(ctx, service.ReversalRequest{SourceType: "x", SourceID: uuid.New(), RuleCode: "A-12", BusinessDate: oct19, Reason: "x"})
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound))

	rules, views, err := h.svc.PostingRules(h.ctx())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rules), 30)
	assert.NotEmpty(t, views["A-01"])
}

func TestPeriodsEnsure(t *testing.T) {
	h := setup(t)
	_, err := h.svc.CreateUpcomingPeriods(h.ctx())
	require.NoError(t, err)
	n, err := h.svc.EnsureUpcomingPeriods(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n, "idempotent")
	list, err := h.svc.ListPeriods(h.ctx(), 2026)
	require.NoError(t, err)
	labels := map[string]domain.PeriodStatus{}
	for _, p := range list {
		labels[p.Label()] = p.Status
	}
	assert.Equal(t, domain.PeriodOpen, labels["Okt 2026"])
	assert.Equal(t, domain.PeriodOpen, labels["Nov 2026"])
	_, err = h.svc.CreateUpcomingPeriods(as(service.PermFiscalPeriodRead))
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}

// --- Jurnal Umum (S-127, S-128) ---------------------------------------------------------------

func (h *harness) post(e domain.Entry) domain.Entry {
	h.t.Helper()
	posted, err := h.svc.PostManualJournal(h.ctx(), e.ID, e.Version)
	require.NoError(h.t, err)
	return posted
}

func TestManualJournal(t *testing.T) {
	h := setup(t)
	ctx := h.ctx()
	bank, capital, expense := h.account("1111"), h.account("3101"), h.account("6306")
	div := domain.Dimensions{DivisionID: ptr(h.division)}

	e := h.draft(oct19, line(expense, "250000", "0", domain.Dimensions{}), line(bank, "0", "250000", domain.Dimensions{}))
	assert.Equal(t, domain.EntryDraft, e.Status)
	assert.Empty(t, e.Number, "nomor diberikan saat posting")

	// Dimensi wajib (beban → divisi) diperiksa saat posting.
	_, err := h.svc.PostManualJournal(ctx, e.ID, e.Version)
	assert.Contains(t, fieldsOf(err), "lines.0.divisionId")

	e, err = h.svc.UpdateManualJournal(ctx, e.ID, e.Version, domain.ManualInput{
		BusinessDate: oct19, Description: "Biaya umum", Lines: []domain.Line{line(expense, "250000", "0", div), line(bank, "0", "250000", domain.Dimensions{})},
	})
	require.NoError(t, err)
	posted := h.post(e)
	assert.True(t, posted.IsPosted())
	assert.True(t, strings.HasPrefix(posted.Number, "JV/2026/10/"))

	// Nomor berurutan tanpa loncat.
	second := h.post(h.draft(oct19, line(bank, "1000", "0", domain.Dimensions{}), line(capital, "0", "1000", domain.Dimensions{})))
	assert.Equal(t, seq(t, posted.Number)+1, seq(t, second.Number))

	// Tidak balance ditolak saat posting.
	unbalanced := h.draft(oct19, line(bank, "100", "0", domain.Dimensions{}), line(capital, "0", "99", domain.Dimensions{}))
	_, err = h.svc.PostManualJournal(ctx, unbalanced.ID, unbalanced.Version)
	assert.Contains(t, fieldsOf(err), "lines")
	// Nomor tidak terpakai oleh posting yang gagal.
	third := h.post(h.draft(oct19, line(bank, "1", "0", domain.Dimensions{}), line(capital, "0", "1", domain.Dimensions{})))
	assert.Equal(t, seq(t, second.Number)+1, seq(t, third.Number))

	// Akun kontrol dan akun induk tidak bisa dipakai Jurnal Umum.
	_, err = h.svc.CreateManualJournal(ctx, domain.ManualInput{BusinessDate: oct19, Description: "x",
		Lines: []domain.Line{line(h.account("1201"), "1", "0", domain.Dimensions{}), line(h.account("1100"), "0", "1", domain.Dimensions{})}})
	assert.Contains(t, fieldsOf(err), "lines.0.accountId")
	assert.Contains(t, fieldsOf(err), "lines.1.accountId")

	// Dimensi tidak dikenal ditolak.
	_, err = h.svc.CreateManualJournal(ctx, domain.ManualInput{BusinessDate: oct19, Description: "x",
		Lines: []domain.Line{line(expense, "1", "0", domain.Dimensions{DivisionID: ptr(uuid.New())}), line(bank, "0", "1", domain.Dimensions{})}})
	assert.Contains(t, fieldsOf(err), "lines.0.divisionId")

	// Periode tertutup ditolak saat membuat draft.
	h.closePeriod(2026, time.July)
	_, err = h.svc.CreateManualJournal(ctx, domain.ManualInput{BusinessDate: clock.NewBusinessDate(2026, time.July, 10), Description: "x",
		Lines: []domain.Line{line(bank, "1", "0", domain.Dimensions{}), line(capital, "0", "1", domain.Dimensions{})}})
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))

	// Draft bisa dihapus; jurnal terposting tidak bisa diubah atau dihapus (ACC-13).
	require.NoError(t, h.svc.DeleteManualJournal(ctx, unbalanced.ID, unbalanced.Version))
	_, err = h.svc.GetJournal(ctx, unbalanced.ID)
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound))
	_, err = h.svc.UpdateManualJournal(ctx, posted.ID, posted.Version, domain.ManualInput{BusinessDate: oct19, Description: "x",
		Lines: []domain.Line{line(bank, "1", "0", domain.Dimensions{}), line(capital, "0", "1", domain.Dimensions{})}})
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(h.svc.DeleteManualJournal(ctx, posted.ID, posted.Version), apperr.CodeInvalidStateTransition))

	// Tanpa permission posting.
	d := h.draft(oct19, line(bank, "1", "0", domain.Dimensions{}), line(capital, "0", "1", domain.Dimensions{}))
	_, err = h.svc.PostManualJournal(as(service.PermJournalWrite), d.ID, d.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))

	res, err := h.svc.ListJournals(ctx, postgres.EntryFilter{Status: domain.EntryPosted, Query: posted.Number}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, posted.ID, res.Items[0].ID)
}

func seq(t *testing.T, number string) int {
	t.Helper()
	parts := strings.Split(number, "/")
	require.Len(t, parts, 4)
	n := 0
	for _, c := range parts[3] {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestPostedJournalImmutableInDB(t *testing.T) {
	h := setup(t)
	bank, capital := h.account("1111"), h.account("3101")
	posted := h.post(h.draft(oct19, line(bank, "500", "0", domain.Dimensions{}), line(capital, "0", "500", domain.Dimensions{})))
	ctx := context.Background()
	for _, q := range []string{
		`UPDATE journal_entries SET description = 'ubah' WHERE id = $1`,
		`DELETE FROM journal_entries WHERE id = $1`,
		`UPDATE journal_lines SET memo = 'ubah' WHERE journal_entry_id = $1`,
		`DELETE FROM journal_lines WHERE journal_entry_id = $1`,
	} {
		_, err := h.pool.Exec(ctx, q, posted.ID)
		require.Error(t, err, q)
	}
	_, err := h.pool.Exec(ctx, `
		INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit_amount, created_by)
		VALUES ($1, 99, $2, 1, $3)`, posted.ID, bank.ID, uuid.Nil)
	require.Error(t, err, "baris tidak bisa ditambah ke jurnal terposting")
	_, err = h.pool.Exec(ctx, `
		INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit_amount, credit_amount, created_by)
		VALUES ($1, 98, $2, 1, 1, $3)`, uuid.New(), bank.ID, uuid.Nil)
	require.Error(t, err, "debit XOR kredit")

	// Draft tidak balance tidak bisa dipaksa diposting lewat SQL.
	d := h.draft(oct19, line(bank, "10", "0", domain.Dimensions{}), line(capital, "0", "9", domain.Dimensions{}))
	_, err = h.pool.Exec(ctx, `UPDATE journal_entries SET status = 'posted', number = 'X-1', fiscal_period_id = $2, posted_at = now(), posted_by = $3 WHERE id = $1`,
		d.ID, *posted.FiscalPeriodID, uuid.Nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not balanced")
}

func TestReverseJournal(t *testing.T) {
	h := setup(t)
	ctx := h.ctx()
	bank, revenue := h.account("1111"), h.account("7101")
	div := domain.Dimensions{DivisionID: ptr(h.division)}
	original := h.post(h.draft(oct19, line(bank, "750000", "0", domain.Dimensions{}), line(revenue, "0", "750000", div)))

	_, err := h.svc.ReverseManualJournal(ctx, original.ID, original.Version, oct19, "")
	assert.Contains(t, fieldsOf(err), "reason")

	h.clock.Advance(24 * time.Hour)
	r, err := h.svc.ReverseManualJournal(ctx, original.ID, original.Version, oct19.AddDays(1), "Salah akun pendapatan")
	require.NoError(t, err)
	assert.True(t, r.IsPosted())
	assert.Equal(t, domain.EntryReversal, r.EntryType)
	assert.Equal(t, original.ID, *r.ReversalOfID)
	assert.Equal(t, original.Number, r.ReversalOfNumber)
	assert.Contains(t, r.Description, original.Number)
	assert.Equal(t, "750000.00", r.Lines[0].Credit.String())
	assert.Equal(t, "750000.00", r.Lines[1].Debit.String())
	assert.Equal(t, div, r.Lines[1].Dimensions)

	reloaded, err := h.svc.GetJournal(ctx, original.ID)
	require.NoError(t, err)
	assert.Equal(t, r.ID, *reloaded.ReversedByID)
	assert.Equal(t, original.Description, reloaded.Description, "jurnal asal tidak berubah")

	_, err = h.svc.ReverseManualJournal(ctx, original.ID, reloaded.Version, oct19.AddDays(1), "lagi")
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "hanya sekali")
	_, err = h.svc.ReverseManualJournal(ctx, r.ID, r.Version, oct19.AddDays(1), "balik pembalik")
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))

	auto, err := h.svc.Post(actor.System(context.Background()), h.request("A-12", map[string]string{"usage_value": "10"}))
	require.NoError(t, err)
	_, err = h.svc.ReverseManualJournal(ctx, auto.ID, auto.Version, oct19, "x")
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "jurnal otomatis dibalik lewat dokumen sumber")
}

func TestGeneralLedger(t *testing.T) {
	h := setup(t)
	ctx := h.ctx()
	parent := h.account("1100")
	acc, err := h.svc.CreateAccount(ctx, domain.AccountInput{Code: "1150", Name: "Bank — Buku Besar test", NormalBalance: domain.NormalDebit,
		ParentID: &parent.ID, IsPostable: true, AllowManualPosting: true, IsActive: true})
	if apperr.HasCode(err, apperr.CodeDuplicateCode) {
		t.Skip("DB dipakai ulang")
	}
	require.NoError(t, err)
	capital := h.account("3101")
	sep1, oct5, oct20 := clock.NewBusinessDate(2026, time.September, 1), clock.NewBusinessDate(2026, time.October, 5), clock.NewBusinessDate(2026, time.October, 20)
	h.post(h.draft(sep1, line(acc, "1000", "0", domain.Dimensions{}), line(capital, "0", "1000", domain.Dimensions{})))
	h.post(h.draft(oct5, line(acc, "500", "0", domain.Dimensions{}), line(capital, "0", "500", domain.Dimensions{})))
	h.post(h.draft(oct20, line(capital, "200", "0", domain.Dimensions{}), line(acc, "0", "200", domain.Dimensions{})))
	h.draft(oct20, line(acc, "999", "0", domain.Dimensions{}), line(capital, "0", "999", domain.Dimensions{})) // draft tidak masuk Buku Besar

	l, err := h.svc.GeneralLedger(ctx, postgres.LedgerFilter{AccountID: acc.ID, DateFrom: clock.NewBusinessDate(2026, time.October, 1), DateTo: clock.NewBusinessDate(2026, time.October, 31)}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Equal(t, "1000.00", l.OpeningBalance.String())
	assert.Equal(t, "500.00", l.PeriodDebit.String())
	assert.Equal(t, "200.00", l.PeriodCredit.String())
	assert.Equal(t, "1300.00", l.ClosingBalance.String())
	require.Len(t, l.Lines, 2)
	assert.Equal(t, "1500.00", l.Lines[0].Balance.String())
	assert.Equal(t, "1300.00", l.Lines[1].Balance.String())
	assert.Equal(t, int64(2), l.Total)

	// Akun bersaldo kredit: saldo berjalan kredit − debit.
	cl, err := h.svc.GeneralLedger(ctx, postgres.LedgerFilter{AccountID: capital.ID, DateFrom: oct20, DateTo: oct20}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Equal(t, cl.OpeningBalance.Sub(money.FromInt(200)).String(), cl.ClosingBalance.String())

	_, err = h.svc.GeneralLedger(ctx, postgres.LedgerFilter{AccountID: acc.ID, DateFrom: oct20, DateTo: oct5}, paging.New(nil, nil))
	assert.Contains(t, fieldsOf(err), "dateTo")
	_, err = h.svc.GeneralLedger(ctx, postgres.LedgerFilter{AccountID: uuid.New(), DateFrom: oct5, DateTo: oct20}, paging.New(nil, nil))
	assert.Contains(t, fieldsOf(err), "accountId")
	_, err = h.svc.GeneralLedger(as(), postgres.LedgerFilter{AccountID: acc.ID, DateFrom: oct5, DateTo: oct20}, paging.New(nil, nil))
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
}
