-- name: InsertBankStatement :one
INSERT INTO bank_statements (
    bank_account_id, file_id, file_name, period_from, period_to, line_count, total_in, total_out, created_by, updated_by)
VALUES (
    sqlc.arg('bank_account_id'), sqlc.arg('file_id'), sqlc.arg('file_name'), sqlc.arg('period_from'),
    sqlc.arg('period_to'), sqlc.arg('line_count'), sqlc.arg('total_in'), sqlc.arg('total_out'), sqlc.arg('actor_id'),
    sqlc.arg('actor_id'))
RETURNING id, created_at;

-- name: InsertBankStatementLine :exec
INSERT INTO bank_statement_lines (statement_id, line_no, transaction_date, description, reference, amount, balance)
VALUES (sqlc.arg('statement_id'), sqlc.arg('line_no'), sqlc.arg('transaction_date'), sqlc.arg('description'),
        sqlc.arg('reference'), sqlc.arg('amount'), sqlc.narg('balance'));

-- name: ListBankStatements :many
SELECT s.*, b.code AS bank_code, b.name AS bank_name,
       (SELECT count(*) FROM bank_statement_lines l WHERE l.statement_id = s.id AND l.status = 'matched')::bigint AS matched_count,
       (SELECT count(*) FROM bank_statement_lines l WHERE l.statement_id = s.id AND l.status = 'suggested')::bigint AS suggested_count,
       count(*) OVER () AS total_count
FROM bank_statements s
JOIN bank_accounts b ON b.id = s.bank_account_id
WHERE (sqlc.narg('bank_account_id')::uuid IS NULL OR s.bank_account_id = sqlc.narg('bank_account_id'))
ORDER BY s.period_to DESC, s.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: GetBankStatement :one
SELECT s.*, b.code AS bank_code, b.name AS bank_name,
       (SELECT count(*) FROM bank_statement_lines l WHERE l.statement_id = s.id AND l.status = 'matched')::bigint AS matched_count,
       (SELECT count(*) FROM bank_statement_lines l WHERE l.statement_id = s.id AND l.status = 'suggested')::bigint AS suggested_count,
       0::bigint AS total_count
FROM bank_statements s
JOIN bank_accounts b ON b.id = s.bank_account_id
WHERE s.id = $1;

-- name: StatementLines :many
SELECT l.*, s.bank_account_id, coalesce(j.number, '')::text AS journal_number
FROM bank_statement_lines l
JOIN bank_statements s ON s.id = l.statement_id
LEFT JOIN journal_entries j ON j.id = l.journal_entry_id
WHERE l.statement_id = $1
ORDER BY l.line_no;

-- name: LockStatementLines :many
SELECT l.id FROM bank_statement_lines l WHERE l.statement_id = $1 ORDER BY l.line_no FOR UPDATE;

-- name: GetStatementLine :one
SELECT l.*, s.bank_account_id, coalesce(j.number, '')::text AS journal_number
FROM bank_statement_lines l
JOIN bank_statements s ON s.id = l.statement_id
LEFT JOIN journal_entries j ON j.id = l.journal_entry_id
WHERE l.id = $1;

-- name: LockStatementLine :one
SELECT version FROM bank_statement_lines WHERE id = $1 FOR UPDATE;

-- name: SaveStatementLine :execrows
UPDATE bank_statement_lines
SET status = sqlc.arg('status'), match_kind = sqlc.arg('match_kind'), match_rule = sqlc.arg('match_rule'),
    journal_line_id = sqlc.narg('journal_line_id'), journal_entry_id = sqlc.narg('journal_entry_id'),
    candidate_source = sqlc.arg('candidate_source'), candidate_document_id = sqlc.narg('candidate_document_id'),
    candidate_label = sqlc.arg('candidate_label'), payment_id = sqlc.narg('payment_id'),
    matched_at = sqlc.narg('matched_at'), matched_by = sqlc.narg('matched_by'), updated_at = now(), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');
