-- +goose Up
-- Posting rule, jurnal, dan baris jurnal (ACC-01, ACC-02, ACC-13, ACC-17; Technical Guideline 5.8, 6.3).

-- Rule jurnal otomatis A-01 … A-36 (Desain Finance & Accounting bagian 2). Dimensi wajib per rule:
-- channel, sub_channel, division, campaign.
CREATE TABLE posting_rules (
    id                  uuid        NOT NULL DEFAULT uuidv7(),
    code                text        NOT NULL,
    name                text        NOT NULL,
    description         text        NOT NULL DEFAULT '',
    required_dimensions text[]      NOT NULL DEFAULT '{}',
    release             text        NOT NULL DEFAULT '',
    is_active           boolean     NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid        NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid        NOT NULL,
    version             integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_posting_rules PRIMARY KEY (id),
    CONSTRAINT uq_posting_rules_code UNIQUE (code),
    CONSTRAINT ck_posting_rules_required_dimensions CHECK (
        required_dimensions <@ ARRAY['channel', 'sub_channel', 'division', 'campaign']::text[])
);

-- Pemetaan nilai bernama (amount_key) ke sisi debit/kredit dan akun. Effective-dated agar Head F&A bisa
-- menyesuaikan akun tanpa ubah kode (Technical Guideline 5.8.1, 5.14). Resolver selain `fixed` berarti akun
-- dipilih dokumen sumber (mis. akun persediaan sesuai tipe lokasi); account_id menjadi akun default.
CREATE TABLE posting_rule_accounts (
    id                uuid        NOT NULL DEFAULT uuidv7(),
    posting_rule_code text        NOT NULL,
    amount_key        text        NOT NULL,
    side              text        NOT NULL,
    account_resolver  text        NOT NULL DEFAULT 'fixed',
    account_id        uuid,
    valid_from        date        NOT NULL,
    valid_to          date,
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid        NOT NULL,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    updated_by        uuid        NOT NULL,
    version           integer     NOT NULL DEFAULT 1,
    CONSTRAINT pk_posting_rule_accounts PRIMARY KEY (id),
    CONSTRAINT fk_posting_rule_accounts_posting_rule_code FOREIGN KEY (posting_rule_code) REFERENCES posting_rules (code),
    CONSTRAINT fk_posting_rule_accounts_account_id FOREIGN KEY (account_id) REFERENCES accounts (id),
    CONSTRAINT ck_posting_rule_accounts_side CHECK (side IN ('debit', 'credit')),
    CONSTRAINT ck_posting_rule_accounts_amount_key CHECK (amount_key ~ '^[a-z][a-z0-9_]{1,49}$'),
    CONSTRAINT ck_posting_rule_accounts_resolver CHECK (account_resolver IN (
        'fixed', 'location_inventory_account', 'material_account', 'receivable_account', 'payable_account',
        'bank_account', 'expense_type_account', 'withholding_account', 'payment_account', 'fixed_asset_account',
        'accumulated_depreciation_account', 'counter_account', 'opening_balance_account')),
    CONSTRAINT ck_posting_rule_accounts_fixed CHECK (account_resolver <> 'fixed' OR account_id IS NOT NULL),
    CONSTRAINT ck_posting_rule_accounts_validity CHECK (valid_to IS NULL OR valid_to > valid_from),
    CONSTRAINT ex_posting_rule_accounts_period EXCLUDE USING gist (
        posting_rule_code WITH =, amount_key WITH =, side WITH =, daterange(valid_from, valid_to) WITH &&)
);
CREATE INDEX idx_posting_rule_accounts_account_id ON posting_rule_accounts (account_id);

