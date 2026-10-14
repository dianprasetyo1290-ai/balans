#!/usr/bin/env bash
# Menghasilkan ulang semua kode dari kontrak: bundle OpenAPI, server Go (oapi-codegen), query (sqlc),
# dan tipe TypeScript web-erp. CI gagal jika hasilnya berbeda dengan yang di-commit (Technical Guideline 8.1).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/backend"

echo "→ bundle OpenAPI"
if ! out="$(pnpm --silent --package=@redocly/cli@2.55.0 dlx redocly bundle api/openapi.yaml -o api/openapi.bundle.yaml 2>&1)"; then
  echo "$out"
  exit 1
fi

echo "→ oapi-codegen"
for config in api/codegen/*.yaml; do
  go tool oapi-codegen -config "$config" api/openapi.bundle.yaml
done

echo "→ sqlc"
if command -v sqlc >/dev/null 2>&1; then
  sqlc generate
else
  # sqlc butuh cgo; di Windows dijalankan lewat image resmi.
  MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W 2>/dev/null || pwd):/src" -w /src sqlc/sqlc:1.30.0 generate
fi

echo "→ lint config"
go run ./tools/lintconfig > .golangci.yml

if [ -f "$root/web-erp/package.json" ]; then
  echo "→ tipe TypeScript web-erp"
  (cd "$root/web-erp" && pnpm --silent generate:api)
fi
echo "Selesai."
