package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// Tipe file mutasi yang diterima.
const (
	ContentTypeCSV  = "text/csv"
	ContentTypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	// SourceStatementLine adalah source_type pembayaran yang dibuat dari baris mutasi tak cocok.
	SourceStatementLine = "bank_statement_line"
	// candidateWindowDays adalah rentang tanggal kandidat di sekitar periode file mutasi.
	candidateWindowDays = 31
)

// StatementDetail adalah file mutasi beserta barisnya.
type StatementDetail struct {
	Summary domain.StatementSummary
	Lines   []domain.StatementLine
}

// readStatementRows membaca file CSV (pemisah koma, titik koma, atau tab) atau sheet pertama Excel.
func readStatementRows(f FileContent) ([][]string, error) {
	name := strings.ToLower(f.Name)
	if f.ContentType == ContentTypeXLSX || strings.HasSuffix(name, ".xlsx") {
		book, err := excelize.OpenReader(bytes.NewReader(f.Data), excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, apperr.Validation(map[string]string{"fileId": "File Excel tidak bisa dibaca"})
		}
		defer func() { _ = book.Close() }()
		sheets := book.GetSheetList()
		if len(sheets) == 0 {
			return nil, apperr.Validation(map[string]string{"fileId": "File Excel tidak berisi sheet"})
		}
		rows, err := book.GetRows(sheets[0], excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, apperr.Validation(map[string]string{"fileId": "Sheet mutasi tidak bisa dibaca"})
		}
		return rows, nil
	}
	data := bytes.TrimPrefix(f.Data, []byte("\xef\xbb\xbf"))
	first, _, _ := bytes.Cut(data, []byte("\n"))
	sep := ','
	for _, c := range []rune{';', '\t', '|'} {
		if bytes.Count(first, []byte(string(c))) > bytes.Count(first, []byte(string(sep))) {
			sep = c
		}
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes = sep, -1, true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, apperr.Validation(map[string]string{"fileId": "File CSV tidak valid: " + err.Error()})
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

// ImportStatement membaca file mutasi rekening (CSV/Excel) yang sudah diunggah ke filestore, menyimpan
// barisnya, lalu menjalankan pencocokan otomatis. File yang sama tidak bisa diimpor dua kali.
func (s *Service) ImportStatement(ctx context.Context, bankAccountID, fileID uuid.UUID) (StatementDetail, error) {
	a, err := actor.Require(ctx, PermBankReconImport)
	if err != nil {
		return StatementDetail{}, err
	}
	var id uuid.UUID
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		b, err := s.usableBank(ctx, bankAccountID, "bankAccountId")
		if err != nil {
			return err
		}
		if b.Kind != domain.BankKindBank {
			return apperr.Validation(map[string]string{"bankAccountId": "Rekonsiliasi hanya untuk rekening bank"})
		}
		content, err := s.Files.Read(ctx, fileID, FileOwnerBankStatement)
		if err != nil {
			return err
		}
		rows, err := readStatementRows(content)
		if err != nil {
			return err
		}
		parsed, err := domain.ParseStatementRows(rows)
		if err != nil {
			return err
		}
		sum := domain.NewStatementSummary(parsed.Rows)
		sum.BankAccountID, sum.FileID, sum.FileName = b.ID, fileID, content.Name
		if id, err = s.Finance.InsertStatement(ctx, sum, parsed.Rows, a.UserID); err != nil {
			return err
		}
		if err := s.Files.Attach(ctx, fileID, FileOwnerBankStatement, id); err != nil {
			return err
		}
		if _, err := s.suggest(ctx, id); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionImport, EntityType: EntityBankStatement, EntityID: id, EntityLabel: content.Name,
			After: map[string]any{
				"bankAccountCode": b.Code, "periodFrom": sum.PeriodFrom.String(), "periodTo": sum.PeriodTo.String(),
				"lineCount": sum.LineCount, "skipped": parsed.Skipped, "totalIn": sum.TotalIn.String(), "totalOut": sum.TotalOut.String(),
			},
		})
	})
	if err != nil {
		return StatementDetail{}, err
	}
	return s.statementDetail(ctx, id)
}

