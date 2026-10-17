-- +goose Up
-- Pengiriman konsinyasi / buffer vending lewat Delivery Order logistik (S-303, S-219): Sales MT mengajukan draft →
-- stok FEFO direservasi untuk baris pengiriman dan DO Draft dibuat (status Diproses, belum bernomor sampai DO
-- dikirim). source_type/source_id menunjuk DO yang sedang memproses pengiriman ('delivery_order'); data lama yang
-- dikirim langsung dari halaman Konsinyasi tetap 'consignment_delivery'.
ALTER TABLE consignment_deliveries DROP CONSTRAINT ck_consignment_deliveries_status;
ALTER TABLE consignment_deliveries ADD CONSTRAINT ck_consignment_deliveries_status
    CHECK (status IN ('draft', 'in_progress', 'in_transit', 'completed', 'cancelled'));
ALTER TABLE consignment_deliveries DROP CONSTRAINT ck_consignment_deliveries_number;
ALTER TABLE consignment_deliveries ADD CONSTRAINT ck_consignment_deliveries_number
    CHECK (status IN ('draft', 'in_progress', 'cancelled') OR number IS NOT NULL);
ALTER TABLE consignment_deliveries ADD CONSTRAINT ck_consignment_deliveries_source
    CHECK (status <> 'in_progress' OR (source_type = 'delivery_order' AND source_id IS NOT NULL));

-- +goose Down
ALTER TABLE consignment_deliveries DROP CONSTRAINT ck_consignment_deliveries_source;
ALTER TABLE consignment_deliveries DROP CONSTRAINT ck_consignment_deliveries_number;
ALTER TABLE consignment_deliveries ADD CONSTRAINT ck_consignment_deliveries_number
    CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL);
ALTER TABLE consignment_deliveries DROP CONSTRAINT ck_consignment_deliveries_status;
ALTER TABLE consignment_deliveries ADD CONSTRAINT ck_consignment_deliveries_status
    CHECK (status IN ('draft', 'in_transit', 'completed', 'cancelled'));
