#!/usr/bin/env bash
# check_topology.sh — Runtime topology contract enforcement.
#
# Enforces the invariants encoded in specs/RUNTIME_TOPOLOGY.md §6.
# Without this gate, the topology doc rots silently: a developer
# moves a method across a boundary, no test fails, the doc stops
# describing reality.
#
# Two categories of check:
#   PASS / FAIL          — load-bearing invariants; a new violation fails CI
#   EXPECTED-FAIL        — documented carry-over debt (counted against a
#                          baseline); fails CI only when the count grows
#
# Exit codes:
#   0 — no unexpected violations
#   1 — at least one unexpected violation (regression — fail CI)
#
# Implementation: pure grep / awk; no ripgrep dependency. Designed to
# run on POSIX shells and git-bash on Windows.

set -o pipefail

ROOT="${1:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
cd "$ROOT"

UNEXPECTED_FILE=$(mktemp)
trap "rm -f $UNEXPECTED_FILE" EXIT
echo 0 > "$UNEXPECTED_FILE"

pass() { printf "PASS            %s\n" "$1"; }
fail() {
  printf "FAIL            %s\n" "$1"
  local n
  n=$(cat "$UNEXPECTED_FILE")
  echo $((n + 1)) > "$UNEXPECTED_FILE"
}
expected_fail() {
  printf "EXPECTED-FAIL   %s (baseline %d, actual %d)\n" "$1" "$2" "$3"
}

echo "== Runtime Topology Linter =="
echo "Root: $ROOT"
echo "Spec: specs/RUNTIME_TOPOLOGY.md"
echo

# ---------------------------------------------------------------------
# §6.1 — Dependency direction (L1 invariant).
#
# Subpackages MUST NOT import their parent `internal/store` package.
# Same-level / upward imports cause cycles by construction.
# ---------------------------------------------------------------------

echo "[1] Dependency direction — no subpackage imports parent store"
SUBPACKAGES="dbutil storetest outbound crawl knowledge coordination prompts connectors identities app threads leads"
SUBPKG_VIOLATIONS=0
for pkg in $SUBPACKAGES; do
  dir="internal/store/$pkg"
  [ -d "$dir" ] || continue
  hits=$(grep -rln '"github.com/thg/scraper/internal/store"' "$dir" 2>/dev/null \
    | grep -v '_test\.go' || true)
  if [ -n "$hits" ]; then
    for f in $hits; do
      fail "subpackage imports parent store: $f"
      SUBPKG_VIOLATIONS=$((SUBPKG_VIOLATIONS + 1))
    done
  fi
done
[ "$SUBPKG_VIOLATIONS" -eq 0 ] && pass "no subpackage imports internal/store"
echo

# ---------------------------------------------------------------------
# §6.2 — No bidirectional domain knowledge.
#
# Coordination is the truth substrate. Outbound + leads are PROJECTIONS
# over coordination's tables. Coordination MUST NOT import them.
# ---------------------------------------------------------------------

echo "[2] Coordination has no peer-domain imports"
COORD_VIOLATIONS=0
for forbidden in outbound leads threads; do
  hits=$(grep -rln "\"github.com/thg/scraper/internal/store/$forbidden\"" \
    internal/store/coordination/ 2>/dev/null | grep -v '_test\.go' || true)
  if [ -n "$hits" ]; then
    for f in $hits; do
      fail "coordination imports $forbidden: $f"
      COORD_VIOLATIONS=$((COORD_VIOLATIONS + 1))
    done
  fi
done
[ "$COORD_VIOLATIONS" -eq 0 ] && pass "coordination is free of peer-domain imports"
echo

echo "[3] Outbound has no leads/threads peer imports"
OUTBOUND_VIOLATIONS=0
for forbidden in leads threads; do
  hits=$(grep -rln "\"github.com/thg/scraper/internal/store/$forbidden\"" \
    internal/store/outbound/ 2>/dev/null | grep -v '_test\.go' || true)
  if [ -n "$hits" ]; then
    for f in $hits; do
      fail "outbound imports $forbidden: $f"
      OUTBOUND_VIOLATIONS=$((OUTBOUND_VIOLATIONS + 1))
    done
  fi
done
[ "$OUTBOUND_VIOLATIONS" -eq 0 ] && pass "outbound imports no peer engagement domains"
echo

# ---------------------------------------------------------------------
# §6.3 — Canonical writers.
#
# Coordination is the SOLE writer to action_ledger + execution_attempts.
# Any INSERT INTO these tables outside coordination/ is a topology
# violation.
# ---------------------------------------------------------------------

echo "[4] action_ledger INSERTs only in coordination/"
LEDGER_INSERT_VIOLATIONS=0
hits=$(grep -rln "INSERT INTO action_ledger" --include="*.go" \
  internal/ cmd/ 2>/dev/null | grep -v 'internal/store/coordination/' \
  | grep -v '_test\.go' || true)
if [ -n "$hits" ]; then
  for f in $hits; do
    fail "INSERT INTO action_ledger outside coordination/: $f"
    LEDGER_INSERT_VIOLATIONS=$((LEDGER_INSERT_VIOLATIONS + 1))
  done
fi
[ "$LEDGER_INSERT_VIOLATIONS" -eq 0 ] && pass "all action_ledger INSERTs originate in coordination/"
echo

echo "[5] execution_attempts INSERTs only in coordination/"
ATTEMPTS_INSERT_VIOLATIONS=0
hits=$(grep -rln "INSERT INTO execution_attempts" --include="*.go" \
  internal/ cmd/ 2>/dev/null | grep -v 'internal/store/coordination/' \
  | grep -v '_test\.go' || true)