// ListStatements mengembalikan file mutasi yang sudah diimpor.
func (s *Service) ListStatements(ctx context.Context, bankAccountID *uuid.UUID, page paging.Page) (paging.Result[domain.StatementSummary], error) {
	if _, err := actor.Require(ctx, PermBankReconRead); err != nil {
		return paging.Result[domain.StatementSummary]{}, err
	}
	return s.Finance.ListStatements(ctx, bankAccountID, page)
}

// GetStatement membaca file mutasi beserta baris dan usulan pencocokannya.
func (s *Service) GetStatement(ctx context.Context, id uuid.UUID) (StatementDetail, error) {
	if _, err := actor.Require(ctx, PermBankReconRead); err != nil {
		return StatementDetail{}, err
	}
	return s.statementDetail(ctx, id)
}

func (s *Service) statementDetail(ctx context.Context, id uuid.UUID) (StatementDetail, error) {
	sum, err := s.Finance.GetStatement(ctx, id)
	if err != nil {
		return StatementDetail{}, err
	}
	lines, err := s.Finance.StatementLines(ctx, id, false)
	if err != nil {
		return StatementDetail{}, err
	}
	return StatementDetail{Summary: sum, Lines: lines}, nil
}

// AutoMatch menjalankan ulang pencocokan otomatis untuk baris yang belum direkonsiliasi.
func (s *Service) AutoMatch(ctx context.Context, statementID uuid.UUID) (StatementDetail, error) {
	if _, err := actor.Require(ctx, PermBankReconWrite); err != nil {
		return StatementDetail{}, err
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		_, err := s.suggest(ctx, statementID)
		return err
	})
	if err != nil {
		return StatementDetail{}, err
	}
	return s.statementDetail(ctx, statementID)
}

// candidates mengumpulkan baris jurnal rekening yang belum direkonsiliasi dan dokumen terbuka dari penyedia.
func (s *Service) candidates(ctx context.Context, b domain.BankAccount, from, to clock.BusinessDate, dirs ...domain.PaymentDirection) ([]domain.MatchCandidate, error) {
	out, err := s.Finance.UnreconciledBankLines(ctx, b.AccountID, from.AddDays(-candidateWindowDays), to.AddDays(candidateWindowDays))
	if err != nil {
		return nil, err
	}
	for _, p := range s.allProviders() {
		for _, dir := range dirs {
			list, err := p.PaymentCandidates(ctx, CandidateQuery{BankAccountID: b.ID, Direction: dir, DateFrom: from, DateTo: to})
			if err != nil {
				return nil, fmt.Errorf("kandidat %s: %w", p.Source(), err)
			}
			for _, c := range list {
				out = append(out, documentCandidate(c))
			}
		}
	}
	return out, nil
}

func documentCandidate(c PaymentCandidate) domain.MatchCandidate {
	amount := c.OpenAmount
	if c.Direction == domain.PaymentOutgoing {
		amount = amount.Neg()
	}
	label := c.DocumentLabel
	if c.PartyName != "" {
		label += " · " + c.PartyName
	}
	return domain.MatchCandidate{
		Kind: domain.MatchDocument, Source: c.Source, DocumentID: c.DocumentID, Label: label, Date: c.DocumentDate,
		Amount: amount, References: append([]string{c.DocumentLabel}, c.References...),
	}
}

