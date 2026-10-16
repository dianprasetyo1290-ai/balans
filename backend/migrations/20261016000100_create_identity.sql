-- +goose Up
CREATE TABLE roles (
    id           uuid        NOT NULL DEFAULT uuidv7(),
    code         text        NOT NULL,
    name         text        NOT NULL,
    description  text        NOT NULL DEFAULT '',
    requires_mfa boolean     NOT NULL DEFAULT false,
    -- has_division_scope: approver/pembaca dibatasi divisi user (Head Divisi).
    has_division_scope boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid        NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    updated_by   uuid        NOT NULL,
    version      integer     NOT NULL DEFAULT 1,
    deleted_at   timestamptz,
    deleted_by   uuid,
    CONSTRAINT pk_roles PRIMARY KEY (id),
    CONSTRAINT uq_roles_code UNIQUE (code)
);

CREATE TABLE permissions (
    id              uuid        NOT NULL DEFAULT uuidv7(),
    code            text        NOT NULL,
    module          text        NOT NULL,
    description     text        NOT NULL,
    requires_reauth boolean     NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_permissions PRIMARY KEY (id),
    CONSTRAINT uq_permissions_code UNIQUE (code),
    CONSTRAINT ck_permissions_code_format CHECK (code ~ '^[a-z_]+\.[a-z_]+\.[a-z_]+$')
);

CREATE TABLE role_permissions (
    role_id       uuid        NOT NULL,
    permission_id uuid        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by    uuid        NOT NULL,
    CONSTRAINT pk_role_permissions PRIMARY KEY (role_id, permission_id),
    CONSTRAINT fk_role_permissions_role_id FOREIGN KEY (role_id) REFERENCES roles (id) ON DELETE CASCADE,
    CONSTRAINT fk_role_permissions_permission_id FOREIGN KEY (permission_id) REFERENCES permissions (id) ON DELETE CASCADE
);
CREATE INDEX idx_role_permissions_permission_id ON role_permissions (permission_id);

CREATE TABLE users (
    id               uuid        NOT NULL DEFAULT uuidv7(),
    email            text        NOT NULL,
    full_name        text        NOT NULL,
    password_hash    text,
    is_active        boolean     NOT NULL DEFAULT true,
    token_version    integer     NOT NULL DEFAULT 1,
    mfa_secret_enc   text,
    is_mfa_enabled   boolean     NOT NULL DEFAULT false,
    employee_id      uuid,
    division_id      uuid,
    last_login_at    timestamptz,
    password_changed_at timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    deleted_at       timestamptz,
    deleted_by       uuid,
    CONSTRAINT pk_users PRIMARY KEY (id),
    CONSTRAINT ck_users_email_lowercase CHECK (email = lower(email)),
    CONSTRAINT ck_users_mfa_secret CHECK (NOT is_mfa_enabled OR mfa_secret_enc IS NOT NULL)
);
CREATE UNIQUE INDEX uq_users_email ON users (email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_division_id ON users (division_id);
CREATE INDEX idx_users_employee_id ON users (employee_id);

CREATE TABLE user_roles (
    user_id           uuid        NOT NULL,
    role_id           uuid        NOT NULL,
    scope_division_id uuid,
    scope_partner_id  uuid,
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid        NOT NULL,
    CONSTRAINT pk_user_roles PRIMARY KEY (user_id, role_id),
    CONSTRAINT fk_user_roles_user_id FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_user_roles_role_id FOREIGN KEY (role_id) REFERENCES roles (id)
);
CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);

CREATE TABLE sessions (
    id                 uuid        NOT NULL DEFAULT uuidv7(),
    user_id            uuid        NOT NULL,
    audience           text        NOT NULL,
    family_id          uuid        NOT NULL,
    refresh_token_hash bytea       NOT NULL,
    expires_at         timestamptz NOT NULL,
    idle_expires_at    timestamptz NOT NULL,
    revoked_at         timestamptz,
    revoke_reason      text,
    replaced_by        uuid,
    user_agent         text        NOT NULL DEFAULT '',
    ip                 text        NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_sessions PRIMARY KEY (id),
    CONSTRAINT uq_sessions_refresh_token_hash UNIQUE (refresh_token_hash),
    CONSTRAINT fk_sessions_user_id FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_sessions_audience CHECK (audience IN ('backoffice', 'partner', 'reseller', 'customer'))
);
CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_family_id ON sessions (family_id);

