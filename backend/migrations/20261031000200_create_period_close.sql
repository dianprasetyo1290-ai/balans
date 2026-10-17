-- +goose Up
-- Tutup buku dengan checklist 9 langkah (ACC-08, BRD 5.15) dan buka kembali periode dengan approval (ACC-14).

-- Periode `pending_approval` = permintaan buka kembali menunggu Head F&A → Direksi; tetap terkunci.
ALTER TABLE fiscal_periods DROP CONSTRAINT ck_fiscal_periods_status;
ALTER TABLE fiscal_periods DROP CONSTRAINT ck_fiscal_periods_closed;
ALTER TABLE fiscal_periods
    ADD CONSTRAINT ck_fiscal_periods_status CHECK (status IN ('open', 'closed', 'pending_approval')),
    ADD CONSTRAINT ck_fiscal_periods_closed CHECK ((status <> 'open') = (closed_at IS NOT NULL));

-- Satu baris per langkah per periode; dibuat saat checklist pertama dibuka. pic_user_id = PIC yang ditunjuk.
CREATE TABLE period_close_checklists (
    id               uuid        NOT NULL DEFAULT uuidv7(),
    fiscal_period_id uuid        NOT NULL,
    step_code        text        NOT NULL,
    status           text        NOT NULL DEFAULT 'pending',
    pic_user_id      uuid,
    completed_by     uuid,
    completed_at     timestamptz,
    note             text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_period_close_checklists PRIMARY KEY (id),
    CONSTRAINT uq_period_close_checklists_step UNIQUE (fiscal_period_id, step_code),
    CONSTRAINT fk_period_close_checklists_fiscal_period_id FOREIGN KEY (fiscal_period_id) REFERENCES fiscal_periods (id),
    CONSTRAINT ck_period_close_checklists_step_code CHECK (step_code IN (
        'operational_documents', 'consignment_vending_reconciliation', 'settlement_reconciliation',
        'bank_petty_cash_reconciliation', 'payroll', 'depreciation', 'tax_recap', 'inventory_review',
        'financial_review')),
    CONSTRAINT ck_period_close_checklists_status CHECK (status IN ('pending', 'done', 'skipped')),
    CONSTRAINT ck_period_close_checklists_completed CHECK ((status = 'pending') = (completed_at IS NULL)),
    CONSTRAINT ck_period_close_checklists_note CHECK (length(note) <= 1000)
);

-- Permintaan buka kembali periode (approval `fiscal_period_reopen`). Satu permintaan berjalan per periode.
CREATE TABLE fiscal_period_reopens (
    id               uuid        NOT NULL DEFAULT uuidv7(),
    fiscal_period_id uuid        NOT NULL,
    reason           text        NOT NULL,
    status           text        NOT NULL DEFAULT 'pending_approval',
    requested_by     uuid        NOT NULL,
    requested_at     timestamptz NOT NULL DEFAULT now(),
    decided_by       uuid,
    decided_at       timestamptz,
    decision_note    text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_fiscal_period_reopens PRIMARY KEY (id),
    CONSTRAINT fk_fiscal_period_reopens_fiscal_period_id FOREIGN KEY (fiscal_period_id) REFERENCES fiscal_periods (id),
    CONSTRAINT ck_fiscal_period_reopens_status CHECK (status IN ('pending_approval', 'approved', 'rejected')),
    CONSTRAINT ck_fiscal_period_reopens_reason CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
    CONSTRAINT ck_fiscal_period_reopens_decided CHECK ((status = 'pending_approval') = (decided_at IS NULL))
);
CREATE INDEX idx_fiscal_period_reopens_fiscal_period_id ON fiscal_period_reopens (fiscal_period_id, requested_at DESC);
CREATE UNIQUE INDEX uq_fiscal_period_reopens_pending ON fiscal_period_reopens (fiscal_period_id)
    WHERE status = 'pending_approval';

-- +goose Down
DROP TABLE fiscal_period_reopens;
DROP TABLE period_close_checklists;
UPDATE fiscal_periods SET status = 'closed' WHERE status = 'pending_approval';
ALTER TABLE fiscal_periods DROP CONSTRAINT ck_fiscal_periods_status;
ALTER TABLE fiscal_periods DROP CONSTRAINT ck_fiscal_periods_closed;
ALTER TABLE fiscal_periods
    ADD CONSTRAINT ck_fiscal_periods_status CHECK (status IN ('open', 'closed')),
    ADD CONSTRAINT ck_fiscal_periods_closed CHECK ((status = 'closed') = (closed_at IS NOT NULL));