// suggest menghapus usulan lama baris file mutasi lalu menyimpan usulan baru (Technical Guideline 5.8.5).
func (s *Service) suggest(ctx context.Context, statementID uuid.UUID) (int, error) {
	sum, err := s.Finance.GetStatement(ctx, statementID)
	if err != nil {
		return 0, err
	}
	b, err := s.Finance.GetBankAccount(ctx, sum.BankAccountID, false)
	if err != nil {
		return 0, err
	}
	lines, err := s.Finance.StatementLines(ctx, statementID, true)
	if err != nil {
		return 0, err
	}
	open := make([]domain.StatementLine, 0, len(lines))
	for i := range lines {
		if lines[i].Status == domain.LineMatched {
			continue
		}
		if lines[i].Status == domain.LineSuggested {
			lines[i].ClearSuggestion()
			if err := s.Finance.SaveStatementLine(ctx, lines[i]); err != nil {
				return 0, err
			}
			lines[i].Version++
		}
		open = append(open, lines[i])
	}
	if len(open) == 0 {
		return 0, nil
	}
	cands, err := s.candidates(ctx, b, sum.PeriodFrom, sum.PeriodTo, domain.PaymentIncoming, domain.PaymentOutgoing)
	if err != nil {
		return 0, err
	}
	suggestions := domain.SuggestMatches(open, cands, domain.DefaultMatchToleranceDays)
	for _, sg := range suggestions {
		i := slices.IndexFunc(open, func(l domain.StatementLine) bool { return l.ID == sg.LineID })
		l := open[i]
		if err := l.Suggest(sg.Candidate, sg.Rule); err != nil {
			return 0, err
		}
		if err := s.Finance.SaveStatementLine(ctx, l); err != nil {
			return 0, err
		}
	}
	return len(suggestions), nil
}

// MatchTarget adalah pasangan manual baris mutasi: baris jurnal rekening atau dokumen penyedia kandidat.
type MatchTarget struct {
	JournalLineID *uuid.UUID
	Source        string
	DocumentID    *uuid.UUID
}

// ConfirmLine mengonfirmasi usulan pencocokan satu baris.
func (s *Service) ConfirmLine(ctx context.Context, lineID uuid.UUID, version int) (domain.StatementLine, error) {
	if _, err := actor.Require(ctx, PermBankReconPost); err != nil {
		return domain.StatementLine{}, err
	}
	return s.mutateLine(ctx, lineID, version, func(ctx context.Context, l *domain.StatementLine) (string, error) {
		if l.Status != domain.LineSuggested {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, "Baris mutasi tidak punya usulan pencocokan.")
		}
		target := MatchTarget{JournalLineID: l.JournalLineID, Source: l.CandidateSource, DocumentID: l.CandidateDocID}
		return "Konfirmasi usulan " + string(l.MatchRule), s.applyMatch(ctx, l, target, l.MatchRule)
	})
}

// ConfirmSuggestions mengonfirmasi semua usulan satu file mutasi; mengembalikan jumlah baris yang dikonfirmasi.
func (s *Service) ConfirmSuggestions(ctx context.Context, statementID uuid.UUID) (int, error) {
	if _, err := actor.Require(ctx, PermBankReconPost); err != nil {
		return 0, err
	}
	n := 0
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		lines, err := s.Finance.StatementLines(ctx, statementID, true)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if l.Status != domain.LineSuggested {
				continue
			}
			if _, err := s.ConfirmLine(ctx, l.ID, l.Version); err != nil {
				return fmt.Errorf("baris %d: %w", l.LineNo, err)
			}
			n++
		}
		return nil
	})
	return n, err
}

// MatchLine mencocokkan baris mutasi secara manual dengan baris jurnal rekening atau dokumen terbuka.
func (s *Service) MatchLine(ctx context.Context, lineID uuid.UUID, version int, target MatchTarget) (domain.StatementLine, error) {
	if _, err := actor.Require(ctx, PermBankReconPost); err != nil {
		return domain.StatementLine{}, err
	}
	if (target.JournalLineID == nil) == (target.DocumentID == nil) {
		return domain.StatementLine{}, apperr.Validation(map[string]string{"journalLineId": "Pilih satu baris jurnal atau satu dokumen"})
	}
	return s.mutateLine(ctx, lineID, version, func(ctx context.Context, l *domain.StatementLine) (string, error) {
		if l.Status == domain.LineMatched {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, "Baris mutasi sudah direkonsiliasi.")
		}
		return "Pencocokan manual", s.applyMatch(ctx, l, target, domain.RuleManual)
	})
}

