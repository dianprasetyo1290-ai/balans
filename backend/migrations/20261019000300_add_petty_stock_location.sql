-- +goose Up
-- Petty Stock dipisah dari E-commerce Stock (keputusan PO 17 Okt 2026, PRD F1-114): 7 lokasi standar per gudang.
ALTER TABLE locations DROP CONSTRAINT ck_locations_type;
ALTER TABLE locations ADD CONSTRAINT ck_locations_type CHECK (location_type IN (
    'inbound', 'finished_goods', 'ecommerce', 'petty_stock', 'raw_material', 'goods_material', 'returns', 'event',
    'toll_manufacturer', 'consignment', 'vending_buffer', 'vending_machine'));

UPDATE locations SET name = 'E-commerce Stock', updated_at = now()
WHERE is_standard AND location_type = 'ecommerce' AND name = 'E-commerce Stock (Petty Stock)';

INSERT INTO locations (code, name, warehouse_id, location_type, is_sellable, is_standard, is_active, created_by, updated_by, deleted_at, deleted_by)
SELECT w.code || '-PTY', 'Petty Stock', w.id, 'petty_stock', true, true, true,
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000', w.deleted_at, w.deleted_by
FROM warehouses w
WHERE NOT EXISTS (SELECT 1 FROM locations l WHERE l.warehouse_id = w.id AND l.location_type = 'petty_stock');

-- +goose Down
DELETE FROM locations WHERE location_type = 'petty_stock' AND is_standard;
ALTER TABLE locations DROP CONSTRAINT ck_locations_type;
ALTER TABLE locations ADD CONSTRAINT ck_locations_type CHECK (location_type IN (
    'inbound', 'finished_goods', 'ecommerce', 'raw_material', 'goods_material', 'returns', 'event',
    'toll_manufacturer', 'consignment', 'vending_buffer', 'vending_machine'));
