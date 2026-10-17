package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/audit"
	"balans/internal/inventory"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/sales/domain"
	"balans/internal/sales/postgres"
)

// Permission konsinyasi. Pengiriman titipan memakai permission Sales Order (buat/ubah/ajukan oleh Sales MT); kirim
// dan konfirmasi diterima dikerjakan Logistik di Delivery Order (data lama tanpa DO: konfirmasi diterima dengan
// permission DO post); rekonsiliasi memakai sales.consignment_reconciliation.
const (
	PermDeliveryRead          = "logistics.delivery_order.read"
	PermDeliveryPost          = "logistics.delivery_order.post"
	PermReconciliationRead    = "sales.consignment_reconciliation.read"
	PermReconciliationWrite   = "sales.consignment_reconciliation.write"
	PermReconciliationSubmit  = "sales.consignment_reconciliation.submit"
	EntityConsignmentDelivery = "consignment_delivery"
	EntityReconciliation      = "consignment_reconciliation"
)

// Posting rule dan jenis dokumen sumber konsinyasi (Desain Finance & Accounting 2.1).
const (
	RuleConsignmentSales    = "A-08"
	RuleConsignmentVariance = "A-23"
	// SourceConsignmentDelivery adalah dokumen asal DO pengiriman titipan dan pemilik reservasi stoknya;
	// SourceDeliveryOrder adalah source_type transfer stok pengiriman yang dikirim lewat DO.
	SourceConsignmentDelivery = domain.SourceConsignmentDelivery
	SourceDeliveryOrder       = domain.SourceDeliveryOrder
	// SourceReconciliation adalah source_type issue stok dan jurnal A-08/A-23 rekonsiliasi.
	SourceReconciliation = "consignment_reconciliation"
)

// DocReconciliation adalah jenis nomor Rekonsiliasi Konsinyasi. Nomor pengiriman titipan = nomor DO logistik
// (satu counter DO).
const DocReconciliation DocumentType = "KSN"

// Jenis pemilik file: Berita Acara Rekonsiliasi bertanda tangan dan laporan penjualan mitra.
const (
	FileOwnerConsignmentBA     = "sales.consignment_ba"
	FileOwnerConsignmentReport = "sales.consignment_partner_report"
)

const (
	deliveryPathPrefix       = "/sales/consignments/"
	reconciliationPathPrefix = "/sales/consignment-reconciliations/"
	// countWindowDays: stock count mitra harus dilakukan paling jauh 7 hari sebelum/sesudah cut-off.
	countWindowDays = 7
)

// ReconciliationPath adalah path detail Rekonsiliasi Konsinyasi.
func ReconciliationPath(id uuid.UUID) string { return reconciliationPathPrefix + id.String() }

// AgreementRef adalah PKS konsinyasi berlaku (modul party).
type AgreementRef struct {
	ID              uuid.UUID
	Number          string
	PaymentTermDays int
	MarginPercent   *decimal.Decimal
	PriceListID     *uuid.UUID
	Rules           domain.VarianceRules
}

// Agreements membaca PKS konsinyasi mitra. RequireConsignment menolak (VALIDATION_FAILED `partnerId`) bila tidak
// ada PKS konsinyasi berlaku pada tanggal tersebut.
type Agreements interface {
	RequireConsignment(ctx context.Context, partnerID uuid.UUID, on clock.BusinessDate) (AgreementRef, error)
}

// StockTransferInput adalah perpindahan stok ke lokasi mitra (warehouse.TransferStock).
type StockTransferInput struct {
	SourceType     string
	SourceID       uuid.UUID
	FromLocationID uuid.UUID
	ToLocationID   uuid.UUID
	BusinessDate   clock.BusinessDate
	Note           string
	Lines          []StockTransferLine
	// ReservationSourceType/ReservationSourceID: dokumen pemilik reservasi stok asal (baris dengan
	// ReservationLineID diambil dari reserved, bukan Tersedia).
	ReservationSourceType string
	ReservationSourceID   uuid.UUID
}

// TransferredLine adalah satu batch yang dipindah beserta nilai FIFO-nya.
type TransferredLine struct {
	ProductID uuid.UUID
	BatchID   *uuid.UUID
	Qty       decimal.Decimal
	Cost      money.Amount
}

// StockTransferResult adalah Transfer Stok (TRF) yang dibuat.
type StockTransferResult struct {
	ID     uuid.UUID
	Number string
	Lines  []TransferredLine
}

// CountedLine adalah qty hitung satu produk × batch di stock count.
type CountedLine struct {
	ProductID  uuid.UUID
	BatchID    *uuid.UUID
	CountedQty decimal.Decimal
}

// StockCountRef adalah hasil stock count lokasi mitra (warehouse.StockCountResult).
type StockCountRef struct {
	ID           uuid.UUID
	Number       string
	CountType    string
	LocationID   uuid.UUID
	Status       string
	BusinessDate clock.BusinessDate
	Lines        []CountedLine
}

