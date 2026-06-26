#!/usr/bin/env bash
# ai_status.sh — one-glance status for AI-assisted development (Autopilot v2.1).
# Prints branch, HEAD, working-tree status, and the lockless-queue lifecycle
# snapshot (first READY / IN_PROGRESS / REVIEW item). Warns if the central queue
# index was edited on a normal (non-queue-governance) branch.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

branch="$(git rev-parse --abbrev-ref HEAD)"
items_dir="docs/ai/queue/items"

echo "branch: $branch"
echo "HEAD:   $(git log -1 --format='%h %s')"

echo
echo "status:"
git status --short || true

# first_with STATUS -> path of the first item file in that status, or "(none)".
first_with() {
  local want="$1" f
  for f in "$items_dir"/*.md; do
    [[ -e "$f" ]] || continue
    if [[ "$(sed -n 's/^status:[[:space:]]*//p' "$f" | head -1)" == "$want" ]]; then
      echo "$f"
      return 0
    fi
  done
  echo "(none)"
}

echo
echo "queue:"
echo "  READY:       $(first_with READY)"
echo "  IN_PROGRESS: $(first_with IN_PROGRESS)"
echo "  REVIEW:      $(first_with REVIEW)"

# Lockless-queue guard: the central index must not change in normal work PRs.
index="docs/ai/AUTOPILOT_QUEUE.md"
if ! git diff --quiet -- "$index" 2>/dev/null || ! git diff --cached --quiet -- "$index" 2>/dev/null; then
  case "$branch" in
    *queue*|*autopilot*|*workflow*) ;; # queue-governance branch: editing the index is expected
    *)
      echo
      echo "WARN $index is modified on '$branch' — work PRs must edit only $items_dir/*.md (lockless queue)"
      ;;
  esac
fi
