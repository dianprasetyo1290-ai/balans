package service

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// PettyCashFilter adalah filter daftar transaksi kas kecil.
type PettyCashFilter = postgres.PettyCashFilter

// PettyCashLimits adalah batas kas kecil yang berlaku (konstanta sampai modul setting tersedia).
type PettyCashLimits struct {
	MaxPerTransaction money.Amount
	DefaultCeiling    money.Amount
}

// CurrentPettyCashLimits mengembalikan batas kas kecil.
func CurrentPettyCashLimits() PettyCashLimits {
	return PettyCashLimits{MaxPerTransaction: domain.PettyCashMaxPerTransaction, DefaultCeiling: domain.PettyCashDefaultCeiling}
}

func fundSnapshot(f domain.PettyCashFund) map[string]any {
	return map[string]any{
		"code": f.Code, "name": f.Name, "holderUserId": f.HolderUserID, "divisionName": f.DivisionName,
		"ceiling": f.Ceiling.String(), "balance": f.Balance.String(), "isActive": f.IsActive, "description": f.Description,
	}
}

func pettySnapshot(t domain.PettyCashTxn) map[string]any {
	return map[string]any{
		"type": string(t.Type), "fundCode": t.FundCode, "businessDate": t.BusinessDate.String(), "amount": t.Amount.String(),
		"expenseTypeCode": t.ExpenseTypeCode, "divisionName": t.DivisionName, "description": t.Description,
		"status": string(t.Status), "cancelReason": t.CancelReason,
	}
}

// PettyCashActions mengembalikan aksi transaksi kas kecil yang boleh dilakukan actor.
func PettyCashActions(ctx context.Context, t domain.PettyCashTxn) []string {
	return filterActions(ctx, t.Actions(), map[domain.Action]string{
		domain.ActionEdit: PermPettyCashWrite, domain.ActionSubmit: PermPettyCashWrite, domain.ActionRevise: PermPettyCashWrite,
		domain.ActionCancel: PermPettyCashWrite, domain.ActionPost: PermPettyCashPost,
	})
}

func (s *Service) withHolderNames(ctx context.Context, funds []domain.PettyCashFund) []domain.PettyCashFund {
	ids := make([]uuid.UUID, len(funds))
	for i, f := range funds {
		ids[i] = f.HolderUserID
	}
	names := s.names(ctx, ids)
	for i := range funds {
		funds[i].HolderName = names[funds[i].HolderUserID]
	}
	return funds
}

// ListPettyCashFunds mengembalikan dana kas kecil beserta saldo (rekap saldo imprest).
func (s *Service) ListPettyCashFunds(ctx context.Context, includeInactive bool) ([]domain.PettyCashFund, error) {
	if _, err := actor.Require(ctx, PermPettyCashRead); err != nil {
		return nil, err
	}
	funds, err := s.Finance.ListFunds(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	return s.withHolderNames(ctx, funds), nil
}

// CreatePettyCashFund menambah dana kas kecil bersaldo nol.
func (s *Service) CreatePettyCashFund(ctx context.Context, in domain.FundInput) (domain.PettyCashFund, error) {
	a, err := actor.Require(ctx, PermPettyCashWrite)
	if err != nil {
		return domain.PettyCashFund{}, err
	}
	f, err := domain.NewFund(in)
	if err != nil {
		return domain.PettyCashFund{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.CheckDivision(ctx, f.DivisionID, s.today(), "divisionId"); err != nil {
			return err
		}
		if f.ID, err = s.Finance.InsertFund(ctx, f, a.UserID); err != nil {
			return err
		}
		if f, err = s.Finance.GetFund(ctx, f.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPettyCashFund, EntityID: f.ID, EntityLabel: f.Code, After: fundSnapshot(f),
		})
	})
	if err != nil {
		return domain.PettyCashFund{}, err
	}
	return s.withHolderNames(ctx, []domain.PettyCashFund{f})[0], nil
}

// UpdatePettyCashFund mengubah dana (plafon tidak boleh di bawah saldo).
func (s *Service) UpdatePettyCashFund(ctx context.Context, id uuid.UUID, version int, in domain.FundInput) (domain.PettyCashFund, error) {
	a, err := actor.Require(ctx, PermPettyCashWrite)
	if err != nil {
		return domain.PettyCashFund{}, err
	}
	var f domain.PettyCashFund
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if f, err = s.Finance.GetFund(ctx, id, true); err != nil {
			return err
		}
		if f.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := fundSnapshot(f)
		if err := f.Update(in); err != nil {
			return err
		}
		if err := s.CheckDivision(ctx, f.DivisionID, s.today(), "divisionId"); err != nil {
			return err
		}
		if err := s.Finance.SaveFund(ctx, f, version, a.UserID); err != nil {
			return err
		}
		if f, err = s.Finance.GetFund(ctx, id, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityPettyCashFund, EntityID: f.ID, EntityLabel: f.Code,
			Before: before, After: fundSnapshot(f),
		})
	})
	if err != nil {
		return domain.PettyCashFund{}, err
	}
	return s.withHolderNames(ctx, []domain.PettyCashFund{f})[0], nil
}

