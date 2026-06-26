#!/usr/bin/env bash
# ai_queue_check.sh — lockless autopilot queue integrity check.
#
# Verifies the stable index exists and every per-item file under
# docs/ai/queue/items/ has the required frontmatter with a valid status + lane.
# Simple grep/sed only — no YAML parser, no dependencies. FAILs on a structural
# problem; prints the first READY item.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

index="docs/ai/AUTOPILOT_QUEUE.md"
items_dir="docs/ai/queue/items"
required_keys=(id status lane risk depends_on parallel_safe)
valid_status="READY IN_PROGRESS REVIEW DONE BLOCKED"
valid_lane="GREEN YELLOW RED"

fail=0

echo "== autopilot queue check =="

[[ -f "$index" ]]     || { echo "FAIL missing queue index: $index"; fail=1; }
[[ -d "$items_dir" ]] || { echo "FAIL missing items dir: $items_dir"; fail=1; }

# field FILE KEY -> first frontmatter value for KEY (empty if absent).
field() {
  local file="$1" key="$2"
  sed -n "s/^$key:[[:space:]]*//p" "$file" | head -1
}

# in_list NEEDLE "a b c" -> 0 if NEEDLE is a whole-word member.
in_list() {
  local needle="$1" haystack="$2"
  case " $haystack " in *" $needle "*) return 0;; *) return 1;; esac
}

shopt -s nullglob
items=("$items_dir"/*.md)
shopt -u nullglob

# status_of_id ID -> status of the item file whose `id:` matches ID, or "MISSING".
status_of_id() {
  local want="$1" g
  for g in "${items[@]}"; do
    if [[ "$(field "$g" id)" == "$want" ]]; then
      field "$g" status
      return 0
    fi
  done
  echo "MISSING"
}

[[ ${#items[@]} -gt 0 ]] || echo "WARN no queue item files under $items_dir"

first_ready=""
for f in "${items[@]}"; do
  for k in "${required_keys[@]}"; do
    if ! grep -qE "^$k:" "$f"; then
      echo "FAIL $f: missing frontmatter key '$k'"
      fail=1
    fi
  done

  status="$(field "$f" status)"
  lane="$(field "$f" lane)"

  if ! in_list "$status" "$valid_status"; then
    echo "FAIL $f: invalid status '$status' (want one of: $valid_status)"
    fail=1
  fi
  if ! in_list "$lane" "$valid_lane"; then
    echo "FAIL $f: invalid lane '$lane' (want one of: $valid_lane)"
    fail=1
  fi

  if [[ -z "$first_ready" && "$status" == "READY" ]]; then
    first_ready="$f"
  fi
done

# Dependency guard for the NEXT-to-execute item: the first READY item must have
# every depends_on entry present AND DONE before it can be started (queue rule).
# Only the first READY item is gated — later READY items legitimately wait behind
# earlier ones in the pipeline, so they are not a failure.
if [[ -n "$first_ready" ]]; then
  deps="$(field "$first_ready" depends_on | tr -d '[],')"
  read -ra dep_ids <<< "$deps"
  for dep in "${dep_ids[@]}"; do
    dep_status="$(status_of_id "$dep")"
    if [[ "$dep_status" != "DONE" ]]; then
      echo "FAIL $first_ready: READY but dependency $dep is '$dep_status' (must be DONE before start)"
      fail=1
    fi
  done
fi

echo "== summary =="
echo "items checked:    ${#items[@]}"
echo "first READY item: ${first_ready:-(none)}"

if [[ "$fail" -ne 0 ]]; then
  echo "status: FAIL"
  exit 1
fi
echo "status: OK"
