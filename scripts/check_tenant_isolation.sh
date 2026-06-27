#!/usr/bin/env bash
# check_tenant_isolation.sh — V2 Architecture tenant-isolation linter.
#
# Enforces directive D1 from the V2 Outbound Refactor design doc:
# every SQL query against tenant-scoped tables must include
# `org_id = ?` in WHERE, and no callsite may invoke the deleted
# non-tenant-scoped legacy API.
#
# Exit codes:
#   0 — no violations
#   1 — one or more violations (fail CI)
#
# Implementation: pure grep / awk; no ripgrep dependency. Designed to
# run on both POSIX shells and git-bash on Windows.

set -o pipefail
# Deliberately NOT set -u: the violation-printing loops echo lines that
# can contain $-tokens from caught SQL text, and we don't want the
# linter to crash on parameter expansion of user-content strings.

ROOT="${1:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
cd "$ROOT"

# Use a temp file for the violation counter so subshells (from pipe)
# can bump it. printf reads the count at summary time.
VIOLATION_FILE=$(mktemp)
trap "rm -f $VIOLATION_FILE" EXIT
echo 0 > "$VIOLATION_FILE"

fail() {
  printf "FAIL  %s\n" "$1"
  local n
  n=$(cat "$VIOLATION_FILE")
  echo $((n + 1)) > "$VIOLATION_FILE"
}
pass() { printf "PASS  %s\n" "$1"; }

echo "== V2 Tenant Isolation Linter =="
echo "Root: $ROOT"
echo

# ---------------------------------------------------------------------
# Check 1: forbidden non-tenant-scoped legacy API callsites.
#
# These 7 functions were deleted in PR-2 (V2 staged refactor). Any
# remaining reference outside comments / specs / the linter itself is
# a violation. This subsumes most "did you scope by org_id?" mistakes
# because the only way to call those legacy functions is to bypass the
# tenant filter.
# ---------------------------------------------------------------------

echo "[1/2] Checking forbidden non-tenant API callsites..."

FORBIDDEN_NAMES=(
  "GetOutbound("
  "GetOutboundByStatus("
  "GetOutboundByFilter("
  "GetSentGroupPosts("
  "DeleteOutbound("
  "CountOutboundByStatus("
  "UpdateOutboundStatus("
  "UpdateOutboundContent("
)

for name in "${FORBIDDEN_NAMES[@]}"; do
  # grep -rn returns "path:lineno:body". Skip *.sh, *.md, _test.go fixtures
  # that explicitly preserve the legacy reference for documentation.
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    file_line="${line%%:*}"
    rest="${line#*:}"
    line_num="${rest%%:*}"
    body="${rest#*:}"

    # Skip if path matches an allowed location.
    case "$file_line" in
      scripts/check_tenant_isolation.sh) continue ;;
      *.md) continue ;;
      *.sh) continue ;;
      */specs/*) continue ;;
    esac
    # Skip comment-only lines.
    if echo "$body" | grep -qE '^\s*//' ; then
      continue
    fi
    # Skip if the match is itself the ForOrg variant (the substring
    # GetOutbound( matches GetOutboundForOrg( as well — the lint only
    # cares about the exact non-ForOrg variant).
    if echo "$body" | grep -qE "${name%(}ForOrg\(" ; then
      continue
    fi
    if echo "$body" | grep -qE "${name%(}ByStatusForOrg\(" ; then
      continue
    fi
    if echo "$body" | grep -qE "${name%(}ByFilterForOrg\(" ; then
      continue
    fi

    fail "$file_line:$line_num  forbidden API: $name"
    printf "      %s\n" "$body"
  done < <(grep -rn -F "$name" --include='*.go' internal cmd 2>/dev/null || true)
done

if [ "$(cat "$VIOLATION_FILE")" -eq 0 ]; then
  pass "No forbidden non-tenant API callsites remain"
fi

# ---------------------------------------------------------------------
# Check 2: SQL queries on tenant-scoped tables must include
# `org_id = ?` somewhere in the query body. Whitelist via inline
# // tenant-ok: <reason> comment within 5 lines above the query.
#
# Heuristic: for each Go file that mentions a tenant table inside a
# backtick block, extract the surrounding 0..40 lines after the opening
# backtick and verify org_id = ? appears OR a tenant-ok comment is
# present nearby.
# ---------------------------------------------------------------------

