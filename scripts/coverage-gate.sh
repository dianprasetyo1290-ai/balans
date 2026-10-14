#!/usr/bin/env bash
# Gate coverage minimal 80% untuk package domain, tax, pricing, money, dan posting rule (Technical Guideline 12).
set -euo pipefail
cd "$(dirname "$0")/../backend"

threshold="${COVERAGE_THRESHOLD:-80}"
mapfile -t packages < <(go list ./... | grep -E '/(domain|tax|pricing|money|postingrule)$' || true)
if [ "${#packages[@]}" -eq 0 ]; then
  echo "Tidak ada package yang terkena gate coverage."
  exit 0
fi

status=0
for pkg in "${packages[@]}"; do
  profile="$(mktemp)"
  go test -count=1 -coverprofile="$profile" "$pkg" >/dev/null
  if [ "$(wc -l < "$profile")" -le 1 ]; then
    echo "LEWATI $pkg (tidak ada statement)"
    rm -f "$profile"
    continue
  fi
  pct="$(go tool cover -func="$profile" | awk '/^total:/ { sub("%", "", $3); print $3 }')"
  rm -f "$profile"
  if awk -v p="$pct" -v t="$threshold" 'BEGIN { exit !(p + 0 < t + 0) }'; then
    echo "GAGAL  $pkg ${pct}% < ${threshold}%"
    status=1
  else
    echo "LOLOS  $pkg ${pct}%"
  fi
done
exit "$status"
