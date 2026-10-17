-- +goose Up
-- Modul procurement R1.2 lanjutan: Penerimaan Barang (GR) termasuk FG maklon, Tagihan Pemasok dengan 3-way match,
-- utang pemasok, dan Retur Pembelian + nota debit (PRC-04, PRC-05, PRC-07, PRC-10, ACC-04, ACC-16, ACC-22;
-- Desain Data Model 5, Desain Status Dokumen 2.3 – 2.5). product_id, batch_id, location_id, dan journal_entry_id
-- merujuk modul catalog/inventory/warehouse/accounting tanpa FK lintas modul (Technical Guideline 2.3).

CREATE TABLE goods_receipts (
    id                        uuid          NOT NULL DEFAULT uuidv7(),
    -- Nomor GR/TAHUN/BULAN/NOMOR diberikan saat diposting.
    number                    text,
    purchase_order_id         uuid          NOT NULL,
    supplier_id               uuid          NOT NULL,
    division_id               uuid          NOT NULL,
    order_type                text          NOT NULL,
    business_date             date          NOT NULL,
    -- Lokasi Inbound tujuan (stok berstatus Karantina sampai QC).
    location_id               uuid          NOT NULL,
    -- PO maklon/reprocess: lokasi eksternal maklon asal FG dan tempat bahan dipakai.
    toll_location_id          uuid,
    delivery_note_number      text          NOT NULL DEFAULT '',
    note                      text          NOT NULL DEFAULT '',
    -- Nilai persediaan masuk (debit A-01/A-03), packaging/bahan terpakai, dan saldo 2102 yang dibentuk.
    inventory_amount          numeric(18,2) NOT NULL DEFAULT 0,
    material_amount           numeric(18,2) NOT NULL DEFAULT 0,
    grni_amount               numeric(18,2) NOT NULL DEFAULT 0,
    status                    text          NOT NULL DEFAULT 'draft',
    -- Dokumen Inbound/QC modul warehouse (diisi saat integrasi Inbound aktif).
    inbound_id                uuid,
    journal_entry_id          uuid,
    reversal_journal_entry_id uuid,
    cancel_reason             text          NOT NULL DEFAULT '',
    posted_at                 timestamptz,
    cancelled_at              timestamptz,
    created_at                timestamptz   NOT NULL DEFAULT now(),
    created_by                uuid          NOT NULL,
    updated_at                timestamptz   NOT NULL DEFAULT now(),
    updated_by                uuid          NOT NULL,
    version                   integer       NOT NULL DEFAULT 1,
    deleted_at                timestamptz,
    deleted_by                uuid,
    CONSTRAINT pk_goods_receipts PRIMARY KEY (id),
    CONSTRAINT uq_goods_receipts_number UNIQUE (number),
    CONSTRAINT fk_goods_receipts_purchase_order_id FOREIGN KEY (purchase_order_id) REFERENCES purchase_orders (id),
    CONSTRAINT ck_goods_receipts_status CHECK (status IN ('draft', 'posted', 'cancelled')),
    CONSTRAINT ck_goods_receipts_posted CHECK (status <> 'posted' OR (number IS NOT NULL AND journal_entry_id IS NOT NULL)),
    CONSTRAINT ck_goods_receipts_amounts CHECK (inventory_amount >= 0 AND material_amount >= 0 AND grni_amount >= 0)
);
CREATE INDEX idx_goods_receipts_purchase_order_id ON goods_receipts (purchase_order_id);
CREATE INDEX idx_goods_receipts_status ON goods_receipts (status, business_date DESC);
CREATE INDEX idx_goods_receipts_supplier_id ON goods_receipts (supplier_id);

