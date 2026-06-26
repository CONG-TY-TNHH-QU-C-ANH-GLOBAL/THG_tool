#!/usr/bin/env bash
# ai_status.sh — one-glance status for AI-assisted development (Autopilot v2.1).
# Prints branch, HEAD, working-tree status, and the lockless-queue snapshot:
# first READY, first EXECUTABLE READY (all deps DONE), IN_PROGRESS, REVIEW, and
# any READY items waiting on non-DONE deps. Warns if the central queue index was
# edited on a normal (non-queue-governance) branch.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

branch="$(git rev-parse --abbrev-ref HEAD)"
items_dir="docs/ai/queue/items"
NONE="(none)" # sentinel for "no item in this slot"

echo "branch: $branch"
echo "HEAD:   $(git log -1 --format='%h %s')"

echo
echo "status:"
git status --short || true

field_of() { # field_of FILE KEY
  local file="$1" key="$2"
  sed -n "s/^$key:[[:space:]]*//p" "$file" | head -1
}
status_of_id() {                                                     # status_of_id ID
  local want="$1" f
  for f in "$items_dir"/*.md; do
    [[ -e "$f" ]] || continue
    [[ "$(field_of "$f" id)" == "$want" ]] && { field_of "$f" status; return 0; }
  done
  echo "MISSING"
}

first_ready="$NONE"; first_inprog="$NONE"; first_review="$NONE"
first_exec="$NONE"; waiting=()
for f in "$items_dir"/*.md; do
  [[ -e "$f" ]] || continue
  st="$(field_of "$f" status)"; id="$(field_of "$f" id)"
  case "$st" in
    READY)       [[ "$first_ready"  == "$NONE" ]] && first_ready="$id" ;;
    IN_PROGRESS) [[ "$first_inprog" == "$NONE" ]] && first_inprog="$id" ;;
    REVIEW)      [[ "$first_review" == "$NONE" ]] && first_review="$id" ;;
    *)           ;; # DONE / BLOCKED / other: no "first of" slot to track
  esac
  [[ "$st" == "READY" ]] || continue
  read -ra deps <<< "$(field_of "$f" depends_on | tr -d '[],')"
  blk=""
  for d in "${deps[@]}"; do
    ds="$(status_of_id "$d")"
    [[ "$ds" != "DONE" && -z "$blk" ]] && blk="$d=$ds"
  done
  if [[ -n "$blk" ]]; then
    waiting+=("$id waits for $blk")
  elif [[ "$first_exec" == "$NONE" ]]; then
    first_exec="$id"
  fi
done

echo
echo "queue:"
echo "  first READY:            $first_ready"
echo "  first executable READY: $first_exec"
echo "  IN_PROGRESS:            $first_inprog"
echo "  REVIEW:                 $first_review"
if [[ ${#waiting[@]} -gt 0 ]]; then
  echo "  blocked/waiting:"
  for w in "${waiting[@]}"; do echo "    - $w"; done
fi

# Lockless-queue guard: the central index must not change in normal work PRs.
index="docs/ai/AUTOPILOT_QUEUE.md"
if ! git diff --quiet -- "$index" 2>/dev/null || ! git diff --cached --quiet -- "$index" 2>/dev/null; then
  case "$branch" in
    *queue*|*autopilot*|*workflow*) ;; # queue-governance branch: editing the index is expected
    *) echo; echo "WARN $index is modified on '$branch' — work PRs must edit only $items_dir/*.md (lockless queue)" ;;
  esac
fi
