package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/accounting"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
)

// AdvanceFilter adalah kriteria daftar uang muka.
type AdvanceFilter = postgres.AdvanceFilter

// Jenis dokumen sumber jurnal dan pihak sub-ledger.
const (
	SourcePurchaseAdvance = "purchase_advance"
	PartySupplier         = "supplier"
)

var advanceActionPerms = map[domain.Action]string{
	domain.ActionEdit: PermAdvanceWrite, domain.ActionPost: PermAdvancePost,
}

// AdvanceActions mengembalikan aksi uang muka yang boleh dilakukan actor. Batal draft cukup hak tulis;
// batal uang muka Diposting (membalik jurnal) butuh hak posting.
func AdvanceActions(ctx context.Context, a domain.Advance) []string {
	perms := map[domain.Action]string{domain.ActionCancel: PermAdvanceWrite}
	if a.Status == domain.StatusPosted {
		perms[domain.ActionCancel] = PermAdvancePost
	}
	for k, v := range advanceActionPerms {
		perms[k] = v
	}
	return filterActions(ctx, a.Actions(), perms)
}

func advanceSnapshot(a domain.Advance) map[string]any {
	return map[string]any{
		"purchaseOrderId": a.PurchaseOrderID, "amount": a.Amount.String(), "paymentDate": a.PaymentDate.String(),
		"bankAccountCode": a.BankAccountCode, "note": a.Note, "status": string(a.Status),
		"appliedAmount": a.Applied.String(), "cancelReason": a.CancelReason,
	}
}

func advanceLabel(a domain.Advance) string {
	if a.Number != "" {
		return a.Number
	}
	return "Draft uang muka " + a.OrderNumber
}

// otherAdvancesTotal menjumlah uang muka aktif (draft + diposting) PO selain uang muka except.
func otherAdvancesTotal(list []domain.Advance, except uuid.UUID) money.Amount {
	total := money.Zero
	for _, a := range list {
		if a.ID != except && a.Counts() {
			total = total.Add(a.Amount)
		}
	}
	return total
}

// ListAdvances mengembalikan daftar uang muka.
func (s *Service) ListAdvances(ctx context.Context, f AdvanceFilter, page paging.Page) (paging.Result[domain.Advance], error) {
	if _, err := actor.Require(ctx, PermAdvanceRead); err != nil {
		return paging.Result[domain.Advance]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListAdvances(ctx, f, page)
}

// GetAdvance mengembalikan detail uang muka.
func (s *Service) GetAdvance(ctx context.Context, id uuid.UUID) (domain.Advance, error) {
	if _, err := actor.Require(ctx, PermAdvanceRead); err != nil {
		return domain.Advance{}, err
	}
	return s.Store.GetAdvance(ctx, id, false)
}

// CreateAdvance menyimpan draft uang muka untuk PO yang sudah disetujui. Total uang muka tidak boleh melebihi
// total PO.
func (s *Service) CreateAdvance(ctx context.Context, in domain.AdvanceInput) (domain.Advance, error) {
	a, err := actor.Require(ctx, PermAdvanceWrite)
	if err != nil {
		return domain.Advance{}, err
	}
	var adv domain.Advance
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.GetOrder(ctx, in.PurchaseOrderID, true)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return apperr.Validation(map[string]string{"purchaseOrderId": "Purchase Order tidak ditemukan"})
		}
		if err != nil {
			return err
		}
		existing, err := s.Store.AdvancesByOrder(ctx, o.ID)
		if err != nil {
			return err
		}
		if adv, err = domain.NewAdvance(in, o, otherAdvancesTotal(existing, uuid.Nil), a.UserID); err != nil {
			return err
		}
		if adv.ID, err = s.Store.InsertAdvance(ctx, adv, a.UserID); err != nil {
			return err
		}
		if adv, err = s.Store.GetAdvance(ctx, adv.ID, false); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPurchaseAdvance, EntityID: adv.ID, EntityLabel: advanceLabel(adv),
			After: advanceSnapshot(adv),
		})
	})
	return adv, err
}

