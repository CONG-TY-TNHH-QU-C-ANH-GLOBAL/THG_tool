#!/usr/bin/env bash
# check_import_boundaries.sh — architecture import guard (WARN-ONLY).
#
# Statically checks the module-boundary rules in
# docs/architecture/MODULE_BOUNDARIES.md (+ MODULE_OWNERSHIP.yml) by grepping Go
# import paths. It is an early guardrail, NOT a full linter:
#
#   * prints WARN lines (rule + file + package + the roadmap phase that fixes it);
#   * annotates KNOWN documented gaps (expected today, tracked in CURRENT_CODE_AUDIT.md);
#   * ALWAYS exits 0 (never fails CI) in this phase;
#   * aspirational modules with no code yet are scaffolds — their rules are reserved
#     and simply find nothing.
#
# Usage:
#   bash scripts/check_import_boundaries.sh
# Output: a WARN line per likely violation, then a summary (rules checked / warnings
# / known gaps). Promotion to fail-hard CI is a later roadmap phase.
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 0

# All rule names (the count reported as "rules checked").
RULE_NAMES="AI_PURE AI_NO_EXECUTION AI_STORE_COUPLED COPILOT_NO_DIRECT_REPO \
OUTBOUND_NO_FACEBOOK OUTBOUND_NO_COPILOT OUTBOUND_APP_FACEBOOK PLATFORM_NO_SERVICES \
SERVICES_NO_SIBLINGS SERVICES_FB_NO_COPILOT SERVICES_FACEBOOK_NO_STORE_SERVER_CMD \
STORE_NO_SERVER NOTIFICATIONS_NO_FB_LOGIC \
NO_CONTRACTS_GODPKG SERVICE_NO_SIBLING WORKER_NO_TRANSPORT SIDECAR_NO_DIRECT_DB"
RULES=$(echo "$RULE_NAMES" | wc -w | tr -d ' ')
WARN=0
KNOWN=0

# Grep pattern that drops Go test files from import scans (path:line: prefix form).
# Centralised so the scan helpers share one definition.
TEST_FILE_GREP_PATTERN='_test\.go:'

# next_phase RULE -> the roadmap phase (REFACTOR_ROADMAP.md) that resolves it.
next_phase() {
  case "$1" in
    AI_PURE|AI_NO_EXECUTION)        echo "B (pure AI boundary)";;
    AI_STORE_COUPLED)               echo "G+ (inject store-derived inputs into generators/policy; stop importing store)";;
    COPILOT_NO_DIRECT_REPO)         echo "D (typed CommandBus) then G (drop store dep)";;
    OUTBOUND_NO_FACEBOOK|OUTBOUND_APP_FACEBOOK) echo "C/I (split neutral core from FB resolution)";;
    OUTBOUND_NO_COPILOT)            echo "C";;
    PLATFORM_NO_SERVICES)           echo "B/C";;
    SERVICES_NO_SIBLINGS)           echo "post-E";;
    SERVICES_FB_NO_COPILOT)         echo "C/G";;
    SERVICES_FACEBOOK_NO_STORE_SERVER_CMD) echo "invariant (services/facebook stays store/server/cmd-free — PR29A–E seam)";;
    STORE_NO_SERVER)                echo "invariant (must stay clean)";;
    NOTIFICATIONS_NO_FB_LOGIC)      echo "E (subscribe to outbox events)";;
    NO_CONTRACTS_GODPKG)            echo "design rule (never create it)";;
    SERVICE_NO_SIBLING)             echo "5/post-E (verticals share primitives via ports, never each other)";;
    WORKER_NO_TRANSPORT)            echo "invariant (worker depends on app/domain, not transport)";;
    SIDECAR_NO_DIRECT_DB)           echo "5 (sidecars call a Go-owned versioned port, never the DB)";;
    *)                              echo "?";;
  esac
}

# known_gap RULE -> 0 (true) when warnings here are EXPECTED today and documented.
known_gap() { case "$1" in COPILOT_NO_DIRECT_REPO|AI_STORE_COUPLED) return 0;; *) return 1;; esac; }

# emit RULE  (reads grep output path:line:content on stdin)
emit() {
  local rule="$1" phase tag="" line file pkg
  phase="$(next_phase "$rule")"
  known_gap "$rule" && tag=" [known gap]"
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    file="${line%%:*}"
    pkg="$(printf '%s' "$line" | grep -oE 'github\.com/thg/scraper/(internal|cmd)/[a-z0-9_/]+' | head -1)"
    printf 'WARN [%s]%s %s imports %s  -> fix in phase: %s\n' "$rule" "$tag" "$file" "$pkg" "$phase"
    WARN=$((WARN + 1))
    known_gap "$rule" && KNOWN=$((KNOWN + 1))
  done
}

