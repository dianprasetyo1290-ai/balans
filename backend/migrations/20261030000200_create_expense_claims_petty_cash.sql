-- +goose Up
-- Pengajuan biaya/reimbursement (S-404, ACC-11, A-15) dan kas kecil imprest (S-405, ACC-12, A-16, A-17).
-- Desain Status Dokumen 6.1 – 6.2.

CREATE TABLE expense_claims (
    id                 uuid          NOT NULL DEFAULT uuidv7(),
    number             text,
    claim_type         text          NOT NULL DEFAULT 'expense',
    requester_id       uuid          NOT NULL,
    division_id        uuid          NOT NULL,
    campaign_id        uuid,
    payee_type         text          NOT NULL DEFAULT 'other',
    payee_id           uuid,
    payee_name         text          NOT NULL,
    payee_tax_id_enc   text          NOT NULL DEFAULT '',
    claim_date         date          NOT NULL,
    description        text          NOT NULL DEFAULT '',
    total_amount       numeric(18,2) NOT NULL DEFAULT 0,
    withholding_amount numeric(18,2) NOT NULL DEFAULT 0,
    payable_amount     numeric(18,2) NOT NULL DEFAULT 0,
    status             text          NOT NULL DEFAULT 'draft',
    submitted_at       timestamptz,
    decided_at         timestamptz,
    decision_note      text          NOT NULL DEFAULT '',
    verified_at        timestamptz,
    verified_by        uuid,
    verify_date        date,
    journal_entry_id   uuid,
    payment_id         uuid,
    paid_at            timestamptz,
    cancel_reason      text          NOT NULL DEFAULT '',
    created_at         timestamptz   NOT NULL DEFAULT now(),
    created_by         uuid          NOT NULL,
    updated_at         timestamptz   NOT NULL DEFAULT now(),
    updated_by         uuid          NOT NULL,
    version            integer       NOT NULL DEFAULT 1,
    deleted_at         timestamptz,
    deleted_by         uuid,
    CONSTRAINT pk_expense_claims PRIMARY KEY (id),
    CONSTRAINT uq_expense_claims_number UNIQUE (number),
    CONSTRAINT fk_expense_claims_division_id FOREIGN KEY (division_id) REFERENCES divisions (id),
    CONSTRAINT fk_expense_claims_campaign_id FOREIGN KEY (campaign_id) REFERENCES campaigns (id),
    CONSTRAINT fk_expense_claims_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT fk_expense_claims_payment_id FOREIGN KEY (payment_id) REFERENCES payments (id),
    CONSTRAINT ck_expense_claims_type CHECK (claim_type IN ('expense', 'reimbursement')),
    CONSTRAINT ck_expense_claims_payee_type CHECK (payee_type IN ('employee', 'supplier', 'other')),
    CONSTRAINT ck_expense_claims_payee_name CHECK (btrim(payee_name) <> ''),
    CONSTRAINT ck_expense_claims_status CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'rejected', 'awaiting_payment', 'paid', 'cancelled')),
    CONSTRAINT ck_expense_claims_amounts CHECK (total_amount >= 0 AND withholding_amount >= 0
        AND payable_amount = total_amount - withholding_amount),
    CONSTRAINT ck_expense_claims_number CHECK (status IN ('draft', 'cancelled') OR number IS NOT NULL),
    CONSTRAINT ck_expense_claims_verified CHECK (status NOT IN ('awaiting_payment', 'paid') OR journal_entry_id IS NOT NULL),
    CONSTRAINT ck_expense_claims_deleted CHECK (deleted_at IS NULL OR status = 'draft')
);
CREATE INDEX idx_expense_claims_division_id ON expense_claims (division_id);
CREATE INDEX idx_expense_claims_campaign_id ON expense_claims (campaign_id);
CREATE INDEX idx_expense_claims_journal_entry_id ON expense_claims (journal_entry_id);
CREATE INDEX idx_expense_claims_payment_id ON expense_claims (payment_id);
CREATE INDEX idx_expense_claims_requester ON expense_claims (requester_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_expense_claims_status ON expense_claims (status, created_at DESC) WHERE deleted_at IS NULL;

-- Baris biaya. Kolom withholding_* diisi saat verifikasi F&A (PPh per baris dari kalkulator tax).
CREATE TABLE expense_claim_lines (
    id                 uuid          NOT NULL DEFAULT uuidv7(),
    claim_id           uuid          NOT NULL,
    line_no            integer       NOT NULL,
    expense_type_id    uuid          NOT NULL,
    description        text          NOT NULL DEFAULT '',
    amount             numeric(18,2) NOT NULL,
    receipt_file_id    uuid,
    withholding_type   text          NOT NULL DEFAULT 'none',
    object_code        text          NOT NULL DEFAULT '',
    withholding_rate   numeric(9,4)  NOT NULL DEFAULT 0,
    withholding_amount numeric(18,2) NOT NULL DEFAULT 0,
    created_at         timestamptz   NOT NULL DEFAULT now(),
    created_by         uuid          NOT NULL,
    CONSTRAINT pk_expense_claim_lines PRIMARY KEY (id),
    CONSTRAINT uq_expense_claim_lines_line_no UNIQUE (claim_id, line_no),
    CONSTRAINT fk_expense_claim_lines_claim_id FOREIGN KEY (claim_id) REFERENCES expense_claims (id) ON DELETE CASCADE,
    CONSTRAINT fk_expense_claim_lines_expense_type_id FOREIGN KEY (expense_type_id) REFERENCES expense_types (id),
    CONSTRAINT ck_expense_claim_lines_amount CHECK (amount > 0),
    CONSTRAINT ck_expense_claim_lines_withholding CHECK (withholding_type IN ('none', 'pph21', 'pph23', 'pph4_2')),
    CONSTRAINT ck_expense_claim_lines_withholding_amount CHECK (withholding_amount >= 0 AND withholding_amount <= amount)
);
CREATE INDEX idx_expense_claim_lines_expense_type_id ON expense_claim_lines (expense_type_id);

-- Dana kas kecil (imprest): plafon per dana, saldo diperbarui setiap pengeluaran/pengisian diposting.
-- Akun GL selalu 1102 Kas Kecil (rule A-16/A-17 fixed).
CREATE TABLE petty_cash_funds (
    id             uuid          NOT NULL DEFAULT uuidv7(),
    code           text          NOT NULL,
    name           text          NOT NULL,
    holder_user_id uuid          NOT NULL,
    division_id    uuid          NOT NULL,
    ceiling_amount numeric(18,2) NOT NULL,
    balance_amount numeric(18,2) NOT NULL DEFAULT 0,
    is_active      boolean       NOT NULL DEFAULT true,
    description    text          NOT NULL DEFAULT '',
    created_at     timestamptz   NOT NULL DEFAULT now(),
    created_by     uuid          NOT NULL,
    updated_at     timestamptz   NOT NULL DEFAULT now(),
    updated_by     uuid          NOT NULL,
    version        integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_petty_cash_funds PRIMARY KEY (id),
    CONSTRAINT uq_petty_cash_funds_code UNIQUE (code),
    CONSTRAINT fk_petty_cash_funds_division_id FOREIGN KEY (division_id) REFERENCES divisions (id),
    CONSTRAINT ck_petty_cash_funds_code CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{1,29}$'),
    CONSTRAINT ck_petty_cash_funds_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_petty_cash_funds_ceiling CHECK (ceiling_amount > 0),
    CONSTRAINT ck_petty_cash_funds_balance CHECK (balance_amount >= 0 AND balance_amount <= ceiling_amount)
);
CREATE INDEX idx_petty_cash_funds_division_id ON petty_cash_funds (division_id);

-- Transaksi kas kecil (KK): pengeluaran `draft → posted` (A-17), pengisian kembali
-- `draft → pending_approval → approved → posted` (A-16).
CREATE TABLE petty_cash_transactions (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    number           text,
    fund_id          uuid          NOT NULL,
    transaction_type text          NOT NULL,
    business_date    date          NOT NULL,
    expense_type_id  uuid,
    division_id      uuid          NOT NULL,
    campaign_id      uuid,
    description      text          NOT NULL DEFAULT '',
    amount           numeric(18,2) NOT NULL,
    receipt_file_id  uuid,
    bank_account_id  uuid,
    status           text          NOT NULL DEFAULT 'draft',
    decision_note    text          NOT NULL DEFAULT '',
    journal_entry_id uuid,
    posted_at        timestamptz,
    posted_by        uuid,
    cancel_reason    text          NOT NULL DEFAULT '',
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    updated_at       timestamptz   NOT NULL DEFAULT now(),
    updated_by       uuid          NOT NULL,
    version          integer       NOT NULL DEFAULT 1,
    CONSTRAINT pk_petty_cash_transactions PRIMARY KEY (id),
    CONSTRAINT uq_petty_cash_transactions_number UNIQUE (number),
    CONSTRAINT fk_petty_cash_transactions_fund_id FOREIGN KEY (fund_id) REFERENCES petty_cash_funds (id),
    CONSTRAINT fk_petty_cash_transactions_expense_type_id FOREIGN KEY (expense_type_id) REFERENCES expense_types (id),
    CONSTRAINT fk_petty_cash_transactions_division_id FOREIGN KEY (division_id) REFERENCES divisions (id),
    CONSTRAINT fk_petty_cash_transactions_campaign_id FOREIGN KEY (campaign_id) REFERENCES campaigns (id),
    CONSTRAINT fk_petty_cash_transactions_bank_account_id FOREIGN KEY (bank_account_id) REFERENCES bank_accounts (id),
    CONSTRAINT fk_petty_cash_transactions_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT ck_petty_cash_transactions_type CHECK (transaction_type IN ('expense', 'replenishment')),
    CONSTRAINT ck_petty_cash_transactions_amount CHECK (amount > 0),
    CONSTRAINT ck_petty_cash_transactions_status CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'rejected', 'posted', 'cancelled')),
    CONSTRAINT ck_petty_cash_transactions_expense CHECK (transaction_type <> 'expense' OR expense_type_id IS NOT NULL),
    CONSTRAINT ck_petty_cash_transactions_posted CHECK (status <> 'posted' OR (number IS NOT NULL AND journal_entry_id IS NOT NULL))
);
CREATE INDEX idx_petty_cash_transactions_fund_id ON petty_cash_transactions (fund_id, business_date DESC);
CREATE INDEX idx_petty_cash_transactions_expense_type_id ON petty_cash_transactions (expense_type_id);
CREATE INDEX idx_petty_cash_transactions_division_id ON petty_cash_transactions (division_id);
CREATE INDEX idx_petty_cash_transactions_campaign_id ON petty_cash_transactions (campaign_id);
CREATE INDEX idx_petty_cash_transactions_bank_account_id ON petty_cash_transactions (bank_account_id);
CREATE INDEX idx_petty_cash_transactions_journal_entry_id ON petty_cash_transactions (journal_entry_id);

-- +goose Down
DROP TABLE petty_cash_transactions;
DROP TABLE petty_cash_funds;
DROP TABLE expense_claim_lines;
DROP TABLE expense_claims;
