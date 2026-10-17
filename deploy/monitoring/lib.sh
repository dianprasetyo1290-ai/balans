# shellcheck shell=bash
# Fungsi bersama skrip monitoring dan uji restore (Runbook 7). Di-source, bukan dijalankan langsung.
#
# monitoring.env (chmod 600) berisi URL webhook & heartbeat; lihat deploy/monitoring/monitoring.env.example.

balans_home="${BALANS_HOME:-/opt/balans}"
# shellcheck source=/dev/null
[ -f "$balans_home/monitoring.env" ] && . "$balans_home/monitoring.env"

# send_alert <teks>: pesan ke webhook chat tim IT (Slack memakai field text, Discord field content).
# Tanpa ALERT_WEBHOOK_URL pesan hanya dicetak; monitor heartbeat tetap menjadi jalur alert utama.
send_alert() {
  local url="${ALERT_WEBHOOK_URL:-}" text esc
  text="[Balans $(hostname)] $1"
  if [ -z "$url" ]; then
    echo "alert (ALERT_WEBHOOK_URL kosong): $text"
    return 0
  fi
  esc=${text//\\/\\\\}
  esc=${esc//\"/\\\"}
  esc=${esc//$'\n'/ }
  curl -fsS -m 10 -H 'Content-Type: application/json' \
    -d "{\"text\":\"$esc\",\"content\":\"$esc\"}" "$url" >/dev/null \
    || echo "gagal mengirim alert ke webhook"
}

# ping_heartbeat <url> [akhiran] [isi]: akhiran "" (sukses), "/start", atau "/fail" mengikuti konvensi
# healthchecks.io / Better Stack. Isi (opsional) dikirim sebagai body POST dan tampil di log monitor.
ping_heartbeat() {
  local url="$1" suffix="${2:-}" body="${3:-}"
  [ -z "$url" ] && return 0
  if [ -n "$body" ]; then
    curl -fsS -m 10 --retry 3 --data-raw "$body" "$url$suffix" >/dev/null || echo "gagal mengirim heartbeat $url$suffix"
  else
    curl -fsS -m 10 --retry 3 "$url$suffix" >/dev/null || echo "gagal mengirim heartbeat $url$suffix"
  fi
}