CREATE TABLE login_attempts (
    id         uuid        NOT NULL DEFAULT uuidv7(),
    email_hash bytea       NOT NULL,
    user_id    uuid,
    ip         text        NOT NULL DEFAULT '',
    succeeded  boolean     NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_login_attempts PRIMARY KEY (id)
);
CREATE INDEX idx_login_attempts_email_hash_created_at ON login_attempts (email_hash, created_at DESC);

CREATE TABLE password_tokens (
    id         uuid        NOT NULL DEFAULT uuidv7(),
    user_id    uuid        NOT NULL,
    purpose    text        NOT NULL,
    token_hash bytea       NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid        NOT NULL,
    CONSTRAINT pk_password_tokens PRIMARY KEY (id),
    CONSTRAINT uq_password_tokens_token_hash UNIQUE (token_hash),
    CONSTRAINT fk_password_tokens_user_id FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_password_tokens_purpose CHECK (purpose IN ('invite', 'reset'))
);
CREATE INDEX idx_password_tokens_user_id ON password_tokens (user_id);

-- Role awal Fase 1 (Desain Role & Approval bagian 1). Permission dan hak awalnya disemai dari kode saat proses api start.
INSERT INTO roles (code, name, description, requires_mfa, has_division_scope, created_by, updated_by) VALUES
    ('system_admin',       'Admin Sistem',              'Pengguna, role, pengaturan, nomor & template dokumen', true,  false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('purchasing',         'Purchasing / Logistik',     'Pengadaan, Delivery Order, transfer; lihat persediaan', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('warehouse_staff',    'Staf Gudang',               'Inbound/QC, transfer, picking/packing Outbound, stock count', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('logistics_leader',   'Logistik Leader',           'Dokumen PO & DO, cetak label, verifikasi retur MT, meneruskan permintaan barang', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('fulfillment_leader', 'Fulfillment Leader',        'Petty Stock, QC Outbound, pengiriman & pickup, penugasan Merchandiser', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('merchandiser',       'Merchandiser',              'Kurir internal, pickup retur MT & sisa event, QC Outbound, stock count lokasi konsinyasi', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('head_logistics',     'Head Logistik',             'Approval penyesuaian stok & disposisi retur, laporan gudang', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('finance',            'Finance & Accounting',      'Invoice, pembayaran, rekonsiliasi, pajak, aset, jurnal', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('head_finance',       'Head Finance & Accounting', 'Approval keuangan, tutup buku, buka kembali periode', true,  false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('sales_mt',           'Sales MT',                  'MT beli putus, B2B, konsinyasi & rekonsiliasi', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('sales_ecommerce',    'Sales E-commerce',          'Penjualan marketplace (manual/import)', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('customer_service',   'Customer Service',          'Order WA/IG, retur customer', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('head_sales',         'Head Sales',                'Approval harga & diskon', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('marketing',          'Marketing',                 'Pemakaian internal marketing, pengajuan biaya, kampanye', false, false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('hrga',               'HRGA / Payroll',            'Data karyawan, sample karyawan', true,  false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('division_head',      'Head Divisi',               'Approver level 1 PR, pengajuan biaya, pemakaian internal divisinya', false, true, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('director',           'Direksi',                   'Approval tingkat atas, semua laporan, read-only transaksi', true,  false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('president_director', 'Direktur Utama',            'Approval PO > Rp50 jt', true,  false, '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000');

-- +goose Down
DROP TABLE password_tokens;
DROP TABLE login_attempts;
DROP TABLE sessions;
DROP TABLE user_roles;
DROP TABLE users;
DROP TABLE role_permissions;
DROP TABLE permissions;
DROP TABLE roles;
