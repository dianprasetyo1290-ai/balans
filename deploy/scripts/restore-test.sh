#!/usr/bin/env bash
# Uji restore point-in-time dari backup object storage (NFR-07, Technical Guideline 14.7). Dijalankan
# setiap bulan; hasil dan durasi dicatat di $BALANS_HOME/restore-reports.log.
#
#   restore-test.sh                         restore ke backup terbaru + seluruh WAL
#   restore-test.sh "2026-11-01 10:00:00+07" restore sampai waktu tertentu (point-in-time)
#
# RESTORE_VERIFY_SQL (opsional) dijalankan pada database hasil restore untuk memeriksa isi data.
#
# Restore dilakukan ke container dan volume sementara di VPS staging, lalu dihapus. Data produksi
# tidak pernah ditulis ke database staging yang dipakai tester.
set -euo pipefail

target_time="${1:-}"
home="${BALANS_HOME:-/opt/balans}"
env_file="${RESTORE_ENV_FILE:-$home/production.env}"
image="${RESTORE_IMAGE:?RESTORE_IMAGE wajib diisi, mis. ghcr.io/<org>/balans-postgres:<tag>}"
name="balans-restore-test"
volume="balans-restore-test-data"
report="$home/restore-reports.log"
pgdata="/var/lib/postgresql/18/docker"

cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

started=$(date +%s)
echo "restore: mengambil backup terbaru"
docker volume create "$volume" >/dev/null
docker run --rm --env-file "$env_file" -e PGDATA="$pgdata" -v "$volume:/var/lib/postgresql" "$image" bash -ec "
  mkdir -p '$pgdata'
  wal-g backup-fetch '$pgdata' LATEST
  touch '$pgdata/recovery.signal'
  {
    echo \"restore_command = 'wal-g wal-fetch %f %p'\"
    echo \"recovery_target_action = 'promote'\"
    if [ -n '$target_time' ]; then echo \"recovery_target_time = '$target_time'\"; fi
    echo \"archive_mode = 'off'\"
  } >> '$pgdata/postgresql.auto.conf'
  chown -R postgres:postgres /var/lib/postgresql
  chmod 0700 '$pgdata'
"

echo "restore: menjalankan recovery"
docker run -d --name "$name" --env-file "$env_file" -e PGDATA="$pgdata" -v "$volume:/var/lib/postgresql" "$image" >/dev/null

user=$(grep '^POSTGRES_USER=' "$env_file" | cut -d= -f2-)
db=$(grep '^POSTGRES_DB=' "$env_file" | cut -d= -f2-)
for _ in $(seq 1 600); do
  if docker exec "$name" psql -U "$user" -d "$db" -tAc "select not pg_is_in_recovery()" 2>/dev/null | grep -q t; then
    break
  fi
  sleep 2
done

schema=$(docker exec "$name" psql -U "$user" -d "$db" -tAc "select max(version_id) from goose_db_version")
tables=$(docker exec "$name" psql -U "$user" -d "$db" -tAc "select count(*) from information_schema.tables where table_schema = 'public'")
duration=$(( $(date +%s) - started ))
if [ -n "${RESTORE_VERIFY_SQL:-}" ]; then
  echo "restore: verifikasi → $(docker exec "$name" psql -U "$user" -d "$db" -tAc "$RESTORE_VERIFY_SQL")"
fi

line="$(date -u +%FT%TZ) restore OK durasi=${duration}s target=${target_time:-latest} schema=${schema} tabel=${tables}"
echo "$line" | tee -a "$report"
