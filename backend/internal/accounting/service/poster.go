package service

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Post membuat jurnal otomatis dari posting rule (Technical Guideline 5.8.1). Dipanggil modul sumber di dalam
// transaksi bisnisnya; tidak memeriksa permission karena modul sumber sudah memeriksa aksi user.
// Ditolak bila: rule tidak dikenal/nonaktif, jurnal tidak balance, dimensi wajib kosong atau tidak valid,
// akun nonaktif/akun induk, periode tertutup (PERIOD_LOCKED), atau dokumen sumber sudah dijurnal untuk rule
// yang sama (ErrAlreadyPosted, uq_journal_entries_source).
func (s *Service) Post(ctx context.Context, req domain.PostingRequest) (domain.Entry, error) {
	if err := req.Validate(); err != nil {
		return domain.Entry{}, err
	}
	var e domain.Entry
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		rule, err := s.Store.GetPostingRule(ctx, req.RuleCode, req.BusinessDate)
		if err != nil {
			return err
		}
		planned, err := rule.Plan(req)
		if err != nil {
			return err
		}
		if _, exists, err := s.Store.EntryIDBySource(ctx, req.SourceType, req.SourceID, rule.Code); err != nil {
			return err
		} else if exists {
			return &apperr.Error{
				Code: apperr.CodeInvalidStateTransition, Err: postgres.ErrAlreadyPosted,
				Message: fmt.Sprintf("Dokumen %s sudah dijurnal dengan rule %s.", req.SourceLabel, rule.Code),
			}
		}
		lines, err := s.resolvePlannedLines(ctx, planned, req)
		if err != nil {
			return err
		}
		description := strings.TrimSpace(req.Description)
		if description == "" {
			description = strings.TrimSpace(rule.Name + " " + req.SourceLabel)
		}
		if e.ID, err = uuid.NewV7(); err != nil {
			return fmt.Errorf("new journal id: %w", err)
		}
		e.BusinessDate, e.EntryType, e.Status = req.BusinessDate, domain.EntryAutomatic, domain.EntryDraft
		e.SourceType, e.SourceID, e.SourceLabel = req.SourceType, req.SourceID, req.SourceLabel
		e.PostingRuleCode, e.Description, e.Lines = rule.Code, description, lines
		e.TotalAmount, _ = e.Totals()

		by := actor.UserID(ctx)
		if err := s.Store.InsertEntry(ctx, e, by); err != nil {
			return err
		}
		if err := s.post(ctx, &e, 1, by); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: "journal_entry", EntityID: e.ID, EntityLabel: e.Number, After: entrySnapshot(e),
			Note: fmt.Sprintf("Jurnal otomatis %s dari %s %s", rule.Code, req.SourceType, req.SourceLabel),
		})
	})
	if err != nil {
		return domain.Entry{}, err
	}
	return e, nil
}

// resolvePlannedLines mengubah kode akun menjadi akun yang boleh diposting dan memeriksa dimensi di DB.
func (s *Service) resolvePlannedLines(ctx context.Context, planned []domain.PlannedLine, req domain.PostingRequest) ([]domain.Line, error) {
	codes := make([]string, 0, len(planned))
	for _, p := range planned {
		codes = append(codes, p.AccountCode)
	}
	accounts, err := s.Store.AccountsByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	fields := map[string]string{}
	lines := make([]domain.Line, len(planned))
	for i, p := range planned {
		acc, ok := accounts[p.AccountCode]
		switch {
		case !ok:
			fields["accounts."+p.AmountKey] = "Akun " + p.AccountCode + " tidak ditemukan"
		case acc.AutoPostingError() != "":
			fields["accounts."+p.AmountKey] = acc.AutoPostingError()
		}
		l := domain.Line{
			LineNo: i + 1, AccountID: acc.ID, AccountCode: acc.Code, AccountName: acc.Name, Dimensions: req.Dimensions,
			Party: req.Party, AmountKey: p.AmountKey, Memo: p.Memo, Debit: money.Zero, Credit: money.Zero,
		}
		if p.Side == domain.SideDebit {
			l.Debit = p.Amount
		} else {
			l.Credit = p.Amount
		}
		lines[i] = l
	}
	dimErrs, err := s.checkDimensions(ctx, req.Dimensions, req.BusinessDate, "dimensions.")
	if err != nil {
		return nil, err
	}
	maps.Copy(fields, dimErrs)
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	return lines, nil
}

// ReversalRequest membatalkan jurnal otomatis dokumen sumber, mis. saat GR atau invoice dibatalkan
// (Technical Guideline 5.8.3).
type ReversalRequest struct {
	SourceType   string
	SourceID     uuid.UUID
	RuleCode     string
	BusinessDate clock.BusinessDate
	Reason       string
}

// Reverse membuat jurnal pembalik untuk jurnal otomatis dokumen sumber di transaksi aktif.
func (s *Service) Reverse(ctx context.Context, req ReversalRequest) (domain.Entry, error) {
	var r domain.Entry
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		id, ok, err := s.Store.EntryIDBySource(ctx, req.SourceType, req.SourceID, req.RuleCode)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.WithMessage(apperr.CodeNotFound, fmt.Sprintf("Jurnal rule %s untuk dokumen sumber tidak ditemukan.", req.RuleCode))
		}
		original, err := s.lockedEntry(ctx, id, 0)
		if err != nil {
			return err
		}
		r, err = s.reverse(ctx, original, req.BusinessDate, req.Reason)
		return err
	})
	return r, err
}

// PostingRules mengembalikan posting rule dan pemetaan akun yang berlaku hari ini (halaman Posting Rule).
func (s *Service) PostingRules(ctx context.Context) ([]domain.PostingRule, map[string][]postgres.RuleAccountView, error) {
	if _, err := actor.Require(ctx, PermPostingRuleRead); err != nil {
		return nil, nil, err
	}
	return s.Store.ListPostingRules(ctx, clock.Today(s.Clock))
}
