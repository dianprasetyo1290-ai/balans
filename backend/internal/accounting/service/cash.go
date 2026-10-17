package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/platform/softdelete"
)

// --- Rekening kas & bank (S-401) ---------------------------------------------------------------

func bankSnapshot(b domain.BankAccount) map[string]any {
	return map[string]any{
		"code": b.Code, "name": b.Name, "kind": string(b.Kind), "bankName": b.BankName,
		"accountNumber": b.MaskedNumber(), "accountHolder": b.AccountHolder, "accountCode": b.AccountCode,
		"isActive": b.IsActive, "description": b.Description,
	}
}

// ListBankAccounts mengembalikan rekening kas & bank. Pemegang hak baca pembayaran, rekonsiliasi, kas kecil,
// atau pajak boleh membaca (pilihan rekening).
func (s *Service) ListBankAccounts(ctx context.Context, includeInactive, deleted bool) ([]domain.BankAccount, error) {
	if _, err := actor.RequireAny(ctx, PermPaymentRead, PermBankReconRead, PermPettyCashRead, PermWithholdingRead,
		PermFinalTaxRead, PermExpenseClaimPost); err != nil {
		return nil, err
	}
	return s.Finance.ListBankAccounts(ctx, includeInactive, deleted)
}

// GetBankAccount membaca detail rekening (nomor lengkap).
func (s *Service) GetBankAccount(ctx context.Context, id uuid.UUID) (domain.BankAccount, error) {
	if _, err := actor.Require(ctx, PermPaymentRead); err != nil {
		return domain.BankAccount{}, err
	}
	return s.Finance.GetBankAccount(ctx, id, false)
}

func (s *Service) checkBankGL(ctx context.Context, accountID uuid.UUID) error {
	a, err := s.Store.GetAccount(ctx, accountID, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return apperr.Validation(map[string]string{"accountId": "Akun tidak ditemukan"})
	}
	if err != nil {
		return err
	}
	return domain.ValidateCashBankAccount(a)
}

// CreateBankAccount menambah rekening kas/bank.
func (s *Service) CreateBankAccount(ctx context.Context, in domain.BankAccountInput) (domain.BankAccount, error) {
	a, err := actor.Require(ctx, PermPaymentWrite)
	if err != nil {
		return domain.BankAccount{}, err
	}
	b, err := domain.NewBankAccount(in)
	if err != nil {
		return domain.BankAccount{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.checkBankGL(ctx, b.AccountID); err != nil {
			return err
		}
		if b.ID, err = s.Finance.InsertBankAccount(ctx, b, a.UserID); err != nil {
			return err
		}
		if b, err = s.Finance.GetBankAccount(ctx, b.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityBankAccount, EntityID: b.ID, EntityLabel: b.Code, After: bankSnapshot(b),
		})
	})
	return b, err
}

// UpdateBankAccount mengubah rekening.
func (s *Service) UpdateBankAccount(ctx context.Context, id uuid.UUID, version int, in domain.BankAccountInput) (domain.BankAccount, error) {
	a, err := actor.Require(ctx, PermPaymentWrite)
	if err != nil {
		return domain.BankAccount{}, err
	}
	var b domain.BankAccount
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if b, err = s.Finance.GetBankAccount(ctx, id, true); err != nil {
			return err
		}
		if b.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := bankSnapshot(b)
		oldGL := b.AccountID
		if err := b.Update(in); err != nil {
			return err
		}
		if b.AccountID != oldGL {
			if used, err := s.Finance.BankAccountUsed(ctx, id); err != nil {
				return err
			} else if used {
				return apperr.Validation(map[string]string{"accountId": "Akun GL tidak bisa diganti karena rekening sudah dipakai transaksi"})
			}
			if err := s.checkBankGL(ctx, b.AccountID); err != nil {
				return err
			}
		}
		if err := s.Finance.UpdateBankAccount(ctx, b, version, a.UserID); err != nil {
			return err
		}
		if b, err = s.Finance.GetBankAccount(ctx, id, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityBankAccount, EntityID: b.ID, EntityLabel: b.Code,
			Before: before, After: bankSnapshot(b),
		})
	})
	return b, err
}

