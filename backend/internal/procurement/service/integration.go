package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/procurement/domain"
)

// API untuk Penerimaan Barang (S-206/S-207) dan Tagihan Pemasok (S-208). Dipanggil di dalam transaksi dokumen
// pemanggil; tidak memeriksa permission karena pemanggil memeriksa aksi user-nya sendiri.

// Order membaca PO beserta baris dan bahan maklon tanpa memeriksa permission.
func (s *Service) Order(ctx context.Context, id uuid.UUID) (domain.PurchaseOrder, error) {
	return s.Store.GetOrder(ctx, id, false)
}

// Receipt membaca Penerimaan Barang tanpa memeriksa permission (modul Inbound/QC).
func (s *Service) Receipt(ctx context.Context, id uuid.UUID) (domain.GoodsReceipt, error) {
	return s.Store.GetReceipt(ctx, id, false)
}

// ReceivableOrder mengembalikan PO yang bisa diterima (Disetujui atau Diterima Sebagian) beserta baris, sisa qty,
// bahan maklon, dan lokasi maklon. INVALID_STATE_TRANSITION bila PO belum disetujui atau sudah selesai.
func (s *Service) ReceivableOrder(ctx context.Context, id uuid.UUID) (domain.PurchaseOrder, error) {
	o, err := s.Store.GetOrder(ctx, id, false)
	if err != nil {
		return domain.PurchaseOrder{}, err
	}
	if !o.Receivable() {
		return domain.PurchaseOrder{}, apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
			"Purchase Order %s berstatus %s; hanya PO Disetujui atau Diterima Sebagian yang bisa diterima.",
			orderLabel(o), o.Status.Label()))
	}
	return o, nil
}

// applyLineQty mengunci PO, menerapkan perubahan qty, menyimpan, menghitung ulang PR, dan mencatat audit.
func (s *Service) applyLineQty(ctx context.Context, id uuid.UUID, note string, fn func(o *domain.PurchaseOrder) error) (domain.PurchaseOrder, error) {
	var o domain.PurchaseOrder
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if o, err = s.Store.GetOrder(ctx, id, true); err != nil {
			return err
		}
		version, before := o.Version, o.Status
		if err := fn(&o); err != nil {
			return err
		}
		if err := s.Store.SaveOrderLineQty(ctx, o.Lines); err != nil {
			return err
		}
		if err := s.Store.SaveOrder(ctx, o, version, actor.UserID(ctx), false); err != nil {
			return err
		}
		o.Version = version + 1
		if o.Status == before {
			return nil
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityPurchaseOrder, EntityID: o.ID, EntityLabel: o.Number,
			Before: map[string]any{"status": string(before)}, After: map[string]any{"status": string(o.Status)}, Note: note,
		})
	})
	return o, err
}

// RecordReceipt menambah (atau, untuk pembatalan penerimaan, mengurangi dengan qty negatif) qty diterima per
// baris PO dan menyesuaikan status Diterima Sebagian/Diterima. Toleransi qty 0%.
func (s *Service) RecordReceipt(ctx context.Context, id uuid.UUID, receiptLabel string, deltas []domain.QtyDelta) (domain.PurchaseOrder, error) {
	return s.applyLineQty(ctx, id, "Penerimaan "+receiptLabel, func(o *domain.PurchaseOrder) error {
		return o.ApplyReceipt(deltas)
	})
}

// RecordBilling menambah (atau mengurangi) qty yang ditagih per baris; PO Diterima yang tertagih penuh menjadi
// Selesai.
func (s *Service) RecordBilling(ctx context.Context, id uuid.UUID, billLabel string, deltas []domain.QtyDelta) (domain.PurchaseOrder, error) {
	return s.applyLineQty(ctx, id, "Tagihan "+billLabel, func(o *domain.PurchaseOrder) error {
		return o.ApplyBilling(deltas)
	})
}

// OutstandingAdvances mengembalikan uang muka Diposting satu PO yang masih punya sisa, urut tanggal bayar.
func (s *Service) OutstandingAdvances(ctx context.Context, orderID uuid.UUID) ([]domain.Advance, error) {
	list, err := s.Store.AdvancesByOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Advance, 0, len(list))
	for _, a := range list {
		if a.Outstanding().IsPositive() {
			out = append(out, a)
		}
	}
	return out, nil
}

// AdvanceApplication adalah potongan satu uang muka oleh tagihan.
type AdvanceApplication struct {
	AdvanceID     uuid.UUID
	AdvanceNumber string
	Amount        money.Amount
}

// ApplyAdvances memotong uang muka PO sebesar maksimal amount (FIFO tanggal bayar) dan mengembalikan rincian
// potongannya. Tagihan pemasok memposting A-19 `advance_applied` (Dr 2101 / Cr 1401) sebesar total potongan.
func (s *Service) ApplyAdvances(ctx context.Context, orderID uuid.UUID, amount money.Amount) ([]AdvanceApplication, error) {
	if amount.IsNegative() {
		return nil, apperr.Validation(map[string]string{"advanceAppliedAmount": "Potongan uang muka tidak boleh negatif"})
	}
	var out []AdvanceApplication
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := s.Store.GetOrder(ctx, orderID, true); err != nil {
			return err
		}
		list, err := s.Store.AdvancesByOrder(ctx, orderID)
		if err != nil {
			return err
		}
		alloc := domain.AllocateAdvances(list, amount)
		for _, a := range list {
			take, ok := alloc[a.ID]
			if !ok {
				continue
			}
			version := a.Version
			if err := a.Apply(take); err != nil {
				return err
			}
			if err := s.Store.SaveAdvance(ctx, a, version, actor.UserID(ctx)); err != nil {
				return err
			}
			out = append(out, AdvanceApplication{AdvanceID: a.ID, AdvanceNumber: a.Number, Amount: take})
		}
		return nil
	})
	return out, err
}

