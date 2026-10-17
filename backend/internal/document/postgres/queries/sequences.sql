-- name: NextNumber :one
-- Baris tahun baru dibuat otomatis; baris yang ada dinaikkan (UPDATE … RETURNING) dan terkunci sampai
-- transaksi pemanggil selesai, sehingga rollback mengembalikan nomor.
INSERT INTO document_sequences (document_type, year, last_number)
VALUES (sqlc.arg('document_type'), sqlc.arg('year'), 1)
ON CONFLICT (document_type, year)
DO UPDATE SET last_number = document_sequences.last_number + 1, updated_at = now()
RETURNING last_number;

-- name: ListSequences :many
SELECT * FROM document_sequences
WHERE year = sqlc.arg('year')
ORDER BY document_type;