CREATE TABLE goods_receipt_lines (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    goods_receipt_id       uuid          NOT NULL,
    line_no                integer       NOT NULL,
    purchase_order_line_id uuid          NOT NULL,
    product_id             uuid          NOT NULL,
    good_qty               numeric(18,3) NOT NULL,
    reject_qty             numeric(18,3) NOT NULL DEFAULT 0,
    -- return_to_supplier: reject ditolak di tempat dan diganti pemasok (tidak masuk stok/tagihan);
    -- absorbed: reject ditanggung Balans, ikut ditagih, nilainya diserap ke HPP barang baik (BRD 5.2, 5.3).
    reject_disposition     text          NOT NULL DEFAULT 'absorbed',
    batch_number           text          NOT NULL DEFAULT '',
    production_date        date,
    expiry_date            date,
    batch_id               uuid,
    -- Harga PO per unit (harga jasa untuk maklon/reprocess).
    unit_price_amount      numeric(18,2) NOT NULL,
    -- Qty & nilai yang masuk 2102 (= baik + reject diserap) × harga PO.
    grni_qty               numeric(18,3) NOT NULL,
    grni_amount            numeric(18,2) NOT NULL,
    -- Maklon: bagian packaging/bahan terpakai yang dialokasikan ke baris ini.
    material_cost_amount   numeric(18,2) NOT NULL DEFAULT 0,
    -- Nilai batch dan HPP per unit = nilai batch ÷ qty baik.
    inventory_amount       numeric(18,2) NOT NULL DEFAULT 0,
    unit_cost              numeric(18,4) NOT NULL DEFAULT 0,
    inventory_account_code text          NOT NULL DEFAULT '',
    -- 3-way match & retur.
    billed_qty             numeric(18,3) NOT NULL DEFAULT 0,
    billed_grni_amount     numeric(18,2) NOT NULL DEFAULT 0,
    returned_qty           numeric(18,3) NOT NULL DEFAULT 0,
    returned_unbilled_qty  numeric(18,3) NOT NULL DEFAULT 0,
    returned_grni_amount   numeric(18,2) NOT NULL DEFAULT 0,
    note                   text          NOT NULL DEFAULT '',
    CONSTRAINT pk_goods_receipt_lines PRIMARY KEY (id),
    CONSTRAINT fk_goods_receipt_lines_goods_receipt_id FOREIGN KEY (goods_receipt_id)
        REFERENCES goods_receipts (id) ON DELETE CASCADE,
    CONSTRAINT fk_goods_receipt_lines_purchase_order_line_id FOREIGN KEY (purchase_order_line_id)
        REFERENCES purchase_order_lines (id),
    CONSTRAINT uq_goods_receipt_lines_line_no UNIQUE (goods_receipt_id, line_no),
    CONSTRAINT ck_goods_receipt_lines_qty CHECK (good_qty > 0 AND reject_qty >= 0),
    CONSTRAINT ck_goods_receipt_lines_disposition CHECK (reject_disposition IN ('return_to_supplier', 'absorbed')),
    CONSTRAINT ck_goods_receipt_lines_grni CHECK (grni_qty >= good_qty AND grni_amount >= 0),
    CONSTRAINT ck_goods_receipt_lines_billed CHECK (
        billed_qty >= 0 AND returned_unbilled_qty >= 0 AND billed_qty + returned_unbilled_qty <= grni_qty),
    CONSTRAINT ck_goods_receipt_lines_returned CHECK (returned_qty >= 0 AND returned_qty <= good_qty),
    CONSTRAINT ck_goods_receipt_lines_dates CHECK (
        production_date IS NULL OR expiry_date IS NULL OR expiry_date >= production_date)
);
CREATE INDEX idx_goods_receipt_lines_purchase_order_line_id ON goods_receipt_lines (purchase_order_line_id);
CREATE INDEX idx_goods_receipt_lines_product_id ON goods_receipt_lines (product_id);
CREATE INDEX idx_goods_receipt_lines_batch_id ON goods_receipt_lines (batch_id);

-- Pemakaian packaging (maklon) atau bahan GM (reprocess) dari lokasi maklon: baik + reject (PRC-10).
CREATE TABLE goods_receipt_material_usages (
    id                  uuid          NOT NULL DEFAULT uuidv7(),
    goods_receipt_id    uuid          NOT NULL,
    line_no             integer       NOT NULL,
    material_product_id uuid          NOT NULL,
    from_location_id    uuid          NOT NULL,
    used_good_qty       numeric(18,3) NOT NULL DEFAULT 0,
    used_reject_qty     numeric(18,3) NOT NULL DEFAULT 0,
    -- HPP FIFO packaging terpakai (diisi saat diposting).
    cost_amount         numeric(18,2) NOT NULL DEFAULT 0,
    account_code        text          NOT NULL DEFAULT '',
    note                text          NOT NULL DEFAULT '',
    CONSTRAINT pk_goods_receipt_material_usages PRIMARY KEY (id),
    CONSTRAINT fk_goods_receipt_material_usages_goods_receipt_id FOREIGN KEY (goods_receipt_id)
        REFERENCES goods_receipts (id) ON DELETE CASCADE,
    CONSTRAINT uq_goods_receipt_material_usages_line_no UNIQUE (goods_receipt_id, line_no),
    CONSTRAINT uq_goods_receipt_material_usages_product UNIQUE (goods_receipt_id, material_product_id),
    CONSTRAINT ck_goods_receipt_material_usages_qty CHECK (
        used_good_qty >= 0 AND used_reject_qty >= 0 AND used_good_qty + used_reject_qty > 0)
);
CREATE INDEX idx_goods_receipt_material_usages_material_product_id ON goods_receipt_material_usages (material_product_id);