// ListPettyCash mengembalikan transaksi kas kecil.
func (s *Service) ListPettyCash(ctx context.Context, f PettyCashFilter, page paging.Page) (paging.Result[domain.PettyCashTxn], error) {
	if _, err := actor.Require(ctx, PermPettyCashRead); err != nil {
		return paging.Result[domain.PettyCashTxn]{}, err
	}
	return s.Finance.ListPettyCash(ctx, f, page)
}

// GetPettyCash membaca transaksi kas kecil.
func (s *Service) GetPettyCash(ctx context.Context, id uuid.UUID) (domain.PettyCashTxn, error) {
	if _, err := actor.Require(ctx, PermPettyCashRead); err != nil {
		return domain.PettyCashTxn{}, err
	}
	t, err := s.Finance.GetPettyCash(ctx, id, false)
	if err != nil {
		return domain.PettyCashTxn{}, err
	}
	if t.ReceiptFileID != nil {
		t.ReceiptFileName = s.fileNames(ctx, []uuid.UUID{*t.ReceiptFileID})[*t.ReceiptFileID]
	}
	return t, nil
}

// preparePettyCash memeriksa divisi, kampanye, dan jenis biaya transaksi.
func (s *Service) preparePettyCash(ctx context.Context, t *domain.PettyCashTxn) (domain.ExpenseType, error) {
	if err := s.CheckDivision(ctx, t.DivisionID, t.BusinessDate, "divisionId"); err != nil {
		return domain.ExpenseType{}, err
	}
	if t.CampaignID != nil {
		check, err := s.Store.CheckDimensions(ctx, domain.Dimensions{CampaignID: t.CampaignID}, t.BusinessDate)
		if err != nil {
			return domain.ExpenseType{}, err
		}
		if !check.CampaignOK {
			return domain.ExpenseType{}, apperr.Validation(map[string]string{"campaignId": "Kampanye tidak ditemukan atau tidak berlaku"})
		}
	}
	if t.ExpenseTypeID == nil {
		return domain.ExpenseType{}, nil
	}
	et, err := s.Store.GetExpenseType(ctx, *t.ExpenseTypeID, false)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return domain.ExpenseType{}, apperr.Validation(map[string]string{"expenseTypeId": "Jenis biaya tidak ditemukan"})
	}
	if err != nil {
		return domain.ExpenseType{}, err
	}
	if msg := et.UsableError(); msg != "" {
		return domain.ExpenseType{}, apperr.Validation(map[string]string{"expenseTypeId": msg})
	}
	t.ExpenseTypeCode, t.ExpenseTypeName, t.ExpenseAccountCode = et.Code, et.Name, et.ExpenseAccountCode
	return et, nil
}

// CreatePettyCash menyimpan draft pengeluaran atau pengisian kembali kas kecil. Divisi kosong = divisi dana;
// nominal pengisian kosong = selisih plafon dan saldo.
func (s *Service) CreatePettyCash(ctx context.Context, in domain.PettyCashInput) (domain.PettyCashTxn, error) {
	a, err := actor.Require(ctx, PermPettyCashWrite)
	if err != nil {
		return domain.PettyCashTxn{}, err
	}
	var t domain.PettyCashTxn
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		f, err := s.Finance.GetFund(ctx, in.FundID, false)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return apperr.Validation(map[string]string{"fundId": "Dana kas kecil tidak ditemukan"})
		}
		if err != nil {
			return err
		}
		if !f.IsActive {
			return apperr.Validation(map[string]string{"fundId": "Dana kas kecil " + f.Code + " tidak aktif"})
		}
		if in.DivisionID == uuid.Nil {
			in.DivisionID = f.DivisionID
		}
		if in.Type == domain.PettyCashReplenishment && in.Amount.IsZero() {
			in.Amount = f.Shortfall()
		}
		if t, err = domain.NewPettyCashTxn(in); err != nil {
			return err
		}
		if _, err := s.preparePettyCash(ctx, &t); err != nil {
			return err
		}
		if t.ID, err = s.Finance.InsertPettyCash(ctx, t, a.UserID); err != nil {
			return err
		}
		if err := s.attach(ctx, t.ReceiptFileID, FileOwnerPettyCash, t.ID); err != nil {
			return err
		}
		if t, err = s.Finance.GetPettyCash(ctx, t.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPettyCashTxn, EntityID: t.ID, EntityLabel: t.Label(), After: pettySnapshot(t),
		})
	})
	return t, err
}