echo
echo "[2/2] Checking SQL queries scope by org_id..."

TENANT_TABLES=(
  "outbound_messages"
  "action_policies"
  "execution_attempts"
  "reels"
  "reel_scripts"
  "reel_shots"
)

# Collect Go files that reference any tenant table.
FILES=$(grep -rl -E "outbound_messages|action_policies|execution_attempts|reel_scripts|reel_shots|reels" \
  --include='*.go' internal cmd 2>/dev/null | sort -u || true)

for f in $FILES; do
  # Skip DDL files (schema bootstrap) and the linter / test fixtures.
  case "$f" in
    */schema.go) continue ;;
    *_test.go) continue ;;
  esac

  # awk extracts every backtick-delimited SQL block that mentions one
  # of the tenant tables, paired with its starting line number. For
  # each such block, we check inside it for org_id = ? OR a tenant-ok
  # comment in the 5 lines above the block.
  awk -v file="$f" '
  BEGIN { in_block = 0; block = ""; start_line = 0 }
  {
    line = $0
    nf = length(line)
    i = 1
    while (i <= nf) {
      c = substr(line, i, 1)
      if (c == "`") {
        if (in_block == 0) {
          in_block = 1
          start_line = NR
          block = ""
        } else {
          # Closing backtick — flush block.
          in_block = 0
          # Check whether the block references a tenant table.
          if (block ~ /outbound_messages|action_policies|execution_attempts|reel_scripts|reel_shots|reels/) {
            # Block must START with an actual SQL verb — skip plain
            # Go strings, prompt text, doc comments that mention the
            # table name in prose.
            trimmed = block
            sub(/^[[:space:]\n]+/, "", trimmed)
            is_sql = (trimmed ~ /^(SELECT|UPDATE|DELETE|INSERT|WITH)[[:space:]]/)
            # Skip DDL blocks.
            is_ddl = (trimmed ~ /^(CREATE|ALTER|DROP)[[:space:]]/)
            # Skip pure INSERTs (no WHERE needed; org_id is a column value).
            is_insert = (trimmed ~ /^INSERT[[:space:]]+(OR[[:space:]]+(IGNORE|REPLACE)[[:space:]]+)?INTO/)
            if (is_sql && !is_ddl && !is_insert) {
              # Verify some `org_id =` correlation exists. Accept any
              // RHS: `org_id = ?` (typical), `om.org_id = l.org_id`
              // (subquery join), `org_id IN (?)` (batch).
              if (block !~ /org_id[[:space:]]*(=|IN)[[:space:]]*/) {
                print file ":" start_line
              }
            }
          }
          block = ""
        }
      } else if (in_block) {
        block = block c
      }
      i++
    }
    if (in_block) block = block "\n"
  }
  ' "$f" 2>/dev/null | while IFS= read -r hit; do
    [ -z "$hit" ] && continue
    file_line="${hit%%:*}"
    line_num="${hit#*:}"

    # Check tenant-ok whitelist within 15 lines above the SQL block
    # (covers function-level docstrings, not just per-line annotations).
    start=$((line_num > 15 ? line_num - 15 : 1))
    if sed -n "${start},${line_num}p" "$file_line" 2>/dev/null | grep -q "tenant-ok:"; then
      continue
    fi

    fail "$file_line:$line_num  SQL missing org_id = ?"
  done
done

if [ "$(cat "$VIOLATION_FILE")" -eq 0 ]; then
  pass "All tenant-table SQL queries scope by org_id"
fi

# ---------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------

echo
VIOLATIONS=$(cat "$VIOLATION_FILE")
if [ "$VIOLATIONS" -eq 0 ]; then
  echo "OK: 0 tenant isolation violations."
  exit 0
fi

echo "FAILED: $VIOLATIONS violation(s)."
echo
echo "Resolution:"
echo "  - Migrate callers to the ForOrg variant of the function."
echo "  - For SQL queries: add 'AND org_id = ?' to the WHERE clause."
echo "  - For an intentional exception (DDL, cross-org reconciler with audit):"
echo "    add a '// tenant-ok: <reason>' comment within 5 lines of the SQL."
exit 1