// applyMatch mengonfirmasi pasangan: baris jurnal langsung ditandai; dokumen dibayar lewat pembayaran baru
// (A-PB) dan penyedia mencatat pelunasannya.
func (s *Service) applyMatch(ctx context.Context, l *domain.StatementLine, target MatchTarget, rule domain.MatchRule) error {
	b, err := s.Finance.GetBankAccount(ctx, l.BankAccountID, false)
	if err != nil {
		return err
	}
	now, by := s.Clock.Now(), actor.UserID(ctx)
	if target.JournalLineID != nil {
		jl, accountID, err := s.Finance.JournalLine(ctx, *target.JournalLineID)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return apperr.Validation(map[string]string{"journalLineId": "Baris jurnal tidak ditemukan"})
		}
		if err != nil {
			return err
		}
		if accountID != b.AccountID {
			return apperr.Validation(map[string]string{"journalLineId": "Baris jurnal bukan mutasi akun rekening " + b.Code})
		}
		if !jl.Amount.Equal(l.Amount) {
			return apperr.Validation(map[string]string{"journalLineId": "Nominal jurnal " + jl.Amount.Rupiah() + " berbeda dengan mutasi " + l.Amount.Rupiah()})
		}
		return l.Confirm(domain.MatchJournalLine, rule, jl.JournalLineID, jl.JournalEntryID, nil, jl.Label, now, by)
	}
	p, ok := s.provider(target.Source)
	if !ok {
		return apperr.Validation(map[string]string{"source": "Penyedia dokumen " + target.Source + " tidak dikenal"})
	}
	c, open, err := p.PaymentCandidate(ctx, *target.DocumentID)
	if err != nil {
		return err
	}
	if !open {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Dokumen sudah tidak menunggu pembayaran.")
	}
	dir, amount := domain.PaymentIncoming, l.Amount
	if amount.IsNegative() {
		dir, amount = domain.PaymentOutgoing, amount.Neg()
	}
	if c.Direction != dir {
		return apperr.Validation(map[string]string{"documentId": "Arah dokumen tidak sesuai dengan mutasi"})
	}
	if amount.Cmp(c.OpenAmount) > 0 {
		return apperr.Validation(map[string]string{"documentId": "Mutasi " + amount.Rupiah() + " melebihi sisa dokumen " + c.OpenAmount.Rupiah()})
	}
	pay, err := s.recordPayment(ctx, PaymentRequest{
		Direction: dir, BankAccountID: b.ID, PaymentDate: l.TransactionDate, Amount: amount,
		CounterAccountCode: c.CounterAccountCode, PartyType: c.PartyType, PartyID: c.PartyID, PartyName: c.PartyName,
		Reference: l.Reference, Description: "Rekonsiliasi bank " + c.DocumentLabel, SourceType: c.Source,
		SourceID: c.DocumentID, SourceLabel: c.DocumentLabel,
	})
	if err != nil {
		return err
	}
	if err := p.SettlePayment(ctx, c, pay); err != nil {
		return err
	}
	jl, err := s.Finance.BankLineOfEntry(ctx, *pay.JournalEntryID, b.AccountID)
	if err != nil {
		return err
	}
	l.CandidateSource, l.CandidateDocID = c.Source, &c.DocumentID
	return l.Confirm(domain.MatchDocument, rule, jl.JournalLineID, jl.JournalEntryID, &pay.ID, c.DocumentLabel+" · "+pay.Number, now, by)
}

// NewEntryInput adalah jurnal baru dari baris mutasi tak cocok (biaya bank, bunga, setoran tunai, dll.).
type NewEntryInput struct {
	CounterAccountID uuid.UUID
	Description      string
}

