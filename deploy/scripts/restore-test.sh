#!/usr/bin/env bash
# Uji restore bulanan dari backup object storage (NFR-07, Technical Guideline 14.7, Runbook 5.2). Dijadwalkan
# systemd timer deploy/systemd/balans-restore-test.timer (tanggal 2 setiap bulan, 03.00 WIB).
#
#   restore-test.sh                           backup terbaru + seluruh WAL
#   restore-test.sh "2026-11-01 10:00:00+07"  point-in-time (pakai RESTORE_BACKUP_NAME bila target sebelum
#                                             backup terbaru)
#
# Langkah:
#   1. backup-fetch ke volume sementara, recovery di container Postgres sementara (jaringan internal sendiri,
#      batas memori RESTORE_MEMORY) — database production tidak disentuh.
#   2. `migrate status` dengan image aplikasi yang sedang dirilis: versi skema hasil restore = versi terbaru.
#   3. Pemeriksaan isi: jumlah baris tabel kunci, jurnal posted balance, saldo stok = jumlah mutasi ledger.
#   4. Arsip file filestore terbaru (backup-files) diambil; setiap file yang tercatat terunggah sebelum arsip
#      dibuat harus ada di arsip.
#   5. Hasil dicatat satu baris di $BALANS_HOME/restore-reports.log, dikirim ke RESTORE_HEARTBEAT_URL (sukses
#      atau /fail), dan alert ke ALERT_WEBHOOK_URL bila gagal. Container dan volume sementara dihapus.
#
# Env (default dibaca dari $BALANS_HOME/production.deploy dan monitoring.env):
#   RESTORE_IMAGE       image balans-postgres (default REGISTRY/balans-postgres:POSTGRES_IMAGE_TAG)
#   RESTORE_APP_IMAGE   image balans-api untuk migrate status (default REGISTRY/balans-api:IMAGE_TAG); "skip"
#                       melewati langkah 2
#   RESTORE_ENV_FILE    env berisi kredensial WAL-G & Postgres (default production.env)
#   RESTORE_BACKUP_NAME nama backup WAL-G (default LATEST)
#   RESTORE_MEMORY      batas memori container restore (default 768m, Runbook 8.1)
#   RESTORE_SKIP_FILES  1 = lewati pemeriksaan arsip file
#   RESTORE_VERIFY_SQL  query tambahan; hasilnya dicetak (bukan penentu lulus)
#   RESTORE_TIMEOUT     batas detik menunggu recovery selesai (default 3600)
set -euo pipefail

target_time="${1:-}"
script_dir="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=../monitoring/lib.sh
. "$script_dir/../monitoring/lib.sh"
home="$balans_home"
deploy_file="${RESTORE_DEPLOY_FILE:-$home/production.deploy}"

deploy_value() {
  [ -f "$deploy_file" ] && grep "^$1=" "$deploy_file" | tail -n 1 | cut -d= -f2-
  return 0
}
registry="$(deploy_value REGISTRY)"
image="${RESTORE_IMAGE:-}"
if [ -z "$image" ] && [ -n "$registry" ]; then
  image="$registry/balans-postgres:$(deploy_value POSTGRES_IMAGE_TAG)"
fi
app_image="${RESTORE_APP_IMAGE:-}"
if [ -z "$app_image" ] && [ -n "$registry" ]; then
  app_image="$registry/balans-api:$(deploy_value IMAGE_TAG)"
fi
[ -n "$image" ] || { echo "RESTORE_IMAGE wajib diisi (atau sediakan $deploy_file)" >&2; exit 2; }

env_file="${RESTORE_ENV_FILE:-$home/production.env}"
[ -f "$env_file" ] || { echo "file env $env_file tidak ada" >&2; exit 2; }
backup_name="${RESTORE_BACKUP_NAME:-LATEST}"
memory="${RESTORE_MEMORY:-768m}"
timeout_s="${RESTORE_TIMEOUT:-3600}"
report="${RESTORE_LOG:-$home/restore-reports.log}"
heartbeat_url="${RESTORE_HEARTBEAT_URL:-}"
name="balans-restore-test"
volume="balans-restore-test-data"
network="balans-restore-test"
pgdata="/var/lib/postgresql/18/docker"