-- Satu jurnal. Jurnal Umum manual: source_type `manual_journal`, source_id = id sendiri. Jurnal otomatis:
-- dokumen sumber + posting_rule_code. Balik Jurnal: source_type `journal_reversal`, source_id = jurnal asal.
-- NULLS NOT DISTINCT: satu dokumen sumber hanya sekali per rule, termasuk satu pembalik per jurnal.
CREATE TABLE journal_entries (
    id                uuid          NOT NULL DEFAULT uuidv7(),
    number            text,
    business_date     date          NOT NULL,
    fiscal_period_id  uuid,
    entry_type        text          NOT NULL,
    source_type       text          NOT NULL,
    source_id         uuid          NOT NULL,
    source_label      text          NOT NULL DEFAULT '',
    posting_rule_code text,
    description       text          NOT NULL DEFAULT '',
    reversal_of_id    uuid,
    status            text          NOT NULL DEFAULT 'draft',
    total_amount      numeric(18,2) NOT NULL DEFAULT 0,
    posted_at         timestamptz,
    posted_by         uuid,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    created_by        uuid          NOT NULL,
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    updated_by        uuid          NOT NULL,
    version           integer       NOT NULL DEFAULT 1,
    deleted_at        timestamptz,
    deleted_by        uuid,
    CONSTRAINT pk_journal_entries PRIMARY KEY (id),
    CONSTRAINT uq_journal_entries_number UNIQUE (number),
    CONSTRAINT uq_journal_entries_source UNIQUE NULLS NOT DISTINCT (source_type, source_id, posting_rule_code),
    CONSTRAINT fk_journal_entries_fiscal_period_id FOREIGN KEY (fiscal_period_id) REFERENCES fiscal_periods (id),
    CONSTRAINT fk_journal_entries_posting_rule_code FOREIGN KEY (posting_rule_code) REFERENCES posting_rules (code),
    CONSTRAINT fk_journal_entries_reversal_of_id FOREIGN KEY (reversal_of_id) REFERENCES journal_entries (id),
    CONSTRAINT ck_journal_entries_entry_type CHECK (entry_type IN ('manual', 'automatic', 'reversal')),
    CONSTRAINT ck_journal_entries_status CHECK (status IN ('draft', 'posted')),
    CONSTRAINT ck_journal_entries_posted CHECK (status <> 'posted' OR (
        number IS NOT NULL AND fiscal_period_id IS NOT NULL AND posted_at IS NOT NULL AND posted_by IS NOT NULL)),
    CONSTRAINT ck_journal_entries_rule CHECK ((entry_type = 'automatic') = (posting_rule_code IS NOT NULL)),
    CONSTRAINT ck_journal_entries_reversal CHECK ((entry_type = 'reversal') = (reversal_of_id IS NOT NULL)),
    CONSTRAINT ck_journal_entries_deleted CHECK (deleted_at IS NULL OR status = 'draft'),
    CONSTRAINT ck_journal_entries_total CHECK (total_amount >= 0)
);
CREATE INDEX idx_journal_entries_business_date ON journal_entries (business_date DESC, number DESC);
CREATE INDEX idx_journal_entries_fiscal_period_id ON journal_entries (fiscal_period_id);
CREATE INDEX idx_journal_entries_posting_rule_code ON journal_entries (posting_rule_code);
CREATE INDEX idx_journal_entries_reversal_of_id ON journal_entries (reversal_of_id);
CREATE INDEX idx_journal_entries_status ON journal_entries (status, created_at DESC) WHERE deleted_at IS NULL;