// DeleteBankAccount menghapus rekening (soft delete). Rekening yang sudah dipakai sebaiknya dinonaktifkan;
// penghapusan tetap diizinkan karena dokumen lama membaca rekening tanpa filter hapus.
func (s *Service) DeleteBankAccount(ctx context.Context, id uuid.UUID, version int) error {
	a, err := actor.Require(ctx, PermPaymentWrite)
	if err != nil {
		return err
	}
	return s.setBankDeleted(ctx, a, id, version, true)
}

// RestoreBankAccount memulihkan rekening terhapus (Admin Sistem + hak tulis).
func (s *Service) RestoreBankAccount(ctx context.Context, id uuid.UUID, version int) error {
	a, err := softdelete.RequireRestore(ctx, PermPaymentWrite)
	if err != nil {
		return err
	}
	return s.setBankDeleted(ctx, a, id, version, false)
}

func (s *Service) setBankDeleted(ctx context.Context, a actor.Actor, id uuid.UUID, version int, del bool) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		b, err := s.Finance.GetBankAccount(ctx, id, true)
		if err != nil {
			return err
		}
		if b.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		action := audit.ActionRestore
		var at *string
		if del {
			if b.DeletedAt != nil {
				return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Rekening sudah dihapus.")
			}
			now := s.Clock.Now()
			if err := s.Finance.SetBankAccountDeleted(ctx, id, version, &now, a.UserID); err != nil {
				return err
			}
			action = audit.ActionDelete
			v := now.Format("2006-01-02T15:04:05Z07:00")
			at = &v
		} else {
			if b.DeletedAt == nil {
				return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Rekening tidak dalam status dihapus.")
			}
			if err := s.Finance.SetBankAccountDeleted(ctx, id, version, nil, a.UserID); err != nil {
				return err
			}
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityBankAccount, EntityID: id, EntityLabel: b.Code,
			Before: map[string]any{"deletedAt": b.DeletedAt}, After: map[string]any{"deletedAt": at},
		})
	})
}

// usableBank membaca rekening untuk transaksi baru.
func (s *Service) usableBank(ctx context.Context, id uuid.UUID, field string) (domain.BankAccount, error) {
	b, err := s.Finance.GetBankAccount(ctx, id, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return domain.BankAccount{}, apperr.Validation(map[string]string{field: "Rekening tidak ditemukan"})
	}
	if err != nil {
		return domain.BankAccount{}, err
	}
	if msg := b.UsableError(); msg != "" {
		return domain.BankAccount{}, apperr.Validation(map[string]string{field: msg})
	}
	return b, nil
}

// --- Pembayaran (S-401) -----------------------------------------------------------------------

func paymentSnapshot(p domain.Payment) map[string]any {
	return map[string]any{
		"direction": string(p.Direction), "bankAccountCode": p.BankAccountCode, "paymentDate": p.PaymentDate.String(),
		"amount": p.Amount.String(), "counterAccountCode": p.CounterCode, "partyName": p.PartyName, "reference": p.Reference,
		"description": p.Description, "sourceLabel": p.SourceLabel, "status": string(p.Status), "cancelReason": p.CancelReason,
	}
}

func paymentLabel(p domain.Payment) string {
	if p.Number != "" {
		return p.Number
	}
	return "Draft pembayaran " + p.Direction.Label()
}

// PaymentActions mengembalikan aksi pembayaran yang boleh dilakukan actor.
func PaymentActions(ctx context.Context, p domain.Payment) []string {
	cancel := PermPaymentWrite
	if p.Status == domain.StatusPosted {
		cancel = PermPaymentPost
	}
	return filterActions(ctx, p.Actions(), map[domain.Action]string{
		domain.ActionEdit: PermPaymentWrite, domain.ActionPost: PermPaymentPost, domain.ActionCancel: cancel,
	})
}

// PaymentFilter adalah filter daftar pembayaran.
type PaymentFilter = postgres.PaymentFilter