// mutateAdvance mengunci PO lalu uang muka (urutan kunci sama dengan CreateAdvance), memeriksa versi, menjalankan
// fn, menyimpan, dan mencatat audit trail.
func (s *Service) mutateAdvance(ctx context.Context, id uuid.UUID, version int, action audit.Action,
	fn func(ctx context.Context, adv *domain.Advance, o domain.PurchaseOrder, others money.Amount) (string, error),
) (domain.Advance, error) {
	a, _ := actor.From(ctx)
	var adv domain.Advance
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetAdvance(ctx, id, false)
		if err != nil {
			return err
		}
		o, err := s.Store.GetOrder(ctx, cur.PurchaseOrderID, true)
		if err != nil {
			return err
		}
		if adv, err = s.Store.GetAdvance(ctx, id, true); err != nil {
			return err
		}
		if adv.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		list, err := s.Store.AdvancesByOrder(ctx, o.ID)
		if err != nil {
			return err
		}
		before := advanceSnapshot(adv)
		note, err := fn(ctx, &adv, o, otherAdvancesTotal(list, adv.ID))
		if err != nil {
			return err
		}
		if err := s.Store.SaveAdvance(ctx, adv, version, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityPurchaseAdvance, EntityID: adv.ID, EntityLabel: advanceLabel(adv),
			Before: before, After: advanceSnapshot(adv), Note: note,
		})
	})
	if err != nil {
		return domain.Advance{}, err
	}
	return s.Store.GetAdvance(ctx, id, false)
}

// UpdateAdvance mengubah draft uang muka.
func (s *Service) UpdateAdvance(ctx context.Context, id uuid.UUID, version int, in domain.AdvanceInput) (domain.Advance, error) {
	if _, err := actor.Require(ctx, PermAdvanceWrite); err != nil {
		return domain.Advance{}, err
	}
	return s.mutateAdvance(ctx, id, version, audit.ActionUpdate, func(_ context.Context, adv *domain.Advance, o domain.PurchaseOrder, others money.Amount) (string, error) {
		return "", adv.Update(in, o, others)
	})
}

// PostAdvance memposting uang muka: nomor BYR dan jurnal A-19 (Dr 1401 Uang Muka Pembelian, Cr akun Kas & Bank)
// dengan dimensi divisi PO dan pihak pemasok, di transaksi yang sama.
func (s *Service) PostAdvance(ctx context.Context, id uuid.UUID, version int) (domain.Advance, error) {
	if _, err := actor.Require(ctx, PermAdvancePost); err != nil {
		return domain.Advance{}, err
	}
	return s.mutateAdvance(ctx, id, version, audit.ActionPost, func(ctx context.Context, adv *domain.Advance, o domain.PurchaseOrder, others money.Amount) (string, error) {
		if err := adv.CanPost(o, others); err != nil {
			return "", err
		}
		number, err := s.Numberer.Next(ctx, DocPayment, adv.PaymentDate)
		if err != nil {
			return "", err
		}
		division := adv.DivisionID
		entry, err := s.Ledger.Post(ctx, accounting.PostingRequest{
			RuleCode: RuleAdvance, SourceType: SourcePurchaseAdvance, SourceID: adv.ID, SourceLabel: number,
			BusinessDate: adv.PaymentDate, Amounts: map[string]money.Amount{"advance_value": adv.Amount},
			Accounts:    map[accounting.Resolver]string{accounting.ResolverBank: adv.BankAccountCode},
			Dimensions:  accounting.Dimensions{DivisionID: &division},
			Party:       &accounting.Party{Type: PartySupplier, ID: adv.SupplierID},
			Description: "Uang muka pembelian " + o.Number + " · " + o.SupplierName,
		})
		if err != nil {
			return "", err
		}
		if err := adv.Post(number, entry.ID, s.Clock.Now()); err != nil {
			return "", err
		}
		return "Jurnal " + entry.Number, nil
	})
}

// CancelAdvance membatalkan uang muka. Uang muka Diposting yang belum dipotong tagihan dibatalkan dengan jurnal
// balik A-19 di transaksi yang sama.
func (s *Service) CancelAdvance(ctx context.Context, id uuid.UUID, version int, reason string) (domain.Advance, error) {
	if _, err := actor.RequireAny(ctx, PermAdvanceWrite, PermAdvancePost); err != nil {
		return domain.Advance{}, err
	}
	return s.mutateAdvance(ctx, id, version, audit.ActionCancel, func(ctx context.Context, adv *domain.Advance, _ domain.PurchaseOrder, _ money.Amount) (string, error) {
		posted := adv.Status == domain.StatusPosted
		need := PermAdvanceWrite
		if posted {
			need = PermAdvancePost
		}
		if _, err := actor.Require(ctx, need); err != nil {
			return "", err
		}
		if err := adv.Cancel(reason); err != nil {
			return "", err
		}
		if !posted {
			return adv.CancelReason, nil
		}
		entry, err := s.Ledger.Reverse(ctx, accounting.ReversalRequest{
			SourceType: SourcePurchaseAdvance, SourceID: adv.ID, RuleCode: RuleAdvance, BusinessDate: s.today(),
			Reason: "Pembatalan uang muka " + adv.Number + ": " + adv.CancelReason,
		})
		if err != nil {
			return "", err
		}
		adv.ReversalJournalEntryID = &entry.ID
		return adv.CancelReason + " (jurnal balik " + entry.Number + ")", nil
	})
}
