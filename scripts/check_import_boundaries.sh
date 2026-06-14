#!/usr/bin/env bash
# check_import_boundaries.sh — early, warn-only architecture import guard.
#
# Statically checks a SUBSET of the module-boundary rules in
# docs/architecture/MODULE_BOUNDARIES.md by grepping Go import paths. It is an
# early guardrail, NOT a full linter, and it is intentionally permissive:
#
#   * it prints WARN lines for likely violations (rule name + file + package);
#   * it ALWAYS exits 0 (never fails CI) in this phase;
#   * aspirational modules that don't exist yet (services/taobao, drivers split)
#     are documented in MODULE_BOUNDARIES.md, not checked here.
#
# Usage:
#   bash scripts/check_import_boundaries.sh
#   OK                          # no warnings
#   WARN [RULE] path imports github.com/thg/scraper/internal/...   # a violation
#
# Promotion to fail-hard CI is a later phase (REFACTOR_ROADMAP.md), once the
# warnings are triaged to zero or an explicit allowlist.
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 0
WARN=0

# emit RULE < grep-output(path:line:content)
emit() {
  local rule="$1"
  local line file pkg
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    file="${line%%:*}"
    pkg="$(printf '%s' "$line" | grep -oE 'github\.com/thg/scraper/internal/[a-z0-9_/]+' | head -1)"
    printf 'WARN [%s] %s imports %s\n' "$rule" "$file" "$pkg"
    WARN=$((WARN + 1))
  done
}

# scan_dir RULE FORBIDDEN_ERE DIR
# Greps non-test Go files under DIR for an import of a forbidden thg package.
# Uses a here-string (not a pipe) into emit so the WARN counter survives.
scan_dir() {
  local rule="$1" forbidden="$2" dir="$3" out
  [ -d "$dir" ] || return 0
  out="$(grep -rnE "\"github\.com/thg/scraper/internal/(${forbidden})" "$dir" --include='*.go' 2>/dev/null \
    | grep -v '_test\.go:')"
  [ -n "$out" ] && emit "$rule" <<< "$out"
  return 0
}

# scan_glob RULE FORBIDDEN_ERE GLOB...  (non-recursive; file-specific rules)
scan_glob() {
  local rule="$1" forbidden="$2"; shift 2
  local out
  out="$(grep -nE "\"github\.com/thg/scraper/internal/(${forbidden})" "$@" 2>/dev/null \
    | grep -v '_test\.go:')"
  [ -n "$out" ] && emit "$rule" <<< "$out"
  return 0
}

echo "== import-boundary check (warn-only) =="

# 1. ai/comment is PURE intelligence — only models + stdlib.
scan_dir AI_PURE 'store|server|browsergateway|jobs|jobhandlers|leadingest|fburl|telegram|connectors' internal/ai/comment

# 2. AI must not import the execution layer (outbound/connectors/browser/jobs).
scan_dir AI_NO_EXECUTION 'store/outbound|store/connectors|browsergateway|jobhandlers' internal/ai

# 3. Copilot DRIVER must not import DB repositories directly (root internal/ai files).
scan_glob COPILOT_NO_DIRECT_REPO 'store' internal/ai/*.go

# 4. outbound store must stay vertical-neutral (no Facebook / no copilot).
scan_dir OUTBOUND_NO_FACEBOOK 'fburl|jobhandlers|leadingest' internal/store/outbound
scan_dir OUTBOUND_NO_COPILOT 'ai($|/)' internal/store/outbound

# 5. outbound application orchestrator (cmd/scraper) — surface FB coupling (advisory).
scan_glob OUTBOUND_APP_FACEBOOK 'fburl|jobhandlers' cmd/scraper/outbound_actions.go

# 6. platform must not import business services / intelligence.
scan_dir PLATFORM_NO_BUSINESS 'ai($|/)|jobhandlers|leadingest' internal/platform

# 7. store must not import the HTTP server (transport must stay above the store).
scan_dir STORE_NO_SERVER 'server($|/)' internal/store

# 8. notifications must not own Facebook lead logic.
scan_dir NOTIFICATIONS_NO_FB_LOGIC 'leadingest|jobhandlers|fburl|store/leads' internal/telegram/control

# 9. no global contracts god-package.
if [ -d internal/contracts ]; then
  printf 'WARN [NO_CONTRACTS_GODPKG] internal/contracts exists — interfaces belong with their consumer (see PORTS_AND_ADAPTERS.md)\n'
  WARN=$((WARN + 1))
fi

echo "== summary =="
if [ "$WARN" -eq 0 ]; then
  echo "OK"
else
  echo "${WARN} warning(s) — WARN-ONLY, exit 0 (see docs/architecture/MODULE_BOUNDARIES.md)"
fi
exit 0