env_value() { grep "^$1=" "$env_file" | tail -n 1 | cut -d= -f2-; }
db_user="$(env_value POSTGRES_USER)"
db_name="$(env_value POSTGRES_DB)"

cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

started=$(date +%s)
failures=()
fail() { failures+=("$1"); echo "restore: GAGAL — $1" >&2; }

finish() {
  local status="OK" duration line
  duration=$(( $(date +%s) - started ))
  [ "${#failures[@]}" -eq 0 ] || status="GAGAL"
  line="$(date -u +%FT%TZ) restore $status durasi=${duration}s target=${target_time:-latest} backup=${backup_used:-?}"
  line+=" skema=${schema:-?} tabel=${tables:-?} jurnal_tidak_balance=${unbalanced:-?} stok_selisih=${stock_diff:-?}"
  line+=" file=${files_result:-?} data_terakhir=${last_activity:-?} baris=${row_counts:-?}"
  [ "${#failures[@]}" -eq 0 ] || line+=" alasan=\"$(printf '%s; ' "${failures[@]}")\""
  mkdir -p "$(dirname "$report")"
  echo "$line" | tee -a "$report"
  if [ "$status" = "OK" ]; then
    ping_heartbeat "$heartbeat_url" "" "$line"
  else
    ping_heartbeat "$heartbeat_url" /fail "$line"
    send_alert "Uji restore GAGAL: $(printf '%s; ' "${failures[@]}")"
    exit 1
  fi
}

# Perintah tak terduga yang gagal tetap dilaporkan sebagai uji gagal (bukan keluar diam-diam).
on_error() {
  trap - ERR
  fail "perintah gagal di baris $1"
  finish
}
trap 'on_error $LINENO' ERR

ping_heartbeat "$heartbeat_url" /start
docker network create "$network" >/dev/null
docker volume create "$volume" >/dev/null

# --- 1. Ambil backup dan jalankan recovery ------------------------------------------------------------------
echo "restore: mengambil backup $backup_name dengan $image"
# Stdout skrip di container hanya nama backup yang dipakai (LATEST diterjemahkan); log WAL-G ke stderr.
if ! backup_used=$(docker run --rm -i --env-file "$env_file" -e PGDATA="$pgdata" -e TARGET_TIME="$target_time" \
  -e BACKUP_NAME="$backup_name" -v "$volume:/var/lib/postgresql" "$image" bash -es <<'FETCH'
    if [ "$BACKUP_NAME" = "LATEST" ]; then
      BACKUP_NAME=$(wal-g backup-list 2>/dev/null | awk 'NR > 1 { name = $1 } END { print name }')
      [ -n "$BACKUP_NAME" ] || { echo "tidak ada backup di storage" >&2; exit 1; }
    fi
    mkdir -p "$PGDATA"
    wal-g backup-fetch "$PGDATA" "$BACKUP_NAME" >&2
    touch "$PGDATA/recovery.signal"
    {
      echo "restore_command = 'wal-g wal-fetch %f %p'"
      echo "recovery_target_action = 'promote'"
      if [ -n "$TARGET_TIME" ]; then echo "recovery_target_time = '$TARGET_TIME'"; fi
      echo "archive_mode = 'off'"
    } >> "$PGDATA/postgresql.auto.conf"
    chown -R postgres:postgres /var/lib/postgresql
    chmod 0700 "$PGDATA"
    echo "$BACKUP_NAME"
FETCH
); then
  fail "backup-fetch $backup_name gagal"
  finish
fi

echo "restore: menjalankan recovery (memori $memory)"
# Memori kecil agar uji restore tidak berebut dengan production (Runbook 8.1). max_connections, max_worker_processes,
# dll. tidak boleh lebih kecil dari primary (recovery ditolak), jadi hanya buffer yang diperkecil.
docker run -d --name "$name" --network "$network" --memory "$memory" --memory-swap "$memory" --cpus 1 \
  --env-file "$env_file" -e PGDATA="$pgdata" -v "$volume:/var/lib/postgresql" "$image" \
  postgres -c shared_buffers=128MB -c work_mem=4MB -c archive_mode=off -c max_wal_size=1GB >/dev/null

