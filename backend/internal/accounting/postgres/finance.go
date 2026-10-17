package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
	"balans/internal/tax"
)

// FinanceStore menyimpan Kas & Bank, rekonsiliasi bank, pengajuan biaya, kas kecil, dan pajak (R1.4). Nomor
// rekening dan NPWP penerima dienkripsi dengan Cipher (pola modul party).
type FinanceStore struct {
	pool   *pgxpool.Pool
	cipher *pii.Cipher
}

// NewFinanceStore membuat FinanceStore.
func NewFinanceStore(pool *pgxpool.Pool, cipher *pii.Cipher) *FinanceStore {
	return &FinanceStore{pool: pool, cipher: cipher}
}

func (s *FinanceStore) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func (s *FinanceStore) encrypt(v string) (string, error) {
	if s.cipher == nil {
		if v == "" {
			return "", nil
		}
		return "", errors.New("accounting: cipher PII belum dipasang")
	}
	return s.cipher.Encrypt(v)
}

func (s *FinanceStore) decrypt(v string) string {
	if s.cipher == nil || v == "" {
		return ""
	}
	out, err := s.cipher.Decrypt(v)
	if err != nil {
		return ""
	}
	return out
}

func checkRows(n int64, err error, what string) error {
	if err != nil {
		return mapFinanceError(err, what)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// mapFinanceError menerjemahkan pelanggaran constraint tabel keuangan.
func mapFinanceError(err error, what string) error {
	pe := pgError(err)
	if pe == nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	dup := func(field, msg string) error {
		return &apperr.Error{Code: apperr.CodeDuplicateCode, Message: msg, Fields: map[string]string{field: msg}, Err: err}
	}
	switch {
	case pe.Code == "23505" && (pe.ConstraintName == "uq_bank_accounts_code" || pe.ConstraintName == "uq_petty_cash_funds_code"):
		return dup("code", "Kode sudah dipakai (termasuk data yang sudah dihapus)")
	case pe.Code == "23505" && pe.ConstraintName == "uq_bank_accounts_account_id":
		return dup("accountId", "Akun ini sudah dipakai rekening lain")
	case pe.Code == "23505" && pe.ConstraintName == "uq_bank_statements_file_id":
		return dup("fileId", "File mutasi ini sudah pernah diimpor")
	case pe.Code == "23505" && pe.ConstraintName == "uq_bank_statement_lines_reconciled":
		return &apperr.Error{Code: apperr.CodeInvalidStateTransition, Message: "Baris jurnal sudah direkonsiliasi dengan mutasi lain.", Err: err}
	case pe.Code == "23505" && pe.ConstraintName == "uq_tax_deposits_period":
		return &apperr.Error{Code: apperr.CodeInvalidStateTransition, Message: "Pajak masa ini sudah disetor.", Err: err}
	case pe.Code == "23505" && pe.ConstraintName == "uq_final_tax_runs_period":
		return &apperr.Error{Code: apperr.CodeInvalidStateTransition, Message: "PPh Final masa ini sudah dihitung.", Err: err}
	case pe.Code == "23505" && pe.ConstraintName == "uq_final_tax_settings_valid_from":
		return dup("validFrom", "Sudah ada pengaturan PPh Final dengan tanggal berlaku ini")
	case pe.Code == "23P01" && pe.ConstraintName == "ex_withholding_tax_rates_period":
		return &apperr.Error{Code: apperr.CodeValidationFailed, Message: apperr.CodeValidationFailed.DefaultMessage(),
			Fields: map[string]string{"validFrom": "Periode berlaku tumpang tindih dengan versi tarif lain"}, Err: err}
	case pe.Code == "23514" && pe.ConstraintName == "ck_petty_cash_funds_balance":
		return apperr.Validation(map[string]string{"amount": "Saldo kas kecil tidak boleh negatif atau melebihi plafon"})
	}
	return mapWriteError(err, what)
}

func nullDate(d *clock.BusinessDate) pgtype.Date { return date(d) }

func datePtr(d pgtype.Date) *clock.BusinessDate {
	if !d.Valid {
		return nil
	}
	v := clock.NewBusinessDate(d.Time.Year(), d.Time.Month(), d.Time.Day())
	return &v
}

func strOf(t pgtype.Text) string { return t.String }

func numText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// --- Rekening ---------------------------------------------------------------------------------

func (s *FinanceStore) toBankAccount(r sqlcgen.GetBankAccountRow) domain.BankAccount {
	return domain.BankAccount{
		ID: r.ID, Code: r.Code, Name: r.Name, Kind: domain.BankAccountKind(r.Kind), BankName: r.BankName,
		AccountNumber: s.decrypt(r.AccountNumberEnc), AccountHolder: r.AccountHolder, AccountID: r.AccountID,
		AccountCode: r.GlCode, AccountName: r.GlName, IsActive: r.IsActive, Description: r.Description,
		DeletedAt: r.DeletedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListBankAccounts mengembalikan rekening; deleted=true untuk daftar Dihapus.
func (s *FinanceStore) ListBankAccounts(ctx context.Context, includeInactive, deleted bool) ([]domain.BankAccount, error) {
	rows, err := s.q(ctx).ListBankAccounts(ctx, sqlcgen.ListBankAccountsParams{Deleted: deleted, IncludeInactive: includeInactive})
	if err != nil {
		return nil, fmt.Errorf("list bank accounts: %w", err)
	}
	out := make([]domain.BankAccount, len(rows))
	for i, r := range rows {
		out[i] = s.toBankAccount(sqlcgen.GetBankAccountRow(r))
	}
	return out, nil
}

// GetBankAccount membaca rekening (termasuk yang dihapus); forUpdate mengunci baris.
func (s *FinanceStore) GetBankAccount(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.BankAccount, error) {
	var (
		r   sqlcgen.GetBankAccountRow
		err error
	)
	if forUpdate {
		var row sqlcgen.GetBankAccountForUpdateRow
		row, err = s.q(ctx).GetBankAccountForUpdate(ctx, id)
		r = sqlcgen.GetBankAccountRow(row)
	} else {
		r, err = s.q(ctx).GetBankAccount(ctx, id)
	}
	if err != nil {
		return domain.BankAccount{}, notFound(err, "get bank account")
	}
	return s.toBankAccount(r), nil
}

// InsertBankAccount menyimpan rekening baru.
func (s *FinanceStore) InsertBankAccount(ctx context.Context, b domain.BankAccount, actorID uuid.UUID) (uuid.UUID, error) {
	enc, err := s.encrypt(b.AccountNumber)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := s.q(ctx).InsertBankAccount(ctx, sqlcgen.InsertBankAccountParams{
		Code: b.Code, Name: b.Name, Kind: string(b.Kind), BankName: b.BankName, AccountNumberEnc: enc,
		AccountNumberLast4: b.Last4(), AccountHolder: b.AccountHolder, AccountID: b.AccountID, IsActive: b.IsActive,
		Description: b.Description, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert bank account")
	}
	return id, nil
}

// UpdateBankAccount menyimpan perubahan rekening.
func (s *FinanceStore) UpdateBankAccount(ctx context.Context, b domain.BankAccount, version int, actorID uuid.UUID) error {
	enc, err := s.encrypt(b.AccountNumber)
	if err != nil {
		return err
	}
	n, err := s.q(ctx).UpdateBankAccount(ctx, sqlcgen.UpdateBankAccountParams{
		Name: b.Name, Kind: string(b.Kind), BankName: b.BankName, AccountNumberEnc: enc, AccountNumberLast4: b.Last4(),
		AccountHolder: b.AccountHolder, AccountID: b.AccountID, IsActive: b.IsActive, Description: b.Description,
		ActorID: actorID, ID: b.ID, Version: int32(version), //nolint:gosec // versi kecil
	})
	return checkRows(n, err, "update bank account")
}

// SetBankAccountDeleted menghapus (at != nil) atau memulihkan rekening.
func (s *FinanceStore) SetBankAccountDeleted(ctx context.Context, id uuid.UUID, version int, at *time.Time, actorID uuid.UUID) error {
	var by uuid.NullUUID
	if at != nil {
		by = uuid.NullUUID{UUID: actorID, Valid: true}
	}
	n, err := s.q(ctx).SetBankAccountDeleted(ctx, sqlcgen.SetBankAccountDeletedParams{
		DeletedAt: at, DeletedBy: by, ActorID: actorID, ID: id, Version: int32(version), //nolint:gosec // versi kecil
	})
	return checkRows(n, err, "delete bank account")
}

// BankAccountUsed: rekening sudah dipakai pembayaran aktif atau file mutasi.
func (s *FinanceStore) BankAccountUsed(ctx context.Context, id uuid.UUID) (bool, error) {
	u, err := s.q(ctx).BankAccountUsage(ctx, id)
	if err != nil {
		return false, fmt.Errorf("bank account usage: %w", err)
	}
	return u.Payments > 0 || u.Statements > 0, nil
}

// --- Pembayaran -------------------------------------------------------------------------------

func toPayment(r sqlcgen.GetPaymentRow) domain.Payment {
	return domain.Payment{
		ID: r.ID, Number: strOf(r.Number), Direction: domain.PaymentDirection(r.Direction), BankAccountID: r.BankAccountID,
		BankAccountCode: r.BankCode, BankAccountName: r.BankName, BankGLCode: r.BankGlCode, PaymentDate: r.PaymentDate,
		Amount: money.FromDecimal(r.Amount), CounterAccountID: r.CounterAccountID, CounterCode: r.CounterCode,
		CounterName: r.CounterName, PartyType: r.PartyType, PartyID: uuidPtr(r.PartyID), PartyName: r.PartyName,
		Reference: r.Reference, Description: r.Description, ProofFileID: uuidPtr(r.ProofFileID), SourceType: r.SourceType,
		SourceID: uuidPtr(r.SourceID), SourceLabel: r.SourceLabel, Status: domain.DocStatus(r.Status),
		JournalEntryID: uuidPtr(r.JournalEntryID), JournalNumber: strOf(r.JournalNumber), PostedAt: r.PostedAt,
		PostedBy: uuidPtr(r.PostedBy), CancelReason: r.CancelReason, Reconciled: r.Reconciled, CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// PaymentFilter adalah filter daftar pembayaran.
type PaymentFilter struct {
	BankAccountID *uuid.UUID
	Direction     string
	Status        string
	DateFrom      *clock.BusinessDate
	DateTo        *clock.BusinessDate
	Search        string
}

// ListPayments mengembalikan daftar pembayaran.
func (s *FinanceStore) ListPayments(ctx context.Context, f PaymentFilter, page paging.Page) (paging.Result[domain.Payment], error) {
	rows, err := s.q(ctx).ListPayments(ctx, sqlcgen.ListPaymentsParams{
		BankAccountID: nullUUID(f.BankAccountID), Direction: text(f.Direction), Status: text(f.Status),
		DateFrom: date(f.DateFrom), DateTo: date(f.DateTo), Q: text(f.Search), Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.Payment]{}, fmt.Errorf("list payments: %w", err)
	}
	out := paging.Result[domain.Payment]{Page: page, Items: make([]domain.Payment, len(rows))}
	for i, r := range rows {
		out.Items[i] = toPayment(sqlcgen.GetPaymentRow(r))
		out.Total = r.TotalCount
	}
	return out, nil
}

// GetPayment membaca pembayaran; lock mengunci baris lebih dulu.
func (s *FinanceStore) GetPayment(ctx context.Context, id uuid.UUID, lock bool) (domain.Payment, error) {
	if lock {
		if _, err := s.q(ctx).LockPayment(ctx, id); err != nil {
			return domain.Payment{}, notFound(err, "lock payment")
		}
	}
	r, err := s.q(ctx).GetPayment(ctx, id)
	if err != nil {
		return domain.Payment{}, notFound(err, "get payment")
	}
	return toPayment(r), nil
}

// InsertPayment menyimpan draft pembayaran.
func (s *FinanceStore) InsertPayment(ctx context.Context, p domain.Payment, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertPayment(ctx, sqlcgen.InsertPaymentParams{
		Direction: string(p.Direction), BankAccountID: p.BankAccountID, PaymentDate: p.PaymentDate, Amount: p.Amount.Decimal(),
		CounterAccountID: p.CounterAccountID, PartyType: p.PartyType, PartyID: nullUUID(p.PartyID), PartyName: p.PartyName,
		Reference: p.Reference, Description: p.Description, ProofFileID: nullUUID(p.ProofFileID), SourceType: p.SourceType,
		SourceID: nullUUID(p.SourceID), SourceLabel: p.SourceLabel, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert payment")
	}
	return id, nil
}

// SavePayment menyimpan perubahan pembayaran (optimistic locking).
func (s *FinanceStore) SavePayment(ctx context.Context, p domain.Payment, version int, actorID uuid.UUID, cancelledAt *time.Time) error {
	var cancelledBy uuid.NullUUID
	if cancelledAt != nil {
		cancelledBy = uuid.NullUUID{UUID: actorID, Valid: true}
	}
	n, err := s.q(ctx).SavePayment(ctx, sqlcgen.SavePaymentParams{
		Number: numText(p.Number), Direction: string(p.Direction), BankAccountID: p.BankAccountID, PaymentDate: p.PaymentDate,
		Amount: p.Amount.Decimal(), CounterAccountID: p.CounterAccountID, PartyType: p.PartyType, PartyID: nullUUID(p.PartyID),
		PartyName: p.PartyName, Reference: p.Reference, Description: p.Description, ProofFileID: nullUUID(p.ProofFileID),
		Status: string(p.Status), JournalEntryID: nullUUID(p.JournalEntryID), PostedAt: p.PostedAt, PostedBy: nullUUID(p.PostedBy),
		CancelledAt: cancelledAt, CancelledBy: cancelledBy, CancelReason: p.CancelReason, ActorID: actorID, ID: p.ID,
		Version: int32(version), //nolint:gosec // versi kecil
	})
	return checkRows(n, err, "save payment")
}

// --- Mutasi rekening --------------------------------------------------------------------------

// BankMutations mengembalikan saldo awal, jumlah baris, dan mutasi akun GL rekening pada rentang tanggal.
func (s *FinanceStore) BankMutations(ctx context.Context, accountID uuid.UUID, from, to clock.BusinessDate, page paging.Page) (money.Amount, int64, []domain.BankMutation, error) {
	q := s.q(ctx)
	opening, err := q.BankMutationOpening(ctx, sqlcgen.BankMutationOpeningParams{AccountID: accountID, DateFrom: from})
	if err != nil {
		return money.Zero, 0, nil, fmt.Errorf("bank mutation opening: %w", err)
	}
	total, err := q.BankMutationCount(ctx, sqlcgen.BankMutationCountParams{AccountID: accountID, DateFrom: from, DateTo: to})
	if err != nil {
		return money.Zero, 0, nil, fmt.Errorf("bank mutation count: %w", err)
	}
	rows, err := q.BankMutations(ctx, sqlcgen.BankMutationsParams{
		AccountID: accountID, DateFrom: from, DateTo: to, Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return money.Zero, 0, nil, fmt.Errorf("bank mutations: %w", err)
	}
	open := money.FromDecimal(opening)
	out := make([]domain.BankMutation, len(rows))
	for i, r := range rows {
		out[i] = domain.BankMutation{
			JournalLineID: r.LineID, JournalEntryID: r.EntryID, JournalNumber: strOf(r.Number), BusinessDate: r.BusinessDate,
			Description: r.Description, SourceType: r.SourceType, SourceLabel: r.SourceLabel, Memo: r.Memo,
			Amount: money.FromDecimal(r.Amount), Balance: open.Add(money.FromDecimal(r.Cumulative)), Reconciled: r.Reconciled,
		}
	}
	return open, total, out, nil
}

// UnreconciledBankLines mengembalikan baris jurnal akun GL rekening yang belum direkonsiliasi sebagai kandidat.
func (s *FinanceStore) UnreconciledBankLines(ctx context.Context, accountID uuid.UUID, from, to clock.BusinessDate) ([]domain.MatchCandidate, error) {
	rows, err := s.q(ctx).UnreconciledBankLines(ctx, sqlcgen.UnreconciledBankLinesParams{AccountID: accountID, DateFrom: from, DateTo: to})
	if err != nil {
		return nil, fmt.Errorf("unreconciled bank lines: %w", err)
	}
	out := make([]domain.MatchCandidate, len(rows))
	for i, r := range rows {
		label := strOf(r.Number)
		if r.SourceLabel != "" {
			label += " · " + r.SourceLabel
		}
		if r.PartyName != "" {
			label += " · " + r.PartyName
		}
		out[i] = domain.MatchCandidate{
			Kind: domain.MatchJournalLine, JournalLineID: r.LineID, JournalEntryID: r.EntryID, Label: label,
			Date: r.BusinessDate, Amount: money.FromDecimal(r.Amount),
			References: []string{r.PaymentReference, r.PaymentNumber, strOf(r.Number), r.SourceLabel},
		}
	}
	return out, nil
}

// BankLineOfEntry mengembalikan baris jurnal akun GL rekening pada jurnal tertentu.
func (s *FinanceStore) BankLineOfEntry(ctx context.Context, entryID, accountID uuid.UUID) (domain.MatchCandidate, error) {
	r, err := s.q(ctx).JournalLineOnAccount(ctx, sqlcgen.JournalLineOnAccountParams{EntryID: entryID, AccountID: accountID})
	if err != nil {
		return domain.MatchCandidate{}, notFound(err, "journal line on account")
	}
	return domain.MatchCandidate{
		Kind: domain.MatchJournalLine, JournalLineID: r.LineID, JournalEntryID: r.EntryID, Label: strOf(r.Number),
		Date: r.BusinessDate, Amount: money.FromDecimal(r.Amount),
	}, nil
}

// JournalLine membaca baris jurnal posted beserta akun dan nominal bertandanya.
func (s *FinanceStore) JournalLine(ctx context.Context, id uuid.UUID) (domain.MatchCandidate, uuid.UUID, error) {
	r, err := s.q(ctx).GetJournalLine(ctx, id)
	if err != nil {
		return domain.MatchCandidate{}, uuid.Nil, notFound(err, "get journal line")
	}
	return domain.MatchCandidate{
		Kind: domain.MatchJournalLine, JournalLineID: r.LineID, JournalEntryID: r.EntryID, Label: strOf(r.Number),
		Date: r.BusinessDate, Amount: money.FromDecimal(r.Amount),
	}, r.AccountID, nil
}

// --- Mutasi bank (file) -----------------------------------------------------------------------

func toSummary(r sqlcgen.GetBankStatementRow) domain.StatementSummary {
	return domain.StatementSummary{
		ID: r.ID, BankAccountID: r.BankAccountID, BankAccountCode: r.BankCode, BankAccountName: r.BankName, FileID: r.FileID,
		FileName: r.FileName, PeriodFrom: r.PeriodFrom, PeriodTo: r.PeriodTo, LineCount: int(r.LineCount),
		TotalIn: money.FromDecimal(r.TotalIn), TotalOut: money.FromDecimal(r.TotalOut), MatchedCount: int(r.MatchedCount),
		SuggestedCount: int(r.SuggestedCount), CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
	}
}

// InsertStatement menyimpan file mutasi beserta barisnya.
func (s *FinanceStore) InsertStatement(ctx context.Context, sum domain.StatementSummary, rows []domain.StatementRow, actorID uuid.UUID) (uuid.UUID, error) {
	q := s.q(ctx)
	r, err := q.InsertBankStatement(ctx, sqlcgen.InsertBankStatementParams{
		BankAccountID: sum.BankAccountID, FileID: sum.FileID, FileName: sum.FileName, PeriodFrom: sum.PeriodFrom,
		PeriodTo: sum.PeriodTo, LineCount: int32(sum.LineCount), //nolint:gosec // dibatasi MaxStatementLines
		TotalIn: sum.TotalIn.Decimal(), TotalOut: sum.TotalOut.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert bank statement")
	}
	for i, row := range rows {
		if err := q.InsertBankStatementLine(ctx, sqlcgen.InsertBankStatementLineParams{
			StatementID: r.ID, LineNo: int32(i + 1), //nolint:gosec // dibatasi MaxStatementLines
			TransactionDate: row.Date, Description: row.Description, Reference: row.Reference,
			Amount: row.Amount.Decimal(), Balance: nullAmount(row.Balance),
		}); err != nil {
			return uuid.Nil, mapFinanceError(err, "insert bank statement line")
		}
	}
	return r.ID, nil
}

// ListStatements mengembalikan file mutasi terbaru.
func (s *FinanceStore) ListStatements(ctx context.Context, bankAccountID *uuid.UUID, page paging.Page) (paging.Result[domain.StatementSummary], error) {
	rows, err := s.q(ctx).ListBankStatements(ctx, sqlcgen.ListBankStatementsParams{
		BankAccountID: nullUUID(bankAccountID), Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.StatementSummary]{}, fmt.Errorf("list bank statements: %w", err)
	}
	out := paging.Result[domain.StatementSummary]{Page: page, Items: make([]domain.StatementSummary, len(rows))}
	for i, r := range rows {
		out.Items[i] = toSummary(sqlcgen.GetBankStatementRow(r))
		out.Total = r.TotalCount
	}
	return out, nil
}

// GetStatement membaca ringkasan file mutasi.
func (s *FinanceStore) GetStatement(ctx context.Context, id uuid.UUID) (domain.StatementSummary, error) {
	r, err := s.q(ctx).GetBankStatement(ctx, id)
	if err != nil {
		return domain.StatementSummary{}, notFound(err, "get bank statement")
	}
	return toSummary(r), nil
}

func toStatementLine(r sqlcgen.GetStatementLineRow) domain.StatementLine {
	return domain.StatementLine{
		ID: r.ID, StatementID: r.StatementID, BankAccountID: r.BankAccountID, LineNo: int(r.LineNo),
		TransactionDate: r.TransactionDate, Description: r.Description, Reference: r.Reference,
		Amount: money.FromDecimal(r.Amount), Balance: amountPtr(r.Balance), Status: domain.LineStatus(r.Status),
		MatchKind: domain.MatchKind(r.MatchKind), MatchRule: domain.MatchRule(r.MatchRule), JournalLineID: uuidPtr(r.JournalLineID),
		JournalEntryID: uuidPtr(r.JournalEntryID), JournalNumber: r.JournalNumber, CandidateSource: r.CandidateSource,
		CandidateDocID: uuidPtr(r.CandidateDocumentID), CandidateLabel: r.CandidateLabel, PaymentID: uuidPtr(r.PaymentID),
		MatchedAt: r.MatchedAt, MatchedBy: uuidPtr(r.MatchedBy), Version: int(r.Version),
	}
}

// StatementLines mengembalikan baris file mutasi; lock mengunci seluruh baris lebih dulu.
func (s *FinanceStore) StatementLines(ctx context.Context, statementID uuid.UUID, lock bool) ([]domain.StatementLine, error) {
	if lock {
		if _, err := s.q(ctx).LockStatementLines(ctx, statementID); err != nil {
			return nil, fmt.Errorf("lock statement lines: %w", err)
		}
	}
	rows, err := s.q(ctx).StatementLines(ctx, statementID)
	if err != nil {
		return nil, fmt.Errorf("statement lines: %w", err)
	}
	out := make([]domain.StatementLine, len(rows))
	for i, r := range rows {
		out[i] = toStatementLine(sqlcgen.GetStatementLineRow(r))
	}
	return out, nil
}

// GetStatementLine membaca satu baris mutasi; lock mengunci baris.
func (s *FinanceStore) GetStatementLine(ctx context.Context, id uuid.UUID, lock bool) (domain.StatementLine, error) {
	if lock {
		if _, err := s.q(ctx).LockStatementLine(ctx, id); err != nil {
			return domain.StatementLine{}, notFound(err, "lock statement line")
		}
	}
	r, err := s.q(ctx).GetStatementLine(ctx, id)
	if err != nil {
		return domain.StatementLine{}, notFound(err, "get statement line")
	}
	return toStatementLine(r), nil
}

// SaveStatementLine menyimpan usulan/hasil pencocokan baris.
func (s *FinanceStore) SaveStatementLine(ctx context.Context, l domain.StatementLine) error {
	n, err := s.q(ctx).SaveStatementLine(ctx, sqlcgen.SaveStatementLineParams{
		Status: string(l.Status), MatchKind: string(l.MatchKind), MatchRule: string(l.MatchRule),
		JournalLineID: nullUUID(l.JournalLineID), JournalEntryID: nullUUID(l.JournalEntryID), CandidateSource: l.CandidateSource,
		CandidateDocumentID: nullUUID(l.CandidateDocID), CandidateLabel: l.CandidateLabel, PaymentID: nullUUID(l.PaymentID),
		MatchedAt: l.MatchedAt, MatchedBy: nullUUID(l.MatchedBy), ID: l.ID,
		Version: int32(l.Version), //nolint:gosec // versi kecil
	})
	return checkRows(n, err, "save statement line")
}

// --- Pengajuan biaya --------------------------------------------------------------------------

func (s *FinanceStore) toClaim(r sqlcgen.GetExpenseClaimRow) domain.ExpenseClaim {
	return domain.ExpenseClaim{
		ID: r.ID, Number: strOf(r.Number), ClaimType: domain.ClaimType(r.ClaimType), RequesterID: r.RequesterID,
		DivisionID: r.DivisionID, DivisionName: r.DivisionName, CampaignID: uuidPtr(r.CampaignID), CampaignName: r.CampaignName,
		PayeeType: domain.PayeeType(r.PayeeType), PayeeID: uuidPtr(r.PayeeID), PayeeName: r.PayeeName,
		PayeeTaxID: s.decrypt(r.PayeeTaxIDEnc), ClaimDate: r.ClaimDate, Description: r.Description,
		Total: money.FromDecimal(r.TotalAmount), Withholding: money.FromDecimal(r.WithholdingAmount),
		Payable: money.FromDecimal(r.PayableAmount), Status: domain.DocStatus(r.Status), SubmittedAt: r.SubmittedAt,
		DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote, VerifiedAt: r.VerifiedAt, VerifiedBy: uuidPtr(r.VerifiedBy),
		VerifyDate: datePtr(r.VerifyDate), JournalEntryID: uuidPtr(r.JournalEntryID), JournalNumber: r.JournalNumber,
		PaymentID: uuidPtr(r.PaymentID), PaymentNumber: r.PaymentNumber, PaidAt: r.PaidAt, CancelReason: r.CancelReason,
		CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ClaimFilter adalah filter daftar pengajuan biaya.
type ClaimFilter struct {
	RequesterID *uuid.UUID
	Status      string
	DivisionID  *uuid.UUID
	DateFrom    *clock.BusinessDate
	DateTo      *clock.BusinessDate
	Search      string
}

// ListClaims mengembalikan daftar pengajuan (tanpa baris).
func (s *FinanceStore) ListClaims(ctx context.Context, f ClaimFilter, page paging.Page) (paging.Result[domain.ExpenseClaim], error) {
	rows, err := s.q(ctx).ListExpenseClaims(ctx, sqlcgen.ListExpenseClaimsParams{
		RequesterID: nullUUID(f.RequesterID), Status: text(f.Status), DivisionID: nullUUID(f.DivisionID),
		DateFrom: date(f.DateFrom), DateTo: date(f.DateTo), Q: text(f.Search), Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.ExpenseClaim]{}, fmt.Errorf("list expense claims: %w", err)
	}
	out := paging.Result[domain.ExpenseClaim]{Page: page, Items: make([]domain.ExpenseClaim, len(rows))}
	for i, r := range rows {
		out.Items[i] = s.toClaim(sqlcgen.GetExpenseClaimRow(r))
		out.Total = r.TotalCount
	}
	return out, nil
}

// GetClaim membaca pengajuan beserta baris; lock mengunci baris pengajuan.
func (s *FinanceStore) GetClaim(ctx context.Context, id uuid.UUID, lock bool) (domain.ExpenseClaim, error) {
	q := s.q(ctx)
	if lock {
		if _, err := q.LockExpenseClaim(ctx, id); err != nil {
			return domain.ExpenseClaim{}, notFound(err, "lock expense claim")
		}
	}
	r, err := q.GetExpenseClaim(ctx, id)
	if err != nil {
		return domain.ExpenseClaim{}, notFound(err, "get expense claim")
	}
	c := s.toClaim(r)
	lines, err := q.ExpenseClaimLines(ctx, id)
	if err != nil {
		return domain.ExpenseClaim{}, fmt.Errorf("expense claim lines: %w", err)
	}
	c.Lines = make([]domain.ClaimLine, len(lines))
	for i, l := range lines {
		c.Lines[i] = domain.ClaimLine{
			ID: l.ID, LineNo: int(l.LineNo), ExpenseTypeID: l.ExpenseTypeID, ExpenseTypeCode: l.TypeCode,
			ExpenseTypeName: l.TypeName, ExpenseAccountCode: l.AccountCode, Description: l.Description,
			Amount: money.FromDecimal(l.Amount), ReceiptFileID: uuidPtr(l.ReceiptFileID),
			WithholdingType: domain.WithholdingType(l.WithholdingType), ObjectCode: l.ObjectCode,
			WithholdingRate: l.WithholdingRate, WithholdingAmount: money.FromDecimal(l.WithholdingAmount),
		}
	}
	return c, nil
}

// InsertClaim menyimpan draft pengajuan beserta baris.
func (s *FinanceStore) InsertClaim(ctx context.Context, c domain.ExpenseClaim, actorID uuid.UUID) (uuid.UUID, error) {
	enc, err := s.encrypt(c.PayeeTaxID)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := s.q(ctx).InsertExpenseClaim(ctx, sqlcgen.InsertExpenseClaimParams{
		ClaimType: string(c.ClaimType), RequesterID: c.RequesterID, DivisionID: c.DivisionID, CampaignID: nullUUID(c.CampaignID),
		PayeeType: string(c.PayeeType), PayeeID: nullUUID(c.PayeeID), PayeeName: c.PayeeName, PayeeTaxIDEnc: enc,
		ClaimDate: c.ClaimDate, Description: c.Description, TotalAmount: c.Total.Decimal(),
		WithholdingAmount: c.Withholding.Decimal(), PayableAmount: c.Payable.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert expense claim")
	}
	c.ID = id
	return id, s.replaceClaimLines(ctx, c, actorID)
}

func (s *FinanceStore) replaceClaimLines(ctx context.Context, c domain.ExpenseClaim, actorID uuid.UUID) error {
	q := s.q(ctx)
	if err := q.DeleteExpenseClaimLines(ctx, c.ID); err != nil {
		return fmt.Errorf("delete expense claim lines: %w", err)
	}
	for _, l := range c.Lines {
		if err := q.InsertExpenseClaimLine(ctx, sqlcgen.InsertExpenseClaimLineParams{
			ClaimID: c.ID, LineNo: int32(l.LineNo), ExpenseTypeID: l.ExpenseTypeID, //nolint:gosec // dibatasi MaxClaimLines
			Description: l.Description, Amount: l.Amount.Decimal(), ReceiptFileID: nullUUID(l.ReceiptFileID),
			WithholdingType: string(l.WithholdingType), ObjectCode: l.ObjectCode, WithholdingRate: l.WithholdingRate,
			WithholdingAmount: l.WithholdingAmount.Decimal(), ActorID: actorID,
		}); err != nil {
			return mapFinanceError(err, "insert expense claim line")
		}
	}
	return nil
}

// SaveClaim menyimpan header pengajuan; replaceLines menulis ulang baris (draft), selain itu hanya kolom PPh baris.
func (s *FinanceStore) SaveClaim(ctx context.Context, c domain.ExpenseClaim, version int, replaceLines bool, actorID uuid.UUID) error {
	enc, err := s.encrypt(c.PayeeTaxID)
	if err != nil {
		return err
	}
	n, err := s.q(ctx).SaveExpenseClaim(ctx, sqlcgen.SaveExpenseClaimParams{
		Number: numText(c.Number), ClaimType: string(c.ClaimType), DivisionID: c.DivisionID, CampaignID: nullUUID(c.CampaignID),
		PayeeType: string(c.PayeeType), PayeeID: nullUUID(c.PayeeID), PayeeName: c.PayeeName, PayeeTaxIDEnc: enc,
		ClaimDate: c.ClaimDate, Description: c.Description, TotalAmount: c.Total.Decimal(),
		WithholdingAmount: c.Withholding.Decimal(), PayableAmount: c.Payable.Decimal(), Status: string(c.Status),
		SubmittedAt: c.SubmittedAt, DecidedAt: c.DecidedAt, DecisionNote: c.DecisionNote, VerifiedAt: c.VerifiedAt,
		VerifiedBy: nullUUID(c.VerifiedBy), VerifyDate: nullDate(c.VerifyDate), JournalEntryID: nullUUID(c.JournalEntryID),
		PaymentID: nullUUID(c.PaymentID), PaidAt: c.PaidAt, CancelReason: c.CancelReason, ActorID: actorID, ID: c.ID,
		Version: int32(version), //nolint:gosec // versi kecil
	})
	if err := checkRows(n, err, "save expense claim"); err != nil {
		return err
	}
	if replaceLines {
		return s.replaceClaimLines(ctx, c, actorID)
	}
	for _, l := range c.Lines {
		if err := s.q(ctx).UpdateExpenseClaimLineTax(ctx, sqlcgen.UpdateExpenseClaimLineTaxParams{
			WithholdingType: string(l.WithholdingType), ObjectCode: l.ObjectCode, WithholdingRate: l.WithholdingRate,
			WithholdingAmount: l.WithholdingAmount.Decimal(), ID: l.ID,
		}); err != nil {
			return mapFinanceError(err, "update expense claim line tax")
		}
	}
	return nil
}

// ClaimPayable adalah pengajuan Menunggu Pembayaran untuk kandidat rekonsiliasi.
type ClaimPayable struct {
	ID         uuid.UUID
	Number     string
	PayeeType  string
	PayeeID    *uuid.UUID
	PayeeName  string
	Amount     money.Amount
	VerifyDate clock.BusinessDate
}

// ClaimsAwaitingPayment mengembalikan pengajuan Menunggu Pembayaran (claimID opsional).
func (s *FinanceStore) ClaimsAwaitingPayment(ctx context.Context, claimID *uuid.UUID) ([]ClaimPayable, error) {
	rows, err := s.q(ctx).ClaimsAwaitingPayment(ctx, nullUUID(claimID))
	if err != nil {
		return nil, fmt.Errorf("claims awaiting payment: %w", err)
	}
	out := make([]ClaimPayable, len(rows))
	for i, r := range rows {
		out[i] = ClaimPayable{
			ID: r.ID, Number: strOf(r.Number), PayeeType: r.PayeeType, PayeeID: uuidPtr(r.PayeeID), PayeeName: r.PayeeName,
			Amount: money.FromDecimal(r.PayableAmount),
		}
		if d := datePtr(r.VerifyDate); d != nil {
			out[i].VerifyDate = *d
		}
	}
	return out, nil
}

// --- Kas kecil --------------------------------------------------------------------------------

func toFund(r sqlcgen.GetPettyCashFundRow) domain.PettyCashFund {
	return domain.PettyCashFund{
		ID: r.ID, Code: r.Code, Name: r.Name, HolderUserID: r.HolderUserID, DivisionID: r.DivisionID,
		DivisionName: r.DivisionName, Ceiling: money.FromDecimal(r.CeilingAmount), Balance: money.FromDecimal(r.BalanceAmount),
		IsActive: r.IsActive, Description: r.Description, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListFunds mengembalikan dana kas kecil.
func (s *FinanceStore) ListFunds(ctx context.Context, includeInactive bool) ([]domain.PettyCashFund, error) {
	rows, err := s.q(ctx).ListPettyCashFunds(ctx, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("list petty cash funds: %w", err)
	}
	out := make([]domain.PettyCashFund, len(rows))
	for i, r := range rows {
		out[i] = toFund(sqlcgen.GetPettyCashFundRow(r))
	}
	return out, nil
}

// GetFund membaca dana; lock mengunci baris.
func (s *FinanceStore) GetFund(ctx context.Context, id uuid.UUID, lock bool) (domain.PettyCashFund, error) {
	if lock {
		if _, err := s.q(ctx).LockPettyCashFund(ctx, id); err != nil {
			return domain.PettyCashFund{}, notFound(err, "lock petty cash fund")
		}
	}
	r, err := s.q(ctx).GetPettyCashFund(ctx, id)
	if err != nil {
		return domain.PettyCashFund{}, notFound(err, "get petty cash fund")
	}
	return toFund(r), nil
}

// InsertFund menyimpan dana baru.
func (s *FinanceStore) InsertFund(ctx context.Context, f domain.PettyCashFund, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertPettyCashFund(ctx, sqlcgen.InsertPettyCashFundParams{
		Code: f.Code, Name: f.Name, HolderUserID: f.HolderUserID, DivisionID: f.DivisionID, CeilingAmount: f.Ceiling.Decimal(),
		IsActive: f.IsActive, Description: f.Description, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert petty cash fund")
	}
	return id, nil
}

// SaveFund menyimpan dana (termasuk saldo).
func (s *FinanceStore) SaveFund(ctx context.Context, f domain.PettyCashFund, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SavePettyCashFund(ctx, sqlcgen.SavePettyCashFundParams{
		Name: f.Name, HolderUserID: f.HolderUserID, DivisionID: f.DivisionID, CeilingAmount: f.Ceiling.Decimal(),
		BalanceAmount: f.Balance.Decimal(), IsActive: f.IsActive, Description: f.Description, ActorID: actorID, ID: f.ID,
		Version: int32(version), //nolint:gosec // versi kecil
	})
	return checkRows(n, err, "save petty cash fund")
}

func toPettyTxn(r sqlcgen.GetPettyCashTransactionRow) domain.PettyCashTxn {
	return domain.PettyCashTxn{
		ID: r.ID, Number: strOf(r.Number), FundID: r.FundID, FundCode: r.FundCode, FundName: r.FundName,
		Type: domain.PettyCashType(r.TransactionType), BusinessDate: r.BusinessDate, ExpenseTypeID: uuidPtr(r.ExpenseTypeID),
		ExpenseTypeCode: r.TypeCode, ExpenseTypeName: r.TypeName, ExpenseAccountCode: r.AccountCode, DivisionID: r.DivisionID,
		DivisionName: r.DivisionName, CampaignID: uuidPtr(r.CampaignID), CampaignName: r.CampaignName, Description: r.Description,
		Amount: money.FromDecimal(r.Amount), ReceiptFileID: uuidPtr(r.ReceiptFileID), BankAccountID: uuidPtr(r.BankAccountID),
		BankAccountName: r.BankName, Status: domain.DocStatus(r.Status), DecisionNote: r.DecisionNote,
		JournalEntryID: uuidPtr(r.JournalEntryID), JournalNumber: r.JournalNumber, PostedAt: r.PostedAt,
		PostedBy: uuidPtr(r.PostedBy), CancelReason: r.CancelReason, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// PettyCashFilter adalah filter daftar transaksi kas kecil.
type PettyCashFilter struct {
	FundID   *uuid.UUID
	Type     string
	Status   string
	DateFrom *clock.BusinessDate
	DateTo   *clock.BusinessDate
}

// ListPettyCash mengembalikan transaksi kas kecil.
func (s *FinanceStore) ListPettyCash(ctx context.Context, f PettyCashFilter, page paging.Page) (paging.Result[domain.PettyCashTxn], error) {
	rows, err := s.q(ctx).ListPettyCashTransactions(ctx, sqlcgen.ListPettyCashTransactionsParams{
		FundID: nullUUID(f.FundID), TransactionType: text(f.Type), Status: text(f.Status), DateFrom: date(f.DateFrom),
		DateTo: date(f.DateTo), Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.PettyCashTxn]{}, fmt.Errorf("list petty cash: %w", err)
	}
	out := paging.Result[domain.PettyCashTxn]{Page: page, Items: make([]domain.PettyCashTxn, len(rows))}
	for i, r := range rows {
		out.Items[i] = toPettyTxn(sqlcgen.GetPettyCashTransactionRow(r))
		out.Total = r.TotalCount
	}
	return out, nil
}

// GetPettyCash membaca transaksi kas kecil; lock mengunci baris.
func (s *FinanceStore) GetPettyCash(ctx context.Context, id uuid.UUID, lock bool) (domain.PettyCashTxn, error) {
	if lock {
		if _, err := s.q(ctx).LockPettyCashTransaction(ctx, id); err != nil {
			return domain.PettyCashTxn{}, notFound(err, "lock petty cash transaction")
		}
	}
	r, err := s.q(ctx).GetPettyCashTransaction(ctx, id)
	if err != nil {
		return domain.PettyCashTxn{}, notFound(err, "get petty cash transaction")
	}
	return toPettyTxn(r), nil
}

// InsertPettyCash menyimpan draft transaksi kas kecil.
func (s *FinanceStore) InsertPettyCash(ctx context.Context, t domain.PettyCashTxn, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertPettyCashTransaction(ctx, sqlcgen.InsertPettyCashTransactionParams{
		FundID: t.FundID, TransactionType: string(t.Type), BusinessDate: t.BusinessDate, ExpenseTypeID: nullUUID(t.ExpenseTypeID),
		DivisionID: t.DivisionID, CampaignID: nullUUID(t.CampaignID), Description: t.Description, Amount: t.Amount.Decimal(),
		ReceiptFileID: nullUUID(t.ReceiptFileID), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert petty cash transaction")
	}
	return id, nil
}

// SavePettyCash menyimpan perubahan transaksi kas kecil.
func (s *FinanceStore) SavePettyCash(ctx context.Context, t domain.PettyCashTxn, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SavePettyCashTransaction(ctx, sqlcgen.SavePettyCashTransactionParams{
		Number: numText(t.Number), BusinessDate: t.BusinessDate, ExpenseTypeID: nullUUID(t.ExpenseTypeID),
		DivisionID: t.DivisionID, CampaignID: nullUUID(t.CampaignID), Description: t.Description, Amount: t.Amount.Decimal(),
		ReceiptFileID: nullUUID(t.ReceiptFileID), BankAccountID: nullUUID(t.BankAccountID), Status: string(t.Status),
		DecisionNote: t.DecisionNote, JournalEntryID: nullUUID(t.JournalEntryID), PostedAt: t.PostedAt,
		PostedBy: nullUUID(t.PostedBy), CancelReason: t.CancelReason, ActorID: actorID, ID: t.ID,
		Version: int32(version), //nolint:gosec // versi kecil
	})
	return checkRows(n, err, "save petty cash transaction")
}

// --- Pajak ------------------------------------------------------------------------------------

type bracketJSON struct {
	UpTo        string `json:"upTo"`
	RatePercent string `json:"ratePercent"`
}

func encodeBrackets(bs []tax.Bracket) ([]byte, error) {
	out := make([]bracketJSON, len(bs))
	for i, b := range bs {
		out[i] = bracketJSON{RatePercent: b.RatePercent.String()}
		if !b.UpTo.IsZero() {
			out[i].UpTo = b.UpTo.String()
		}
	}
	return json.Marshal(out)
}

func decodeBrackets(raw []byte) []tax.Bracket {
	var in []bracketJSON
	if len(raw) == 0 || json.Unmarshal(raw, &in) != nil {
		return nil
	}
	out := make([]tax.Bracket, 0, len(in))
	for _, b := range in {
		rate, err := decimal.NewFromString(b.RatePercent)
		if err != nil {
			continue
		}
		upTo := money.Zero
		if b.UpTo != "" {
			if v, err := money.Parse(b.UpTo); err == nil {
				upTo = v
			}
		}
		out = append(out, tax.Bracket{UpTo: upTo, RatePercent: rate})
	}
	return out
}

func toRate(r sqlcgen.WithholdingTaxRate) domain.RateVersion {
	return domain.RateVersion{
		ID: r.ID,
		Rate: tax.Rate{
			Type: tax.Type(r.TaxType), ObjectCode: r.ObjectCode, Name: r.Name, RatePercent: r.RatePercent,
			Brackets: decodeBrackets(r.Brackets), BasePercent: r.BasePercent, NoTaxIDSurchargePercent: r.NoTaxIDSurchargePercent,
			ValidFrom: r.ValidFrom, ValidTo: datePtr(r.ValidTo),
		},
		IsDefault: r.IsDefault, Note: r.Note, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListRates mengembalikan seluruh versi tarif (taxType kosong = semua).
func (s *FinanceStore) ListRates(ctx context.Context, taxType string) ([]domain.RateVersion, error) {
	rows, err := s.q(ctx).ListTaxRates(ctx, text(taxType))
	if err != nil {
		return nil, fmt.Errorf("list tax rates: %w", err)
	}
	out := make([]domain.RateVersion, len(rows))
	for i, r := range rows {
		out[i] = toRate(r)
	}
	return out, nil
}

// LatestRate mengunci versi tarif terakhir jenis + objek pajak; ok=false bila belum ada.
func (s *FinanceStore) LatestRate(ctx context.Context, t tax.Type, objectCode string) (domain.RateVersion, bool, error) {
	r, err := s.q(ctx).LatestTaxRateForUpdate(ctx, sqlcgen.LatestTaxRateForUpdateParams{TaxType: string(t), ObjectCode: objectCode})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RateVersion{}, false, nil
	}
	if err != nil {
		return domain.RateVersion{}, false, fmt.Errorf("latest tax rate: %w", err)
	}
	return toRate(r), true, nil
}

// EndRate menutup versi tarif pada tanggal validTo.
func (s *FinanceStore) EndRate(ctx context.Context, id uuid.UUID, validTo clock.BusinessDate, actorID uuid.UUID) error {
	_, err := s.q(ctx).EndTaxRate(ctx, sqlcgen.EndTaxRateParams{ValidTo: date(&validTo), ActorID: actorID, ID: id})
	if err != nil {
		return mapFinanceError(err, "end tax rate")
	}
	return nil
}

// InsertRate menyimpan versi tarif baru.
func (s *FinanceStore) InsertRate(ctx context.Context, r domain.RateVersion, actorID uuid.UUID) (uuid.UUID, error) {
	brackets, err := encodeBrackets(r.Brackets)
	if err != nil {
		return uuid.Nil, err
	}
	base := r.BasePercent
	if base.IsZero() {
		base = decimal.NewFromInt(100)
	}
	id, err := s.q(ctx).InsertTaxRate(ctx, sqlcgen.InsertTaxRateParams{
		TaxType: string(r.Type), ObjectCode: r.ObjectCode, Name: r.Name, RatePercent: r.RatePercent, Brackets: brackets,
		BasePercent: base, NoTaxIDSurchargePercent: r.NoTaxIDSurchargePercent, IsDefault: r.IsDefault, ValidFrom: r.ValidFrom,
		ValidTo: date(r.ValidTo), Note: r.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert tax rate")
	}
	return id, nil
}

func (s *FinanceStore) toWithholding(r sqlcgen.GetWithholdingForUpdateRow) domain.WithholdingRecord {
	return domain.WithholdingRecord{
		ID: r.ID, SourceType: r.SourceType, SourceID: r.SourceID, SourceLabel: r.SourceLabel, BusinessDate: r.BusinessDate,
		Period: tax.Period{Year: int(r.TaxYear), Month: time.Month(r.TaxMonth)}, TaxType: tax.Type(r.TaxType),
		ObjectCode: r.ObjectCode, ObjectName: r.ObjectName, RateID: r.RateID, TaxpayerType: r.TaxpayerType,
		TaxpayerID: uuidPtr(r.TaxpayerID), TaxpayerName: r.TaxpayerName, TaxpayerTaxID: s.decrypt(r.TaxpayerTaxIDEnc),
		HasTaxID: r.HasTaxID, Gross: money.FromDecimal(r.GrossAmount), TaxableBase: money.FromDecimal(r.TaxableBase),
		RatePercent: r.RatePercent, TaxAmount: money.FromDecimal(r.TaxAmount), Status: domain.WithholdingStatus(r.Status),
		DepositID: uuidPtr(r.DepositID), DepositNumber: r.DepositNumber, SlipNumber: r.SlipNumber, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// InsertWithholding menyimpan data bukti potong.
func (s *FinanceStore) InsertWithholding(ctx context.Context, w domain.WithholdingRecord, actorID uuid.UUID) (uuid.UUID, error) {
	enc, err := s.encrypt(w.TaxpayerTaxID)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := s.q(ctx).InsertWithholding(ctx, sqlcgen.InsertWithholdingParams{
		SourceType: w.SourceType, SourceID: w.SourceID, SourceLabel: w.SourceLabel, BusinessDate: w.BusinessDate,
		TaxYear: int32(w.Period.Year), TaxMonth: int32(w.Period.Month), //nolint:gosec // tahun/bulan valid
		TaxType: string(w.TaxType), ObjectCode: w.ObjectCode, ObjectName: w.ObjectName, RateID: w.RateID,
		TaxpayerType: w.TaxpayerType, TaxpayerID: nullUUID(w.TaxpayerID), TaxpayerName: w.TaxpayerName,
		TaxpayerTaxIDEnc: enc, HasTaxID: w.HasTaxID, GrossAmount: w.Gross.Decimal(), TaxableBase: w.TaxableBase.Decimal(),
		RatePercent: w.RatePercent, TaxAmount: w.TaxAmount.Decimal(), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert withholding")
	}
	return id, nil
}

// WithholdingFilter adalah filter rekap bukti potong per masa.
type WithholdingFilter struct {
	Period           tax.Period
	TaxType          string
	IncludeCancelled bool
}

// ListWithholdings mengembalikan bukti potong satu masa.
func (s *FinanceStore) ListWithholdings(ctx context.Context, f WithholdingFilter, page paging.Page) (paging.Result[domain.WithholdingRecord], error) {
	rows, err := s.q(ctx).ListWithholdings(ctx, sqlcgen.ListWithholdingsParams{
		TaxYear: int32(f.Period.Year), TaxMonth: int32(f.Period.Month), //nolint:gosec // tahun/bulan valid
		TaxType: text(f.TaxType), IncludeCancelled: f.IncludeCancelled, Off: page.Offset(), Lim: page.Limit(),
	})
	if err != nil {
		return paging.Result[domain.WithholdingRecord]{}, fmt.Errorf("list withholdings: %w", err)
	}
	out := paging.Result[domain.WithholdingRecord]{Page: page, Items: make([]domain.WithholdingRecord, len(rows))}
	for i, r := range rows {
		out.Items[i] = s.toWithholding(sqlcgen.GetWithholdingForUpdateRow(r))
		out.Total = r.TotalCount
	}
	return out, nil
}

// WithholdingsBySource mengembalikan bukti potong dokumen sumber.
func (s *FinanceStore) WithholdingsBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) ([]domain.WithholdingRecord, error) {
	rows, err := s.q(ctx).WithholdingsBySource(ctx, sqlcgen.WithholdingsBySourceParams{SourceType: sourceType, SourceID: sourceID})
	if err != nil {
		return nil, fmt.Errorf("withholdings by source: %w", err)
	}
	out := make([]domain.WithholdingRecord, len(rows))
	for i, r := range rows {
		out[i] = s.toWithholding(sqlcgen.GetWithholdingForUpdateRow(r))
	}
	return out, nil
}

// GetWithholdingForUpdate mengunci satu bukti potong.
func (s *FinanceStore) GetWithholdingForUpdate(ctx context.Context, id uuid.UUID) (domain.WithholdingRecord, error) {
	r, err := s.q(ctx).GetWithholdingForUpdate(ctx, id)
	if err != nil {
		return domain.WithholdingRecord{}, notFound(err, "get withholding")
	}
	return s.toWithholding(r), nil
}

// SaveWithholding menyimpan status/nomor bukti potong.
func (s *FinanceStore) SaveWithholding(ctx context.Context, w domain.WithholdingRecord, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SaveWithholding(ctx, sqlcgen.SaveWithholdingParams{
		Status: string(w.Status), DepositID: nullUUID(w.DepositID), SlipNumber: w.SlipNumber, ActorID: actorID, ID: w.ID,
		Version: int32(version), //nolint:gosec // versi kecil
	})
	return checkRows(n, err, "save withholding")
}

// OpenWithholdings mengunci bukti potong belum disetor satu jenis satu masa dan mengembalikan ID + total.
func (s *FinanceStore) OpenWithholdings(ctx context.Context, t tax.Type, p tax.Period) ([]uuid.UUID, money.Amount, error) {
	rows, err := s.q(ctx).LockOpenWithholdings(ctx, sqlcgen.LockOpenWithholdingsParams{
		TaxType: string(t), TaxYear: int32(p.Year), TaxMonth: int32(p.Month), //nolint:gosec // tahun/bulan valid
	})
	if err != nil {
		return nil, money.Zero, fmt.Errorf("lock open withholdings: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	total := money.Zero
	for i, r := range rows {
		ids[i] = r.ID
		total = total.Add(money.FromDecimal(r.TaxAmount))
	}
	return ids, total, nil
}

// MarkDeposited menandai bukti potong sudah disetor.
func (s *FinanceStore) MarkDeposited(ctx context.Context, ids []uuid.UUID, depositID, actorID uuid.UUID) error {
	_, err := s.q(ctx).MarkWithholdingsDeposited(ctx, sqlcgen.MarkWithholdingsDepositedParams{
		DepositID: uuid.NullUUID{UUID: depositID, Valid: true}, ActorID: actorID, Ids: ids,
	})
	if err != nil {
		return fmt.Errorf("mark withholdings deposited: %w", err)
	}
	return nil
}

// WithholdingMonth adalah ringkasan bukti potong per jenis per masa.
type WithholdingMonth struct {
	TaxType   tax.Type
	Period    tax.Period
	SlipCount int64
	Open      money.Amount
	Deposited money.Amount
}

// WithholdingSummary meringkas bukti potong setahun.
func (s *FinanceStore) WithholdingSummary(ctx context.Context, year int) ([]WithholdingMonth, error) {
	rows, err := s.q(ctx).WithholdingSummary(ctx, int32(year)) //nolint:gosec // tahun valid
	if err != nil {
		return nil, fmt.Errorf("withholding summary: %w", err)
	}
	out := make([]WithholdingMonth, len(rows))
	for i, r := range rows {
		out[i] = WithholdingMonth{
			TaxType: tax.Type(r.TaxType), Period: tax.Period{Year: int(r.TaxYear), Month: time.Month(r.TaxMonth)},
			SlipCount: r.SlipCount, Open: money.FromDecimal(r.OpenAmount), Deposited: money.FromDecimal(r.DepositedAmount),
		}
	}
	return out, nil
}

// InsertDeposit menyimpan penyetoran pajak.
func (s *FinanceStore) InsertDeposit(ctx context.Context, d domain.TaxDeposit, actorID uuid.UUID) (uuid.UUID, time.Time, error) {
	r, err := s.q(ctx).InsertTaxDeposit(ctx, sqlcgen.InsertTaxDepositParams{
		ID: d.ID, Number: d.Number, TaxType: string(d.TaxType), TaxYear: int32(d.Period.Year), //nolint:gosec // tahun valid
		TaxMonth: int32(d.Period.Month), BankAccountID: d.BankAccountID, DepositDate: d.DepositDate, //nolint:gosec // bulan valid
		Amount: d.Amount.Decimal(), BillingCode: d.BillingCode, Ntpn: d.NTPN, JournalEntryID: d.JournalEntryID, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, time.Time{}, mapFinanceError(err, "insert tax deposit")
	}
	return r.ID, r.CreatedAt, nil
}

// DepositExists: pajak jenis + masa sudah disetor.
func (s *FinanceStore) DepositExists(ctx context.Context, t tax.Type, p tax.Period) (bool, error) {
	_, err := s.q(ctx).TaxDepositByPeriod(ctx, sqlcgen.TaxDepositByPeriodParams{
		TaxType: string(t), TaxYear: int32(p.Year), TaxMonth: int32(p.Month), //nolint:gosec // tahun/bulan valid
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("tax deposit by period: %w", err)
	}
	return true, nil
}

// ListDeposits mengembalikan penyetoran setahun.
func (s *FinanceStore) ListDeposits(ctx context.Context, year int) ([]domain.TaxDeposit, error) {
	rows, err := s.q(ctx).ListTaxDeposits(ctx, int32(year)) //nolint:gosec // tahun valid
	if err != nil {
		return nil, fmt.Errorf("list tax deposits: %w", err)
	}
	out := make([]domain.TaxDeposit, len(rows))
	for i, r := range rows {
		out[i] = domain.TaxDeposit{
			ID: r.ID, Number: r.Number, TaxType: tax.Type(r.TaxType), Period: tax.Period{Year: int(r.TaxYear), Month: time.Month(r.TaxMonth)},
			BankAccountID: r.BankAccountID, BankAccountName: r.BankName, DepositDate: r.DepositDate,
			Amount: money.FromDecimal(r.Amount), BillingCode: r.BillingCode, NTPN: r.Ntpn, JournalEntryID: r.JournalEntryID,
			JournalNumber: strOf(r.JournalNumber), CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		}
	}
	return out, nil
}

// ListFinalTaxSettings mengembalikan riwayat status PPh Final (terbaru dulu).
func (s *FinanceStore) ListFinalTaxSettings(ctx context.Context) ([]domain.FinalTaxSetting, error) {
	rows, err := s.q(ctx).ListFinalTaxSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list final tax settings: %w", err)
	}
	out := make([]domain.FinalTaxSetting, len(rows))
	for i, r := range rows {
		out[i] = domain.FinalTaxSetting{
			ID: r.ID, ValidFrom: r.ValidFrom, IsEnabled: r.IsEnabled, OpeningTurnover: money.FromDecimal(r.OpeningTurnover),
			Note: r.Note, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		}
		if r.OpeningYear.Valid {
			y := int(r.OpeningYear.Int32)
			out[i].OpeningYear = &y
		}
	}
	return out, nil
}

// InsertFinalTaxSetting menyimpan status PPh Final baru.
func (s *FinanceStore) InsertFinalTaxSetting(ctx context.Context, st domain.FinalTaxSetting, actorID uuid.UUID) (uuid.UUID, error) {
	var year pgtype.Int4
	if st.OpeningYear != nil {
		year = pgtype.Int4{Int32: int32(*st.OpeningYear), Valid: true} //nolint:gosec // tahun valid
	}
	r, err := s.q(ctx).InsertFinalTaxSetting(ctx, sqlcgen.InsertFinalTaxSettingParams{
		ValidFrom: st.ValidFrom, IsEnabled: st.IsEnabled, OpeningYear: year, OpeningTurnover: st.OpeningTurnover.Decimal(),
		Note: st.Note, ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, mapFinanceError(err, "insert final tax setting")
	}
	return r.ID, nil
}

// MonthlyRevenue mengembalikan omzet bruto (kredit − debit akun 4xxx) per bulan setahun.
func (s *FinanceStore) MonthlyRevenue(ctx context.Context, year int) (map[time.Month]money.Amount, error) {
	rows, err := s.q(ctx).MonthlyRevenue(ctx, int32(year)) //nolint:gosec // tahun valid
	if err != nil {
		return nil, fmt.Errorf("monthly revenue: %w", err)
	}
	out := map[time.Month]money.Amount{}
	for _, r := range rows {
		out[time.Month(r.Month)] = money.FromDecimal(r.Revenue)
	}
	return out, nil
}

// ListFinalTaxRuns mengembalikan perhitungan PPh Final setahun.
func (s *FinanceStore) ListFinalTaxRuns(ctx context.Context, year int) ([]domain.FinalTaxRun, error) {
	rows, err := s.q(ctx).ListFinalTaxRuns(ctx, int32(year)) //nolint:gosec // tahun valid
	if err != nil {
		return nil, fmt.Errorf("list final tax runs: %w", err)
	}
	out := make([]domain.FinalTaxRun, len(rows))
	for i, r := range rows {
		out[i] = domain.FinalTaxRun{
			ID: r.ID, Period: tax.Period{Year: int(r.TaxYear), Month: time.Month(r.TaxMonth)},
			GrossRevenue: money.FromDecimal(r.GrossRevenueAmount), RateID: r.RateID, RatePercent: r.RatePercent,
			TaxAmount: money.FromDecimal(r.TaxAmount), JournalEntryID: uuidPtr(r.JournalEntryID), JournalNumber: r.JournalNumber,
			DepositID: uuidPtr(r.DepositID), CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
		}
	}
	return out, nil
}

// InsertFinalTaxRun menyimpan perhitungan PPh Final satu masa.
func (s *FinanceStore) InsertFinalTaxRun(ctx context.Context, r domain.FinalTaxRun, actorID uuid.UUID) (uuid.UUID, time.Time, error) {
	row, err := s.q(ctx).InsertFinalTaxRun(ctx, sqlcgen.InsertFinalTaxRunParams{
		ID: r.ID, TaxYear: int32(r.Period.Year), TaxMonth: int32(r.Period.Month), //nolint:gosec // tahun/bulan valid
		GrossRevenueAmount: r.GrossRevenue.Decimal(), RateID: r.RateID, RatePercent: r.RatePercent,
		TaxAmount: r.TaxAmount.Decimal(), JournalEntryID: nullUUID(r.JournalEntryID), ActorID: actorID,
	})
	if err != nil {
		return uuid.Nil, time.Time{}, mapFinanceError(err, "insert final tax run")
	}
	return row.ID, row.CreatedAt, nil
}

// FinalTaxRunForDeposit mengunci perhitungan PPh Final satu masa; ok=false bila belum dihitung.
func (s *FinanceStore) FinalTaxRunForDeposit(ctx context.Context, p tax.Period) (uuid.UUID, money.Amount, bool, bool, error) {
	r, err := s.q(ctx).LockFinalTaxRunForDeposit(ctx, sqlcgen.LockFinalTaxRunForDepositParams{
		TaxYear: int32(p.Year), TaxMonth: int32(p.Month), //nolint:gosec // tahun/bulan valid
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, money.Zero, false, false, nil
	}
	if err != nil {
		return uuid.Nil, money.Zero, false, false, fmt.Errorf("lock final tax run: %w", err)
	}
	return r.ID, money.FromDecimal(r.TaxAmount), true, r.DepositID.Valid, nil
}

// MarkFinalTaxRunDeposited menautkan perhitungan PPh Final ke penyetorannya.
func (s *FinanceStore) MarkFinalTaxRunDeposited(ctx context.Context, runID, depositID uuid.UUID) error {
	if err := s.q(ctx).MarkFinalTaxRunDeposited(ctx, sqlcgen.MarkFinalTaxRunDepositedParams{
		DepositID: uuid.NullUUID{UUID: depositID, Valid: true}, ID: runID,
	}); err != nil {
		return fmt.Errorf("mark final tax run deposited: %w", err)
	}
	return nil
}
