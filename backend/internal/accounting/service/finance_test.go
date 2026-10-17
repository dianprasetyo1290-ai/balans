package service_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

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
	"balans/internal/platform/pii"
	"balans/internal/tax"
)

// --- Fake dependensi modul lain -----------------------------------------------------------------

type docNumbers struct{ docs *document.Module }

func (n docNumbers) NextDocNumber(ctx context.Context, t string, d clock.BusinessDate) (string, error) {
	return n.docs.Next(ctx, document.Type(t), d)
}

type fakeFiles struct {
	mu       sync.Mutex
	contents map[uuid.UUID]service.FileContent
	attached map[uuid.UUID]uuid.UUID
	exports  [][]byte
}

func newFakeFiles() *fakeFiles {
	return &fakeFiles{contents: map[uuid.UUID]service.FileContent{}, attached: map[uuid.UUID]uuid.UUID{}}
}

func (f *fakeFiles) put(name, ct string, data []byte) uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.New()
	f.contents[id] = service.FileContent{Name: name, ContentType: ct, Data: data}
	return id
}

func (f *fakeFiles) Attach(_ context.Context, fileID uuid.UUID, _ string, ownerID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attached[fileID] = ownerID
	return nil
}

func (f *fakeFiles) Names(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		out[id] = "bukti-" + id.String()[:4] + ".pdf"
	}
	return out, nil
}

func (f *fakeFiles) Read(_ context.Context, id uuid.UUID, _ string) (service.FileContent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.contents[id]
	if !ok {
		return service.FileContent{}, apperr.Validation(map[string]string{"fileId": "File tidak ditemukan"})
	}
	return c, nil
}

func (f *fakeFiles) SaveExport(_ context.Context, _ string, _ uuid.UUID, name, _ string, data []byte) (service.DownloadLink, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exports = append(f.exports, data)
	return service.DownloadLink{FileID: uuid.New(), URL: "/files/" + name, ExpiresAt: time.Now().Add(5 * time.Minute)}, nil
}

type fakeApprovals struct {
	mu       sync.Mutex
	subjects []service.ApprovalSubject
}

func (f *fakeApprovals) RequestApproval(_ context.Context, s service.ApprovalSubject) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subjects = append(f.subjects, s)
	return nil
}

func (f *fakeApprovals) last() service.ApprovalSubject {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subjects[len(f.subjects)-1]
}

type fakeNotifier struct{ count atomic.Int32 }

func (f *fakeNotifier) Notify(context.Context, []uuid.UUID, string, string, string, string) error {
	f.count.Add(1)
	return nil
}

type fakeUsers struct{}

func (fakeUsers) UserNames(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		out[id] = "User " + id.String()[:4]
	}
	return out, nil
}

// --- Harness ------------------------------------------------------------------------------------

type finHarness struct {
	*harness
	fin       *service.Service
	files     *fakeFiles
	approvals *fakeApprovals
	notifier  *fakeNotifier
	financeID uuid.UUID
}

var finPerms = []string{
	service.PermPaymentRead, service.PermPaymentWrite, service.PermPaymentPost, service.PermBankReconRead,
	service.PermBankReconWrite, service.PermBankReconPost, service.PermBankReconImport, service.PermExpenseClaimRead,
	service.PermExpenseClaimPost, service.PermPettyCashRead, service.PermPettyCashWrite, service.PermPettyCashPost,
	service.PermWithholdingRead, service.PermWithholdingWrite, service.PermWithholdingPost, service.PermWithholdingExport,
	service.PermFinalTaxRead, service.PermFinalTaxWrite, service.PermFinalTaxPost,
}

var glSeq atomic.Int32

func setupFinance(t *testing.T) *finHarness {
	t.Helper()
	base := setup(t)
	pool := dbtest.Pool(t)
	key := bytes.Repeat([]byte{7}, 32)
	cipher, err := pii.New("k1", map[string][]byte{"k1": key}, bytes.Repeat([]byte{9}, 32))
	require.NoError(t, err)
	h := &finHarness{harness: base, files: newFakeFiles(), approvals: &fakeApprovals{}, notifier: &fakeNotifier{}, financeID: uuid.New()}
	h.fin = service.New(service.Deps{
		Store: postgres.NewStore(pool), Tx: db.NewTxManager(pool), Audit: audit.New(audit.Deps{Pool: pool}), Clock: base.clock,
		Log: base.svc.Log, Numberer: numberer{docs: document.New(document.Deps{Pool: pool, Clock: base.clock})},
		FinanceDeps: service.FinanceDeps{
			Finance: postgres.NewFinanceStore(pool, cipher), Files: h.files, Approvals: h.approvals, Users: fakeUsers{},
			Notifier: h.notifier, DocNumbers: docNumbers{docs: document.New(document.Deps{Pool: pool, Clock: base.clock})},
		},
	})
	return h
}

// finance mengembalikan konteks Finance & Accounting dengan re-autentikasi baru (★).
func (h *finHarness) finance() context.Context {
	set := map[string]struct{}{}
	for _, p := range append(append([]string{}, allPerms...), finPerms...) {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{
		UserID: h.financeID, FullName: "Finance", Permissions: set, ReauthAt: h.clock.Now(),
	})
}

