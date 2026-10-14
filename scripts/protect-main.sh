#!/usr/bin/env bash
# Memproteksi branch main (F0-TEK-01, Technical Guideline 14.3): wajib PR, CI hijau, minimal 1 reviewer.
# Jalankan sekali setelah repository dibuat di GitHub:
#   scripts/protect-main.sh <owner>/<repo>
set -euo pipefail
repo="${1:?format: <owner>/<repo>}"

gh api -X PUT "repos/$repo/branches/main/protection" --input - <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "checks": [
      { "context": "changes" },
      { "context": "backend" },
      { "context": "frontend" }
    ]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}
JSON

# Environment production butuh approval manual sebelum deploy (Technical Guideline 14.3).
gh api -X PUT "repos/$repo/environments/production" --input - <<'JSON'
{ "deployment_branch_policy": { "protected_branches": false, "custom_branch_policies": true } }
JSON
gh api -X POST "repos/$repo/environments/production/deployment-branch-policies" -f name='v*.*.*' -f type=tag >/dev/null
echo "Proteksi main dan environment production terpasang. Tambahkan reviewer production di Settings → Environments."