// ConsignmentStock adalah API gudang untuk konsinyasi (modul warehouse).
type ConsignmentStock interface {
	// PartnerLocation mengembalikan lokasi eksternal aktif jenis locationType milik mitra.
	PartnerLocation(ctx context.Context, locationType string, partnerID uuid.UUID) (LocationRef, bool, error)
	Transfer(ctx context.Context, in StockTransferInput) (StockTransferResult, error)
	// ReceiveAll mencatat seluruh qty transfer diterima di lokasi tujuan dan menutup transfer.
	ReceiveAll(ctx context.Context, transferID uuid.UUID) error
	StockCount(ctx context.Context, id uuid.UUID) (StockCountRef, error)
}

// StockFlows membaca mutasi stok per lokasi (modul inventory).
type StockFlows interface {
	LocationFlows(ctx context.Context, q inventory.LocationFlowQuery) ([]inventory.LocationFlow, error)
}

func (s *DocumentService) requireConsignmentDeps() error {
	if s.Consignment == nil || s.Agreements == nil || s.Flows == nil {
		return apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul konsinyasi belum lengkap (warehouse, party, inventory).")
	}
	return nil
}

// partnerSubChannel mencari channel (kode) dan sub-channel milik mitra untuk dimensi jurnal.
func (s *DocumentService) partnerSubChannel(ctx context.Context, channelCode string, partnerID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	ch, ok, err := s.channelByCode(ctx, channelCode)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if !ok {
		return uuid.Nil, uuid.Nil, apperr.Validation(map[string]string{"partnerId": "Channel " + channelCode + " belum ada di master dimensi"})
	}
	list, err := s.Ledger.Channels(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	for _, c := range list {
		if c.Kind == "sub_channel" && c.ChannelID != nil && *c.ChannelID == ch.ID && c.PartyID != nil && *c.PartyID == partnerID {
			return ch.ID, c.ID, nil
		}
	}
	return uuid.Nil, uuid.Nil, apperr.Validation(map[string]string{"partnerId": fmt.Sprintf(
		"Mitra belum punya sub-channel di channel %s. Tambahkan di Keuangan & Akuntansi › Dimensi.", ch.Name)})
}

// --- Pengiriman konsinyasi / buffer vending (S-303, S-219) ----------------------------------

// DeliveryFilter adalah kriteria daftar pengiriman.
type DeliveryFilter = postgres.DeliveryFilter

// DeliveryView adalah pengiriman beserta label rujukan.
type DeliveryView struct {
	domain.ConsignmentDelivery
	FromLocation   LocationRef
	ToLocation     LocationRef
	Products       map[uuid.UUID]ProductRef
	ChannelName    string
	SubChannelName string
	CreatedByName  string
	// DeliveryOrder adalah DO logistik yang memproses pengiriman (nil = belum diajukan / data lama).
	DeliveryOrder  *DeliveryOrderRef
	AllowedActions []string
}

var deliveryActionPerms = map[domain.Action][]string{
	domain.ActionEdit: {PermOrderWrite}, domain.ActionCancel: {PermOrderWrite}, domain.ActionSubmit: {PermOrderWrite},
	domain.ActionReceive: {PermDeliveryPost},
	domain.ActionPrint:   {PermOrderRead, PermDeliveryRead},
}

func deliverySnapshot(d domain.ConsignmentDelivery) map[string]any {
	lines := make([]map[string]any, len(d.Lines))
	for i, l := range d.Lines {
		lines[i] = map[string]any{"productId": l.ProductID.String(), "qty": l.Qty.String(), "cost": l.Cost.String()}
	}
	return map[string]any{
		"number": d.Number, "deliveryType": string(d.Type), "partner": d.PartnerName, "status": string(d.Status),
		"businessDate": d.BusinessDate.String(), "transfer": d.TransferNumber, "costAmount": d.Cost.String(),
		"note": d.Note, "lines": lines,
	}
}

func (s *DocumentService) deliveryView(ctx context.Context, d domain.ConsignmentDelivery) (DeliveryView, error) {
	v := DeliveryView{ConsignmentDelivery: d}
	locs, err := s.Locations.Locations(ctx, []uuid.UUID{d.FromLocationID, d.ToLocationID})
	if err != nil {
		return DeliveryView{}, err
	}
	v.FromLocation, v.ToLocation = locs[d.FromLocationID], locs[d.ToLocationID]
	ids := make([]uuid.UUID, len(d.Lines))
	for i, l := range d.Lines {
		ids[i] = l.ProductID
	}
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return DeliveryView{}, err
	}
	labels, err := s.Ledger.ChannelLabels(ctx, []uuid.UUID{d.ChannelID, d.SubChannelID})
	if err != nil {
		return DeliveryView{}, err
	}
	v.ChannelName, v.SubChannelName = labels[d.ChannelID].Name, labels[d.SubChannelID].Name
	names, err := s.Users.UserNames(ctx, []uuid.UUID{d.CreatedBy})
	if err != nil {
		return DeliveryView{}, err
	}
	v.CreatedByName = names[d.CreatedBy]
	if v.DeliveryOrder, err = s.currentDeliveryOrder(ctx, d); err != nil {
		return DeliveryView{}, err
	}
	v.AllowedActions = filterActions(ctx, d.Actions(), deliveryActionPerms)
	return v, nil
}