-- Baris jurnal append-only (Technical Guideline 6.2): setelah jurnal diposting tidak ada insert/update/delete.
-- Satu baris hanya debit atau kredit (ck_journal_lines_debit_xor_credit). Dimensi ACC-17.
CREATE TABLE journal_lines (
    id               uuid          NOT NULL DEFAULT uuidv7(),
    journal_entry_id uuid          NOT NULL,
    line_no          integer       NOT NULL,
    account_id       uuid          NOT NULL,
    debit_amount     numeric(18,2) NOT NULL DEFAULT 0,
    credit_amount    numeric(18,2) NOT NULL DEFAULT 0,
    channel_id       uuid,
    sub_channel_id   uuid,
    division_id      uuid,
    campaign_id      uuid,
    party_type       text          NOT NULL DEFAULT '',
    party_id         uuid,
    amount_key       text          NOT NULL DEFAULT '',
    memo             text          NOT NULL DEFAULT '',
    created_at       timestamptz   NOT NULL DEFAULT now(),
    created_by       uuid          NOT NULL,
    CONSTRAINT pk_journal_lines PRIMARY KEY (id),
    CONSTRAINT uq_journal_lines_line_no UNIQUE (journal_entry_id, line_no),
    CONSTRAINT fk_journal_lines_journal_entry_id FOREIGN KEY (journal_entry_id) REFERENCES journal_entries (id),
    CONSTRAINT fk_journal_lines_account_id FOREIGN KEY (account_id) REFERENCES accounts (id),
    CONSTRAINT fk_journal_lines_channel_id FOREIGN KEY (channel_id) REFERENCES channels (id),
    CONSTRAINT fk_journal_lines_sub_channel_id FOREIGN KEY (sub_channel_id) REFERENCES sub_channels (id),
    CONSTRAINT fk_journal_lines_division_id FOREIGN KEY (division_id) REFERENCES divisions (id),
    CONSTRAINT fk_journal_lines_campaign_id FOREIGN KEY (campaign_id) REFERENCES campaigns (id),
    CONSTRAINT ck_journal_lines_debit_xor_credit CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR (credit_amount > 0 AND debit_amount = 0)),
    CONSTRAINT ck_journal_lines_party CHECK ((party_type = '') = (party_id IS NULL)),
    CONSTRAINT ck_journal_lines_sub_channel CHECK (sub_channel_id IS NULL OR channel_id IS NOT NULL)
);
CREATE INDEX idx_journal_lines_account_id ON journal_lines (account_id);
CREATE INDEX idx_journal_lines_channel_id ON journal_lines (channel_id);
CREATE INDEX idx_journal_lines_sub_channel_id ON journal_lines (sub_channel_id);
CREATE INDEX idx_journal_lines_division_id ON journal_lines (division_id);
CREATE INDEX idx_journal_lines_campaign_id ON journal_lines (campaign_id);
CREATE INDEX idx_journal_lines_party ON journal_lines (party_type, party_id) WHERE party_id IS NOT NULL;

-- Jurnal yang sudah diposting tidak bisa diubah atau dihapus (ACC-13, Technical Guideline 5.8.3). Saat
-- draft diposting, DB memeriksa ulang balance agar aturan tidak hanya dijaga di Go.
-- +goose StatementBegin
CREATE FUNCTION journal_entries_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    total_debit  numeric(18,2);
    total_credit numeric(18,2);
    line_count   integer;
BEGIN
    IF OLD.status = 'posted' THEN
        RAISE EXCEPTION 'journal entry % is posted and immutable', OLD.id USING ERRCODE = 'BL001';
    END IF;
    IF TG_OP = 'UPDATE' AND NEW.status = 'posted' THEN
        SELECT coalesce(sum(debit_amount), 0), coalesce(sum(credit_amount), 0), count(*)
        INTO total_debit, total_credit, line_count
        FROM journal_lines WHERE journal_entry_id = NEW.id;
        IF line_count < 2 OR total_debit <> total_credit OR total_debit <= 0 OR total_debit <> NEW.total_amount THEN
            RAISE EXCEPTION 'journal entry % is not balanced (debit %, credit %, lines %)',
                NEW.id, total_debit, total_credit, line_count USING ERRCODE = 'BL002';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_journal_entries_guard
    BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION journal_entries_guard();

-- +goose StatementBegin
CREATE FUNCTION journal_lines_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    entry_status text;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'journal_lines is append-only' USING ERRCODE = 'BL001';
    END IF;
    SELECT status INTO entry_status FROM journal_entries
    WHERE id = CASE WHEN TG_OP = 'DELETE' THEN OLD.journal_entry_id ELSE NEW.journal_entry_id END;
    IF entry_status = 'posted' THEN
        RAISE EXCEPTION 'journal entry is posted and immutable' USING ERRCODE = 'BL001';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_journal_lines_guard
    BEFORE INSERT OR UPDATE OR DELETE ON journal_lines
    FOR EACH ROW EXECUTE FUNCTION journal_lines_guard();

-- +goose Down
DROP TABLE journal_lines;
DROP TABLE journal_entries;
DROP FUNCTION journal_lines_guard();
DROP FUNCTION journal_entries_guard();
DROP TABLE posting_rule_accounts;
DROP TABLE posting_rules;
