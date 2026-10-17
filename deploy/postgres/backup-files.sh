#!/usr/bin/env bash
# Arsip harian folder file modul filestore (PDF PKS, foto QC, berita acara) ke object storage backup yang sama
# dengan WAL-G, selama adapter object storage aplikasi belum dibangun (Runbook 5.1, Technical Guideline 14.7).
#
#   <WALG_S3_PREFIX>/filestore/daily/<YYYYMMDDTHHMMSSZ>.tar.<kompresi>    14 arsip terakhir
#   <WALG_S3_PREFIX>/filestore/monthly/<YYYYMMDDTHHMMSSZ>.tar.<kompresi>  arsip tanggal 1 (WIB), 12 bulan
#
# Upload memakai `wal-g st put` sehingga kompresi (WALG_COMPRESSION_METHOD), enkripsi (bila dikonfigurasi), dan
# kredensial sama dengan backup database. Dipanggil backup-loop setelah backup-now.
set -euo pipefail

dir="${FILESTORE_BACKUP_DIR:-}"
prefix="${FILESTORE_BACKUP_PREFIX:-filestore}"
keep_daily="${FILESTORE_BACKUP_KEEP_DAILY:-14}"

if [ -z "$dir" ]; then
  echo "backup-files: FILESTORE_BACKUP_DIR kosong, dilewati"
  exit 0
fi
[ -d "$dir" ] || { echo "backup-files: folder $dir tidak ada" >&2; exit 1; }
: "${WALG_S3_PREFIX:-${WALG_FILE_PREFIX:?WALG_S3_PREFIX wajib diisi}}"

started=$(date +%s)
name="$(date -u +%Y%m%dT%H%M%SZ).tar"
files=$(find "$dir" -type f | wc -l)
bytes=$(du -sb "$dir" | cut -f1)
echo "backup-files: mulai $name ($files file, $bytes byte)"

# Arsip ditulis langsung ke storage lewat stdin (tanpa file sementara di disk VPS).
tar -C "$dir" -cf - . | wal-g st put --read-stdin "$prefix/daily/$name"
if [ "$(TZ=Asia/Jakarta date +%d)" = "01" ]; then
  tar -C "$dir" -cf - . | wal-g st put --read-stdin "$prefix/monthly/$name"
  echo "backup-files: $name disimpan juga sebagai arsip bulanan"
fi

objects() {
  wal-g st ls "$prefix/$1/" 2>/dev/null | awk '$1 == "obj" { print $NF }' | sort
}

# Retensi: kegagalan hanya peringatan agar arsip yang sudah terunggah tidak dilaporkan gagal.
retention_ok=true
mapfile -t daily < <(objects daily)
if [ "${#daily[@]}" -gt "$keep_daily" ]; then
  for obj in "${daily[@]:0:${#daily[@]}-keep_daily}"; do
    wal-g st rm "$prefix/daily/$obj" || retention_ok=false
  done
fi
cutoff=$(date -u -d "12 months ago" +%Y%m%dT%H%M%SZ)
while read -r obj; do
  [ -n "$obj" ] && [[ "$obj" < "$cutoff" ]] && { wal-g st rm "$prefix/monthly/$obj" || retention_ok=false; }
done < <(objects monthly)
$retention_ok || echo "backup-files: PERINGATAN retensi arsip file tidak seluruhnya diterapkan"

echo "backup-files: selesai $name dalam $(( $(date +%s) - started )) detik"