// ListDeliveries mengembalikan daftar pengiriman konsinyasi/buffer vending.
func (s *DocumentService) ListDeliveries(ctx context.Context, f DeliveryFilter, page paging.Page) (paging.Result[domain.ConsignmentDelivery], error) {
	if _, err := actor.RequireAny(ctx, PermOrderRead, PermDeliveryRead); err != nil {
		return paging.Result[domain.ConsignmentDelivery]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.Store.ListDeliveries(ctx, f, page)
}

// GetDelivery mengembalikan detail pengiriman.
func (s *DocumentService) GetDelivery(ctx context.Context, id uuid.UUID) (DeliveryView, error) {
	if _, err := actor.RequireAny(ctx, PermOrderRead, PermDeliveryRead); err != nil {
		return DeliveryView{}, err
	}
	d, err := s.Store.GetDelivery(ctx, id, false)
	if err != nil {
		return DeliveryView{}, err
	}
	return s.deliveryView(ctx, d)
}

// deliveryRefs memeriksa mitra, PKS konsinyasi berlaku, lokasi asal (FG Non-Ecommerce), lokasi tujuan mitra,
// produk, dan dimensi channel; mengisi field turunan pengiriman.
func (s *DocumentService) deliveryRefs(ctx context.Context, d *domain.ConsignmentDelivery) error {
	if err := s.requireConsignmentDeps(); err != nil {
		return err
	}
	p, ok, err := s.Parties.Partner(ctx, d.PartnerID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Validation(map[string]string{"partnerId": "Mitra tidak ditemukan"})
	}
	d.PartnerCode, d.PartnerName = p.Code, p.Name
	ag, err := s.Agreements.RequireConsignment(ctx, d.PartnerID, d.BusinessDate)
	if err != nil {
		return err
	}
	d.AgreementID, d.AgreementNumber = &ag.ID, ag.Number
	from, err := s.Locations.Locations(ctx, []uuid.UUID{d.FromLocationID})
	if err != nil {
		return err
	}
	if l, ok := from[d.FromLocationID]; !ok || !l.IsActive || l.Deleted || l.Type != domain.LocationFinishedGoods {
		return apperr.Validation(map[string]string{"fromLocationId": "Lokasi asal harus FG Non-Ecommerce yang aktif"})
	}
	to, ok, err := s.Consignment.PartnerLocation(ctx, d.Type.LocationType(), d.PartnerID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Validation(map[string]string{"partnerId": fmt.Sprintf(
			"Mitra belum punya lokasi %s. Tambahkan di Data Master › Gudang & Lokasi.", d.Type.Label())})
	}
	d.ToLocationID = to.ID
	ids := make([]uuid.UUID, len(d.Lines))
	for i, l := range d.Lines {
		ids[i] = l.ProductID
	}
	products, err := s.Catalog.Products(ctx, uniqueIDs(ids))
	if err != nil {
		return err
	}
	f := map[string]string{}
	for i, l := range d.Lines {
		pr, ok := products[l.ProductID]
		switch {
		case !ok || pr.Deleted || !pr.IsActive:
			f[fmt.Sprintf("lines.%d.productId", i)] = "Produk tidak aktif"
		case pr.IsBundle || pr.Type != "finished_good":
			f[fmt.Sprintf("lines.%d.productId", i)] = "Hanya Produk Jadi (bukan bundel) yang bisa dititipkan"
		}
	}
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	d.ChannelID, d.SubChannelID, err = s.partnerSubChannel(ctx, d.Type.ChannelCode(), d.PartnerID)
	return err
}

// CreateDelivery membuat pengiriman konsinyasi/buffer vending Draft (hanya bila PKS konsinyasi berlaku).
func (s *DocumentService) CreateDelivery(ctx context.Context, in domain.DeliveryInput) (DeliveryView, error) {
	a, err := actor.Require(ctx, PermOrderWrite)
	if err != nil {
		return DeliveryView{}, err
	}
	d, err := domain.NewConsignmentDelivery(in)
	if err != nil {
		return DeliveryView{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.deliveryRefs(ctx, &d); err != nil {
			return err
		}
		if d, err = s.Store.InsertDelivery(ctx, d, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityConsignmentDelivery, EntityID: d.ID, EntityLabel: d.Label(),
			After: deliverySnapshot(d),
		})
	})
	if err != nil {
		return DeliveryView{}, err
	}
	return s.GetDelivery(ctx, d.ID)
}

