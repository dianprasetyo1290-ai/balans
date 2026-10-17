-- name: InsertPartner :one
INSERT INTO partners (
    code, name, partner_type, npwp_enc, address, pic_name, pic_phone, sales_user_id, merchandiser_user_id, notes,
    created_by, updated_by)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('partner_type'), sqlc.arg('npwp_enc'), sqlc.arg('address'),
    sqlc.arg('pic_name'), sqlc.arg('pic_phone'), sqlc.narg('sales_user_id'), sqlc.narg('merchandiser_user_id'),
    sqlc.arg('notes'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetPartner :one
SELECT * FROM partners WHERE id = $1 AND deleted_at IS NULL;

-- name: GetPartnerForUpdate :one
SELECT * FROM partners WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: ListPartnersByIDs :many
-- Termasuk mitra terhapus agar nama tetap tampil di data lama.
SELECT * FROM partners WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListPartnersByMerchandiser :many
SELECT * FROM partners WHERE merchandiser_user_id = $1 AND deleted_at IS NULL ORDER BY code;

-- name: ListPartners :many
SELECT * FROM partners
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR code ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('partner_type')::text IS NULL OR partner_type = sqlc.narg('partner_type')::text)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'code' THEN code END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-code' THEN code END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'name' THEN name END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-name' THEN name END DESC,
  CASE WHEN sqlc.arg('sort')::text = 'createdAt' THEN created_at END ASC,
  CASE WHEN sqlc.arg('sort')::text = '-createdAt' THEN created_at END DESC,
  code ASC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountPartners :one
SELECT count(*) FROM partners
WHERE deleted_at IS NULL
  AND (sqlc.narg('search')::text IS NULL
       OR lower(name) LIKE '%' || lower(sqlc.narg('search')::text) || '%'
       OR code ILIKE '%' || sqlc.narg('search')::text || '%')
  AND (sqlc.narg('partner_type')::text IS NULL OR partner_type = sqlc.narg('partner_type')::text);

-- name: UpdatePartner :execrows
UPDATE partners
SET name = sqlc.arg('name'), partner_type = sqlc.arg('partner_type'), npwp_enc = sqlc.arg('npwp_enc'),
    address = sqlc.arg('address'), pic_name = sqlc.arg('pic_name'), pic_phone = sqlc.arg('pic_phone'),
    sales_user_id = sqlc.narg('sales_user_id'), merchandiser_user_id = sqlc.narg('merchandiser_user_id'),
    notes = sqlc.arg('notes'), updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SoftDeletePartner :execrows
UPDATE partners
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: InsertPartnerAgreement :one
INSERT INTO partner_agreements (
    partner_id, agreement_number, agreement_type, valid_from, valid_to, file_object_id, price_list_id,
    consignment_margin_percent, payment_term_days, loss_borne_by, partner_damage_borne_by, product_defect_borne_by,
    near_expiry_borne_by, notes, created_by, updated_by)