// ListPayments mengembalikan daftar pembayaran.
func (s *Service) ListPayments(ctx context.Context, f PaymentFilter, page paging.Page) (paging.Result[domain.Payment], error) {
	if _, err := actor.Require(ctx, PermPaymentRead); err != nil {
		return paging.Result[domain.Payment]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Finance.ListPayments(ctx, f, page)
}

// GetPayment membaca detail pembayaran.
func (s *Service) GetPayment(ctx context.Context, id uuid.UUID) (domain.Payment, error) {
	if _, err := actor.Require(ctx, PermPaymentRead); err != nil {
		return domain.Payment{}, err
	}
	return s.Finance.GetPayment(ctx, id, false)
}

// preparePayment memeriksa rekening dan akun lawan pembayaran manual.
func (s *Service) preparePayment(ctx context.Context, p *domain.Payment, manual bool) error {
	b, err := s.usableBank(ctx, p.BankAccountID, "bankAccountId")
	if err != nil {
		return err
	}
	counter, err := s.Store.GetAccount(ctx, p.CounterAccountID, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return apperr.Validation(map[string]string{"counterAccountId": "Akun lawan tidak ditemukan"})
	}
	if err != nil {
		return err
	}
	if manual {
		if err := domain.ValidateCounterAccount(counter, b.AccountID); err != nil {
			return err
		}
	} else if msg := counter.AutoPostingError(); msg != "" {
		return apperr.Validation(map[string]string{"counterAccountId": msg})
	}
	p.BankAccountCode, p.BankAccountName, p.BankGLCode = b.Code, b.Name, b.AccountCode
	p.CounterCode, p.CounterName = counter.Code, counter.Name
	return nil
}

// CreatePayment menyimpan draft pembayaran manual (penerimaan/pengeluaran di luar piutang/utang).
func (s *Service) CreatePayment(ctx context.Context, in domain.PaymentInput) (domain.Payment, error) {
	a, err := actor.Require(ctx, PermPaymentWrite)
	if err != nil {
		return domain.Payment{}, err
	}
	in.SourceType, in.SourceID, in.SourceLabel = "", nil, ""
	p, err := domain.NewPayment(in)
	if err != nil {
		return domain.Payment{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.preparePayment(ctx, &p, true); err != nil {
			return err
		}
		if p.ID, err = s.Finance.InsertPayment(ctx, p, a.UserID); err != nil {
			return err
		}
		if err := s.attach(ctx, p.ProofFileID, FileOwnerPaymentProof, p.ID); err != nil {
			return err
		}
		if p, err = s.Finance.GetPayment(ctx, p.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPayment, EntityID: p.ID, EntityLabel: paymentLabel(p), After: paymentSnapshot(p),
		})
	})
	return p, err
}

func (s *Service) mutatePayment(ctx context.Context, id uuid.UUID, version int, action audit.Action,
	fn func(ctx context.Context, p *domain.Payment) (string, error),
) (domain.Payment, error) {
	by := actor.UserID(ctx)
	var p domain.Payment
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if p, err = s.Finance.GetPayment(ctx, id, true); err != nil {
			return err
		}
		if p.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := paymentSnapshot(p)
		wasPosted := p.Status == domain.StatusPosted
		note, err := fn(ctx, &p)
		if err != nil {
			return err
		}
		cancelledAt := s.cancelledAt(p.Status == domain.StatusCancelled)
		if err := s.Finance.SavePayment(ctx, p, version, by, cancelledAt); err != nil {
			return err
		}
		if wasPosted && p.Status == domain.StatusCancelled {
			if _, err := s.Reverse(ctx, ReversalRequest{
				SourceType: domain.SourcePayment, SourceID: p.ID, RuleCode: domain.RulePayment, BusinessDate: s.today(),
				Reason: "Pembatalan " + p.Number + ": " + p.CancelReason,
			}); err != nil {
				return err
			}
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityPayment, EntityID: p.ID, EntityLabel: paymentLabel(p),
			Before: before, After: paymentSnapshot(p), Note: note,
		})
	})
	if err != nil {
		return domain.Payment{}, err
	}
	return s.Finance.GetPayment(ctx, id, false)
}

func (s *Service) cancelledAt(cancelled bool) *time.Time {
	if !cancelled {
		return nil
	}
	now := s.Clock.Now()
	return &now
}