psql_q() {
  docker exec "$name" psql -X -q -t -A -U "$db_user" -d "$db_name" -v ON_ERROR_STOP=1 -c "$1"
}

promoted=false
deadline=$(( $(date +%s) + timeout_s ))
while [ "$(date +%s)" -lt "$deadline" ]; do
  if [ "$(psql_q "select not pg_is_in_recovery()" 2>/dev/null)" = "t" ]; then
    promoted=true
    break
  fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null)" != "true" ]; then
    break
  fi
  sleep 2
done
if ! $promoted; then
  docker logs --tail 30 "$name" >&2 || true
  fail "recovery tidak selesai (container berhenti atau melewati ${timeout_s} detik)"
  finish
fi
echo "restore: recovery selesai dalam $(( $(date +%s) - started )) detik"

# --- 2. Versi skema vs image aplikasi -----------------------------------------------------------------------
schema_db=$(psql_q "select coalesce(max(version_id), 0) from goose_db_version where is_applied" 2>/dev/null || echo "?")
schema="$schema_db"
tables=$(psql_q "select count(*) from information_schema.tables where table_schema = 'public'")
if [ -n "$app_image" ] && [ "$app_image" != "skip" ]; then
  database_url="$(env_value DATABASE_URL | sed -E "s#@[^/@]+/#@$name:5432/#")"
  if status_out=$(docker run --rm --network "$network" --env-file "$env_file" -e DATABASE_URL="$database_url" \
    -e LOG_LEVEL=info "$app_image" migrate status 2>&1); then
    current=$(echo "$status_out" | sed -n 's/.*"current":\([0-9]*\).*/\1/p' | tail -n 1)
    latest=$(echo "$status_out" | sed -n 's/.*"latest":\([0-9]*\).*/\1/p' | tail -n 1)
    schema="$current/$latest"
    if [ -z "$current" ] || [ -z "$latest" ]; then
      fail "output migrate status tidak dikenali: ${status_out//$'\n'/ }"
    elif [ "$current" -lt "$latest" ] && [ -z "$target_time" ]; then
      fail "skema hasil restore ($current) tertinggal dari migrasi image aplikasi ($latest)"
    fi
  else
    fail "migrate status gagal: $(echo "$status_out" | tail -n 3 | tr '\n' ' ')"
  fi
else
  echo "restore: migrate status dilewati (RESTORE_APP_IMAGE=skip)"
fi