func (h *finHarness) noReauth() context.Context {
	a, _ := actor.From(h.finance())
	a.ReauthAt = time.Time{}
	return actor.With(context.Background(), a)
}

func staff(id uuid.UUID, perms ...string) context.Context {
	set := map[string]struct{}{}
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: id, FullName: "Staf Marketing", Permissions: set})
}

// newBank membuat akun GL Kas & Bank unik dan rekening bank di atasnya.
func (h *finHarness) newBank(t *testing.T) domain.BankAccount {
	t.Helper()
	n := glSeq.Add(1)
	parent := h.account("1100")
	gl, err := h.svc.CreateAccount(h.finance(), domain.AccountInput{
		Code: fmt.Sprintf("11%02d", 40+n), Name: fmt.Sprintf("Bank test %d", n), NormalBalance: domain.NormalDebit,
		ParentID: &parent.ID, IsPostable: true, AllowManualPosting: true, IsActive: true,
	})
	require.NoError(t, err)
	b, err := h.fin.CreateBankAccount(h.finance(), domain.BankAccountInput{
		Code: fmt.Sprintf("BANK-%d", n), Name: fmt.Sprintf("Bank Test %d", n), BankName: "BCA",
		AccountNumber: fmt.Sprintf("88%08d", n), AccountHolder: "PT Balans", AccountID: gl.ID, IsActive: true,
	})
	require.NoError(t, err)
	return b
}

func (h *finHarness) expenseType(t *testing.T, code string) domain.ExpenseType {
	t.Helper()
	et, err := h.svc.ExpenseTypeByCode(context.Background(), code)
	require.NoError(t, err)
	return et
}