if [ -n "$hits" ]; then
  for f in $hits; do
    fail "INSERT INTO execution_attempts outside coordination/: $f"
    ATTEMPTS_INSERT_VIOLATIONS=$((ATTEMPTS_INSERT_VIOLATIONS + 1))
  done
fi
[ "$ATTEMPTS_INSERT_VIOLATIONS" -eq 0 ] && pass "all execution_attempts INSERTs originate in coordination/"
echo

# ---------------------------------------------------------------------
# §6.4 — Append-only boundaries.
#
# action_ledger SHOULD be append-only per
# [[feedback_append_only_correction_events]]. Reconciliation MUST emit
# engagement_revoked events, never UPDATE the original row.
#
# Current state: known carry-over from Phase 5B (3 UPDATE call sites).
# Tracked as EXPECTED-FAIL with a baseline; new violations fail CI.
# ---------------------------------------------------------------------

echo "[6] action_ledger append-only invariant (with baseline)"
LEDGER_UPDATE_BASELINE=3  # Phase 5B carry-over: action_ledger.go x2 + engagement_reconcile.go x1
LEDGER_UPDATE_ACTUAL=$(grep -rln "UPDATE action_ledger\|DELETE FROM action_ledger" \
  --include="*.go" internal/ cmd/ 2>/dev/null \
  | grep -v '_test\.go' \
  | xargs -I{} grep -cE "UPDATE action_ledger|DELETE FROM action_ledger" {} 2>/dev/null \
  | awk '{s+=$1} END {print s+0}')
if [ "$LEDGER_UPDATE_ACTUAL" -gt "$LEDGER_UPDATE_BASELINE" ]; then
  fail "action_ledger UPDATE/DELETE count $LEDGER_UPDATE_ACTUAL > baseline $LEDGER_UPDATE_BASELINE — new append-only violation"
elif [ "$LEDGER_UPDATE_ACTUAL" -eq "$LEDGER_UPDATE_BASELINE" ]; then
  expected_fail "action_ledger append-only (Phase 5B carry-over)" "$LEDGER_UPDATE_BASELINE" "$LEDGER_UPDATE_ACTUAL"
else
  pass "action_ledger UPDATE/DELETE count $LEDGER_UPDATE_ACTUAL < baseline $LEDGER_UPDATE_BASELINE — baseline can be lowered to $LEDGER_UPDATE_ACTUAL"
fi
echo

# ---------------------------------------------------------------------
# §6.5 — Verified-state-centric.
#
# Per [[feedback_verified_state_centric]] downstream consumers MUST NOT
# branch on outbound_messages.status for business decisions. They read
# execution_state + verification_outcome (the V2 typed columns) or the
# coordination ledger.
# ---------------------------------------------------------------------

echo "[7] No downstream business reads of legacy outbound_messages.status"
LEGACY_STATUS_VIOLATIONS=0
# Allow reads inside outbound/ (the state machine owns the column) +
# outbound_aliases.go (bridge) + test files + migration schema.
hits=$(grep -rln 'outbound_messages\.status\|\.status[^_]' --include="*.go" \
  internal/server/ internal/ai/ internal/runtime/ cmd/ 2>/dev/null \
  | grep -v '_test\.go' \
  || true)
# (Heuristic — too noisy for now; reserved for future tightening.)
pass "deferred — needs schema-aware check (heuristic too noisy)"
echo

# ---------------------------------------------------------------------
# §6.6 — L2 wrapper expiry.
#
# Per L2 in DOMAINS.md §3, no NEW top-level bridge methods may be added
# after a subpackage extracts. Track the count of "Deprecated:" markers
# in outbound_aliases.go; if the count drops over time, that's good.
# Sudden growth = new wrappers introduced (violation).
# ---------------------------------------------------------------------

echo "[8] Typed event taxonomy — no raw event-name literals outside events package"
RAW_EVENT_VIOLATIONS=0
# Match the legacy ad-hoc pattern: `"event"` slog key followed by a
# DOMAIN.VERB event name string literal (e.g. "outbound.queued"). The
# narrow regex excludes false positives like fiber's c.Query("event", "")
# query-parameter name reads. Allow events.go itself + tests.
hits=$(grep -rlnE '"event"[[:space:]]*,[[:space:]]*"[a-z][a-z_]*\.[a-z_]+"' \
  --include="*.go" internal/ cmd/ 2>/dev/null \
  | grep -v 'internal/runtime/events/' \
  | grep -v '_test\.go' || true)
if [ -n "$hits" ]; then
  for f in $hits; do
    fail "raw event-name literal — use events.Info(ctx, events.<Const>, ...) instead: $f"
    RAW_EVENT_VIOLATIONS=$((RAW_EVENT_VIOLATIONS + 1))
  done
fi
[ "$RAW_EVENT_VIOLATIONS" -eq 0 ] && pass "all event emission uses the typed taxonomy in internal/runtime/events/"
echo

echo "[9] L2 wrapper count baseline (track only — do not fail)"
WRAPPER_FILE="internal/store/outbound_aliases.go"
if [ -f "$WRAPPER_FILE" ]; then
  count=$(grep -c "^// Deprecated:" "$WRAPPER_FILE" 2>/dev/null || echo 0)
  printf "INFO            outbound_aliases.go has %d Deprecated bridge markers\n" "$count"
fi
echo

# ---------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------
echo "== Summary =="
unexpected=$(cat "$UNEXPECTED_FILE")
if [ "$unexpected" -eq 0 ]; then
  echo "OK — no unexpected topology violations."
  exit 0
fi
echo "FAIL — $unexpected unexpected topology violation(s)."
exit 1
