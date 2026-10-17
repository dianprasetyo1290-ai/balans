-- name: GetOpeningSet :one
SELECT sqlc.embed(s), coalesce(e.number, '')::text AS journal_number
FROM opening_balance_sets s
LEFT JOIN journal_entries e ON e.id = s.journal_entry_id
LIMIT 1;

-- name: LockOpeningSet :one
SELECT version FROM opening_balance_sets WHERE id = $1 FOR UPDATE;

-- name: InsertOpeningSet :one
INSERT INTO opening_balance_sets (cutover_date, created_by, updated_by)
VALUES (sqlc.arg('cutover_date'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
RETURNING id;

-- name: UpdateOpeningSet :execrows
UPDATE opening_balance_sets
SET cutover_date = sqlc.arg('cutover_date'), status = sqlc.arg('status'), journal_entry_id = sqlc.narg('journal_entry_id'),
    posted_at = sqlc.narg('posted_at'), posted_by = sqlc.narg('posted_by'), updated_at = now(),
    updated_by = sqlc.arg('actor_id'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: ListOpeningLines :many
SELECT sqlc.embed(l), a.code AS account_code, a.name AS account_name
FROM opening_balance_lines l
JOIN accounts a ON a.id = l.account_id
WHERE l.opening_balance_set_id = $1
ORDER BY a.code;

-- name: GetOpeningLine :one
SELECT sqlc.embed(l), a.code AS account_code, a.name AS account_name
FROM opening_balance_lines l
JOIN accounts a ON a.id = l.account_id
WHERE l.id = $1;

-- name: UpsertOpeningLine :one
INSERT INTO opening_balance_lines (opening_balance_set_id, account_id, debit_amount, credit_amount, note, created_by, updated_by)
VALUES (sqlc.arg('opening_balance_set_id'), sqlc.arg('account_id'), sqlc.arg('debit_amount'), sqlc.arg('credit_amount'),
        sqlc.arg('note'), sqlc.arg('actor_id'), sqlc.arg('actor_id'))
ON CONFLICT (opening_balance_set_id, account_id) DO UPDATE
SET debit_amount = EXCLUDED.debit_amount, credit_amount = EXCLUDED.credit_amount, note = EXCLUDED.note,
    updated_at = now(), updated_by = EXCLUDED.updated_by, version = opening_balance_lines.version + 1
RETURNING id;

-- name: DeleteOpeningLine :execrows
DELETE FROM opening_balance_lines WHERE id = $1 AND opening_balance_set_id = $2;

-- name: OpeningLineTotalsByAccount :many
-- Saldo Neraca Saldo Awal per kode akun (debit − kredit) untuk rekonsiliasi sebelum diposting.
SELECT a.code, (l.debit_amount - l.credit_amount)::numeric AS net_amount
FROM opening_balance_lines l
JOIN accounts a ON a.id = l.account_id
WHERE l.opening_balance_set_id = $1;

-- name: InsertOpeningItem :one
INSERT INTO opening_items (
    opening_balance_set_id, item_type, party_type, party_id, party_code, document_number, document_date, due_date,
    amount, open_amount, account_id, channel_id, note, created_by, updated_by)
VALUES (
    sqlc.arg('opening_balance_set_id'), sqlc.arg('item_type'), sqlc.arg('party_type'), sqlc.arg('party_id'),
    sqlc.arg('party_code'), sqlc.arg('document_number'), sqlc.arg('document_date'), sqlc.arg('due_date'),
    sqlc.arg('amount'), sqlc.arg('open_amount'), sqlc.arg('account_id'), sqlc.narg('channel_id'), sqlc.arg('note'),
    sqlc.arg('actor_id'), sqlc.arg('actor_id'))
ON CONFLICT (item_type, party_type, party_id, document_number) DO UPDATE
SET document_date = EXCLUDED.document_date, due_date = EXCLUDED.due_date, amount = EXCLUDED.amount,
    open_amount = EXCLUDED.open_amount, account_id = EXCLUDED.account_id, channel_id = EXCLUDED.channel_id,
    note = EXCLUDED.note, party_code = EXCLUDED.party_code, updated_at = now(), updated_by = EXCLUDED.updated_by,
    version = opening_items.version + 1
WHERE opening_items.settled_amount = 0
RETURNING id;

-- name: ListOpeningItems :many
SELECT sqlc.embed(i), a.code AS account_code
FROM opening_items i
JOIN accounts a ON a.id = i.account_id
WHERE (sqlc.narg('item_type')::text IS NULL OR i.item_type = sqlc.narg('item_type'))
  AND (sqlc.narg('party_type')::text IS NULL OR i.party_type = sqlc.narg('party_type'))
  AND (sqlc.narg('party_id')::uuid IS NULL OR i.party_id = sqlc.narg('party_id'))
  AND (NOT sqlc.arg('outstanding_only')::boolean OR i.open_amount > i.settled_amount)
  AND (sqlc.narg('q')::text IS NULL OR i.document_number ILIKE '%' || sqlc.narg('q') || '%'
       OR i.party_code ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY i.due_date, i.document_number
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountOpeningItems :one
SELECT count(*), coalesce(sum(i.open_amount - i.settled_amount), 0)::numeric AS outstanding_amount
FROM opening_items i
WHERE (sqlc.narg('item_type')::text IS NULL OR i.item_type = sqlc.narg('item_type'))
  AND (sqlc.narg('party_type')::text IS NULL OR i.party_type = sqlc.narg('party_type'))
  AND (sqlc.narg('party_id')::uuid IS NULL OR i.party_id = sqlc.narg('party_id'))
  AND (NOT sqlc.arg('outstanding_only')::boolean OR i.open_amount > i.settled_amount)
  AND (sqlc.narg('q')::text IS NULL OR i.document_number ILIKE '%' || sqlc.narg('q') || '%'
       OR i.party_code ILIKE '%' || sqlc.narg('q') || '%');

-- name: GetOpeningItemForUpdate :one
SELECT sqlc.embed(i), a.code AS account_code
FROM opening_items i
JOIN accounts a ON a.id = i.account_id
WHERE i.id = $1
FOR UPDATE OF i;

-- name: OpeningItemSettlementExists :one
SELECT EXISTS (
    SELECT 1 FROM opening_item_settlements
    WHERE opening_item_id = sqlc.arg('opening_item_id') AND source_type = sqlc.arg('source_type') AND source_id = sqlc.arg('source_id')
)::boolean;

-- name: InsertOpeningItemSettlement :exec
INSERT INTO opening_item_settlements (opening_item_id, source_type, source_id, business_date, amount, created_by)
VALUES (sqlc.arg('opening_item_id'), sqlc.arg('source_type'), sqlc.arg('source_id'), sqlc.arg('business_date'),
        sqlc.arg('amount'), sqlc.arg('actor_id'));

-- name: SetOpeningItemSettled :exec
UPDATE opening_items SET settled_amount = sqlc.arg('settled_amount'), updated_at = now(), updated_by = sqlc.arg('actor_id'),
    version = version + 1
WHERE id = sqlc.arg('id');

-- name: OpeningItemTotals :many
-- Sisa saldo terbuka per tanggal cut-over per jenis & akun (rekonsiliasi; pelunasan setelah go-live tidak dihitung).
SELECT i.item_type, a.code AS account_code, coalesce(sum(i.open_amount), 0)::numeric AS open_amount
FROM opening_items i
JOIN accounts a ON a.id = i.account_id
GROUP BY i.item_type, a.code
ORDER BY i.item_type, a.code;

-- name: GetJournalImportRef :one
SELECT journal_entry_id FROM journal_import_refs WHERE import_key = $1 AND reference = $2;

-- name: InsertJournalImportRef :exec
INSERT INTO journal_import_refs (import_key, reference, journal_entry_id) VALUES ($1, $2, $3);