// UpdatePayment mengubah draft pembayaran manual.
func (s *Service) UpdatePayment(ctx context.Context, id uuid.UUID, version int, in domain.PaymentInput) (domain.Payment, error) {
	if _, err := actor.Require(ctx, PermPaymentWrite); err != nil {
		return domain.Payment{}, err
	}
	return s.mutatePayment(ctx, id, version, audit.ActionUpdate, func(ctx context.Context, p *domain.Payment) (string, error) {
		in.SourceType, in.SourceID, in.SourceLabel = p.SourceType, p.SourceID, p.SourceLabel
		if err := p.Update(in); err != nil {
			return "", err
		}
		if err := s.preparePayment(ctx, p, p.SourceID == nil); err != nil {
			return "", err
		}
		return "", s.attach(ctx, p.ProofFileID, FileOwnerPaymentProof, p.ID)
	})
}

// requirePaymentPost memeriksa hak posting pembayaran; pembayaran keluar butuh re-autentikasi (★).
func (s *Service) requirePaymentPost(ctx context.Context, dir domain.PaymentDirection) error {
	if dir == domain.PaymentOutgoing {
		_, err := actor.RequireReauth(ctx, s.Clock.Now(), PermPaymentPost)
		return err
	}
	_, err := actor.Require(ctx, PermPaymentPost)
	return err
}

// PostPayment memposting draft pembayaran: nomor BYR dan jurnal A-PB di transaksi yang sama.
func (s *Service) PostPayment(ctx context.Context, id uuid.UUID, version int) (domain.Payment, error) {
	if _, err := actor.Require(ctx, PermPaymentPost); err != nil {
		return domain.Payment{}, err
	}
	return s.mutatePayment(ctx, id, version, audit.ActionPost, func(ctx context.Context, p *domain.Payment) (string, error) {
		if err := s.requirePaymentPost(ctx, p.Direction); err != nil {
			return "", err
		}
		if err := s.preparePayment(ctx, p, p.SourceID == nil); err != nil {
			return "", err
		}
		return "", s.postPayment(ctx, p)
	})
}

// postPayment memberi nomor BYR dan memposting jurnal A-PB (Dr/Cr akun rekening vs akun lawan).
func (s *Service) postPayment(ctx context.Context, p *domain.Payment) error {
	number, err := s.DocNumbers.NextDocNumber(ctx, DocPayment, p.PaymentDate)
	if err != nil {
		return err
	}
	req := domain.PostingRequest{
		RuleCode: domain.RulePayment, SourceType: domain.SourcePayment, SourceID: p.ID, SourceLabel: number,
		BusinessDate: p.PaymentDate, Amounts: p.Amounts(), Accounts: p.Accounts(),
		Description: strings.TrimSpace(fmt.Sprintf("Pembayaran %s %s %s", strings.ToLower(p.Direction.Label()), number, p.Description)),
	}
	if p.PartyID != nil {
		req.Party = &domain.Party{Type: p.PartyType, ID: *p.PartyID}
	}
	entry, err := s.Post(ctx, req)
	if err != nil {
		return err
	}
	return p.Post(number, entry.ID, s.Clock.Now(), actor.UserID(ctx))
}

// CancelPayment membatalkan draft, atau pembayaran manual Diposting yang belum direkonsiliasi (jurnal dibalik).
func (s *Service) CancelPayment(ctx context.Context, id uuid.UUID, version int, reason string) (domain.Payment, error) {
	if _, err := actor.Require(ctx, PermPaymentWrite); err != nil {
		return domain.Payment{}, err
	}
	return s.mutatePayment(ctx, id, version, audit.ActionCancel, func(ctx context.Context, p *domain.Payment) (string, error) {
		if p.Status == domain.StatusPosted {
			if _, err := actor.Require(ctx, PermPaymentPost); err != nil {
				return "", err
			}
		}
		return reason, p.Cancel(reason)
	})
}

// PaymentRequest adalah pembayaran yang dicatat modul lain (pelunasan tagihan pemasok, invoice, pengajuan biaya)
// lewat RecordPayment. Akun lawan berupa kode (mis. 2101 utang usaha, 1201 piutang usaha, 2103 utang biaya).
type PaymentRequest struct {
	Direction          domain.PaymentDirection
	BankAccountID      uuid.UUID
	PaymentDate        clock.BusinessDate
	Amount             money.Amount
	CounterAccountCode string
	PartyType          string
	PartyID            *uuid.UUID
	PartyName          string
	Reference          string
	Description        string
	ProofFileID        *uuid.UUID
	SourceType         string
	SourceID           uuid.UUID
	SourceLabel        string
}