# scan_dir RULE FORBIDDEN_ERE DIR  (recursive, non-test). here-string keeps counters.
scan_dir() {
  local rule="$1" forbidden="$2" dir="$3" out
  [ -d "$dir" ] || return 0
  out="$(grep -rnE "\"github\.com/thg/scraper/internal/(${forbidden})" "$dir" --include='*.go' 2>/dev/null \
    | grep -v "$TEST_FILE_GREP_PATTERN")"
  [ -n "$out" ] && emit "$rule" <<< "$out"
  return 0
}

# scan_glob RULE FORBIDDEN_ERE GLOB...  (non-recursive; file-specific rules)
scan_glob() {
  local rule="$1" forbidden="$2"; shift 2
  local out
  out="$(grep -nE "\"github\.com/thg/scraper/internal/(${forbidden})" "$@" 2>/dev/null \
    | grep -v "$TEST_FILE_GREP_PATTERN")"
  [[ -n "$out" ]] && emit "$rule" <<< "$out"
  return 0
}

# scan_paths RULE FORBIDDEN_ERE DIR  (recursive, non-test). Like scan_dir but the
# ERE is matched directly after the quoted module ROOT, so a forbidden path may be
# anywhere in the tree (internal/* AND cmd/*), not just under internal/. Each
# alternative MUST carry a ("|/) boundary so it matches both a bare package import
# ("…/internal/store") and any subpackage ("…/internal/store/connectors"). The
# leading literal double-quote anchors on a Go import string, so prose comments
# that merely mention a path (e.g. // isolated from internal/store) never match.
scan_paths() {
  local rule="$1" forbidden="$2" dir="$3" out
  [[ -d "$dir" ]] || return 0
  out="$(grep -rnE "\"github\.com/thg/scraper/(${forbidden})" "$dir" --include='*.go' 2>/dev/null \
    | grep -v "$TEST_FILE_GREP_PATTERN")"
  [[ -n "$out" ]] && emit "$rule" <<< "$out"
  return 0
}