func (s *DocumentService) mutateDelivery(ctx context.Context, id uuid.UUID, ver int, perm string, action audit.Action,
	replace bool, fn func(ctx context.Context, d *domain.ConsignmentDelivery) (string, error),
) (DeliveryView, error) {
	if _, err := actor.Require(ctx, perm); err != nil {
		return DeliveryView{}, err
	}
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDelivery(ctx, id, true)
		if err != nil {
			return err
		}
		if d.Version != ver {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := deliverySnapshot(d)
		note, err := fn(ctx, &d)
		if err != nil {
			return err
		}
		if err := s.Store.SaveDelivery(ctx, &d, ver, actor.UserID(ctx), replace); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityConsignmentDelivery, EntityID: d.ID, EntityLabel: d.Label(),
			Before: before, After: deliverySnapshot(d), Note: note,
		})
	})
	if err != nil {
		return DeliveryView{}, err
	}
	return s.GetDelivery(ctx, id)
}

// UpdateDelivery mengubah pengiriman Draft.
func (s *DocumentService) UpdateDelivery(ctx context.Context, id uuid.UUID, ver int, in domain.DeliveryInput) (DeliveryView, error) {
	return s.mutateDelivery(ctx, id, ver, PermOrderWrite, audit.ActionUpdate, true, func(ctx context.Context, d *domain.ConsignmentDelivery) (string, error) {
		if err := d.Update(in); err != nil {
			return "", err
		}
		return "", s.deliveryRefs(ctx, d)
	})
}

// CancelDelivery membatalkan pengiriman Draft.
func (s *DocumentService) CancelDelivery(ctx context.Context, id uuid.UUID, ver int, reason string) (DeliveryView, error) {
	return s.mutateDelivery(ctx, id, ver, PermOrderWrite, audit.ActionCancel, false, func(_ context.Context, d *domain.ConsignmentDelivery) (string, error) {
		return strings.TrimSpace(reason), d.Cancel(reason)
	})
}

// --- Integrasi Delivery Order logistik (S-303 ↔ S-309 – S-312) -------------------------------

// DeliveryOrderRef adalah Delivery Order logistik milik dokumen penjualan (nomor diberikan saat release).
type DeliveryOrderRef struct {
	ID     uuid.UUID
	Number string
	Status string
}

// Open: DO masih Draft atau Diproses (satu DO terbuka per dokumen asal).
func (r DeliveryOrderRef) Open() bool { return r.Status == "draft" || r.Status == "in_progress" }

// DeliveryOrderBatch adalah qty satu batch reserved yang menjadi picking list Outbound.
type DeliveryOrderBatch struct {
	BatchID *uuid.UUID
	Qty     decimal.Decimal
}

// DeliveryOrderLine adalah baris DO; SourceLineID = baris pengiriman titipan yang memegang reservasi.
type DeliveryOrderLine struct {
	SourceLineID uuid.UUID
	ProductID    uuid.UUID
	Qty          decimal.Decimal
	Batches      []DeliveryOrderBatch
}

// DeliveryOrderRequest adalah DO Draft jenis konsinyasi / buffer vending untuk pengiriman titipan.
type DeliveryOrderRequest struct {
	Type           domain.DeliveryType
	DeliveryID     uuid.UUID
	SourceLabel    string
	PartnerID      uuid.UUID
	RecipientName  string
	RecipientPhone string
	ShipToAddress  string
	LocationID     uuid.UUID
	BusinessDate   clock.BusinessDate
	Note           string
	Lines          []DeliveryOrderLine
}

// DeliveryOrders adalah API Delivery Order modul logistics (adapter di internal/app; diisi setelah logistics
// dibangun). Nil = pengajuan pengiriman titipan ditolak PROVIDER_UNAVAILABLE.
type DeliveryOrders interface {
	// CreateConsignmentDeliveryOrder membuat DO Draft di transaksi aktif (source_type consignment_delivery).
	CreateConsignmentDeliveryOrder(ctx context.Context, in DeliveryOrderRequest) (DeliveryOrderRef, error)
	// DeliveryOrdersBySource membaca DO milik dokumen asal (mis. sales_order, consignment_delivery).
	DeliveryOrdersBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) ([]DeliveryOrderRef, error)
}

// currentDeliveryOrder membaca DO yang memproses pengiriman (nil bila belum diajukan / data lama).
func (s *DocumentService) currentDeliveryOrder(ctx context.Context, d domain.ConsignmentDelivery) (*DeliveryOrderRef, error) {
	if !d.ViaDeliveryOrder() {
		return nil, nil
	}
	ref := &DeliveryOrderRef{ID: *d.SourceID, Status: "unknown"}
	if s.DeliveryOrders == nil {
		return ref, nil
	}
	list, err := s.DeliveryOrders.DeliveryOrdersBySource(ctx, EntityConsignmentDelivery, d.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.ID == *d.SourceID {
			return &r, nil
		}
	}
	return ref, nil
}

func reservationRef(deliveryID uuid.UUID, lineID *uuid.UUID) *inventory.ReservationRef {
	return &inventory.ReservationRef{SourceType: EntityConsignmentDelivery, SourceID: deliveryID, SourceLineID: lineID}
}