-- Rincian batch & HPP packaging yang keluar (untuk mengembalikan stok saat GR dibatalkan).
CREATE TABLE goods_receipt_material_consumptions (
    id        uuid          NOT NULL DEFAULT uuidv7(),
    usage_id  uuid          NOT NULL,
    batch_id  uuid,
    qty       numeric(18,3) NOT NULL,
    unit_cost numeric(18,4) NOT NULL,
    CONSTRAINT pk_goods_receipt_material_consumptions PRIMARY KEY (id),
    CONSTRAINT fk_goods_receipt_material_consumptions_usage_id FOREIGN KEY (usage_id)
        REFERENCES goods_receipt_material_usages (id) ON DELETE CASCADE,
    CONSTRAINT ck_goods_receipt_material_consumptions_qty CHECK (qty > 0)
);
CREATE INDEX idx_goods_receipt_material_consumptions_usage_id ON goods_receipt_material_consumptions (usage_id);

-- Tagihan pemasok per PO (PRC-07, A-18). Nomor = nomor tagihan dari pemasok, unik per pemasok.
CREATE TABLE supplier_bills (
    id                       uuid          NOT NULL DEFAULT uuidv7(),
    number                   text          NOT NULL,
    supplier_id              uuid          NOT NULL,
    purchase_order_id        uuid          NOT NULL,
    division_id              uuid          NOT NULL,
    bill_date                date          NOT NULL,
    due_date                 date          NOT NULL,
    note                     text          NOT NULL DEFAULT '',
    -- Subtotal = Σ qty × harga tagihan; grni = Σ qty × harga PO; selisih harga ke 5103.
    subtotal_amount          numeric(18,2) NOT NULL DEFAULT 0,
    grni_amount              numeric(18,2) NOT NULL DEFAULT 0,
    price_variance_amount    numeric(18,2) NOT NULL DEFAULT 0,
    withholding_type         text          NOT NULL DEFAULT 'none',
    withholding_rate         numeric(9,4)  NOT NULL DEFAULT 0,
    withholding_amount       numeric(18,2) NOT NULL DEFAULT 0,
    advance_applied_amount   numeric(18,2) NOT NULL DEFAULT 0,
    total_payable_amount     numeric(18,2) NOT NULL DEFAULT 0,
    paid_amount              numeric(18,2) NOT NULL DEFAULT 0,
    match_status             text          NOT NULL DEFAULT 'matched',
    status                   text          NOT NULL DEFAULT 'draft',
    journal_entry_id         uuid,
    advance_journal_entry_id uuid,
    cancel_reason            text          NOT NULL DEFAULT '',
    submitted_at             timestamptz,
    posted_at                timestamptz,
    created_at               timestamptz   NOT NULL DEFAULT now(),
    created_by               uuid          NOT NULL,
    updated_at               timestamptz   NOT NULL DEFAULT now(),
    updated_by               uuid          NOT NULL,
    version                  integer       NOT NULL DEFAULT 1,
    deleted_at               timestamptz,
    deleted_by               uuid,
    CONSTRAINT pk_supplier_bills PRIMARY KEY (id),
    CONSTRAINT fk_supplier_bills_purchase_order_id FOREIGN KEY (purchase_order_id) REFERENCES purchase_orders (id),
    CONSTRAINT ck_supplier_bills_status CHECK (status IN (
        'draft', 'pending_approval', 'rejected', 'awaiting_payment', 'partially_paid', 'paid', 'cancelled')),
    CONSTRAINT ck_supplier_bills_match_status CHECK (match_status IN ('matched', 'exceeds_tolerance', 'approved_exception')),
    CONSTRAINT ck_supplier_bills_withholding_type CHECK (withholding_type IN ('none', 'pph23', 'pph21', 'pph4_2')),
    CONSTRAINT ck_supplier_bills_withholding_rate CHECK (withholding_rate >= 0 AND withholding_rate <= 100),
    CONSTRAINT ck_supplier_bills_amounts CHECK (
        subtotal_amount >= 0 AND grni_amount >= 0 AND withholding_amount >= 0 AND advance_applied_amount >= 0
        AND paid_amount >= 0 AND total_payable_amount = subtotal_amount - withholding_amount - advance_applied_amount),
    CONSTRAINT ck_supplier_bills_due_date CHECK (due_date >= bill_date),
    CONSTRAINT ck_supplier_bills_posted CHECK (
        status NOT IN ('awaiting_payment', 'partially_paid', 'paid') OR journal_entry_id IS NOT NULL)
);
-- Nomor tagihan pemasok unik per pemasok; draft yang dibatalkan tidak mengunci nomor.
CREATE UNIQUE INDEX uq_supplier_bills_supplier_number ON supplier_bills (supplier_id, upper(number))
    WHERE status <> 'cancelled';
