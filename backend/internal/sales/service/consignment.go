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

// Permission konsinyasi. Pengiriman titipan memakai permission Sales Order (buat/ubah oleh Sales MT) dan
// Delivery Order (kirim & konfirmasi diterima oleh Logistik); rekonsiliasi memakai sales.consignment_reconciliation.
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
	// SourceConsignmentDelivery adalah source_type transfer stok & jurnal A-07/A-24 pengiriman dari halaman
	// Konsinyasi; DO logistics memakai SourceType miliknya sendiri lewat ShipConsignment.
	SourceConsignmentDelivery = domain.SourceConsignmentDelivery
	// SourceReconciliation adalah source_type issue stok dan jurnal A-08/A-23 rekonsiliasi.
	SourceReconciliation = "consignment_reconciliation"
)

// Jenis nomor dokumen konsinyasi.
const (
	DocDeliveryOrder  DocumentType = "DO"
	DocReconciliation DocumentType = "KSN"
)

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
	Lines          []domain.DeliveryLine
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
	AllowedActions []string
}

var deliveryActionPerms = map[domain.Action][]string{
	domain.ActionEdit: {PermOrderWrite}, domain.ActionCancel: {PermOrderWrite},
	domain.ActionShip: {PermDeliveryPost}, domain.ActionReceive: {PermDeliveryPost},
	domain.ActionPrint: {PermOrderRead, PermDeliveryRead},
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

// ShipDelivery mengirim pengiriman Draft dari halaman Konsinyasi: nomor DO, Transfer Stok ke lokasi mitra
// (Dalam Pengiriman) dan jurnal A-07/A-24 di transaksi yang sama. PKS diperiksa ulang pada tanggal kirim.
func (s *DocumentService) ShipDelivery(ctx context.Context, id uuid.UUID, ver int) (DeliveryView, error) {
	return s.mutateDelivery(ctx, id, ver, PermDeliveryPost, audit.ActionStatusChange, true, func(ctx context.Context, d *domain.ConsignmentDelivery) (string, error) {
		if err := s.deliveryRefs(ctx, d); err != nil {
			return "", err
		}
		number, err := s.Numberer.Next(ctx, DocDeliveryOrder, d.BusinessDate)
		if err != nil {
			return "", err
		}
		sid := d.ID
		d.SourceID = &sid
		if err := s.shipConsignment(ctx, d, number); err != nil {
			return "", err
		}
		return "Dikirim, " + d.TransferNumber, nil
	})
}

// shipConsignment memindah stok ke lokasi mitra dan memposting A-07 (Dr 1312 Cr 1311) atau A-24 (Dr 1313 Cr 1311)
// sebesar nilai FIFO dengan dimensi channel konsinyasi/vending + sub-channel mitra.
func (s *DocumentService) shipConsignment(ctx context.Context, d *domain.ConsignmentDelivery, number string) error {
	res, err := s.Consignment.Transfer(ctx, StockTransferInput{
		SourceType: d.SourceType, SourceID: *d.SourceID, FromLocationID: d.FromLocationID, ToLocationID: d.ToLocationID,
		BusinessDate: d.BusinessDate, Note: fmt.Sprintf("%s %s %s", d.Type.Label(), number, d.PartnerName), Lines: d.Lines,
	})
	if err != nil {
		return err
	}
	lines := make([]domain.DeliveryLine, len(res.Lines))
	for i, l := range res.Lines {
		lines[i] = domain.DeliveryLine{ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty, Cost: l.Cost}
	}
	if err := d.MarkShipped(number, res.ID, res.Number, lines, s.Clock.Now()); err != nil {
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

// ReceiveDelivery mencatat barang titipan diterima mitra: transfer Dalam Pengiriman → Tersedia di lokasi mitra.
func (s *DocumentService) ReceiveDelivery(ctx context.Context, id uuid.UUID, ver int) (DeliveryView, error) {
	return s.mutateDelivery(ctx, id, ver, PermDeliveryPost, audit.ActionStatusChange, false, func(ctx context.Context, d *domain.ConsignmentDelivery) (string, error) {
		if err := s.requireConsignmentDeps(); err != nil {
			return "", err
		}
		if d.TransferID != nil && d.Status == domain.StatusInTransit {
			if err := s.Consignment.ReceiveAll(ctx, *d.TransferID); err != nil {
				return "", err
			}
		}
		return "Diterima mitra", d.MarkReceived(s.Clock.Now())
	})
}

// ConsignmentShipmentInput adalah pengiriman titipan yang dikirim lewat dokumen logistik (DO konsinyasi).
type ConsignmentShipmentInput struct {
	Type      domain.DeliveryType
	PartnerID uuid.UUID
	// SourceType/SourceID/SourceLabel adalah dokumen pengiriman, mis. "delivery_order" + ID DO + nomor DO; satu
	// dokumen sumber hanya dicatat sekali (pemanggilan ulang mengembalikan pengiriman yang sama).
	SourceType     string
	SourceID       uuid.UUID
	SourceLabel    string
	FromLocationID uuid.UUID
	BusinessDate   clock.BusinessDate
	Note           string
	Lines          []domain.DeliveryLineInput
}

// ShipConsignment adalah titik integrasi DO konsinyasi/buffer vending (modul logistics) di transaksi DO ship:
// PKS konsinyasi wajib berlaku, stok pindah ke lokasi mitra (TRF Dalam Pengiriman) dan jurnal A-07/A-24 dibuat.
// Tanpa cek permission (pemanggil memeriksa aksi user-nya).
func (s *DocumentService) ShipConsignment(ctx context.Context, in ConsignmentShipmentInput) (domain.ConsignmentDelivery, error) {
	if strings.TrimSpace(in.SourceType) == "" || in.SourceID == uuid.Nil || in.SourceType == SourceConsignmentDelivery {
		return domain.ConsignmentDelivery{}, apperr.Validation(map[string]string{"source": "Dokumen pengiriman wajib diisi"})
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = s.today()
	}
	var out domain.ConsignmentDelivery
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if d, ok, err := s.Store.DeliveryBySource(ctx, in.SourceType, in.SourceID); err != nil || ok {
			out = d
			return err
		}
		d, err := domain.NewConsignmentDelivery(domain.DeliveryInput{
			Type: in.Type, PartnerID: in.PartnerID, FromLocationID: in.FromLocationID, BusinessDate: in.BusinessDate,
			Note: in.Note, Lines: in.Lines,
		})
		if err != nil {
			return err
		}
		sid := in.SourceID
		d.SourceType, d.SourceID = in.SourceType, &sid
		if err := s.deliveryRefs(ctx, &d); err != nil {
			return err
		}
		if d, err = s.Store.InsertDelivery(ctx, d, actor.UserID(ctx)); err != nil {
			return err
		}
		ver := d.Version
		number := in.SourceLabel
		if number == "" {
			if number, err = s.Numberer.Next(ctx, DocDeliveryOrder, d.BusinessDate); err != nil {
				return err
			}
		}
		if err := s.shipConsignment(ctx, &d, number); err != nil {
			return err
		}
		if err := s.Store.SaveDelivery(ctx, &d, ver, actor.UserID(ctx), true); err != nil {
			return err
		}
		out = d
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityConsignmentDelivery, EntityID: d.ID, EntityLabel: d.Label(),
			After: deliverySnapshot(d), Note: "Dikirim lewat " + number,
		})
	})
	return out, err
}

// ConfirmConsignmentReceived menandai pengiriman titipan dari DO sudah diterima mitra (DO deliver).
func (s *DocumentService) ConfirmConsignmentReceived(ctx context.Context, deliveryID uuid.UUID) (domain.ConsignmentDelivery, error) {
	var out domain.ConsignmentDelivery
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		d, err := s.Store.GetDelivery(ctx, deliveryID, true)
		if err != nil {
			return err
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
