#!/usr/bin/env bash
# Pemeriksaan liveness worker (river) dari cron setiap 5 menit (Technical Guideline 13, Runbook 7.3):
#   */5 * * * * /opt/balans/deploy/monitoring/worker-check.sh production >> /opt/balans/worker-check.log 2>&1
#
# Container worker tidak membuka port sehingga healthcheck Docker tidak bisa memeriksanya. Skrip ini membaca
# tabel river di database lingkungan tersebut:
#   1. Leader river aktif (river_leader.expires_at > now) — worker hidup dan terhubung ke database.
#   2. Tidak ada job `available` yang terlambat > WORKER_BACKLOG_MINUTES (default 10) — worker memproses antrean.
#   3. Job dead-letter (`discarded`) baru dalam 5 menit terakhir — dikirim sebagai alert (TG 13).
# Butir 1 dan 2 menentukan heartbeat WORKER_HEARTBEAT_URL (sukses atau /fail); monitor heartbeat mengirim
# alert jika ping /fail datang atau tidak ada ping selama 15 menit (worker, Postgres, atau VPS mati).
set -uo pipefail

env_name="${1:-production}"
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$(dirname "$0")/lib.sh"

env_file="$balans_home/$env_name.env"
container="${WORKER_CHECK_DB_CONTAINER:-balans-$env_name-postgres-1}"
backlog_minutes="${WORKER_BACKLOG_MINUTES:-10}"
case "$env_name" in
  production) heartbeat_url="${WORKER_HEARTBEAT_URL:-}" ;;
  *) heartbeat_url="${STAGING_WORKER_HEARTBEAT_URL:-}" ;;
esac

db_user=$(grep '^POSTGRES_USER=' "$env_file" 2>/dev/null | cut -d= -f2-)
db_name=$(grep '^POSTGRES_DB=' "$env_file" 2>/dev/null | cut -d= -f2-)

psql_q() {
  docker exec "$container" psql -X -q -t -A -U "${db_user:-balans}" -d "${db_name:-balans}" -v ON_ERROR_STOP=1 -c "$1"
}

problems=()
if ! leader=$(psql_q "select count(*) from river_leader where expires_at > now()" 2>&1); then
  problems+=("database $container tidak bisa dibaca: ${leader//$'\n'/ }")
  leader=0
  backlog=0
  discarded=""
else
  [ "$leader" -ge 1 ] || problems+=("tidak ada leader river aktif (worker mati atau tidak terhubung ke database)")
  backlog=$(psql_q "select count(*) from river_job where state = 'available'
    and scheduled_at < now() - interval '$backlog_minutes minutes'")
  [ "${backlog:-0}" -eq 0 ] || problems+=("$backlog job menunggu lebih dari $backlog_minutes menit (worker tidak memproses antrean)")
  discarded=$(psql_q "select kind || ' #' || id from river_job where state = 'discarded'
    and finalized_at > now() - interval '5 minutes' order by id limit 20" | paste -sd, -)
fi

summary="worker-check $env_name: leader=$leader backlog=$backlog dead_letter=${discarded:-0}"
if [ "${#problems[@]}" -gt 0 ]; then
  msg="Worker $env_name bermasalah: $(printf '%s; ' "${problems[@]}")"
  send_alert "$msg"
  ping_heartbeat "$heartbeat_url" /fail "$msg"
else
  ping_heartbeat "$heartbeat_url" "" "$summary"
fi
if [ -n "$discarded" ]; then
  send_alert "Job $env_name masuk dead-letter (river_job state discarded, Runbook 7.5): $discarded"
fi

echo "$(date -u +%FT%TZ) $summary problems=${#problems[@]}"