CREATE INDEX idx_supplier_bills_purchase_order_id ON supplier_bills (purchase_order_id);
CREATE INDEX idx_supplier_bills_status ON supplier_bills (status, bill_date DESC);

CREATE TABLE supplier_bill_lines (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    supplier_bill_id       uuid          NOT NULL,
    line_no                integer       NOT NULL,
    goods_receipt_line_id  uuid          NOT NULL,
    purchase_order_line_id uuid          NOT NULL,
    product_id             uuid          NOT NULL,
    qty                    numeric(18,3) NOT NULL,
    -- Qty GR yang belum ditagih saat tagihan disimpan/diposting (pembanding toleransi qty).
    receipt_open_qty       numeric(18,3) NOT NULL,
    unit_price_amount      numeric(18,2) NOT NULL,
    po_unit_price_amount   numeric(18,2) NOT NULL,
    line_amount            numeric(18,2) NOT NULL,
    grni_amount            numeric(18,2) NOT NULL,
    price_variance_amount  numeric(18,2) NOT NULL,
    exceeds_tolerance      boolean       NOT NULL DEFAULT false,
    note                   text          NOT NULL DEFAULT '',
    CONSTRAINT pk_supplier_bill_lines PRIMARY KEY (id),
    CONSTRAINT fk_supplier_bill_lines_supplier_bill_id FOREIGN KEY (supplier_bill_id)
        REFERENCES supplier_bills (id) ON DELETE CASCADE,
    CONSTRAINT fk_supplier_bill_lines_goods_receipt_line_id FOREIGN KEY (goods_receipt_line_id)
        REFERENCES goods_receipt_lines (id),
    CONSTRAINT uq_supplier_bill_lines_line_no UNIQUE (supplier_bill_id, line_no),
    CONSTRAINT uq_supplier_bill_lines_receipt_line UNIQUE (supplier_bill_id, goods_receipt_line_id),
    CONSTRAINT ck_supplier_bill_lines_qty CHECK (qty > 0),
    CONSTRAINT ck_supplier_bill_lines_amounts CHECK (unit_price_amount >= 0 AND line_amount >= 0 AND grni_amount >= 0)
);
CREATE INDEX idx_supplier_bill_lines_goods_receipt_line_id ON supplier_bill_lines (goods_receipt_line_id);

-- Uang muka yang dipotong tagihan (A-19 advance_applied).
CREATE TABLE supplier_bill_advances (
    id                   uuid          NOT NULL DEFAULT uuidv7(),
    supplier_bill_id     uuid          NOT NULL,
    purchase_advance_id  uuid          NOT NULL,
    amount               numeric(18,2) NOT NULL,
    CONSTRAINT pk_supplier_bill_advances PRIMARY KEY (id),
    CONSTRAINT fk_supplier_bill_advances_supplier_bill_id FOREIGN KEY (supplier_bill_id)
        REFERENCES supplier_bills (id) ON DELETE CASCADE,
    CONSTRAINT fk_supplier_bill_advances_purchase_advance_id FOREIGN KEY (purchase_advance_id)
        REFERENCES purchase_advances (id),
    CONSTRAINT ck_supplier_bill_advances_amount CHECK (amount > 0)
);
CREATE INDEX idx_supplier_bill_advances_supplier_bill_id ON supplier_bill_advances (supplier_bill_id);
CREATE INDEX idx_supplier_bill_advances_purchase_advance_id ON supplier_bill_advances (purchase_advance_id);

