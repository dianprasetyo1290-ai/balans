#!/usr/bin/env bash
# Menjalankan backup-now (database) lalu backup-files (folder filestore) setiap hari pukul BACKUP_HOUR_WIB
# (default 01) dan melapor ke monitor heartbeat. Heartbeat sukses hanya dikirim jika keduanya berhasil; jika
# heartbeat tidak diterima, monitor eksternal mengirim alert "backup gagal" (Technical Guideline 13).
set -uo pipefail

hour="${BACKUP_HOUR_WIB:-01}"
heartbeat="${BACKUP_HEARTBEAT_URL:-}"

ping_heartbeat() {
  [ -z "$heartbeat" ] && return 0
  curl -fsS -m 10 --retry 3 "$heartbeat$1" >/dev/null || echo "backup: gagal mengirim heartbeat $1"
}

seconds_until_next_run() {
  local now next
  now=$(date +%s)
  next=$(TZ=Asia/Jakarta date -d "today ${hour}:00" +%s)
  if [ "$next" -le "$now" ]; then
    next=$(TZ=Asia/Jakarta date -d "tomorrow ${hour}:00" +%s)
  fi
  echo $(( next - now ))
}

echo "backup-loop: jadwal harian pukul ${hour}.00 WIB"
while true; do
  wait_for=$(seconds_until_next_run)
  echo "backup-loop: backup berikutnya dalam ${wait_for} detik"
  sleep "$wait_for"

  ping_heartbeat "/start"
  ok=true
  backup-now || { echo "backup-loop: BACKUP DATABASE GAGAL"; ok=false; }
  backup-files || { echo "backup-loop: BACKUP FILE GAGAL"; ok=false; }
  if $ok; then
    ping_heartbeat ""
  else
    ping_heartbeat "/fail"
  fi
done