// SubmitDelivery mengajukan pengiriman Draft ke Logistik (Sales MT): PKS, lokasi, dan sub-channel diperiksa
// ulang, stok FEFO lokasi asal direservasi per baris (SourceLineID = baris pengiriman), lalu Delivery Order Draft
// jenis konsinyasi / buffer vending dibuat dengan batch reserved sebagai picking list. Status → Diproses.
func (s *DocumentService) SubmitDelivery(ctx context.Context, id uuid.UUID, ver int) (DeliveryView, error) {
	return s.mutateDelivery(ctx, id, ver, PermOrderWrite, audit.ActionSubmit, false, func(ctx context.Context, d *domain.ConsignmentDelivery) (string, error) {
		if s.DeliveryOrders == nil {
			return "", apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul logistics (Delivery Order) tidak aktif.")
		}
		if d.Status != domain.StatusDraft {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Pengiriman %s berstatus %s; hanya Draft yang bisa diajukan.", d.Label(), d.Status.Label()))
		}
		if err := s.deliveryRefs(ctx, d); err != nil {
			return "", err
		}
		p, _, err := s.Parties.Partner(ctx, d.PartnerID)
		if err != nil {
			return "", err
		}
		reserve := make([]inventory.ReserveLine, len(d.Lines))
		for i, l := range d.Lines {
			lineID := l.ID
			reserve[i] = inventory.ReserveLine{SourceLineID: &lineID, ProductID: l.ProductID, LocationID: d.FromLocationID, Qty: l.Qty}
		}
		label := fmt.Sprintf("%s %s", d.Type.Label(), d.PartnerName)
		res, err := s.Stock.Reserve(ctx, inventory.ReserveRequest{
			Source: inventory.Source{
				Type: EntityConsignmentDelivery, ID: d.ID, Action: fmt.Sprintf("reserve:v%d", d.Version), Label: label,
				BusinessDate: s.today(),
			},
			Lines: reserve,
		})
		if err != nil {
			return "", err
		}
		req := DeliveryOrderRequest{
			Type: d.Type, DeliveryID: d.ID, SourceLabel: label, PartnerID: d.PartnerID, RecipientName: d.PartnerName,
			RecipientPhone: p.Phone, ShipToAddress: p.Address, LocationID: d.FromLocationID, BusinessDate: d.BusinessDate,
			Note: d.Note,
		}
		for i, l := range d.Lines {
			line := DeliveryOrderLine{SourceLineID: l.ID, ProductID: l.ProductID, Qty: l.Qty}
			if i < len(res.Lines) {
				for _, m := range res.Lines[i].Movements {
					if m.Type == inventory.MovementMoveIn && m.Status == inventory.StatusReserved && m.Qty.IsPositive() {
						line.Batches = append(line.Batches, DeliveryOrderBatch{BatchID: m.BatchID, Qty: m.Qty})
					}
				}
			}
			req.Lines = append(req.Lines, line)
		}
		do, err := s.DeliveryOrders.CreateConsignmentDeliveryOrder(ctx, req)
		if err != nil {
			return "", err
		}
		return "Diajukan ke Logistik; stok direservasi FEFO dan Delivery Order dibuat", d.Submit(do.ID)
	})
}

// ConsignmentShippedLine adalah barang lulus QC Outbound per baris pengiriman × batch.
type ConsignmentShippedLine struct {
	SourceLineID uuid.UUID
	BatchID      *uuid.UUID
	Qty          decimal.Decimal
}

// ConsignmentShipmentInput adalah pengiriman titipan yang diserahkan ke kurir lewat DO.
type ConsignmentShipmentInput struct {
	DeliveryID uuid.UUID
	// SourceType/SourceID/SourceLabel adalah DO yang mengirim ("delivery_order" + ID DO + nomor DO).
	SourceType   string
	SourceID     uuid.UUID
	SourceLabel  string
	BusinessDate clock.BusinessDate
	Lines        []ConsignmentShippedLine
}