func (h *finHarness) journalLines(t *testing.T, entryID uuid.UUID) map[string]string {
	t.Helper()
	e, err := h.svc.GetJournal(h.finance(), entryID)
	require.NoError(t, err)
	out := map[string]string{}
	accounts, err := h.svc.ListAccounts(h.finance(), true)
	require.NoError(t, err)
	codes := map[uuid.UUID]string{}
	for _, a := range accounts {
		codes[a.ID] = a.Code
	}
	for _, l := range e.Lines {
		side := "Dr"
		amt := l.Debit
		if l.Credit.IsPositive() {
			side, amt = "Cr", l.Credit
		}
		out[side+" "+codes[l.AccountID]] = money.Sum(money.MustParse(orZero(out[side+" "+codes[l.AccountID]])), amt).String()
	}
	return out
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// --- S-401 Kas & Bank ---------------------------------------------------------------------------

func TestBankAccountsAndPayments(t *testing.T) {
	h := setupFinance(t)
	ctx := h.finance()
	b := h.newBank(t)
	assert.Equal(t, "BCA", b.BankName)
	assert.Contains(t, b.MaskedNumber(), "••••")

	var enc string
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT account_number_enc FROM bank_accounts WHERE id = $1`, b.ID).Scan(&enc))
	assert.NotContains(t, enc, b.AccountNumber, "nomor rekening terenkripsi")

	_, err := h.fin.CreateBankAccount(ctx, domain.BankAccountInput{Code: "DUP", Name: "Dup", BankName: "BCA", AccountNumber: "123456", AccountID: b.AccountID, IsActive: true})
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode), "satu akun GL satu rekening")
	_, err = h.fin.CreateBankAccount(ctx, domain.BankAccountInput{Code: "KK", Name: "KK", Kind: domain.BankKindCash, AccountID: h.account("1102").ID, IsActive: true})
	assert.Contains(t, fieldsOf(err), "accountId", "kas kecil bukan rekening")

	fee := h.account("7202")
	p, err := h.fin.CreatePayment(ctx, domain.PaymentInput{
		Direction: domain.PaymentOutgoing, BankAccountID: b.ID, PaymentDate: oct19, Amount: money.FromInt(25_000),
		CounterAccountID: fee.ID, Description: "Biaya admin",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"edit", "post", "cancel"}, service.PaymentActions(ctx, p))

	_, err = h.fin.CreatePayment(ctx, domain.PaymentInput{
		Direction: domain.PaymentIncoming, BankAccountID: b.ID, PaymentDate: oct19, Amount: money.FromInt(1),
		CounterAccountID: h.account("1201").ID,
	})
	assert.Contains(t, fieldsOf(err), "counterAccountId", "akun kontrol piutang ditolak")

	_, err = h.fin.PostPayment(h.noReauth(), p.ID, p.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthReauthRequired), "pembayaran keluar butuh re-autentikasi")

	p, err = h.fin.PostPayment(ctx, p.ID, p.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPosted, p.Status)
	assert.True(t, strings.HasPrefix(p.Number, "BYR/2026/10/"))
	assert.Equal(t, map[string]string{"Dr 7202": "25000.00", "Cr " + b.AccountCode: "25000.00"}, h.journalLines(t, *p.JournalEntryID))

	in, err := h.fin.CreatePayment(ctx, domain.PaymentInput{
		Direction: domain.PaymentIncoming, BankAccountID: b.ID, PaymentDate: oct19, Amount: money.FromInt(1_000_000),
		CounterAccountID: h.account("7101").ID,
	})
	require.NoError(t, err)
	in, err = h.fin.PostPayment(h.noReauth(), in.ID, in.Version)
	require.NoError(t, err, "pembayaran masuk tanpa re-autentikasi")

	mut, err := h.fin.BankMutations(ctx, b.ID, oct19.AddDays(-1), oct19, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, mut.Items, 2)
	assert.Equal(t, "0.00", mut.Opening.String())
	assert.Equal(t, "975000.00", mut.Items[1].Balance.String(), "saldo berjalan")

	p, err = h.fin.CancelPayment(ctx, p.ID, p.Version, "salah catat")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, p.Status)
	mut, err = h.fin.BankMutations(ctx, b.ID, oct19, oct19, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Len(t, mut.Items, 3, "jurnal pembalik ikut di mutasi")
	assert.Equal(t, "1000000.00", mut.Items[2].Balance.String())

	_, err = h.fin.ListPayments(staff(uuid.New(), service.PermExpenseClaimRead), service.PaymentFilter{}, paging.New(nil, nil))
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))
	list, err := h.fin.ListPayments(ctx, service.PaymentFilter{BankAccountID: &b.ID}, paging.New(nil, nil))
	require.NoError(t, err)
	assert.EqualValues(t, 2, list.Total)

	require.NoError(t, h.fin.DeleteBankAccount(ctx, b.ID, b.Version))
	_, err = h.fin.CreatePayment(ctx, domain.PaymentInput{
		Direction: domain.PaymentIncoming, BankAccountID: b.ID, PaymentDate: oct19, Amount: money.FromInt(1), CounterAccountID: fee.ID,
	})
	assert.Contains(t, fieldsOf(err), "bankAccountId", "rekening dihapus tidak bisa dipakai")
}

// --- S-404 Pengajuan biaya + S-406 PPh potong ---------------------------------------------------

func (h *finHarness) approvedClaim(t *testing.T, requester uuid.UUID, lines []domain.ClaimLineInput, payee string) service.ClaimView {
	t.Helper()
	req := staff(requester, service.PermExpenseClaimRead, service.PermExpenseClaimWrite, service.PermExpenseClaimSubmit)
	n := glSeq.Add(1)
	c, err := h.svc.CreateDimension(h.finance(), domain.KindCampaign, domain.DimensionInput{
		Code: fmt.Sprintf("KMP-202610-T%d", n), Name: fmt.Sprintf("Kampanye Test %d", n), IsActive: true,
		ValidFrom: oct19.AddDays(-30), ValidTo: oct19.AddDays(60),
	})
	require.NoError(t, err)
	campaign := &c.ID
	v, err := h.fin.CreateExpenseClaim(req, domain.ClaimInput{
		DivisionID: h.division, CampaignID: campaign, PayeeName: payee, ClaimDate: oct19, Description: "Booth event", Lines: lines,
	})
	require.NoError(t, err)
	v, err = h.fin.SubmitExpenseClaim(req, v.Claim.ID, v.Claim.Version)
	require.NoError(t, err)
	require.NoError(t, h.fin.OnExpenseClaimDecided(actor.System(context.Background()), v.Claim.ID, true, "ok"))
	v, err = h.fin.GetExpenseClaim(h.finance(), v.Claim.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusApproved, v.Claim.Status)
	return v
}

func TestExpenseClaimFlow(t *testing.T) {
	h := setupFinance(t)
	b := h.newBank(t)
	event, office := h.expenseType(t, "EVENT"), h.expenseType(t, "OFFICE_SUPPLY")
	requester := uuid.New()
	req := staff(requester, service.PermExpenseClaimRead, service.PermExpenseClaimWrite, service.PermExpenseClaimSubmit)

	opts, err := h.fin.ExpenseClaimOptions(req)
	require.NoError(t, err)
	assert.NotEmpty(t, opts.Divisions, "semua divisi bisa mengajukan tanpa akses master dimensi")
	assert.NotEmpty(t, opts.ExpenseTypes)

	// Bukti wajib untuk jenis biaya yang mewajibkan bukti (seed: semua jenis biaya).
	draft, err := h.fin.CreateExpenseClaim(req, domain.ClaimInput{
		DivisionID: h.division, PayeeType: domain.PayeeEmployee, ClaimDate: oct19,
		Lines: []domain.ClaimLineInput{{ExpenseTypeID: office.ID, Amount: money.FromInt(100_000)}},
	})
	require.NoError(t, err)
	assert.Equal(t, requester, *draft.Claim.PayeeID, "reimbursement dibayar ke pengaju")
	assert.Equal(t, "Staf Marketing", draft.Claim.PayeeName)
	_, err = h.fin.SubmitExpenseClaim(req, draft.Claim.ID, draft.Claim.Version)
	assert.Contains(t, fieldsOf(err), "lines.0.receiptFileId")
	_, err = h.fin.UpdateExpenseClaim(staff(uuid.New(), service.PermExpenseClaimWrite), draft.Claim.ID, draft.Claim.Version, domain.ClaimInput{})
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden), "hanya pengaju yang mengubah")

	// Pengaju lain tidak melihat pengajuan ini; F&A melihat semua.
	other, err := h.fin.ListExpenseClaims(staff(uuid.New(), service.PermExpenseClaimRead), service.ClaimFilter{}, paging.New(nil, nil))
	require.NoError(t, err)
	for _, c := range other.Items {
		assert.NotEqual(t, draft.Claim.ID, c.ID)
	}
	_, err = h.fin.GetExpenseClaim(staff(uuid.New(), service.PermExpenseClaimRead), draft.Claim.ID)
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound))

	r1, r2 := uuid.New(), uuid.New()
	v := h.approvedClaim(t, requester, []domain.ClaimLineInput{
		{ExpenseTypeID: event.ID, Amount: money.FromInt(10_000_000), ReceiptFileID: &r1, Description: "Sewa booth"},
		{ExpenseTypeID: office.ID, Amount: money.FromInt(500_000), ReceiptFileID: &r2, Description: "ATK"},
	}, "PT Event Organizer")
	subj := h.approvals.last()
	assert.Equal(t, domain.ApprovalSubjectExpenseClaim, subj.Type)
	assert.Equal(t, h.division, *subj.Division, "approval per divisi pengaju")
	assert.Equal(t, "10500000.00", subj.Amount.String())
	assert.True(t, strings.HasPrefix(v.Claim.Number, "EXP/2026/10/"))
	assert.Equal(t, []string{"verify"}, service.ClaimActions(h.finance(), v.Claim))

	taxID := "01.234.567.8-901.000"
	in := service.VerifyInput{PayeeTaxID: &taxID}
	preview, total, err := h.fin.PreviewExpenseClaimVerification(h.finance(), v.Claim.ID, in)
	require.NoError(t, err)
	require.Len(t, preview, 1)
	assert.Equal(t, "200000.00", total.String(), "PPh 23 2% × 10 jt")

	_, err = h.fin.VerifyExpenseClaim(req, v.Claim.ID, v.Claim.Version, in)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden), "verifikasi hanya F&A")
	v, err = h.fin.VerifyExpenseClaim(h.finance(), v.Claim.ID, v.Claim.Version, in)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAwaitingPayment, v.Claim.Status)
	assert.Equal(t, "200000.00", v.Claim.Withholding.String())
	assert.Equal(t, "10300000.00", v.Claim.Payable.String())
	require.Len(t, v.Withholdings, 1)
	assert.Equal(t, tax.PPh23, v.Withholdings[0].TaxType)
	assert.Equal(t, "012345678901000", v.Withholdings[0].TaxpayerTaxID)
	assert.Equal(t, map[string]string{
		"Dr 6102": "10000000.00", "Dr 6303": "500000.00", "Cr 2103": "10300000.00", "Cr 2302": "200000.00",
	}, h.journalLines(t, *v.Claim.JournalEntryID), "jurnal A-15")

	_, err = h.fin.PayExpenseClaim(h.noReauth(), v.Claim.ID, v.Claim.Version, service.PayInput{BankAccountID: b.ID, PaymentDate: oct19})
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthReauthRequired))
	v, err = h.fin.PayExpenseClaim(h.finance(), v.Claim.ID, v.Claim.Version, service.PayInput{BankAccountID: b.ID, PaymentDate: oct19, Reference: "TRF-9"})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPaid, v.Claim.Status)
	pay, err := h.fin.GetPayment(h.finance(), *v.Claim.PaymentID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Dr 2103": "10300000.00", "Cr " + b.AccountCode: "10300000.00"}, h.journalLines(t, *pay.JournalEntryID))
	assert.Empty(t, service.PaymentActions(h.finance(), pay), "pembayaran dokumen tidak dibatalkan dari Kas & Bank")
	assert.GreaterOrEqual(t, h.notifier.count.Load(), int32(2), "notifikasi disetujui dan dibayar")

	// Penolakan → revisi → batal.
	d2, err := h.fin.CreateExpenseClaim(req, domain.ClaimInput{
		DivisionID: h.division, PayeeName: "Toko", ClaimDate: oct19,
		Lines: []domain.ClaimLineInput{{ExpenseTypeID: office.ID, Amount: money.FromInt(50_000), ReceiptFileID: &r1}},
	})
	require.NoError(t, err)
	d2, err = h.fin.SubmitExpenseClaim(req, d2.Claim.ID, d2.Claim.Version)
	require.NoError(t, err)
	require.NoError(t, h.fin.OnExpenseClaimDecided(actor.System(context.Background()), d2.Claim.ID, false, "bukti buram"))
	d2, err = h.fin.ReviseExpenseClaim(req, d2.Claim.ID, d2.Claim.Version+1)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, d2.Claim.Status)
	d2, err = h.fin.CancelExpenseClaim(req, d2.Claim.ID, d2.Claim.Version, "tidak jadi")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, d2.Claim.Status)
}

// --- S-402 Rekonsiliasi bank ------------------------------------------------------------------

func TestBankReconciliation(t *testing.T) {
	h := setupFinance(t)
	ctx := h.finance()
	b := h.newBank(t)

	// Transaksi yang sudah dijurnal: penerimaan lain-lain dengan referensi transfer.
	income, err := h.fin.CreatePayment(ctx, domain.PaymentInput{
		Direction: domain.PaymentIncoming, BankAccountID: b.ID, PaymentDate: oct19.AddDays(-3), Amount: money.FromInt(2_500_000),
		CounterAccountID: h.account("7101").ID, Reference: "FT2610150001",
	})
	require.NoError(t, err)
	income, err = h.fin.PostPayment(ctx, income.ID, income.Version)
	require.NoError(t, err)
	refund, err := h.fin.CreatePayment(ctx, domain.PaymentInput{
		Direction: domain.PaymentOutgoing, BankAccountID: b.ID, PaymentDate: oct19.AddDays(-2), Amount: money.FromInt(300_000),
		CounterAccountID: h.account("6306").ID,
	})
	require.NoError(t, err)
	refund, err = h.fin.PostPayment(ctx, refund.ID, refund.Version)
	require.NoError(t, err)

	// Pengajuan biaya Menunggu Pembayaran → kandidat dokumen dari penyedia bawaan.
	event := h.expenseType(t, "OFFICE_SUPPLY")
	receipt := uuid.New()
	claim := h.approvedClaim(t, uuid.New(), []domain.ClaimLineInput{{ExpenseTypeID: event.ID, Amount: money.FromInt(750_000), ReceiptFileID: &receipt}}, "Toko ATK")
	claim, err = h.fin.VerifyExpenseClaim(ctx, claim.Claim.ID, claim.Claim.Version, service.VerifyInput{})
	require.NoError(t, err)

	csv := strings.Join([]string{
		"Tanggal;Keterangan;Referensi;Debit;Kredit;Saldo",
		"16/10/2026;TRSF MASUK PT ABC;FT2610150001;;2.500.000,00;2.500.000,00",
		"17/10/2026;TRF KELUAR;;300.000,00;;2.200.000,00",
		fmt.Sprintf("19/10/2026;PEMBAYARAN %s;;750.000,00;;1.450.000,00", claim.Claim.Number),
		"19/10/2026;BIAYA ADM;;6.500,00;;1.443.500,00",
	}, "\n")
	fileID := h.files.put("mutasi-okt.csv", service.ContentTypeCSV, []byte(csv))
	_, err = h.fin.ImportStatement(staff(uuid.New(), service.PermBankReconRead), b.ID, fileID)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden))

	d, err := h.fin.ImportStatement(ctx, b.ID, fileID)
	require.NoError(t, err)
	require.Len(t, d.Lines, 4)
	assert.Equal(t, "2026-10-16", d.Summary.PeriodFrom.String())
	assert.Equal(t, "2500000.00", d.Summary.TotalIn.String())
	assert.Equal(t, d.Summary.ID, h.files.attached[fileID], "file ditautkan ke file mutasi")
	lines := d.Lines
	assert.Equal(t, domain.LineSuggested, lines[0].Status)
	assert.Equal(t, domain.RuleReference, lines[0].MatchRule)
	assert.Equal(t, domain.LineSuggested, lines[1].Status)
	assert.Equal(t, domain.RuleAmountDate, lines[1].MatchRule)
	assert.Equal(t, domain.MatchDocument, lines[2].MatchKind)
	assert.Equal(t, domain.RuleReference, lines[2].MatchRule)
	assert.Equal(t, domain.LineUnmatched, lines[3].Status)

	_, err = h.fin.ImportStatement(ctx, b.ID, fileID)
	assert.True(t, apperr.HasCode(err, apperr.CodeDuplicateCode), "file sama tidak diimpor dua kali")

	cands, err := h.fin.LineCandidates(ctx, lines[3].ID)
	require.NoError(t, err)
	for _, c := range cands {
		assert.True(t, c.Amount.IsNegative(), "kandidat searah (keluar)")
	}

	n, err := h.fin.ConfirmSuggestions(ctx, d.Summary.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	claimAfter, err := h.fin.GetExpenseClaim(ctx, claim.Claim.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPaid, claimAfter.Claim.Status, "konfirmasi dokumen membayar pengajuan biaya")

	fee, err := h.fin.CreateEntryFromLine(ctx, lines[3].ID, lines[3].Version, service.NewEntryInput{CounterAccountID: h.account("7202").ID})
	require.NoError(t, err)
	assert.Equal(t, domain.LineMatched, fee.Status)
	assert.Equal(t, domain.MatchNewEntry, fee.MatchKind)
	assert.Equal(t, map[string]string{"Dr 7202": "6500.00", "Cr " + b.AccountCode: "6500.00"}, h.journalLines(t, *fee.JournalEntryID))

	d, err = h.fin.GetStatement(ctx, d.Summary.ID)
	require.NoError(t, err)
	assert.Equal(t, 4, d.Summary.MatchedCount)

	mut, err := h.fin.BankMutations(ctx, b.ID, oct19.AddDays(-5), oct19, paging.New(nil, nil))
	require.NoError(t, err)
	for _, m := range mut.Items {
		assert.True(t, m.Reconciled, "semua mutasi rekening sudah direkonsiliasi")
	}
	income, err = h.fin.GetPayment(ctx, income.ID)
	require.NoError(t, err)
	assert.True(t, income.Reconciled)
	_, err = h.fin.CancelPayment(ctx, income.ID, income.Version, "x")
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "pembayaran direkonsiliasi tidak bisa dibatalkan")

	// Lepas dan cocokkan manual ke baris jurnal yang sama.
	l0, err := h.fin.UnmatchLine(ctx, lines[0].ID, d.Lines[0].Version)
	require.NoError(t, err)
	assert.Equal(t, domain.LineUnmatched, l0.Status)
	_, err = h.fin.MatchLine(ctx, l0.ID, l0.Version, service.MatchTarget{JournalLineID: lines[1].JournalLineID})
	assert.Error(t, err, "nominal berbeda ditolak")
	jl := d.Lines[0].JournalLineID
	l0, err = h.fin.MatchLine(ctx, l0.ID, l0.Version, service.MatchTarget{JournalLineID: jl})
	require.NoError(t, err)
	assert.Equal(t, domain.RuleManual, l0.MatchRule)

	// Excel: kolom Nominal + D/K.
	book := excelize.NewFile()
	require.NoError(t, book.SetSheetRow("Sheet1", "A1", &[]any{"Tanggal", "Keterangan", "Nominal", "D/K"}))
	require.NoError(t, book.SetSheetRow("Sheet1", "A2", &[]any{"20/10/2026", "SETOR TUNAI", 125000, "K"}))
	var buf bytes.Buffer
	require.NoError(t, book.Write(&buf))
	xlsxID := h.files.put("mutasi.xlsx", service.ContentTypeXLSX, buf.Bytes())
	d2, err := h.fin.ImportStatement(ctx, b.ID, xlsxID)
	require.NoError(t, err)
	require.Len(t, d2.Lines, 1)
	assert.Equal(t, "125000.00", d2.Lines[0].Amount.String())
}

// --- S-405 Kas kecil ----------------------------------------------------------------------------

func TestPettyCash(t *testing.T) {
	h := setupFinance(t)
	ctx := h.finance()
	b := h.newBank(t)
	fund, err := h.fin.CreatePettyCashFund(ctx, domain.FundInput{
		Code: fmt.Sprintf("KK-%d", glSeq.Add(1)), Name: "Kas Kecil HO", HolderUserID: uuid.New(), DivisionID: h.division, IsActive: true,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.PettyCashDefaultCeiling.String(), fund.Ceiling.String())

	rep, err := h.fin.CreatePettyCash(ctx, domain.PettyCashInput{Type: domain.PettyCashReplenishment, FundID: fund.ID, BusinessDate: oct19})
	require.NoError(t, err)
	assert.Equal(t, "5000000.00", rep.Amount.String(), "nominal pengisian default = plafon − saldo")
	_, err = h.fin.PostPettyCash(ctx, rep.ID, rep.Version, service.PostPettyCashInput{BankAccountID: &b.ID})
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "pengisian harus disetujui dulu")
	rep, err = h.fin.SubmitPettyCash(ctx, rep.ID, rep.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.ApprovalSubjectPettyCashReplenishment, h.approvals.last().Type)
	require.NoError(t, h.fin.OnPettyCashDecided(actor.System(context.Background()), rep.ID, true, ""))
	rep, err = h.fin.GetPettyCash(ctx, rep.ID)
	require.NoError(t, err)
	_, err = h.fin.PostPettyCash(h.noReauth(), rep.ID, rep.Version, service.PostPettyCashInput{BankAccountID: &b.ID})
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthReauthRequired))
	rep, err = h.fin.PostPettyCash(ctx, rep.ID, rep.Version, service.PostPettyCashInput{BankAccountID: &b.ID})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(rep.Number, "KK/2026/10/"))
	assert.Equal(t, map[string]string{"Dr 1102": "5000000.00", "Cr " + b.AccountCode: "5000000.00"}, h.journalLines(t, *rep.JournalEntryID))

	office := h.expenseType(t, "OFFICE_SUPPLY")
	big, err := h.fin.CreatePettyCash(ctx, domain.PettyCashInput{
		Type: domain.PettyCashExpense, FundID: fund.ID, BusinessDate: oct19, ExpenseTypeID: &office.ID, Amount: money.FromInt(1_500_000),
	})
	require.NoError(t, err)
	_, err = h.fin.PostPettyCash(ctx, big.ID, big.Version, service.PostPettyCashInput{})
	assert.ElementsMatch(t, []string{"amount", "receiptFileId"}, keys(fieldsOf(err)), "maks. per transaksi dan bukti wajib")

	receipt := uuid.New()
	exp, err := h.fin.CreatePettyCash(ctx, domain.PettyCashInput{
		Type: domain.PettyCashExpense, FundID: fund.ID, BusinessDate: oct19, ExpenseTypeID: &office.ID, Amount: money.FromInt(150_000),
		ReceiptFileID: &receipt, Description: "Kertas",
	})
	require.NoError(t, err)
	assert.Equal(t, exp.ID, h.files.attached[receipt])
	exp, err = h.fin.PostPettyCash(h.noReauth(), exp.ID, exp.Version, service.PostPettyCashInput{})
	require.NoError(t, err, "pengeluaran kas kecil tanpa re-autentikasi")
	assert.Equal(t, map[string]string{"Dr 6303": "150000.00", "Cr 1102": "150000.00"}, h.journalLines(t, *exp.JournalEntryID))

	funds, err := h.fin.ListPettyCashFunds(ctx, false)
	require.NoError(t, err)
	for _, f := range funds {
		if f.ID == fund.ID {
			assert.Equal(t, "4850000.00", f.Balance.String())
			assert.Equal(t, "150000.00", f.Shortfall().String())
		}
	}
	big, err = h.fin.CancelPettyCash(ctx, big.ID, big.Version, "terlalu besar")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, big.Status)
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- S-406 PPh potong: tarif, tagihan pemasok, rekap, setor -------------------------------------

func TestWithholdingRatesDepositAndRecap(t *testing.T) {
	h := setupFinance(t)
	ctx := h.finance()
	b := h.newBank(t)

	// Versi tarif baru menutup versi lama (effective-dated).
	v, err := h.fin.CreateTaxRate(ctx, tax.Rate{
		Type: tax.PPh23, ObjectCode: "24-104-99", Name: "Jasa test", RatePercent: decimal.NewFromInt(2), ValidFrom: oct19.AddDays(-300),
	}, false, "")
	require.NoError(t, err)
	_, err = h.fin.CreateTaxRate(ctx, tax.Rate{
		Type: tax.PPh23, ObjectCode: "24-104-99", Name: "Jasa test", RatePercent: decimal.RequireFromString("2.5"), ValidFrom: oct19.AddDays(5),
	}, false, "naik")
	require.NoError(t, err)
	rates, err := h.fin.ListTaxRates(ctx, string(tax.PPh23))
	require.NoError(t, err)
	for _, r := range rates {
		if r.ID == v.ID {
			require.NotNil(t, r.ValidTo)
			assert.Equal(t, oct19.AddDays(4).String(), r.ValidTo.String())
		}
	}
	_, err = h.fin.CreateTaxRate(ctx, tax.Rate{
		Type: tax.PPh23, ObjectCode: "24-104-99", Name: "Jasa test", RatePercent: decimal.NewFromInt(3), ValidFrom: oct19.AddDays(5),
	}, false, "")
	assert.Error(t, err, "tanggal berlaku harus setelah versi terakhir")

	// API publik untuk tagihan pemasok.
	supplier := uuid.New()
	bill := uuid.New()
	subject := domain.WithholdingSubject{
		SourceType: "supplier_bill", SourceID: bill, SourceLabel: "TAG-001", BusinessDate: oct19, TaxpayerType: "supplier",
		TaxpayerID: &supplier, TaxpayerName: "PT Maklon",
	}
	q, err := h.fin.QuoteWithholding(ctx, service.WithholdingRequest{Subject: subject, Type: domain.WithholdingPPh23, Gross: money.FromInt(5_000_000)})
	require.NoError(t, err)
	assert.Equal(t, "200000.00", q.Record.TaxAmount.String(), "tanpa NPWP: PPh 23 100% lebih tinggi")
	assert.Equal(t, "2302", q.AccountCode)
	assert.Equal(t, uuid.Nil, q.Record.ID)
	subject.TaxpayerTaxID = "0123456789012345"
	res, err := h.fin.Withhold(ctx, service.WithholdingRequest{Subject: subject, Type: domain.WithholdingPPh23, ObjectCode: "24-104-99", Gross: money.FromInt(5_000_000)})
	require.NoError(t, err)
	assert.Equal(t, "100000.00", res.Record.TaxAmount.String())
	assert.NotEqual(t, uuid.Nil, res.Record.ID)
	none, err := h.fin.QuoteWithholding(ctx, service.WithholdingRequest{Subject: subject, Type: domain.WithholdingNone, Gross: money.FromInt(1)})
	require.NoError(t, err)
	assert.True(t, none.Record.TaxAmount.IsZero())

	cancelled := uuid.New()
	subject.SourceID, subject.SourceLabel = cancelled, "TAG-002"
	_, err = h.fin.Withhold(ctx, service.WithholdingRequest{Subject: subject, Type: domain.WithholdingPPh23, Gross: money.FromInt(1_000_000)})
	require.NoError(t, err)
	require.NoError(t, h.fin.CancelWithholdings(ctx, "supplier_bill", cancelled))

	oct := tax.Period{Year: 2026, Month: time.October}
	list, err := h.fin.ListWithholdings(ctx, service.WithholdingFilter{Period: oct, TaxType: string(tax.PPh23)}, paging.New(nil, nil))
	require.NoError(t, err)
	var found *domain.WithholdingRecord
	for i := range list.Items {
		if list.Items[i].SourceID == bill {
			found = &list.Items[i]
		}
		assert.NotEqual(t, cancelled, list.Items[i].SourceID, "bukti potong dibatalkan tidak tampil")
	}
	require.NotNil(t, found)
	slip, err := h.fin.SetSlipNumber(ctx, found.ID, found.Version, "BPPU-0001")
	require.NoError(t, err)
	assert.Equal(t, "BPPU-0001", slip.SlipNumber)

	link, err := h.fin.ExportWithholdingRecap(ctx, oct, "")
	require.NoError(t, err)
	assert.Contains(t, link.URL, "rekap-bukti-potong-2026-10.xlsx")
	book, err := excelize.OpenReader(bytes.NewReader(h.files.exports[len(h.files.exports)-1]))
	require.NoError(t, err)
	rows, err := book.GetRows("PPh 23")
	require.NoError(t, err)
	assert.Equal(t, service.RecapColumns, rows[0])
	assert.GreaterOrEqual(t, len(rows), 2)

	_, err = h.fin.CreateTaxDeposit(h.noReauth(), domain.TaxDepositInput{TaxType: tax.PPh23, Period: oct, BankAccountID: b.ID, DepositDate: oct19})
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthReauthRequired))
	summary, err := h.fin.WithholdingSummary(ctx, 2026)
	require.NoError(t, err)
	open := money.Zero
	for _, m := range summary {
		if m.TaxType == tax.PPh23 && m.Period == oct {
			open = m.Open
		}
	}
	require.True(t, open.IsPositive())
	dep, err := h.fin.CreateTaxDeposit(ctx, domain.TaxDepositInput{TaxType: tax.PPh23, Period: oct, BankAccountID: b.ID, DepositDate: oct19, NTPN: "NTPN123"})
	require.NoError(t, err)
	assert.Equal(t, open.String(), dep.Amount.String(), "setor = PPh 23 belum disetor masa itu")
	assert.Equal(t, map[string]string{"Dr 2302": open.String(), "Cr " + b.AccountCode: open.String()}, h.journalLines(t, dep.JournalEntryID))
	_, err = h.fin.CreateTaxDeposit(ctx, domain.TaxDepositInput{TaxType: tax.PPh23, Period: oct, BankAccountID: b.ID, DepositDate: oct19})
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "satu setoran per jenis per masa")
	assert.Error(t, h.fin.CancelWithholdings(ctx, "supplier_bill", bill), "bukti potong yang sudah disetor tidak bisa dibatalkan")

	deposits, err := h.fin.ListTaxDeposits(ctx, 2026)
	require.NoError(t, err)
	assert.NotEmpty(t, deposits)
}

// --- S-407 PPh Final UMKM -----------------------------------------------------------------------

func TestFinalTax(t *testing.T) {
	h := setupFinance(t)
	ctx := h.finance()
	b := h.newBank(t)
	feb := tax.Period{Year: 2026, Month: time.February}

	_, err := h.fin.RunFinalTax(ctx, feb)
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "PPh Final belum diaktifkan")

	year := 2026
	_, err = h.fin.SetFinalTaxSetting(ctx, domain.FinalTaxSetting{
		ValidFrom: clock.NewBusinessDate(2026, time.January, 1), IsEnabled: true, OpeningYear: &year,
		OpeningTurnover: money.FromInt(4_000_000_000),
	})
	require.NoError(t, err)

	// Omzet Februari 2026: penjualan bruto 400 jt, diskon 20 jt (A-05).
	_, err = h.svc.Post(actor.System(context.Background()), h.request("A-05", map[string]string{
		"receivable_value": "380000000", "discount_value": "20000000", "gross_sales": "400000000",
	}, func(r *domain.PostingRequest) {
		r.BusinessDate = clock.NewBusinessDate(2026, time.February, 15)
		r.Dimensions = domain.Dimensions{ChannelID: ptr(h.marketplace), SubChannelID: ptr(h.shopee)}
	}))
	require.NoError(t, err, "%v", fieldsOf(err))

	run, err := h.fin.RunFinalTax(ctx, feb)
	require.NoError(t, err)
	assert.Equal(t, "380000000.00", run.GrossRevenue.String(), "omzet bruto = kredit − debit akun 4xxx")
	assert.Equal(t, "1900000.00", run.TaxAmount.String(), "0,5% × omzet")
	require.NotNil(t, run.JournalEntryID)
	assert.Equal(t, map[string]string{"Dr 7203": "1900000.00", "Cr 2304": "1900000.00"}, h.journalLines(t, *run.JournalEntryID))
	_, err = h.fin.RunFinalTax(ctx, feb)
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "satu kali per masa")
	_, err = h.fin.RunFinalTax(ctx, tax.Period{Year: 2026, Month: time.October})
	assert.Error(t, err, "masa berjalan belum berakhir")

	sum, err := h.fin.FinalTaxSummary(ctx, 2026)
	require.NoError(t, err)
	require.NotNil(t, sum.Setting)
	assert.True(t, sum.Setting.IsEnabled)
	assert.Equal(t, "4000000000.00", sum.Opening.String())
	assert.NotEqual(t, tax.ThresholdBelow, sum.Threshold.Status, "≥ 4,38 M dari batas 4,8 M")
	require.NotNil(t, sum.Months[1].Run)
	assert.Equal(t, "380000000.00", sum.Months[1].GrossRevenue.String())
	assert.Equal(t, sum.Threshold.Cumulative.String(), sum.Months[11].Cumulative.String())

	dep, err := h.fin.CreateTaxDeposit(ctx, domain.TaxDepositInput{TaxType: tax.FinalUMKM, Period: feb, BankAccountID: b.ID, DepositDate: oct19})
	require.NoError(t, err)
	assert.Equal(t, "1900000.00", dep.Amount.String())
	assert.Equal(t, map[string]string{"Dr 2304": "1900000.00", "Cr " + b.AccountCode: "1900000.00"}, h.journalLines(t, dep.JournalEntryID))
	_, err = h.fin.CreateTaxDeposit(ctx, domain.TaxDepositInput{TaxType: tax.FinalUMKM, Period: tax.Period{Year: 2026, Month: time.March}, BankAccountID: b.ID, DepositDate: oct19})
	assert.Contains(t, fieldsOf(err), "period", "PPh Final Maret belum dihitung")
}
