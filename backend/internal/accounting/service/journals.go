package service

import (
	"context"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

func entryLabel(e domain.Entry) string {
	if e.Number != "" {
		return e.Number
	}
	return "Draft Jurnal Umum " + e.BusinessDate.String()
}

func entrySnapshot(e domain.Entry) map[string]any {
	lines := make([]map[string]any, len(e.Lines))
	for i, l := range e.Lines {
		lines[i] = map[string]any{
			"accountId": l.AccountID, "debit": l.Debit.String(), "credit": l.Credit.String(), "memo": l.Memo,
			"channelId": l.Dimensions.ChannelID, "subChannelId": l.Dimensions.SubChannelID,
			"divisionId": l.Dimensions.DivisionID, "campaignId": l.Dimensions.CampaignID,
		}
	}
	return map[string]any{
		"number": e.Number, "businessDate": e.BusinessDate.String(), "description": e.Description, "status": e.Status,
		"totalAmount": e.TotalAmount.String(), "lines": lines,
	}
}

// validateManualLines memeriksa akun boleh dipakai Jurnal Umum dan dimensi ada/aktif. requireDims memeriksa
// dimensi wajib per kelompok akun (saat posting). Kode & nama akun diisi ke baris.
func (s *Service) validateManualLines(ctx context.Context, e *domain.Entry, requireDims bool) error {
	ids := make([]uuid.UUID, 0, len(e.Lines))
	for _, l := range e.Lines {
		ids = append(ids, l.AccountID)
	}
	accounts, err := s.Store.AccountsByIDs(ctx, ids)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	for i := range e.Lines {
		l := &e.Lines[i]
		prefix := fmt.Sprintf("lines.%d.", i)
		acc, ok := accounts[l.AccountID]
		if !ok {
			fields[prefix+"accountId"] = "Akun tidak ditemukan"
			continue
		}
		l.AccountCode, l.AccountName = acc.Code, acc.Name
		if msg := acc.ManualPostingError(); msg != "" {
			fields[prefix+"accountId"] = msg
			continue
		}
		if requireDims {
			maps.Copy(fields, l.Dimensions.Missing(acc.ManualRequiredDimensions(), prefix, "akun "+acc.Code))
		}
		dimErrs, err := s.checkDimensions(ctx, l.Dimensions, e.BusinessDate, prefix)
		if err != nil {
			return err
		}
		maps.Copy(fields, dimErrs)
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// CreateManualJournal menyimpan draft Jurnal Umum (S-127). Nomor JV diberikan saat posting.
func (s *Service) CreateManualJournal(ctx context.Context, in domain.ManualInput) (domain.Entry, error) {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return domain.Entry{}, err
	}
	e, err := domain.NewManual(in)
	if err != nil {
		return domain.Entry{}, err
	}
	if e.ID, err = uuid.NewV7(); err != nil {
		return domain.Entry{}, fmt.Errorf("new journal id: %w", err)
	}
	e.SourceID = e.ID
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.CheckPeriodOpen(ctx, e.BusinessDate); err != nil {
			return err
		}
		if err := s.validateManualLines(ctx, &e, false); err != nil {
			return err
		}
		if err := s.Store.InsertEntry(ctx, e, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "journal_entry", EntityID: e.ID, EntityLabel: entryLabel(e), After: entrySnapshot(e),
		})
	})
	if err != nil {
		return domain.Entry{}, err
	}
	return s.Store.GetEntry(ctx, e.ID)
}

// lockedEntry mengunci jurnal dan memeriksa versi yang dilihat user.
func (s *Service) lockedEntry(ctx context.Context, id uuid.UUID, version int) (domain.Entry, error) {
	if _, err := s.Store.LockEntry(ctx, id); err != nil {
		return domain.Entry{}, err
	}
	e, err := s.Store.GetEntry(ctx, id)
	if err != nil {
		return domain.Entry{}, err
	}
	if version != 0 && e.Version != version {
		return domain.Entry{}, apperr.New(apperr.CodeVersionConflict)
	}
	return e, nil
}

// UpdateManualJournal mengganti isi draft Jurnal Umum. Jurnal terposting ditolak (ACC-13).
func (s *Service) UpdateManualJournal(ctx context.Context, id uuid.UUID, version int, in domain.ManualInput) (domain.Entry, error) {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return domain.Entry{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		e, err := s.lockedEntry(ctx, id, version)
		if err != nil {
			return err
		}
		before := e
		if err := e.UpdateManual(in); err != nil {
			return err
		}
		if err := s.CheckPeriodOpen(ctx, e.BusinessDate); err != nil {
			return err
		}
		if err := s.validateManualLines(ctx, &e, false); err != nil {
			return err
		}
		if err := s.Store.ReplaceDraft(ctx, e, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "journal_entry", EntityID: e.ID, EntityLabel: entryLabel(e),
			Before: entrySnapshot(before), After: entrySnapshot(e),
		})
	})
	if err != nil {
		return domain.Entry{}, err
	}
	return s.Store.GetEntry(ctx, id)
}