// ShipConsignment adalah titik integrasi DO ship (serah ke kurir) di transaksi DO: PKS konsinyasi wajib berlaku
// pada tanggal kirim, stok reserved pengiriman (batch lulus QC) pindah ke lokasi mitra berstatus Dalam
// Pengiriman (Transfer Stok), jurnal A-07/A-24 nilai FIFO dibuat sekali, dan sisa reservasi yang tidak ikut
// dikirim dilepas. Pemanggilan ulang untuk DO yang sama mengembalikan pengiriman yang sama. Tanpa cek
// permission (pemanggil memeriksa aksi user-nya).
func (s *DocumentService) ShipConsignment(ctx context.Context, in ConsignmentShipmentInput) (domain.ConsignmentDelivery, error) {
	if in.DeliveryID == uuid.Nil || in.SourceType != domain.SourceDeliveryOrder || in.SourceID == uuid.Nil || strings.TrimSpace(in.SourceLabel) == "" {
		return domain.ConsignmentDelivery{}, apperr.Validation(map[string]string{"source": "Delivery Order pengiriman wajib diisi"})
	}
	if err := s.requireConsignmentDeps(); err != nil {
		return domain.ConsignmentDelivery{}, err
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = s.today()
	}
	var out domain.ConsignmentDelivery
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDelivery(ctx, in.DeliveryID, true)
		if err != nil {
			return err
		}
		sameDO := d.SourceID != nil && *d.SourceID == in.SourceID && d.SourceType == in.SourceType
		if sameDO && (d.Status == domain.StatusInTransit || d.Status == domain.StatusCompleted) {
			out = d
			return nil
		}
		if !sameDO || d.Status != domain.StatusInProgress {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Pengiriman titipan %s berstatus %s dan tidak diproses oleh DO ini.", d.Label(), d.Status.Label()))
		}
		ag, err := s.Agreements.RequireConsignment(ctx, d.PartnerID, in.BusinessDate)
		if err != nil {
			return err
		}
		d.AgreementID, d.AgreementNumber = &ag.ID, ag.Number
		products := map[uuid.UUID]uuid.UUID{}
		for _, l := range d.Lines {
			products[l.ID] = l.ProductID
		}
		var lines []StockTransferLine
		for _, l := range in.Lines {
			pid, ok := products[l.SourceLineID]
			if !ok {
				return apperr.Validation(map[string]string{"lines": "Baris DO tidak cocok dengan baris pengiriman titipan"})
			}
			if !l.Qty.IsPositive() {
				continue
			}
			lineID := l.SourceLineID
			lines = append(lines, StockTransferLine{ProductID: pid, BatchID: l.BatchID, Qty: l.Qty, ReservationLineID: &lineID})
		}
		if len(lines) == 0 {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Tidak ada barang lulus QC untuk dikirim.")
		}
		before, ver := deliverySnapshot(d), d.Version
		d.BusinessDate = in.BusinessDate
		if err := s.shipConsignment(ctx, &d, in.SourceLabel, lines); err != nil {
			return err
		}
		// Sisa reservasi (baris ditahan yang tidak ikut dikirim) kembali Tersedia.
		if _, err := s.Stock.Release(ctx, inventory.ReleaseRequest{
			Source: inventory.Source{
				Type: EntityConsignmentDelivery, ID: d.ID, Action: "release:ship:" + in.SourceID.String(), Label: in.SourceLabel,
				BusinessDate: in.BusinessDate,
			},
			Reservation: *reservationRef(d.ID, nil),
		}); err != nil {
			return err
		}
		if err := s.Store.SaveDelivery(ctx, &d, ver, actor.UserID(ctx), true); err != nil {
			return err
		}
		out = d
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityConsignmentDelivery, EntityID: d.ID, EntityLabel: d.Label(),
			Before: before, After: deliverySnapshot(d), Note: "Dikirim lewat " + in.SourceLabel + ", " + d.TransferNumber,
		})
	})
	return out, err
}

// StockTransferLine adalah baris transfer stok reserved ke lokasi mitra.
type StockTransferLine struct {
	ProductID uuid.UUID
	BatchID   *uuid.UUID
	Qty       decimal.Decimal
	// ReservationLineID: baris pengiriman yang memegang reservasi stok asal.
	ReservationLineID *uuid.UUID
	Note              string
}

// shipConsignment memindah stok reserved ke lokasi mitra dan memposting A-07 (Dr 1312 Cr 1311) atau A-24
// (Dr 1313 Cr 1311) sebesar nilai FIFO dengan dimensi channel konsinyasi/vending + sub-channel mitra.
func (s *DocumentService) shipConsignment(ctx context.Context, d *domain.ConsignmentDelivery, number string, lines []StockTransferLine) error {
	res, err := s.Consignment.Transfer(ctx, StockTransferInput{
		SourceType: d.SourceType, SourceID: *d.SourceID, FromLocationID: d.FromLocationID, ToLocationID: d.ToLocationID,
		BusinessDate: d.BusinessDate, Note: fmt.Sprintf("%s %s %s", d.Type.Label(), number, d.PartnerName), Lines: lines,
		ReservationSourceType: EntityConsignmentDelivery, ReservationSourceID: d.ID,
	})
	if err != nil {
		return err
	}
	shipped := make([]domain.DeliveryLine, len(res.Lines))
	for i, l := range res.Lines {
		shipped[i] = domain.DeliveryLine{ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty, Cost: l.Cost}
	}
	if err := d.MarkShipped(number, res.ID, res.Number, shipped, s.Clock.Now()); err != nil {
		return err
	}
	if !d.Cost.IsPositive() {
		return nil
	}
	_, err = s.Ledger.Post(ctx, accounting.PostingRequest{
		RuleCode: d.Type.RuleCode(), SourceType: EntityConsignmentDelivery, SourceID: d.ID, SourceLabel: number,
		BusinessDate: d.BusinessDate, Amounts: map[string]money.Amount{"inventory_value": d.Cost},
		Dimensions:  salesDimensions(d.ChannelID, d.SubChannelID),
		Description: fmt.Sprintf("Kirim %s %s ke %s", strings.ToLower(d.Type.Label()), number, d.PartnerName),
	})
	return err
}

