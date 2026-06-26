#!/usr/bin/env bash
# check_docs_governance.sh — keep Markdown/specs/debt from sprawling at the repo
# root. WARN-only on unmanaged root .md (legacy docs are tolerated, never
# deleted); FAIL only if a required governance doc is missing. See
# docs/DOCS_GOVERNANCE.md.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Root .md files permitted at the repo root (thin agent entrypoints + readme).
allowed_root_md=(README.md AGENTS.md CLAUDE.md SPEC_GOVERNANCE.md)

# Governance docs that must exist (deleting one is a hard failure).
required_docs=(
  docs/DOCS_GOVERNANCE.md
  docs/INDEX.md
  docs/ai/AUTOPILOT_QUEUE.md
  docs/ai/ESCALATION_PLAYBOOK.md
  docs/ai/AGENT_REPORT_TEMPLATE.md
)

is_allowed_root() {
  local name="$1" allowed
  for allowed in "${allowed_root_md[@]}"; do
    [[ "$name" == "$allowed" ]] && return 0
  done
  return 1
}

echo "== docs governance check =="

# 1. Unmanaged root markdown (warn-only; report, never delete).
unmanaged=0
for f in *.md; do
  [[ -e "$f" ]] || continue          # no root .md → glob stays literal, skip
  if ! is_allowed_root "$f"; then
    echo "WARN unmanaged root doc: $f — move under docs/<category>/ (docs/DOCS_GOVERNANCE.md)"
    unmanaged=$((unmanaged + 1))
  fi
done

# 2. Required governance docs must exist (hard failure).
missing=0
for d in "${required_docs[@]}"; do
  if [[ ! -f "$d" ]]; then
    echo "FAIL missing required doc: $d"
    missing=$((missing + 1))
  fi
done

echo "== summary =="
echo "allowed root markdown: ${allowed_root_md[*]}"
echo "unmanaged root docs:   ${unmanaged} (warn-only)"
echo "missing required docs: ${missing}"

if [[ "$missing" -gt 0 ]]; then
  echo "status: FAIL"
  exit 1
fi
echo "status: OK"
