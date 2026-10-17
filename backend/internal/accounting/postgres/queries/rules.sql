-- name: GetPostingRule :one
SELECT * FROM posting_rules WHERE code = $1;

-- name: ListPostingRules :many
SELECT * FROM posting_rules ORDER BY code;

-- name: ListRuleAccounts :many
-- Pemetaan yang berlaku pada tanggal posting, debit lebih dulu. Kosong = semua rule.
SELECT m.posting_rule_code, m.amount_key, m.side, m.account_resolver,
       coalesce(a.code, '')::text AS account_code, coalesce(a.name, '')::text AS account_name
FROM posting_rule_accounts m
LEFT JOIN accounts a ON a.id = m.account_id
WHERE (sqlc.narg('rule_code')::text IS NULL OR m.posting_rule_code = sqlc.narg('rule_code'))
  AND m.valid_from <= sqlc.arg('at')::date AND (m.valid_to IS NULL OR m.valid_to > sqlc.arg('at')::date)
ORDER BY m.posting_rule_code, m.side DESC, m.id;