# --- 3. Pemeriksaan isi data --------------------------------------------------------------------------------
# Jumlah baris tabel kunci yang ada di skema (tabel modul yang belum dirilis dilewati).
row_counts=$(psql_q "
  select string_agg(t || ':' || (xpath('/row/c/text()',
           query_to_xml(format('select count(*) as c from %I', t), false, true, '')))[1]::text, ',' order by ord)
  from unnest(array['users','audit_logs','accounts','journal_entries','journal_lines','products','customers',
                    'partners','suppliers','employees','stock_movements','stock_balances','purchase_orders',
                    'inbounds','file_objects','approval_requests']) with ordinality as k(t, ord)
  where to_regclass('public.' || t) is not null")
[ -n "$row_counts" ] || fail "tabel kunci tidak ditemukan"

if [ "$(psql_q "select to_regclass('public.journal_lines') is not null")" = "t" ]; then
  # Setiap jurnal posted: total debit = total kredit = total_amount.
  unbalanced=$(psql_q "
    select count(*) from (
      select e.id from journal_entries e join journal_lines l on l.journal_entry_id = e.id
      where e.status = 'posted'
      group by e.id, e.total_amount
      having sum(l.debit_amount) <> sum(l.credit_amount) or sum(l.debit_amount) <> e.total_amount) x")
  [ "$unbalanced" = "0" ] || fail "$unbalanced jurnal posted tidak balance"
else
  unbalanced="-"
fi

if [ "$(psql_q "select to_regclass('public.stock_balances') is not null")" = "t" ]; then
  # Read model saldo stok harus sama dengan jumlah mutasi ledger per produk × lokasi × batch × status.
  stock_diff=$(psql_q "
    select count(*) from (
      select product_id, location_id, batch_id, stock_status, sum(qty) as qty
      from stock_movements group by 1, 2, 3, 4) m
    full join stock_balances b
      on b.product_id = m.product_id and b.location_id = m.location_id
     and b.batch_id is not distinct from m.batch_id and b.stock_status = m.stock_status
    where coalesce(m.qty, 0) <> coalesce(b.qty, 0)")
  [ "$stock_diff" = "0" ] || fail "$stock_diff saldo stok berbeda dari ledger mutasi"
else
  stock_diff="-"
fi

if [ "$(psql_q "select to_regclass('public.audit_logs') is not null")" = "t" ]; then
  last_activity=$(psql_q "select coalesce(to_char(max(created_at) at time zone 'Asia/Jakarta', 'YYYY-MM-DD\"T\"HH24:MI\"WIB\"'), '-') from audit_logs")
fi

if [ -n "${RESTORE_VERIFY_SQL:-}" ]; then
  echo "restore: verifikasi tambahan → $(psql_q "$RESTORE_VERIFY_SQL" | tr '\n' ' ')"
fi

# --- 4. Arsip file filestore --------------------------------------------------------------------------------
if [ "${RESTORE_SKIP_FILES:-0}" = "1" ]; then
  files_result="dilewati"
elif [ "$(psql_q "select to_regclass('public.file_objects') is not null")" != "t" ]; then
  files_result="-"
else
  # Satu container sementara mengambil arsip terbaru dan mencetak nama arsip + daftar object key di dalamnya.
  if listing=$(docker run --rm -i --env-file "$env_file" -e PREFIX="${FILESTORE_BACKUP_PREFIX:-filestore}" "$image" \
    bash -es <<'FILES'
      latest=$(wal-g st ls "$PREFIX/daily/" 2>/dev/null | awk '$1 == "obj" { print $NF }' | sort | tail -n 1)
      if [ -z "$latest" ]; then
        echo "ARCHIVE -"
        exit 0
      fi
      echo "ARCHIVE $latest"
      wal-g st get "$PREFIX/daily/$latest" /tmp/files.tar >&2
      tar -tf /tmp/files.tar | awk -F/ 'NF && $NF != "" { print "KEY " $NF }'
FILES
  ); then
    archive=$(echo "$listing" | sed -n 's/^ARCHIVE //p')
    if [ "$archive" = "-" ]; then
      archive_ts="1970-01-01T00:00:00Z"
    else
      # Nama arsip YYYYMMDDTHHMMSSZ.tar.<ext> → timestamp ISO.
      archive_ts="${archive:0:4}-${archive:4:2}-${archive:6:2}T${archive:9:2}:${archive:11:2}:${archive:13:2}Z"
    fi
    expected=$(psql_q "select object_key from file_objects
      where status = 'uploaded' and uploaded_at <= '$archive_ts'::timestamptz order by 1")
    missing=$(comm -23 <(echo "$expected" | grep . | sort) <(echo "$listing" | sed -n 's/^KEY //p' | sort) | wc -l)
    total=$(echo "$expected" | grep -c . || true)
    files_result="$(( total - missing ))/$total@${archive%%.*}"
    [ "$missing" -eq 0 ] || fail "$missing file terunggah sebelum arsip $archive tidak ada di arsip"
    if [ "$archive" = "-" ]; then
      uploaded=$(psql_q "select count(*) from file_objects where status = 'uploaded'")
      [ "$uploaded" -eq 0 ] || fail "belum ada arsip file filestore padahal $uploaded file tercatat terunggah"
    fi
  else
    fail "arsip file filestore tidak bisa diambil"
    files_result="gagal"
  fi
fi

finish
