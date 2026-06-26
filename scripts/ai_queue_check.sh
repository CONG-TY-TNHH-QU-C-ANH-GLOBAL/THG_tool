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

# Queue items live in domain/component subfolders under $items_dir
# (e.g. architecture/<component>/, docs/) — discover recursively. Items are
# grouped by stable domain, never by mutable status; resolution is by the
# `id:` frontmatter, so physical location does not affect dependency checks.
mapfile -t items < <(find "$items_dir" -type f -name '*.md' | sort)

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

# Dependency semantics:
#   - a depends_on entry that names a NON-EXISTENT item id is an invalid queue -> FAIL.
#   - a dependency that EXISTS but is not yet DONE is a normal WAITING state, not a
#     failure: the dependent READY item simply cannot execute yet.
# The first EXECUTABLE READY item is the first READY item whose deps are ALL DONE.
first_executable=""
waiting=()
for f in "${items[@]}"; do
  [[ "$(field "$f" status)" == "READY" ]] || continue
  read -ra dep_ids <<< "$(field "$f" depends_on | tr -d '[],')"
  blocked_by=""
  item_missing=0
  for dep in "${dep_ids[@]}"; do
    dep_status="$(status_of_id "$dep")"
    if [[ "$dep_status" == "MISSING" ]]; then
      echo "FAIL $f: depends_on references unknown item '$dep'"
      fail=1
      item_missing=1
    elif [[ "$dep_status" != "DONE" && -z "$blocked_by" ]]; then
      blocked_by="$dep=$dep_status"
    fi
  done
  [[ "$item_missing" -eq 1 ]] && continue
  if [[ -n "$blocked_by" ]]; then
    waiting+=("$(field "$f" id) waits for $blocked_by")
  elif [[ -z "$first_executable" ]]; then
    first_executable="$f"
  fi
done

echo "== summary =="
echo "items checked:          ${#items[@]}"
echo "first READY item:       ${first_ready:-(none)}"
echo "first executable READY: ${first_executable:-(none)}"
if [[ ${#waiting[@]} -gt 0 ]]; then
  echo "waiting:"
  for w in "${waiting[@]}"; do echo "  - $w"; done
fi
if [[ -z "$first_executable" && ${#waiting[@]} -gt 0 ]]; then
  echo "note: no executable READY item — all READY items are waiting on non-DONE dependencies"
fi

if [[ "$fail" -ne 0 ]]; then
  echo "status: FAIL"
  exit 1
fi
echo "status: OK"
