#!/usr/bin/env bash
# Pemeriksaan host dari cron setiap 5 menit (Technical Guideline 13, NFR-10):
#   */5 * * * * /opt/balans/deploy/monitoring/host-check.sh
# Mengirim alert jika disk > 80%, memori tersedia < 10%, atau container tidak sehat, lalu mengirim
# heartbeat agar monitor eksternal tahu VPS masih hidup.
set -uo pipefail

home="${BALANS_HOME:-/opt/balans}"
# shellcheck source=/dev/null
[ -f "$home/monitoring.env" ] && . "$home/monitoring.env"
alert_url="${ALERT_WEBHOOK_URL:-}"      # webhook chat tim IT (Slack/Discord/Telegram bot)
heartbeat_url="${HOST_HEARTBEAT_URL:-}" # monitor heartbeat eksternal
disk_limit="${DISK_ALERT_PERCENT:-80}"

alerts=()

disk=$(df -P / | awk 'NR==2 { gsub("%", "", $5); print $5 }')
[ "$disk" -gt "$disk_limit" ] && alerts+=("Disk / terpakai ${disk}% (batas ${disk_limit}%)")

mem_avail=$(awk '/MemAvailable/ { a=$2 } /MemTotal/ { t=$2 } END { printf "%d", a*100/t }' /proc/meminfo)
[ "$mem_avail" -lt 10 ] && alerts+=("Memori tersedia tinggal ${mem_avail}%")

unhealthy=$(docker ps --filter health=unhealthy --format '{{.Names}}' | paste -sd, -)
[ -n "$unhealthy" ] && alerts+=("Container tidak sehat: $unhealthy")

restarting=$(docker ps --filter status=restarting --format '{{.Names}}' | paste -sd, -)
[ -n "$restarting" ] && alerts+=("Container restart berulang: $restarting")

if [ "${#alerts[@]}" -gt 0 ] && [ -n "$alert_url" ]; then
  text="[Balans VPS $(hostname)] $(printf '%s; ' "${alerts[@]}")"
  curl -fsS -m 10 -H 'Content-Type: application/json' \
    -d "{\"text\":\"${text//\"/\\\"}\",\"content\":\"${text//\"/\\\"}\"}" "$alert_url" >/dev/null \
    || echo "host-check: gagal mengirim alert"
fi

if [ -n "$heartbeat_url" ]; then
  curl -fsS -m 10 "$heartbeat_url" >/dev/null || echo "host-check: gagal mengirim heartbeat"
fi

printf 'host-check: disk=%s%% mem_avail=%s%% alerts=%s\n' "$disk" "$mem_avail" "${#alerts[@]}"