// DeleteManualJournal menghapus (soft delete) draft Jurnal Umum. Jurnal terposting ditolak (ACC-13).
func (s *Service) DeleteManualJournal(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return err
	}
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		e, err := s.lockedEntry(ctx, id, version)
		if err != nil {
			return err
		}
		if err := e.CheckEditable(); err != nil {
			return err
		}
		if err := s.Store.DeleteDraft(ctx, id, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: "journal_entry", EntityID: e.ID, EntityLabel: entryLabel(e), Before: entrySnapshot(e),
		})
	})
}

// post memberi nomor JV dan memposting draft di periode terbuka (dipakai Jurnal Umum, jurnal otomatis, Balik Jurnal).
func (s *Service) post(ctx context.Context, e *domain.Entry, version int, by uuid.UUID) error {
	period, err := s.OpenPeriod(ctx, e.BusinessDate)
	if err != nil {
		return err
	}
	if err := e.CheckBalanced(); err != nil {
		return err
	}
	number, err := s.Numberer.NextJournalNumber(ctx, e.BusinessDate)
	if err != nil {
		return err
	}
	if err := e.Post(number, period.ID, s.Clock.Now(), by); err != nil {
		return err
	}
	return s.Store.PostEntry(ctx, *e, version, by)
}

// PostManualJournal memposting draft Jurnal Umum: balance, periode terbuka, dimensi wajib, nomor JV (S-127).
func (s *Service) PostManualJournal(ctx context.Context, id uuid.UUID, version int) (domain.Entry, error) {
	a, err := actor.Require(ctx, PermJournalPost)
	if err != nil {
		return domain.Entry{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		e, err := s.lockedEntry(ctx, id, version)
		if err != nil {
			return err
		}
		if err := e.CheckEditable(); err != nil {
			return err
		}
		if err := s.validateManualLines(ctx, &e, true); err != nil {
			return err
		}
		if err := s.post(ctx, &e, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: "journal_entry", EntityID: e.ID, EntityLabel: e.Number,
			Before: map[string]any{"status": domain.EntryDraft}, After: map[string]any{"status": e.Status, "number": e.Number},
		})
	})
	if err != nil {
		return domain.Entry{}, err
	}
	return s.Store.GetEntry(ctx, id)
}

// reverse membuat dan memposting jurnal pembalik untuk jurnal terposting (S-128).
func (s *Service) reverse(ctx context.Context, original domain.Entry, date clock.BusinessDate, reason string) (domain.Entry, error) {
	r, err := original.NewReversal(date, reason)
	if err != nil {
		return domain.Entry{}, err
	}
	if r.ID, err = uuid.NewV7(); err != nil {
		return domain.Entry{}, fmt.Errorf("new journal id: %w", err)
	}
	by := actor.UserID(ctx)
	if err := s.Store.InsertEntry(ctx, r, by); err != nil {
		return domain.Entry{}, err
	}
	if err := s.post(ctx, &r, 1, by); err != nil {
		return domain.Entry{}, err
	}
	if err := s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionPost, EntityType: "journal_entry", EntityID: r.ID, EntityLabel: r.Number, After: entrySnapshot(r),
		Note: "Balik Jurnal " + original.Number,
	}); err != nil {
		return domain.Entry{}, err
	}
	if err := s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionCancel, EntityType: "journal_entry", EntityID: original.ID, EntityLabel: original.Number,
		Note: "Dibalik oleh " + r.Number + ": " + reason,
	}); err != nil {
		return domain.Entry{}, err
	}
	return r, nil
}

// ReverseManualJournal menjalankan Balik Jurnal untuk Jurnal Umum yang sudah diposting. Jurnal otomatis dibalik
// lewat pembatalan dokumen sumbernya (Poster.Reverse).
func (s *Service) ReverseManualJournal(ctx context.Context, id uuid.UUID, version int, date clock.BusinessDate, reason string) (domain.Entry, error) {
	if _, err := actor.Require(ctx, PermJournalReverse); err != nil {
		return domain.Entry{}, err
	}
	var reversal domain.Entry
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		e, err := s.lockedEntry(ctx, id, version)
		if err != nil {
			return err
		}
		if e.EntryType != domain.EntryManual {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Jurnal otomatis dibalik dengan membatalkan dokumen sumbernya ("+e.SourceLabel+").")
		}
		reversal, err = s.reverse(ctx, e, date, reason)
		return err
	})
	if err != nil {
		return domain.Entry{}, err
	}
	return s.Store.GetEntry(ctx, reversal.ID)
}

// GetJournal mengembalikan jurnal beserta barisnya.
func (s *Service) GetJournal(ctx context.Context, id uuid.UUID) (domain.Entry, error) {
	if _, err := actor.Require(ctx, PermJournalRead); err != nil {
		return domain.Entry{}, err
	}
	return s.Store.GetEntry(ctx, id)
}

// ListJournals mengembalikan daftar jurnal.
func (s *Service) ListJournals(ctx context.Context, f postgres.EntryFilter, page paging.Page) (paging.Result[domain.Entry], error) {
	if _, err := actor.Require(ctx, PermJournalRead); err != nil {
		return paging.Result[domain.Entry]{}, err
	}
	return s.Store.ListEntries(ctx, f, page)
}

// DimensionNames membaca kode & nama dimensi untuk tampilan.
func (s *Service) DimensionNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]postgres.DimensionName, error) {
	return s.Store.DimensionNames(ctx, ids)
}