-- Retur pembelian (RB) + nota debit (DN) (ACC-16, A-02). origin qc_* diposting tanpa approval (D-24).
CREATE TABLE purchase_returns (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    number            text,
    debit_note_number text,
    supplier_id       uuid          NOT NULL,
    supplier_name     text          NOT NULL,
    purchase_order_id uuid          NOT NULL,
    division_id       uuid          NOT NULL,
    origin            text          NOT NULL DEFAULT 'manual',
    -- Dokumen QC asal (Inbound/Outbound) untuk origin qc_*.
    source_type       text          NOT NULL DEFAULT '',
    source_id         uuid,
    source_label      text          NOT NULL DEFAULT '',
    business_date     date          NOT NULL,
    reason            text          NOT NULL,
    note              text          NOT NULL DEFAULT '',
    total_amount      numeric(18,2) NOT NULL DEFAULT 0,
    -- Bagian yang mengurangi 2102 (belum ditagih) dan 2101 (sudah ditagih, nota debit ke utang).
    unbilled_amount   numeric(18,2) NOT NULL DEFAULT 0,
    billed_amount     numeric(18,2) NOT NULL DEFAULT 0,
    status            text          NOT NULL DEFAULT 'draft',
    journal_entry_id  uuid,
    cancel_reason     text          NOT NULL DEFAULT '',
    submitted_at      timestamptz,
    approved_at       timestamptz,
    posted_at         timestamptz,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    updated_by        uuid          NOT NULL,
    version           integer       NOT NULL DEFAULT 1,
    deleted_at        timestamptz,
    deleted_by        uuid,
    CONSTRAINT pk_purchase_returns PRIMARY KEY (id),
    CONSTRAINT uq_purchase_returns_number UNIQUE (number),
    CONSTRAINT uq_purchase_returns_debit_note_number UNIQUE (debit_note_number),
    CONSTRAINT fk_purchase_returns_purchase_order_id FOREIGN KEY (purchase_order_id) REFERENCES purchase_orders (id),
    CONSTRAINT ck_purchase_returns_origin CHECK (origin IN ('manual', 'qc_inbound', 'qc_outbound')),
    CONSTRAINT ck_purchase_returns_status CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'rejected', 'posted', 'cancelled')),
    CONSTRAINT ck_purchase_returns_posted CHECK (
        status <> 'posted' OR (number IS NOT NULL AND debit_note_number IS NOT NULL AND journal_entry_id IS NOT NULL)),
    CONSTRAINT ck_purchase_returns_amounts CHECK (
        total_amount >= 0 AND unbilled_amount >= 0 AND billed_amount >= 0
        AND (status <> 'posted' OR total_amount = unbilled_amount + billed_amount))
);
CREATE INDEX idx_purchase_returns_purchase_order_id ON purchase_returns (purchase_order_id);
CREATE INDEX idx_purchase_returns_status ON purchase_returns (status, business_date DESC);
CREATE INDEX idx_purchase_returns_supplier_id ON purchase_returns (supplier_id);
CREATE UNIQUE INDEX uq_purchase_returns_source ON purchase_returns (source_type, source_id)
    WHERE source_id IS NOT NULL AND status <> 'cancelled';

CREATE TABLE purchase_return_lines (
    id                     uuid          NOT NULL DEFAULT uuidv7(),
    purchase_return_id     uuid          NOT NULL,
    line_no                integer       NOT NULL,
    goods_receipt_line_id  uuid          NOT NULL,
    product_id             uuid          NOT NULL,
    batch_id               uuid,
    location_id            uuid          NOT NULL,
    stock_status           text          NOT NULL DEFAULT 'quarantine',
    qty                    numeric(18,3) NOT NULL,
    unit_cost              numeric(18,4) NOT NULL DEFAULT 0,
    amount                 numeric(18,2) NOT NULL DEFAULT 0,
    unbilled_qty           numeric(18,3) NOT NULL DEFAULT 0,
    unbilled_amount        numeric(18,2) NOT NULL DEFAULT 0,
    inventory_account_code text          NOT NULL DEFAULT '',
    note                   text          NOT NULL DEFAULT '',
    CONSTRAINT pk_purchase_return_lines PRIMARY KEY (id),
    CONSTRAINT fk_purchase_return_lines_purchase_return_id FOREIGN KEY (purchase_return_id)
        REFERENCES purchase_returns (id) ON DELETE CASCADE,
    CONSTRAINT fk_purchase_return_lines_goods_receipt_line_id FOREIGN KEY (goods_receipt_line_id)
        REFERENCES goods_receipt_lines (id),
    CONSTRAINT uq_purchase_return_lines_line_no UNIQUE (purchase_return_id, line_no),
    CONSTRAINT ck_purchase_return_lines_status CHECK (stock_status IN ('quarantine', 'available')),
    CONSTRAINT ck_purchase_return_lines_qty CHECK (qty > 0 AND unbilled_qty >= 0 AND unbilled_qty <= qty),
    CONSTRAINT ck_purchase_return_lines_amounts CHECK (amount >= 0 AND unbilled_amount >= 0 AND unbilled_amount <= amount)
);
CREATE INDEX idx_purchase_return_lines_goods_receipt_line_id ON purchase_return_lines (goods_receipt_line_id);
CREATE INDEX idx_purchase_return_lines_product_id ON purchase_return_lines (product_id);