func (s *Service) mutatePettyCash(ctx context.Context, id uuid.UUID, version int, action audit.Action,
	fn func(ctx context.Context, t *domain.PettyCashTxn) (string, error),
) (domain.PettyCashTxn, error) {
	by := actor.UserID(ctx)
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Finance.GetPettyCash(ctx, id, false)
		if err != nil {
			return err
		}
		// Urutan kunci: dana lalu transaksi (sama dengan posting).
		if _, err := s.Finance.GetFund(ctx, cur.FundID, true); err != nil {
			return err
		}
		t, err := s.Finance.GetPettyCash(ctx, id, true)
		if err != nil {
			return err
		}
		if version >= 0 && t.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := pettySnapshot(t)
		note, err := fn(ctx, &t)
		if err != nil {
			return err
		}
		if err := s.Finance.SavePettyCash(ctx, t, t.Version, by); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityPettyCashTxn, EntityID: t.ID, EntityLabel: t.Label(),
			Before: before, After: pettySnapshot(t), Note: note,
		})
	})
	if err != nil {
		return domain.PettyCashTxn{}, err
	}
	return s.GetPettyCashUnchecked(ctx, id)
}

// GetPettyCashUnchecked membaca transaksi kas kecil tanpa cek permission.
func (s *Service) GetPettyCashUnchecked(ctx context.Context, id uuid.UUID) (domain.PettyCashTxn, error) {
	return s.Finance.GetPettyCash(ctx, id, false)
}

// UpdatePettyCash mengubah draft transaksi kas kecil.
func (s *Service) UpdatePettyCash(ctx context.Context, id uuid.UUID, version int, in domain.PettyCashInput) (domain.PettyCashTxn, error) {
	if _, err := actor.Require(ctx, PermPettyCashWrite); err != nil {
		return domain.PettyCashTxn{}, err
	}
	return s.mutatePettyCash(ctx, id, version, audit.ActionUpdate, func(ctx context.Context, t *domain.PettyCashTxn) (string, error) {
		if in.DivisionID == uuid.Nil {
			in.DivisionID = t.DivisionID
		}
		if err := t.Update(in); err != nil {
			return "", err
		}
		if _, err := s.preparePettyCash(ctx, t); err != nil {
			return "", err
		}
		return "", s.attach(ctx, t.ReceiptFileID, FileOwnerPettyCash, t.ID)
	})
}

// SubmitPettyCash mengajukan pengisian kembali untuk approval `petty_cash_replenishment` (Head F&A).
func (s *Service) SubmitPettyCash(ctx context.Context, id uuid.UUID, version int) (domain.PettyCashTxn, error) {
	if _, err := actor.Require(ctx, PermPettyCashWrite); err != nil {
		return domain.PettyCashTxn{}, err
	}
	return s.mutatePettyCash(ctx, id, version, audit.ActionSubmit, func(ctx context.Context, t *domain.PettyCashTxn) (string, error) {
		f, err := s.Finance.GetFund(ctx, t.FundID, false)
		if err != nil {
			return "", err
		}
		if err := t.CheckReplenishment(f); err != nil {
			return "", err
		}
		if err := t.Submit(); err != nil {
			return "", err
		}
		division := f.DivisionID
		return "", s.Approvals.RequestApproval(ctx, ApprovalSubject{
			Type: domain.ApprovalSubjectPettyCashReplenishment, ID: t.ID, Label: "Pengisian " + f.Code + " " + t.BusinessDate.String(),
			Link: "/finance/petty-cash?txn=" + t.ID.String(), Summary: f.Name + " · saldo " + f.Balance.Rupiah() + " / plafon " + f.Ceiling.Rupiah(),
			Amount: t.Amount, Division: &division,
		})
	})
}

// RevisePettyCash mengembalikan pengisian ditolak menjadi draft.
func (s *Service) RevisePettyCash(ctx context.Context, id uuid.UUID, version int) (domain.PettyCashTxn, error) {
	if _, err := actor.Require(ctx, PermPettyCashWrite); err != nil {
		return domain.PettyCashTxn{}, err
	}
	return s.mutatePettyCash(ctx, id, version, audit.ActionUpdate, func(_ context.Context, t *domain.PettyCashTxn) (string, error) {
		return "Revisi setelah ditolak", t.Revise()
	})
}