// ConsignmentReplaceInput adalah barang pengiriman titipan yang ditolak QC Outbound DO (atau baris ditahan).
type ConsignmentReplaceInput struct {
	DeliveryID uuid.UUID
	// LineID adalah baris pengiriman (SourceLineID baris Outbound) yang memegang reservasi.
	LineID uuid.UUID
	// RejectBatchID + RejectQty: batch ditolak QC → reserved pindah ke Karantina. RejectQty nol = hanya mengisi
	// baris ditahan.
	RejectBatchID *uuid.UUID
	RejectQty     decimal.Decimal
	// Need adalah qty yang harus dicarikan stok pengganti (qty ditolak atau qty baris ditahan).
	Need decimal.Decimal
	// Ref unik per permintaan (mis. ID baris Outbound); Label untuk kartu stok.
	Ref   string
	Label string
}

// ConsignmentReplacement adalah hasil ganti batch: lokasi Karantina, batch pengganti FEFO, dan kekurangan.
type ConsignmentReplacement struct {
	QuarantineLocationID uuid.UUID
	Replacements         []domain.AllocationBatch
	Shortfall            decimal.Decimal
}

// ReplaceConsignmentBatch: QC Outbound DO menolak barang titipan → stok reserved batch itu pindah ke Karantina
// lokasi asal (memakai reservasi baris pengiriman), lalu stok pengganti FEFO langsung direservasi untuk baris yang
// sama. Kekurangan dikembalikan sebagai Shortfall (baris Outbound ditahan). Tanpa cek permission.
func (s *DocumentService) ReplaceConsignmentBatch(ctx context.Context, in ConsignmentReplaceInput) (ConsignmentReplacement, error) {
	if in.Ref == "" {
		return ConsignmentReplacement{}, apperr.Validation(map[string]string{"ref": "Referensi dokumen wajib diisi"})
	}
	var out ConsignmentReplacement
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDelivery(ctx, in.DeliveryID, true)
		if err != nil {
			return err
		}
		if d.Status != domain.StatusInProgress {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, fmt.Sprintf(
				"Pengiriman titipan %s berstatus %s; batch hanya bisa diganti saat Diproses.", d.Label(), d.Status.Label()))
		}
		var line *domain.DeliveryLine
		for i := range d.Lines {
			if d.Lines[i].ID == in.LineID {
				line = &d.Lines[i]
			}
		}
		if line == nil {
			return apperr.Validation(map[string]string{"lineId": "Baris tidak ditemukan di pengiriman titipan ini"})
		}
		label := d.Type.Label() + " " + d.PartnerName
		if in.Label != "" {
			label = in.Label + " · " + label
		}
		src := func(action string) inventory.Source {
			return inventory.Source{Type: EntityConsignmentDelivery, ID: d.ID, Action: action + ":" + in.Ref, Label: label, BusinessDate: s.today()}
		}
		lineID := line.ID
		out = ConsignmentReplacement{QuarantineLocationID: d.FromLocationID, Shortfall: decimal.Zero}
		note := ""
		if in.RejectQty.IsPositive() {
			if _, err := s.Stock.Post(ctx, inventory.Posting{
				Source: src("qc-reject"),
				Lines: []inventory.Line{{
					Kind: inventory.KindMove, SourceLineID: &lineID, ProductID: line.ProductID, Qty: in.RejectQty, BatchID: in.RejectBatchID,
					From:        &inventory.Place{LocationID: d.FromLocationID, Status: inventory.StatusReserved},
					To:          &inventory.Place{LocationID: d.FromLocationID, Status: inventory.StatusQuarantine},
					Reservation: reservationRef(d.ID, &lineID),
					Note:        "Ditolak QC Outbound",
				}},
			}); err != nil {
				return err
			}
			note = fmt.Sprintf("Ditolak QC Outbound %s → Karantina; ", in.RejectQty.String())
		}
		need := in.Need
		if !need.IsPositive() {
			need = in.RejectQty
		}
		if need.IsPositive() {
			sug, err := s.Stock.SuggestBatches(ctx, line.ProductID, d.FromLocationID, need)
			if err != nil {
				return err
			}
			take := decimal.Min(sug.AllocatedQty, need)
			out.Shortfall = need.Sub(take)
			if take.IsPositive() {
				res, err := s.Stock.Reserve(ctx, inventory.ReserveRequest{
					Source: src("qc-replace"),
					Lines:  []inventory.ReserveLine{{SourceLineID: &lineID, ProductID: line.ProductID, LocationID: d.FromLocationID, Qty: take}},
				})
				if err != nil {
					return err
				}
				for _, m := range res.Movements {
					if m.Type == inventory.MovementMoveIn && m.Status == inventory.StatusReserved {
						out.Replacements = append(out.Replacements, domain.AllocationBatch{BatchID: m.BatchID, Qty: m.Qty})
					}
				}
			}
		}
		note += fmt.Sprintf("Stok pengganti %s untuk QC Outbound (%s)", sumBatches(out.Replacements).String(), label)
		if out.Shortfall.IsPositive() {
			note += "; kurang " + out.Shortfall.String()
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityConsignmentDelivery, EntityID: d.ID, EntityLabel: d.Label(),
			After: map[string]any{"lineId": lineID, "replacements": batchSnapshot(out.Replacements)}, Note: note,
		})
	})
	return out, err
}