VALUES (
    sqlc.arg('partner_id'), sqlc.arg('agreement_number'), sqlc.arg('agreement_type'), sqlc.arg('valid_from'),
    sqlc.arg('valid_to'), sqlc.narg('file_object_id'), sqlc.narg('price_list_id'),
    sqlc.narg('consignment_margin_percent'), sqlc.arg('payment_term_days'), sqlc.arg('loss_borne_by'),
    sqlc.arg('partner_damage_borne_by'), sqlc.arg('product_defect_borne_by'), sqlc.arg('near_expiry_borne_by'),
    sqlc.arg('notes'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetPartnerAgreementForUpdate :one
SELECT * FROM partner_agreements WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: ListPartnerAgreements :many
SELECT * FROM partner_agreements
WHERE partner_id = $1 AND deleted_at IS NULL
ORDER BY valid_from DESC, agreement_type;

-- name: ListAgreementsActiveOn :many
SELECT * FROM partner_agreements
WHERE partner_id = ANY(sqlc.arg('partner_ids')::uuid[]) AND deleted_at IS NULL
  AND valid_from <= sqlc.arg('on_date') AND valid_to >= sqlc.arg('on_date')
ORDER BY partner_id, agreement_type;

-- name: GetActiveAgreement :one
SELECT * FROM partner_agreements
WHERE partner_id = sqlc.arg('partner_id') AND agreement_type = sqlc.arg('agreement_type') AND deleted_at IS NULL
  AND valid_from <= sqlc.arg('on_date') AND valid_to >= sqlc.arg('on_date');

-- name: UpdatePartnerAgreement :execrows
UPDATE partner_agreements
SET agreement_number = sqlc.arg('agreement_number'), agreement_type = sqlc.arg('agreement_type'),
    valid_from = sqlc.arg('valid_from'), valid_to = sqlc.arg('valid_to'), file_object_id = sqlc.narg('file_object_id'),
    price_list_id = sqlc.narg('price_list_id'), consignment_margin_percent = sqlc.narg('consignment_margin_percent'),
    payment_term_days = sqlc.arg('payment_term_days'), loss_borne_by = sqlc.arg('loss_borne_by'),
    partner_damage_borne_by = sqlc.arg('partner_damage_borne_by'),
    product_defect_borne_by = sqlc.arg('product_defect_borne_by'),
    near_expiry_borne_by = sqlc.arg('near_expiry_borne_by'), notes = sqlc.arg('notes'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SoftDeletePartnerAgreement :execrows
UPDATE partner_agreements
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: InsertPartnerProductCode :one
INSERT INTO partner_product_codes (partner_id, product_id, article_code, barcode, description, created_by, updated_by)
VALUES (sqlc.arg('partner_id'), sqlc.arg('product_id'), sqlc.arg('article_code'), sqlc.arg('barcode'),
        sqlc.arg('description'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING *;

-- name: GetPartnerProductCodeForUpdate :one
SELECT * FROM partner_product_codes WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: GetPartnerProductCodeByProduct :one
SELECT * FROM partner_product_codes WHERE partner_id = $1 AND product_id = $2 AND deleted_at IS NULL;

-- name: ListPartnerProductCodes :many
SELECT c.* FROM partner_product_codes c
JOIN partners p ON p.id = c.partner_id
WHERE c.deleted_at IS NULL
  AND (sqlc.narg('partner_id')::uuid IS NULL OR c.partner_id = sqlc.narg('partner_id')::uuid)
  AND (sqlc.narg('product_id')::uuid IS NULL OR c.product_id = sqlc.narg('product_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR c.article_code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR c.barcode ILIKE '%' || sqlc.narg('search')::text || '%'
       OR c.description ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY p.code, c.article_code
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountPartnerProductCodes :one
SELECT count(*) FROM partner_product_codes c
WHERE c.deleted_at IS NULL
  AND (sqlc.narg('partner_id')::uuid IS NULL OR c.partner_id = sqlc.narg('partner_id')::uuid)
  AND (sqlc.narg('product_id')::uuid IS NULL OR c.product_id = sqlc.narg('product_id')::uuid)
  AND (sqlc.narg('search')::text IS NULL
       OR c.article_code ILIKE '%' || sqlc.narg('search')::text || '%'
       OR c.barcode ILIKE '%' || sqlc.narg('search')::text || '%'
       OR c.description ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: UpdatePartnerProductCode :execrows
UPDATE partner_product_codes
SET article_code = sqlc.arg('article_code'), barcode = sqlc.arg('barcode'), description = sqlc.arg('description'),
    updated_at = now(), updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;

-- name: SoftDeletePartnerProductCode :execrows
UPDATE partner_product_codes
SET deleted_at = now(), deleted_by = sqlc.arg('actor_id')::uuid, updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version') AND deleted_at IS NULL;