// CreateEntryFromLine membuat pembayaran Diposting (A-PB) dari baris mutasi yang tidak punya pasangan, lalu
// menandai baris direkonsiliasi. Tidak butuh re-autentikasi karena mencatat mutasi yang sudah terjadi di bank.
func (s *Service) CreateEntryFromLine(ctx context.Context, lineID uuid.UUID, version int, in NewEntryInput) (domain.StatementLine, error) {
	if _, err := actor.Require(ctx, PermBankReconPost); err != nil {
		return domain.StatementLine{}, err
	}
	return s.mutateLine(ctx, lineID, version, func(ctx context.Context, l *domain.StatementLine) (string, error) {
		if l.Status == domain.LineMatched {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, "Baris mutasi sudah direkonsiliasi.")
		}
		b, err := s.usableBank(ctx, l.BankAccountID, "bankAccountId")
		if err != nil {
			return "", err
		}
		counter, err := s.Store.GetAccount(ctx, in.CounterAccountID, false)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return "", apperr.Validation(map[string]string{"counterAccountId": "Akun lawan tidak ditemukan"})
		}
		if err != nil {
			return "", err
		}
		if err := domain.ValidateCounterAccount(counter, b.AccountID); err != nil {
			return "", err
		}
		dir, amount := domain.PaymentIncoming, l.Amount
		if amount.IsNegative() {
			dir, amount = domain.PaymentOutgoing, amount.Neg()
		}
		desc := strings.TrimSpace(in.Description)
		if desc == "" {
			desc = l.Description
		}
		label := fmt.Sprintf("Mutasi %s baris %d", l.TransactionDate.String(), l.LineNo)
		pay, err := s.recordPayment(ctx, PaymentRequest{
			Direction: dir, BankAccountID: b.ID, PaymentDate: l.TransactionDate, Amount: amount,
			CounterAccountCode: counter.Code, Reference: l.Reference, Description: truncateText(desc, 500),
			SourceType: SourceStatementLine, SourceID: l.ID, SourceLabel: label,
		})
		if err != nil {
			return "", err
		}
		jl, err := s.Finance.BankLineOfEntry(ctx, *pay.JournalEntryID, b.AccountID)
		if err != nil {
			return "", err
		}
		return "Jurnal baru " + pay.Number, l.Confirm(domain.MatchNewEntry, domain.RuleManual, jl.JournalLineID, jl.JournalEntryID,
			&pay.ID, pay.Number+" · "+counter.Code+" "+counter.Name, s.Clock.Now(), actor.UserID(ctx))
	})
}

func truncateText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// UnmatchLine melepas rekonsiliasi baris. Pembayaran yang sudah dibuat saat konfirmasi tetap Diposting dan
// barisnya bisa dicocokkan ulang.
func (s *Service) UnmatchLine(ctx context.Context, lineID uuid.UUID, version int) (domain.StatementLine, error) {
	if _, err := actor.Require(ctx, PermBankReconPost); err != nil {
		return domain.StatementLine{}, err
	}
	return s.mutateLine(ctx, lineID, version, func(_ context.Context, l *domain.StatementLine) (string, error) {
		label := l.CandidateLabel
		return "Lepas pencocokan " + label, l.Unmatch()
	})
}

func lineSnapshot(l domain.StatementLine) map[string]any {
	return map[string]any{
		"lineNo": l.LineNo, "transactionDate": l.TransactionDate.String(), "amount": l.Amount.String(),
		"status": string(l.Status), "matchKind": string(l.MatchKind), "matchRule": string(l.MatchRule),
		"label": l.CandidateLabel,
	}
}

func (s *Service) mutateLine(ctx context.Context, lineID uuid.UUID, version int,
	fn func(ctx context.Context, l *domain.StatementLine) (string, error),
) (domain.StatementLine, error) {
	var l domain.StatementLine
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if l, err = s.Finance.GetStatementLine(ctx, lineID, true); err != nil {
			return err
		}
		if l.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := lineSnapshot(l)
		note, err := fn(ctx, &l)
		if err != nil {
			return err
		}
		if err := s.Finance.SaveStatementLine(ctx, l); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityBankStatement, EntityID: l.StatementID,
			EntityLabel: fmt.Sprintf("Baris %d", l.LineNo), Before: before, After: lineSnapshot(l), Note: note,
		})
	})
	if err != nil {
		return domain.StatementLine{}, err
	}
	return s.Finance.GetStatementLine(ctx, lineID, false)
}