// CancelPettyCash membatalkan transaksi yang belum diposting.
func (s *Service) CancelPettyCash(ctx context.Context, id uuid.UUID, version int, reason string) (domain.PettyCashTxn, error) {
	if _, err := actor.Require(ctx, PermPettyCashWrite); err != nil {
		return domain.PettyCashTxn{}, err
	}
	return s.mutatePettyCash(ctx, id, version, audit.ActionCancel, func(_ context.Context, t *domain.PettyCashTxn) (string, error) {
		return reason, t.Cancel(reason)
	})
}

// OnPettyCashDecided dipanggil handler approval `petty_cash_replenishment`.
func (s *Service) OnPettyCashDecided(ctx context.Context, id uuid.UUID, approved bool, note string) error {
	action := audit.ActionApprove
	if !approved {
		action = audit.ActionReject
	}
	_, err := s.mutatePettyCash(ctx, id, -1, action, func(_ context.Context, t *domain.PettyCashTxn) (string, error) {
		if approved {
			return note, t.Approve(note)
		}
		return note, t.Reject(note)
	})
	return err
}

// PostPettyCashInput adalah isian posting: rekening sumber untuk pengisian kembali.
type PostPettyCashInput struct {
	BankAccountID *uuid.UUID
}

// PostPettyCash memposting pengeluaran (A-17: Dr beban / Cr 1102; bukti wajib; maks. per transaksi) atau
// pengisian kembali yang sudah disetujui (A-16: Dr 1102 / Cr bank; re-autentikasi karena dana keluar dari bank).
// Saldo dana diperbarui di transaksi yang sama.
func (s *Service) PostPettyCash(ctx context.Context, id uuid.UUID, version int, in PostPettyCashInput) (domain.PettyCashTxn, error) {
	if _, err := actor.Require(ctx, PermPettyCashPost); err != nil {
		return domain.PettyCashTxn{}, err
	}
	return s.mutatePettyCash(ctx, id, version, audit.ActionPost, func(ctx context.Context, t *domain.PettyCashTxn) (string, error) {
		f, err := s.Finance.GetFund(ctx, t.FundID, false)
		if err != nil {
			return "", err
		}
		et, err := s.preparePettyCash(ctx, t)
		if err != nil {
			return "", err
		}
		division := t.DivisionID
		req := domain.PostingRequest{
			SourceType: domain.SourcePettyCash, SourceID: t.ID, BusinessDate: t.BusinessDate,
			Dimensions: domain.Dimensions{DivisionID: &division, CampaignID: t.CampaignID},
		}
		if t.Type == domain.PettyCashExpense {
			if err := t.CheckExpense(f, et, domain.PettyCashMaxPerTransaction); err != nil {
				return "", err
			}
			req.RuleCode = domain.RulePettyCashExpense
			req.Amounts = map[string]money.Amount{domain.AmountExpense: t.Amount}
			req.Accounts = map[domain.Resolver]string{domain.ResolverExpenseType: et.ExpenseAccountCode}
			f.Balance = f.Balance.Sub(t.Amount)
		} else {
			if _, err := actor.RequireReauth(ctx, s.Clock.Now(), PermPettyCashPost); err != nil {
				return "", err
			}
			if in.BankAccountID == nil {
				return "", apperr.Validation(map[string]string{"bankAccountId": "Rekening sumber wajib dipilih"})
			}
			b, err := s.usableBank(ctx, *in.BankAccountID, "bankAccountId")
			if err != nil {
				return "", err
			}
			if err := t.CheckReplenishment(f); err != nil {
				return "", err
			}
			req.RuleCode = domain.RulePettyCashReplenish
			req.Amounts = map[string]money.Amount{domain.AmountReplenish: t.Amount}
			req.Accounts = map[domain.Resolver]string{domain.ResolverBank: b.AccountCode}
			req.Dimensions.CampaignID = nil
			t.BankAccountID = &b.ID
			f.Balance = f.Balance.Add(t.Amount)
		}
		number, err := s.DocNumbers.NextDocNumber(ctx, DocPettyCash, t.BusinessDate)
		if err != nil {
			return "", err
		}
		req.SourceLabel = number
		req.Description = truncateText(number+" "+f.Code+" "+t.Description, 300)
		entry, err := s.Post(ctx, req)
		if err != nil {
			return "", err
		}
		if err := t.Post(number, entry.ID, s.Clock.Now(), actor.UserID(ctx)); err != nil {
			return "", err
		}
		if err := s.Finance.SaveFund(ctx, f, f.Version, actor.UserID(ctx)); err != nil {
			return "", err
		}
		return "Jurnal " + entry.Number + ", saldo " + f.Balance.Rupiah(), nil
	})
}
