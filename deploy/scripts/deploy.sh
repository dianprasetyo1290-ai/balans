#!/usr/bin/env bash
# Deploy satu lingkungan di VPS (Technical Guideline 14.3, 14.4, 14.6). Dipanggil workflow GitHub Actions
# lewat SSH, atau manual untuk rollback:
#
#   deploy.sh staging    <image-tag>
#   deploy.sh production <image-tag>
#
# Langkah: pull image → (production) backup DB → migrate up → ganti container → tunggu healthcheck →
# smoke test /readyz lewat host publik → rollback otomatis ke tag sebelumnya jika gagal.
set -euo pipefail

env_name="${1:?lingkungan wajib diisi: staging atau production}"
new_tag="${2:?tag image wajib diisi}"
case "$env_name" in
  staging|production) ;;
  *) echo "lingkungan tidak dikenal: $env_name" >&2; exit 2 ;;
esac

home="${BALANS_HOME:-/opt/balans}"
deploy_file="$home/$env_name.deploy"   # REGISTRY, POSTGRES_IMAGE_TAG, ENV_FILE, SMOKE_URL, IMAGE_TAG
[ -f "$deploy_file" ] || { echo "file $deploy_file tidak ada" >&2; exit 2; }

compose() {
  docker compose -p "balans-$env_name" --env-file "$deploy_file" \
    -f "$home/deploy/compose.yaml" -f "$home/deploy/compose.$env_name.yaml" "$@"
}

set_tag() {
  if grep -q '^IMAGE_TAG=' "$deploy_file"; then
    sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=$1/" "$deploy_file"
  else
    echo "IMAGE_TAG=$1" >> "$deploy_file"
  fi
}

smoke_url="$(grep '^SMOKE_URL=' "$deploy_file" | cut -d= -f2-)"
previous_tag="$(grep '^IMAGE_TAG=' "$deploy_file" | cut -d= -f2- || true)"

smoke() {
  for _ in $(seq 1 20); do
    if curl -fsS -m 5 "$smoke_url/readyz" | grep -q '"status":"ready"'; then
      return 0
    fi
    sleep 3
  done
  return 1
}

rollback() {
  if [ -z "$previous_tag" ] || [ "$previous_tag" = "$new_tag" ]; then
    echo "deploy: tidak ada tag sebelumnya untuk rollback" >&2
    return 1
  fi
  echo "deploy: rollback ke $previous_tag"
  set_tag "$previous_tag"
  compose up -d --remove-orphans --wait
}

echo "deploy: $env_name $previous_tag → $new_tag"
set_tag "$new_tag"
compose pull --quiet

if [ "$env_name" = "production" ]; then
  echo "deploy: backup database sebelum migrasi"
  compose up -d --wait postgres
  compose run --rm --no-deps backup backup-now
fi

echo "deploy: migrate up"
if ! compose run --rm migrate; then
  echo "deploy: migrasi gagal, container lama tetap berjalan" >&2
  set_tag "$previous_tag"
  exit 1
fi

if ! compose up -d --remove-orphans --wait || ! smoke; then
  echo "deploy: container tidak sehat atau smoke test gagal" >&2
  rollback || true
  exit 1
fi

docker image prune -f --filter "until=168h" >/dev/null
echo "deploy: $env_name berjalan dengan $new_tag"