// LineCandidates mengembalikan calon pasangan manual satu baris mutasi: baris jurnal rekening searah yang belum
// direkonsiliasi (±31 hari) dan dokumen terbuka searah dari penyedia kandidat.
func (s *Service) LineCandidates(ctx context.Context, lineID uuid.UUID) ([]domain.MatchCandidate, error) {
	if _, err := actor.Require(ctx, PermBankReconRead); err != nil {
		return nil, err
	}
	l, err := s.Finance.GetStatementLine(ctx, lineID, false)
	if err != nil {
		return nil, err
	}
	b, err := s.Finance.GetBankAccount(ctx, l.BankAccountID, false)
	if err != nil {
		return nil, err
	}
	dir := domain.PaymentIncoming
	if l.Amount.IsNegative() {
		dir = domain.PaymentOutgoing
	}
	all, err := s.candidates(ctx, b, l.TransactionDate, l.TransactionDate, dir)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MatchCandidate, 0, len(all))
	for _, c := range all {
		if c.Amount.IsNegative() == l.Amount.IsNegative() {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b domain.MatchCandidate) int {
		da, db := absDiff(a.Amount, l.Amount), absDiff(b.Amount, l.Amount)
		return da.Cmp(db)
	})
	if len(out) > 100 {
		out = out[:100]
	}
	return out, nil
}

func absDiff(a, b money.Amount) money.Amount {
	d := a.Sub(b)
	if d.IsNegative() {
		return d.Neg()
	}
	return d
}

// --- Penyedia kandidat bawaan: pengajuan biaya Menunggu Pembayaran ----------------------------

// SourceClaimCandidates adalah nama penyedia kandidat pengajuan biaya.
const SourceClaimCandidates = "accounting.expense_claim"

type claimCandidates struct{ s *Service }

func (c claimCandidates) Source() string { return SourceClaimCandidates }

func (c claimCandidates) toCandidate(p postgres.ClaimPayable) PaymentCandidate {
	out := PaymentCandidate{
		Source: SourceClaimCandidates, DocumentID: p.ID, DocumentLabel: p.Number, Direction: domain.PaymentOutgoing,
		PartyName: p.PayeeName, OpenAmount: p.Amount, DocumentDate: p.VerifyDate, CounterAccountCode: domain.ExpensePayableCode,
	}
	if p.PayeeType == string(domain.PayeeSupplier) && p.PayeeID != nil {
		out.PartyType, out.PartyID = string(domain.PayeeSupplier), p.PayeeID
	}
	return out
}

func (c claimCandidates) PaymentCandidates(ctx context.Context, q CandidateQuery) ([]PaymentCandidate, error) {
	if q.Direction != domain.PaymentOutgoing {
		return nil, nil
	}
	list, err := c.s.Finance.ClaimsAwaitingPayment(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := make([]PaymentCandidate, len(list))
	for i, p := range list {
		out[i] = c.toCandidate(p)
	}
	return out, nil
}

func (c claimCandidates) PaymentCandidate(ctx context.Context, id uuid.UUID) (PaymentCandidate, bool, error) {
	list, err := c.s.Finance.ClaimsAwaitingPayment(ctx, &id)
	if err != nil || len(list) == 0 {
		return PaymentCandidate{}, false, err
	}
	return c.toCandidate(list[0]), true, nil
}

func (c claimCandidates) SettlePayment(ctx context.Context, cand PaymentCandidate, p domain.Payment) error {
	if !p.Amount.Equal(cand.OpenAmount) {
		return apperr.Validation(map[string]string{"documentId": "Pengajuan biaya harus dibayar penuh " + cand.OpenAmount.Rupiah()})
	}
	return c.s.markClaimPaid(ctx, cand.DocumentID, p)
}
