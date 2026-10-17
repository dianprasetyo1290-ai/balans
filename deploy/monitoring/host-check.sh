#!/usr/bin/env bash
# Pemeriksaan host dari cron setiap 5 menit (Technical Guideline 13, NFR-10, Runbook 7.3):
#   */5 * * * * /opt/balans/deploy/monitoring/host-check.sh >> /opt/balans/host-check.log 2>&1
# Mengirim alert jika disk > 80%, memori tersedia < 10%, container tidak sehat, container restart berulang,
# atau container pernah di-OOM-kill karena melewati batas memorinya, lalu mengirim heartbeat agar monitor
# eksternal tahu VPS masih hidup.
set -uo pipefail

# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$(dirname "$0")/lib.sh"
heartbeat_url="${HOST_HEARTBEAT_URL:-}" # monitor heartbeat eksternal
disk_limit="${DISK_ALERT_PERCENT:-80}"
mem_limit="${MEM_ALERT_PERCENT:-10}"

alerts=()

disk=$(df -P / | awk 'NR==2 { gsub("%", "", $5); print $5 }')
[ "$disk" -gt "$disk_limit" ] && alerts+=("Disk / terpakai ${disk}% (batas ${disk_limit}%)")

mem_avail=$(awk '/MemAvailable/ { a=$2 } /MemTotal/ { t=$2 } END { printf "%d", a*100/t }' /proc/meminfo)
[ "$mem_avail" -lt "$mem_limit" ] && alerts+=("Memori tersedia tinggal ${mem_avail}%")

unhealthy=$(docker ps --filter health=unhealthy --format '{{.Names}}' | paste -sd, -)
[ -n "$unhealthy" ] && alerts+=("Container tidak sehat: $unhealthy")

restarting=$(docker ps --filter status=restarting --format '{{.Names}}' | paste -sd, -)
[ -n "$restarting" ] && alerts+=("Container restart berulang: $restarting")

# Container yang dibunuh OOM killer karena melewati mem_limit (Docker me-restart-nya, sehingga statusnya
# tampak sehat lagi): dibaca dari event Docker sejak pemeriksaan sebelumnya (5 menit).
oom=$(docker events --since 5m --until 0s --filter event=oom --format '{{.Actor.Attributes.name}}' 2>/dev/null \
  | sort -u | paste -sd, -)
[ -n "$oom" ] && alerts+=("Container kehabisan memori (OOM kill): $oom")

if [ "${#alerts[@]}" -gt 0 ]; then
  send_alert "$(printf '%s; ' "${alerts[@]}")"
fi

ping_heartbeat "$heartbeat_url"

printf 'host-check: disk=%s%% mem_avail=%s%% alerts=%s\n' "$disk" "$mem_avail" "${#alerts[@]}"