// ReopenConsignmentDelivery: DO pengiriman titipan dibatalkan sebelum dikirim → reservasi dilepas dan pengiriman
// kembali Draft (Sales MT bisa mengubah lalu mengajukan ulang, atau membatalkan). DO lain / pengiriman yang
// sudah tidak Diproses diabaikan. Tanpa cek permission.
func (s *DocumentService) ReopenConsignmentDelivery(ctx context.Context, deliveryID, deliveryOrderID uuid.UUID, reason string) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDelivery(ctx, deliveryID, true)
		if err != nil {
			return err
		}
		if d.Status != domain.StatusInProgress || d.SourceID == nil || *d.SourceID != deliveryOrderID {
			return nil
		}
		before, ver := deliverySnapshot(d), d.Version
		if _, err := s.Stock.Release(ctx, inventory.ReleaseRequest{
			Source: inventory.Source{
				Type: EntityConsignmentDelivery, ID: d.ID, Action: "release:cancel:" + deliveryOrderID.String(),
				Label: d.Type.Label() + " " + d.PartnerName, BusinessDate: s.today(),
			},
			Reservation: *reservationRef(d.ID, nil),
		}); err != nil {
			return err
		}
		if err := d.Reopen(); err != nil {
			return err
		}
		if err := s.Store.SaveDelivery(ctx, &d, ver, actor.UserID(ctx), false); err != nil {
			return err
		}
		note := "Delivery Order dibatalkan; reservasi stok dilepas"
		if r := strings.TrimSpace(reason); r != "" {
			note += ": " + r
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityConsignmentDelivery, EntityID: d.ID, EntityLabel: d.Label(),
			Before: before, After: deliverySnapshot(d), Note: note,
		})
	})
}

// ReceiveDelivery mencatat barang titipan data lama (dikirim langsung tanpa DO) diterima mitra. Pengiriman lewat
// DO dikonfirmasi dari halaman DO (ConfirmConsignmentReceived).
func (s *DocumentService) ReceiveDelivery(ctx context.Context, id uuid.UUID, ver int) (DeliveryView, error) {
	return s.mutateDelivery(ctx, id, ver, PermDeliveryPost, audit.ActionStatusChange, false, func(ctx context.Context, d *domain.ConsignmentDelivery) (string, error) {
		if err := s.requireConsignmentDeps(); err != nil {
			return "", err
		}
		if d.ViaDeliveryOrder() {
			return "", apperr.WithMessage(apperr.CodeInvalidStateTransition,
				"Pengiriman ini diproses lewat Delivery Order; konfirmasi diterima dari halaman Delivery Order.")
		}
		if d.TransferID != nil && d.Status == domain.StatusInTransit {
			if err := s.Consignment.ReceiveAll(ctx, *d.TransferID); err != nil {
				return "", err
			}
		}
		return "Diterima mitra", d.MarkReceived(s.Clock.Now())
	})
}

// ConfirmConsignmentReceived menandai pengiriman titipan dari DO diterima mitra (DO deliver): transfer Dalam
// Pengiriman → Tersedia di lokasi mitra. Pengiriman yang sudah Selesai dikembalikan apa adanya.
func (s *DocumentService) ConfirmConsignmentReceived(ctx context.Context, deliveryID uuid.UUID) (domain.ConsignmentDelivery, error) {
	if err := s.requireConsignmentDeps(); err != nil {
		return domain.ConsignmentDelivery{}, err
	}
	var out domain.ConsignmentDelivery
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDelivery(ctx, deliveryID, true)
		if err != nil {
			return err
		}
		if d.Status == domain.StatusCompleted {
			out = d
			return nil
		}
		if d.TransferID != nil && d.Status == domain.StatusInTransit {
			if err := s.Consignment.ReceiveAll(ctx, *d.TransferID); err != nil {
				return err
			}
		}
		before := deliverySnapshot(d)
		if err := d.MarkReceived(s.Clock.Now()); err != nil {
			return err
		}
		if err := s.Store.SaveDelivery(ctx, &d, d.Version, actor.UserID(ctx), false); err != nil {
			return err
		}
		out = d
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityConsignmentDelivery, EntityID: d.ID, EntityLabel: d.Label(),
			Before: before, After: deliverySnapshot(d), Note: "Diterima mitra",
		})
	})
	return out, err
}