-- Utang pemasok terbuka (sub-ledger 2101) dari tagihan (positif) dan nota debit (negatif). Dipakai pembayaran
-- R1.4 lewat API procurement.OpenPayables.
CREATE TABLE supplier_payables (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    document_type     text          NOT NULL,
    document_id       uuid          NOT NULL,
    document_number   text          NOT NULL,
    supplier_id       uuid          NOT NULL,
    purchase_order_id uuid          NOT NULL,
    division_id       uuid          NOT NULL,
    business_date     date          NOT NULL,
    due_date          date          NOT NULL,
    amount            numeric(18,2) NOT NULL,
    open_amount       numeric(18,2) NOT NULL,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    updated_by        uuid          NOT NULL,
    version           integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_supplier_payables PRIMARY KEY (id),
    CONSTRAINT uq_supplier_payables_document UNIQUE (document_type, document_id),
    CONSTRAINT ck_supplier_payables_document_type CHECK (document_type IN ('supplier_bill', 'debit_note')),
    CONSTRAINT ck_supplier_payables_sign CHECK (
        (document_type = 'supplier_bill' AND amount >= 0 AND open_amount >= 0 AND open_amount <= amount)
        OR (document_type = 'debit_note' AND amount <= 0 AND open_amount <= 0 AND open_amount >= amount))
);
CREATE INDEX idx_supplier_payables_supplier_id ON supplier_payables (supplier_id, due_date) WHERE open_amount <> 0;

-- Penyesuaian posting rule (Desain Finance & Accounting 2.1):
-- A-02: 2101 (sudah ditagih) dan 2102 (belum ditagih) dipisah per nilai bernama agar satu retur bisa mengurangi
-- keduanya; akun persediaan tetap dipilih per baris (Splits).
UPDATE posting_rule_accounts
SET account_resolver = 'fixed', account_id = (SELECT id FROM accounts WHERE code = '2101'),
    updated_at = now(), version = version + 1
WHERE posting_rule_code = 'A-02' AND amount_key = 'return_value' AND side = 'debit';
INSERT INTO posting_rule_accounts (
    posting_rule_code, amount_key, side, account_resolver, account_id, valid_from, created_by, updated_by)
SELECT 'A-02', 'unbilled_return_value', m.side, m.resolver, a.id, DATE '2026-01-01',
       '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'
FROM (VALUES ('debit', 'fixed', '2102'), ('credit', 'location_inventory_account', '1311')) AS m (side, resolver, code)
JOIN accounts a ON a.code = m.code;
-- A-18: akun PPh dipilih dari jenis PPh default pemasok (PPh 21 → 2301, PPh 23 → 2302, PPh 4(2) → 2303).
UPDATE posting_rule_accounts
SET account_resolver = 'withholding_account', updated_at = now(), version = version + 1
WHERE posting_rule_code = 'A-18' AND amount_key = 'withholding_value';

-- +goose Down
UPDATE posting_rule_accounts SET account_resolver = 'fixed'
WHERE posting_rule_code = 'A-18' AND amount_key = 'withholding_value';
DELETE FROM posting_rule_accounts WHERE posting_rule_code = 'A-02' AND amount_key = 'unbilled_return_value';
UPDATE posting_rule_accounts SET account_resolver = 'payable_account'
WHERE posting_rule_code = 'A-02' AND amount_key = 'return_value' AND side = 'debit';
DROP TABLE supplier_payables;
DROP TABLE purchase_return_lines;
DROP TABLE purchase_returns;
DROP TABLE supplier_bill_advances;
DROP TABLE supplier_bill_lines;
DROP TABLE supplier_bills;
DROP TABLE goods_receipt_material_consumptions;
DROP TABLE goods_receipt_material_usages;
DROP TABLE goods_receipt_lines;
DROP TABLE goods_receipts;