// ReleaseAdvances mengembalikan potongan uang muka saat tagihan pemasok dibatalkan.
func (s *Service) ReleaseAdvances(ctx context.Context, applications []AdvanceApplication) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		for _, app := range applications {
			a, err := s.Store.GetAdvance(ctx, app.AdvanceID, true)
			if err != nil {
				return err
			}
			version := a.Version
			if err := a.Apply(app.Amount.Neg()); err != nil {
				return err
			}
			if err := s.Store.SaveAdvance(ctx, a, version, actor.UserID(ctx)); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- Notifikasi ETA H-1 -----------------------------------------------------------------------

// kindETA adalah jenis notifikasi in-app ETA PO.
const kindETA = "procurement.eta"

// NotifyETAArgs adalah argumen job procurement.notify_eta (Technical Guideline Lampiran B).
type NotifyETAArgs struct{}

// Kind adalah nama job.
func (NotifyETAArgs) Kind() string { return "procurement.notify_eta" }

// InsertOpts: maks. 3 percobaan.
func (NotifyETAArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobqueue.QueueDefault, MaxAttempts: 3}
}

// NotifyETA memberi tahu Logistik Leader (in-app + email) tentang PO yang dijadwalkan tiba besok beserta detail
// barangnya, agar armada & manpower bisa diatur (S-202). Idempotent: setiap PO hanya sekali (eta_notified_at).
func (s *Service) NotifyETA(ctx context.Context) (int, error) {
	ctx = actor.System(ctx)
	tomorrow := s.today().AddDays(1)
	ids, err := s.Store.OrdersDueForETA(ctx, tomorrow)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	leaders, err := s.Users.UsersWithRole(ctx, RoleLogisticsLeader)
	if err != nil {
		return 0, err
	}
	if len(leaders) == 0 {
		s.Log.WarnContext(ctx, "eta notification has no logistics leader", "orders", len(ids))
	}
	sent := 0
	for _, id := range ids {
		err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
			ok, err := s.Store.MarkETANotified(ctx, id, s.Clock.Now())
			if err != nil || !ok || len(leaders) == 0 {
				return err
			}
			o, err := s.Store.GetOrder(ctx, id, false)
			if err != nil {
				return err
			}
			v, err := s.orderView(ctx, o)
			if err != nil {
				return err
			}
			return s.sendETA(ctx, v, leaders)
		})
		if err != nil {
			return sent, fmt.Errorf("notify eta purchase order %s: %w", id, err)
		}
		sent++
	}
	return sent, nil
}

// etaDetail menyusun detail barang PO untuk notifikasi ETA.
func etaDetail(v OrderView) []string {
	lines := make([]string, 0, len(v.Lines)+len(v.Materials))
	for _, l := range v.Lines {
		name, uom := l.Description, ""
		if l.ProductID != nil {
			if p, ok := v.Products[*l.ProductID]; ok {
				name, uom = p.SKU+" "+p.Name, " "+p.UOM
			}
		}
		lines = append(lines, fmt.Sprintf("- %s: %s%s (sisa %s)", name, l.OrderedQty.String(), uom, l.OutstandingQty().String()))
	}
	return lines
}

func (s *Service) sendETA(ctx context.Context, v OrderView, leaders []UserRef) error {
	when := "besok"
	if !v.ExpectedDate.After(s.today()) {
		when = "hari ini"
	}
	title := fmt.Sprintf("Barang tiba %s: %s", when, v.Number)
	detail := etaDetail(v)
	body := fmt.Sprintf("%s · %s · %d baris · tiba %s", v.Type.Label(), v.SupplierName, len(v.Lines), v.ExpectedDate.String())
	ids := make([]uuid.UUID, len(leaders))
	for i, u := range leaders {
		ids[i] = u.ID
	}
	link := OrderPath(v.ID)
	if err := s.Notifier.Notify(ctx, ids, kindETA, title, body, link); err != nil {
		return err
	}
	text := fmt.Sprintf("%s\n\nPemasok: %s\nJenis PO: %s\nTanggal kirim diharapkan: %s\n\nDetail barang:\n%s\n\nBuka Purchase Order:\n%s%s\n",
		title, v.SupplierName, v.Type.Label(), v.ExpectedDate.String(), strings.Join(detail, "\n"), s.BaseURL, link)
	for _, u := range leaders {
		if u.Email == "" {
			continue
		}
		if _, err := s.Notifier.QueueEmail(ctx, u.Email, title, text); err != nil {
			return err
		}
	}
	return nil
}

// NotifyETAWorker menjalankan job procurement.notify_eta.
type NotifyETAWorker struct {
	river.WorkerDefaults[NotifyETAArgs]
	service *Service
}

// NewNotifyETAWorker membuat worker.
func NewNotifyETAWorker(s *Service) *NotifyETAWorker { return &NotifyETAWorker{service: s} }

// Work mengirim notifikasi ETA.
func (w *NotifyETAWorker) Work(ctx context.Context, _ *river.Job[NotifyETAArgs]) error {
	n, err := w.service.NotifyETA(ctx)
	if n > 0 {
		w.service.Log.InfoContext(ctx, "purchase order eta notifications sent", "count", n)
	}
	return err
}