# scan_each_service RULE  — generic sibling-import guard for internal/services/<svc>.
# Any service package importing ANOTHER internal/services/* sibling warns (self-imports
# excluded). Preventive: finds nothing until verticals are extracted. Composition root
# (cmd/*) is intentionally NOT scanned — wiring services together at main is allowed.
scan_each_service() {
  local rule="$1" svc_dir svc_name out
  [[ -d internal/services ]] || return 0
  for svc_dir in internal/services/*/; do
    [[ -d "$svc_dir" ]] || continue
    svc_name="$(basename "$svc_dir")"
    out="$(grep -rnE "\"github\.com/thg/scraper/internal/services/" "$svc_dir" --include='*.go' 2>/dev/null \
      | grep -v "$TEST_FILE_GREP_PATTERN" \
      | grep -vE "\"github\.com/thg/scraper/internal/services/${svc_name}(\"|/)")"
    [[ -n "$out" ]] && emit "$rule" <<< "$out"
  done
  return 0
}

# scan_sidecar_db RULE  — preventive guard: top-level services/* sidecars (Python, etc.)
# must reach data through a Go-owned versioned port, NEVER the multi-tenant DB directly.
# Matches DB-coupling tokens in sidecar source; warn-only. Finds nothing until a sidecar
# adds direct DB access. Not a Go import path, so it does not use emit().
scan_sidecar_db() {
  local rule="$1" phase out file line
  [[ -d services ]] || return 0
  phase="$(next_phase "$rule")"
  out="$(grep -rniE 'DATABASE_URL|DB_PATH|POSTGRES_URL|POSTGRES_DSN|sqlite3|psycopg2?|asyncpg|sqlalchemy|gorm|database/sql' services 2>/dev/null \
    | grep -v '__pycache__' | grep -v '\.pyc:')"
  [[ -z "$out" ]] && return 0
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    file="${line%%:*}"
    printf 'WARN [%s] %s shows direct-DB coupling — sidecars must call a Go-owned port  -> fix in phase: %s\n' "$rule" "$file" "$phase"
    WARN=$((WARN + 1))
  done <<< "$out"
  return 0
}

echo "== architecture import-boundary check (warn-only) =="

# 1. ai/comment is PURE intelligence — only models + stdlib.
scan_dir AI_PURE 'store|server|browsergateway|jobs|jobhandlers|leadingest|fburl|telegram|connectors|platform' internal/ai/comment

# 2. AI must not import the execution layer (outbound/connectors/browser/jobs).
scan_dir AI_NO_EXECUTION 'store/outbound|store/connectors|browsergateway|jobhandlers' internal/ai

# 3. Copilot DRIVER must not import DB repositories directly (now in internal/drivers/copilot).
#    NOTE: forbidden ERE is plain 'store' — `store($|/)` never matched the trailing-quote
#    import path, so the old dir scan was a silent no-op (fixed here after the B.2 move).
scan_dir COPILOT_NO_DIRECT_REPO 'store' internal/drivers/copilot

# 3b. Store-coupled AI generators/policy that REMAINED in internal/ai after the B.2
#     driver move (business/classifier/policy_gate) — a DISTINCT known gap from the
#     copilot driver: these are generators/policy that read business profile / comment
#     policy from the store. Tracked separately so the taxonomy stays honest; the fix
#     is to inject store-derived inputs rather than import store.
scan_dir AI_STORE_COUPLED 'store' internal/ai

# 4. outbound must stay vertical-neutral: no Facebook service, no copilot driver.
scan_dir OUTBOUND_NO_FACEBOOK 'fburl|jobhandlers|leadingest|services/' internal/store/outbound
scan_dir OUTBOUND_NO_FACEBOOK 'fburl|jobhandlers|leadingest|services/' internal/outbound
scan_dir OUTBOUND_NO_COPILOT  'ai($|/)|drivers/copilot' internal/store/outbound
scan_dir OUTBOUND_NO_COPILOT  'ai($|/)|drivers/copilot' internal/outbound

# 5. outbound application orchestrator (cmd/scraper) — surface FB coupling (advisory).
scan_glob OUTBOUND_APP_FACEBOOK 'fburl|jobhandlers' cmd/scraper/outbound_actions.go

# 6. platform must not import business services / intelligence.
scan_dir PLATFORM_NO_SERVICES 'ai($|/)|jobhandlers|leadingest|services($|/)' internal/platform

# 7. service modules must not import each other; facebook must not import the driver.
scan_dir SERVICES_NO_SIBLINGS   'services/(taobao|alibaba1688|1688)' internal/services/facebook
scan_dir SERVICES_FB_NO_COPILOT 'drivers/copilot' internal/services/facebook

# 7b. SERVICES_FACEBOOK_NO_STORE_SERVER_CMD — lock the PR29A–E seam: the FB service
#     module is consumer-of-ports only. It must NEVER import the data layer
#     (internal/store + subpkgs incl. store/connectors), HTTP server transport
#     (internal/server), the composition root (cmd/scraper — also a Go compile
#     error, guarded here for intent), connector/crawler/ingest runtime internals,
#     or sibling verticals. Adapters live in cmd/scraper; neutral primitives live
#     in internal/readiness etc. Uses scan_paths (root-anchored, ("|/)-bounded) so
#     bare AND subpackage imports are caught while comment prose is not.
scan_paths SERVICES_FACEBOOK_NO_STORE_SERVER_CMD \
  'internal/store("|/)|internal/server("|/)|cmd/scraper("|/)|internal/connectors("|/)|internal/jobhandlers("|/)|internal/leadingest("|/)|internal/services/taobao("|/)|internal/services/supplier1688("|/)' \
  internal/services/facebook

# 8. store must not import the HTTP server.
scan_dir STORE_NO_SERVER 'server($|/)' internal/store

# 9. notifications must not own Facebook lead logic.
scan_dir NOTIFICATIONS_NO_FB_LOGIC 'leadingest|jobhandlers|fburl|store/leads' internal/telegram/control
scan_dir NOTIFICATIONS_NO_FB_LOGIC 'leadingest|jobhandlers|fburl|store/leads' internal/notifications

# 10. no global contracts god-package.
if [ -d internal/contracts ]; then
  printf 'WARN [NO_CONTRACTS_GODPKG] internal/contracts exists — interfaces belong with their consumer (PORTS_AND_ADAPTERS.md)  -> fix in phase: design rule\n'
  WARN=$((WARN + 1))
fi

# 11. SERVICE_NO_SIBLING — no service vertical imports another (generic; preventive).
#     Broader than SERVICES_NO_SIBLINGS (rule 7, facebook-specific): covers every
#     internal/services/<svc> in both directions as verticals are added.
scan_each_service SERVICE_NO_SIBLING

# 12. WORKER_NO_TRANSPORT — the crawler/worker role must not import HTTP/server transport.
scan_dir WORKER_NO_TRANSPORT 'server($|/)|drivers/http|drivers/telegram|drivers/connector' cmd/worker

# 13. SIDECAR_NO_DIRECT_DB — top-level services/* sidecars must not touch the DB directly.
scan_sidecar_db SIDECAR_NO_DIRECT_DB

echo "== summary =="
echo "rules checked:        ${RULES}"
echo "warnings:             ${WARN}  (${KNOWN} known documented gap(s), $((WARN - KNOWN)) other)"
if [ "$WARN" -eq 0 ]; then
  echo "status:               OK"
else
  echo "status:               WARN-ONLY (exit 0) — see docs/architecture/MODULE_OWNERSHIP.yml for phases"
fi
exit 0