// RecordPayment membuat dan memposting pembayaran dokumen modul lain di transaksi aktif (nomor BYR, jurnal A-PB).
// Pembayaran keluar butuh re-autentikasi `accounting.payment.post` (★); pembayaran masuk butuh hak posting
// pembayaran. Pembayaran ini tidak bisa dibatalkan dari menu Kas & Bank (dibatalkan dari dokumen sumbernya).
func (s *Service) RecordPayment(ctx context.Context, req PaymentRequest) (domain.Payment, error) {
	if err := s.requirePaymentPost(ctx, req.Direction); err != nil {
		return domain.Payment{}, err
	}
	return s.recordPayment(ctx, req)
}

// recordPayment tanpa cek permission (rekonsiliasi bank memeriksa hak posting rekonsiliasinya sendiri).
func (s *Service) recordPayment(ctx context.Context, req PaymentRequest) (domain.Payment, error) {
	if strings.TrimSpace(req.SourceType) == "" || req.SourceID == uuid.Nil {
		return domain.Payment{}, apperr.Validation(map[string]string{"sourceId": "Dokumen sumber pembayaran wajib diisi"})
	}
	by := actor.UserID(ctx)
	var p domain.Payment
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		accounts, err := s.Store.AccountsByCodes(ctx, []string{req.CounterAccountCode})
		if err != nil {
			return err
		}
		counter, ok := accounts[req.CounterAccountCode]
		if !ok {
			return apperr.Validation(map[string]string{"counterAccountId": "Akun " + req.CounterAccountCode + " tidak ditemukan"})
		}
		sourceID := req.SourceID
		if p, err = domain.NewPayment(domain.PaymentInput{
			Direction: req.Direction, BankAccountID: req.BankAccountID, PaymentDate: req.PaymentDate, Amount: req.Amount,
			CounterAccountID: counter.ID, PartyType: req.PartyType, PartyID: req.PartyID, PartyName: req.PartyName,
			Reference: req.Reference, Description: req.Description, ProofFileID: req.ProofFileID,
			SourceType: req.SourceType, SourceID: &sourceID, SourceLabel: req.SourceLabel,
		}); err != nil {
			return err
		}
		if err := s.preparePayment(ctx, &p, false); err != nil {
			return err
		}
		if p.ID, err = s.Finance.InsertPayment(ctx, p, by); err != nil {
			return err
		}
		if err := s.attach(ctx, p.ProofFileID, FileOwnerPaymentProof, p.ID); err != nil {
			return err
		}
		if err := s.postPayment(ctx, &p); err != nil {
			return err
		}
		if err := s.Finance.SavePayment(ctx, p, 1, by, nil); err != nil {
			return err
		}
		if p, err = s.Finance.GetPayment(ctx, p.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: EntityPayment, EntityID: p.ID, EntityLabel: p.Number, After: paymentSnapshot(p),
			Note: "Pembayaran " + req.SourceLabel,
		})
	})
	return p, err
}

// --- Mutasi rekening --------------------------------------------------------------------------

// BankMutationResult adalah mutasi satu rekening pada rentang tanggal.
type BankMutationResult struct {
	Account domain.BankAccount
	Opening money.Amount
	Items   []domain.BankMutation
	Page    paging.Page
	Total   int64
}

// BankMutations mengembalikan mutasi rekening (Buku Besar akun GL rekening) dengan saldo berjalan.
func (s *Service) BankMutations(ctx context.Context, bankAccountID uuid.UUID, from, to clock.BusinessDate, page paging.Page) (BankMutationResult, error) {
	if _, err := actor.RequireAny(ctx, PermPaymentRead, PermBankReconRead); err != nil {
		return BankMutationResult{}, err
	}
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return BankMutationResult{}, apperr.Validation(map[string]string{"dateTo": "Rentang tanggal tidak valid"})
	}
	b, err := s.Finance.GetBankAccount(ctx, bankAccountID, false)
	if err != nil {
		return BankMutationResult{}, err
	}
	opening, total, items, err := s.Finance.BankMutations(ctx, b.AccountID, from, to, page)
	if err != nil {
		return BankMutationResult{}, err
	}
	return BankMutationResult{Account: b, Opening: opening, Items: items, Page: page, Total: total}, nil
}
