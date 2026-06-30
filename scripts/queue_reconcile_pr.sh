#!/usr/bin/env bash
# queue_reconcile_pr.sh — NON-BLOCKING, worktree-isolated queue reconciliation.
#
# Why: after architecture PRs merge, queue item metadata goes stale (status:
# REVIEW->DONE, pr_url backfill). Writing those .md updates on main or a code
# branch dirties the working tree and risks polluting code PRs. This script keeps
# the reconcile fully isolated: it applies the metadata updates inside a throwaway
# `git worktree` checked out from origin/main, commits ONLY
# docs/ai/queue/items/**/*.md onto a dedicated `chore/queue-reconcile-*` branch,
# pushes it, and removes the worktree -- the PRIMARY working tree is never touched.
# It NEVER merges. It reuses an existing open reconcile branch so it never opens a
# duplicate queue PR. All merge-verification + the "never DONE by assumption" safety
# lives in ai_queue_reconcile.sh, which this script invokes inside the worktree.
#
# Usage:
#   scripts/queue_reconcile_pr.sh --check   # dry-run: report stale merged items only
#   scripts/queue_reconcile_pr.sh --push    # isolate + commit + push the reconcile branch
#
# Exit 0 always (advisory: a queue-reconcile hiccup must never block code work).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

readonly QUEUE_DIR="docs/ai/queue/items"
readonly RECONCILE_GLOB="chore/queue-reconcile-*"
readonly COMMIT_MSG="chore(queue): reconcile merged architecture items"

# reconcile -> thin wrapper over the verifier (keeps it quoted; SC2086-clean).
reconcile() {
  bash scripts/ai_queue_reconcile.sh "$@"
  return $?
}

# stale_count -> number of REVIEW items the verifier would flip to DONE (read-only).
stale_count() {
  reconcile 2>/dev/null | sed -n 's/^reconciled to DONE: //p' | head -1
  return 0
}

# push_isolated -> apply the reconcile in a throwaway worktree off origin/main,
# commit ONLY the queue .md files, and push a dedup'd chore/queue-reconcile-* branch.
push_isolated() {
  local slug remote branch existing wt
  remote="$(git remote get-url origin 2>/dev/null || true)"
  slug="$(printf '%s' "$remote" | sed -E 's#^.*github\.com[:/]+##; s#\.git$##; s#/+$##')"

  git fetch -q origin main || { echo "queue-reconcile: git fetch failed (offline?) -- left for later." >&2; return 0; }

  # Dedup: reuse an existing open reconcile branch; else create today's dated one.
  existing="$(git ls-remote --heads origin "$RECONCILE_GLOB" 2>/dev/null | sed -E 's#.*refs/heads/##' | sort | tail -1)"
  branch="${existing:-chore/queue-reconcile-$(date +%Y%m%d)}"
  [[ -n "$existing" ]] && git fetch -q origin "${branch}:refs/remotes/origin/${branch}" 2>/dev/null

  # Worktree isolation is REQUIRED. If it cannot be created, bail safely -- NEVER
  # fall back to a reset/restore on the primary tree.
  wt="$(mktemp -d 2>/dev/null)" || { echo "queue-reconcile: mktemp failed -- skipped." >&2; return 0; }
  if ! git worktree add -q -B "$branch" "$wt" origin/main 2>/dev/null; then
    echo "queue-reconcile: git worktree unavailable -- skipped (primary tree untouched)." >&2
    rm -rf "$wt"
    return 0
  fi

  # Apply the VERIFIED reconcile inside the worktree only (status/pr_url writes).
  ( cd "$wt" && reconcile --apply >/dev/null 2>&1 )

  # Stage ONLY queue markdown. NEVER `git add -A`. The worktree is a clean origin/main
  # checkout, so the dir holds only queue .md and the reconcile's own edits -- adding
  # the path stages exactly those writes and nothing else.
  git -C "$wt" add -- "$QUEUE_DIR" 2>/dev/null

  if git -C "$wt" diff --cached --quiet; then
    echo "queue-reconcile: no stale items to write -- queue is current."
    git worktree remove --force "$wt" 2>/dev/null || rm -rf "$wt"
    return 0
  fi

  # Dedup: if an existing reconcile branch already carries these exact queue updates,
  # report it instead of force-pushing an identical-content commit (no duplicate work).
  if [[ -n "$existing" ]] && git -C "$wt" diff --quiet "refs/remotes/origin/${branch}" -- "$QUEUE_DIR"; then
    echo "queue-reconcile: ${branch} already reflects the merged state -- nothing new to push (no duplicate PR)."
    git worktree remove --force "$wt" 2>/dev/null || rm -rf "$wt"
    return 0
  fi

  git -C "$wt" commit -q -m "$COMMIT_MSG"
  local -a push_args=(-q -u origin "$branch")
  [[ -n "$existing" ]] && push_args=(-q --force-with-lease -u origin "$branch")
  if git -C "$wt" push "${push_args[@]}" 2>/dev/null; then
    echo "queue-reconcile: pushed ${branch} (queue metadata only; NOT merged)."
    [[ -n "$slug" ]] && echo "  PR/compare: https://github.com/${slug}/compare/main...${branch}?expand=1"
  else
    echo "queue-reconcile: push failed -- left for later (primary tree untouched)." >&2
  fi

  git worktree remove --force "$wt" 2>/dev/null || rm -rf "$wt"
  return 0
}

main() {
  local mode="${1:---check}"
  local n
  n="$(stale_count)"
  n="${n:-0}"

  if [[ "$n" -eq 0 ]]; then
    echo "queue-reconcile: no stale merged items -- queue is current."
    return 0
  fi
  echo "queue-reconcile: ${n} merged queue item(s) need status/pr_url updates."

  case "$mode" in
    --push) push_isolated ;;
    *)
      reconcile                                  # full read-only report
      echo "queue-reconcile: dry-run only -- run with --push to isolate + push." ;;
  esac
  return 0
}

main "$@"
