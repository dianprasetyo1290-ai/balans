#!/usr/bin/env bash
# Full backup sekali jalan + penerapan retensi (Technical Guideline 14.7):
#   - 14 backup harian terakhir dipertahankan;
#   - backup pertama setiap bulan ditandai permanen dan dipertahankan 12 bulan.
# Dipanggil backup-loop setiap hari dan oleh deploy production sebelum migrasi.
set -euo pipefail

: "${PGDATA:?PGDATA wajib diisi}"
: "${WALG_S3_PREFIX:?WALG_S3_PREFIX wajib diisi}"
export PGHOST="${PGHOST:-postgres}" PGUSER="${PGUSER:-${POSTGRES_USER}}" PGPASSWORD="${PGPASSWORD:-${POSTGRES_PASSWORD}}" PGDATABASE="${PGDATABASE:-${POSTGRES_DB}}"

started=$(date +%s)
echo "backup: mulai $(date -u +%FT%TZ)"
wal-g backup-push "$PGDATA"

backup_names() {
  sed -n 's/.*"backup_name":"\([^"]*\)".*/\1/p'
}

apply_retention() {
  local latest cutoff row name time
  latest=$(wal-g backup-list --json | tr '}' '\n' | backup_names | tail -n 1)
  if [ "$(TZ=Asia/Jakarta date +%d)" = "01" ] && [ -n "$latest" ]; then
    wal-g backup-mark "$latest"
    echo "backup: $latest ditandai permanen (bulanan)"
  fi

  wal-g delete retain FULL 14 --confirm || return 1

  # Backup bulanan permanen yang lebih tua dari 12 bulan dilepas tandanya agar ikut terhapus.
  cutoff=$(date -u -d "12 months ago" +%Y-%m-%dT%H:%M:%SZ)
  while read -r row; do
    name=$(echo "$row" | backup_names)
    time=$(echo "$row" | sed -n 's/.*"time":"\([^"]*\)".*/\1/p')
    if [ -n "$name" ] && [[ "$time" < "$cutoff" ]]; then
      wal-g backup-mark -i "$name"
      echo "backup: tanda permanen $name dilepas (lebih dari 12 bulan)"
    fi
  done < <(wal-g backup-list --detail --json | tr '}' '\n' | grep '"is_permanent":true' || true)

  wal-g delete garbage --confirm
}

# Backup di atas sudah sah. Retensi dicoba ulang; jika tetap gagal hanya dicatat sebagai peringatan
# agar backup yang berhasil tidak dilaporkan gagal. Retensi dicoba lagi pada backup berikutnya.
retention_ok=false
for attempt in 1 2 3; do
  if apply_retention; then
    retention_ok=true
    break
  fi
  echo "backup: retensi gagal (percobaan $attempt)"
  sleep $(( attempt * 10 ))
done
$retention_ok || echo "backup: PERINGATAN retensi tidak berhasil diterapkan"

echo "backup: selesai dalam $(( $(date +%s) - started )) detik"
