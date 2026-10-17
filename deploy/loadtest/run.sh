#!/usr/bin/env bash
# Menjalankan uji beban back-office (backoffice.js) dengan image k6 dan merekam pemakaian CPU/memori container
# selama uji (Runbook 8.3). Hasil: $LOADTEST_OUT/<waktu>/{summary.txt,summary.json,docker-stats.csv,k6.log}.
#
#   USERS=a@x,b@x PASSWORD=... bash deploy/loadtest/run.sh                       # default: staging-api di VPS
#   BASE_URL=http://production-api:8080 ... bash deploy/loadtest/run.sh          # (hanya sebelum go-live)
#   DURATION=2m VUS=40 ... bash deploy/loadtest/run.sh                           # uji singkat
#
# Env tambahan: K6_NETWORK (default balans-edge; "host" untuk BASE_URL publik), K6_IMAGE, LOADTEST_OUT
# (default $BALANS_HOME/loadtest-results), STATS_FILTER (prefix nama container yang direkam, default balans-).
# Variabel lain (VUS, DURATION, RAMP, THINK_MIN, THINK_MAX, WRITE, WRITE_RATIO) diteruskan ke backoffice.js.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
home="${BALANS_HOME:-/opt/balans}"
image="${K6_IMAGE:-grafana/k6:1.3.0}"
network="${K6_NETWORK:-balans-edge}"
export BASE_URL="${BASE_URL:-http://staging-api:8080}"
: "${USERS:?USERS wajib diisi (email akun uji dipisah koma)}"
: "${PASSWORD:?PASSWORD wajib diisi}"
out="${LOADTEST_OUT:-$home/loadtest-results}/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out"
chmod 0777 "$out" # k6 berjalan sebagai user non-root di container

# Rekam docker stats setiap 5 detik selama k6 berjalan.
stats_filter="${STATS_FILTER:-balans-}"
echo "waktu,container,cpu_persen,memori,memori_persen" > "$out/docker-stats.csv"
(
  while true; do
    ts=$(date -u +%FT%TZ)
    docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}}' 2>/dev/null \
      | grep "^$stats_filter" | sed "s|^|$ts,|" >> "$out/docker-stats.csv" || true
    sleep 5
  done
) &
stats_pid=$!
trap 'kill "$stats_pid" 2>/dev/null || true' EXIT

pass_env=()
for v in BASE_URL USERS PASSWORD VUS DURATION RAMP THINK_MIN THINK_MAX WRITE WRITE_RATIO; do
  [ -n "${!v:-}" ] && pass_env+=(-e "$v=${!v}")
done

echo "loadtest: $BASE_URL, hasil di $out"
status=0
docker run --rm --network "$network" "${pass_env[@]}" -e SUMMARY_DIR=/out \
  -v "$here:/scripts:ro" -v "$out:/out" "$image" run --quiet /scripts/backoffice.js 2>&1 | tee "$out/k6.log" \
  || status=$?

kill "$stats_pid" 2>/dev/null || true
# Puncak memori & CPU per container selama uji.
echo "Puncak pemakaian container selama uji:" | tee -a "$out/summary.txt"
awk -F, 'NR > 1 {
    split($4, mem, " / "); cpu = $3 + 0; pct = $5 + 0
    if (cpu > maxcpu[$2]) maxcpu[$2] = cpu
    if (pct > maxpct[$2]) { maxpct[$2] = pct; maxmem[$2] = mem[1] " / " mem[2] }
  } END { for (c in maxcpu) printf "  %-34s cpu maks %6.1f%%  memori maks %s (%.1f%%)\n", c, maxcpu[c], maxmem[c], maxpct[c] }' \
  "$out/docker-stats.csv" | sort | tee -a "$out/summary.txt"
exit "$status"
