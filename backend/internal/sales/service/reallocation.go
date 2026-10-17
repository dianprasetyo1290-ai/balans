package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/apperr"
	"balans/internal/sales/domain"
)

// API ganti batch alokasi untuk QC Outbound (WMS-12, D-24). Dipanggil modul logistics di transaksi QC; tidak
// memeriksa permission karena pemanggil memeriksa aksi QC user-nya sendiri.

// ReallocateInput adalah permintaan ganti batch satu alokasi Sales Order.
type ReallocateInput struct {
	OrderID      uuid.UUID
	AllocationID uuid.UUID
	// RejectBatchID + RejectQty: barang yang ditolak QC Outbound; stok reserved batch itu dipindah ke Karantina di
	// lokasi alokasi. RejectQty nol = hanya mengisi kekurangan alokasi (alokasi ulang baris yang ditahan).
	RejectBatchID *uuid.UUID
	RejectQty     decimal.Decimal
	// Ref unik per permintaan (mis. ID baris Outbound) untuk aksi ledger; Label untuk kartu stok.
	Ref   string
	Label string
	Note  string
}

// Reallocation adalah hasil ganti batch: qty yang dipindah ke Karantina, batch pengganti FEFO yang direservasi,
// dan kekurangan yang belum mendapat stok (baris ditahan).
type Reallocation struct {
	Allocation           domain.Allocation
	QuarantineLocationID uuid.UUID
	Rejected             decimal.Decimal
	Replacements         []domain.AllocationBatch
	Shortfall            decimal.Decimal
}

// ReallocateBatch memindah qty batch yang ditolak QC dari reserved ke Karantina (tidak bisa teralokasi lagi),
// lalu langsung mereservasi stok pengganti FEFO dari stok available lain di lokasi yang sama (batch lain atau
// unit lain, TG 5.7.2). Stok pengganti yang kurang dikembalikan sebagai Shortfall; panggil lagi dengan RejectQty
// nol untuk mengisinya setelah stok tersedia.
func (s *DocumentService) ReallocateBatch(ctx context.Context, in ReallocateInput) (Reallocation, error) {
	if in.Ref == "" {
		return Reallocation{}, apperr.Validation(map[string]string{"ref": "Referensi dokumen wajib diisi"})
	}
	var out Reallocation
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.GetOrder(ctx, in.OrderID, true)
		if err != nil {
			return err
		}
		if o.Status != domain.StatusInProgress {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Sales Order %s berstatus %s; batch hanya bisa diganti pada pesanan Diproses.", o.Label(), o.Status.Label()))
		}
		a, ok := o.Allocation(in.AllocationID)
		if !ok {
			return apperr.Validation(map[string]string{"allocationId": "Alokasi tidak ditemukan di Sales Order ini"})
		}
		label := o.Number
		if in.Label != "" {
			label = in.Label + " · " + o.Number
		}
		src := func(action string) inventory.Source {
			return inventory.Source{Type: SourceSalesOrder, ID: o.ID, Action: action + ":" + in.Ref, Label: label, BusinessDate: s.today()}
		}
		out = Reallocation{QuarantineLocationID: a.LocationID, Rejected: decimal.Zero, Shortfall: decimal.Zero}
		before := a.Batches
		if in.RejectQty.IsPositive() {
			if err := a.RemoveBatchQty(in.RejectBatchID, in.RejectQty); err != nil {
				return err
			}
			allocID := a.ID
			if _, err := s.Stock.Post(ctx, inventory.Posting{
				Source: src("qc-reject"),
				Lines: []inventory.Line{{
					Kind: inventory.KindMove, SourceLineID: &allocID, ProductID: a.ProductID, Qty: in.RejectQty, BatchID: in.RejectBatchID,
					From:        &inventory.Place{LocationID: a.LocationID, Status: inventory.StatusReserved},
					To:          &inventory.Place{LocationID: a.LocationID, Status: inventory.StatusQuarantine},
					Reservation: &inventory.ReservationRef{SourceType: SourceSalesOrder, SourceID: o.ID, SourceLineID: &allocID},
					Note:        in.Note,
				}},
			}); err != nil {
				return err
			}
			out.Rejected = in.RejectQty
		}
		if err := s.reserveGap(ctx, a, src("qc-replace"), &out); err != nil {
			return err
		}
		out.Allocation = *a
		if err := s.Store.ReplaceAllocationBatches(ctx, a.ID, a.Batches); err != nil {
			return err
		}
		note := fmt.Sprintf("Stok pengganti %s untuk QC Outbound (%s)", sumBatches(out.Replacements).String(), label)
		if out.Rejected.IsPositive() {
			note = fmt.Sprintf("Ditolak QC Outbound %s → Karantina; ", out.Rejected.String()) + note
		}
		if out.Shortfall.IsPositive() {
			note += "; kurang " + out.Shortfall.String()
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: entityOf(o.Type), EntityID: o.ID, EntityLabel: o.Label(),
			Before: map[string]any{"allocation": a.ID, "batches": batchSnapshot(before)},
			After:  map[string]any{"allocation": a.ID, "batches": batchSnapshot(a.Batches)},
			Note:   note,
		})
	})
	return out, err
}

// reserveGap mereservasi FEFO kekurangan alokasi sebanyak stok available yang ada (tanpa batch kedaluwarsa).
func (s *DocumentService) reserveGap(ctx context.Context, a *domain.Allocation, src inventory.Source, out *Reallocation) error {
	need := a.Gap()
	if !need.IsPositive() {
		return nil
	}
	sug, err := s.Stock.SuggestBatches(ctx, a.ProductID, a.LocationID, need)
	if err != nil {
		return err
	}
	take := decimal.Min(sug.AllocatedQty, need)
	out.Shortfall = need.Sub(take)
	if !take.IsPositive() {
		return nil
	}
	allocID := a.ID
	res, err := s.Stock.Reserve(ctx, inventory.ReserveRequest{
		Source: src,
		Lines:  []inventory.ReserveLine{{SourceLineID: &allocID, ProductID: a.ProductID, LocationID: a.LocationID, Qty: take}},
	})
	if err != nil {
		return err
	}
	for _, m := range res.Movements {
		if m.Type == inventory.MovementMoveIn && m.Status == inventory.StatusReserved {
			a.AddBatch(m.BatchID, m.Qty)
			out.Replacements = append(out.Replacements, domain.AllocationBatch{BatchID: m.BatchID, Qty: m.Qty})
		}
	}
	return nil
}

func sumBatches(bs []domain.AllocationBatch) decimal.Decimal {
	total := decimal.Zero
	for _, b := range bs {
		total = total.Add(b.Qty)
	}
	return total
}

func batchSnapshot(bs []domain.AllocationBatch) []map[string]any {
	out := make([]map[string]any, len(bs))
	for i, b := range bs {
		out[i] = map[string]any{"batchId": b.BatchID, "qty": b.Qty.String()}
	}
	return out
}
