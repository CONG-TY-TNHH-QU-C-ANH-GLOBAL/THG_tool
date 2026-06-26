#!/usr/bin/env bash
# ai_queue_reconcile.sh — reconcile queue items stuck in REVIEW to DONE when their
# PR is VERIFIABLY merged.
#
# SQUASH-MERGE SAFE: verification is driven by GitHub's PR `merged_at` field, NOT
# by branch ancestry. GitHub squash/rebase merges rewrite commits, so
# `git branch --merged` / `git merge-base --is-ancestor` would report the original
# branch tip as NOT merged even though the PR is merged. Those git checks are used
# only as a positive-only secondary fallback (they can confirm a merge-commit/rebase
# merge, never disprove a squash merge), and never mark DONE on their own ambiguity.
#
# Verification order, per REVIEW item:
#   1. pr_url present  -> GET /pulls/<n>                       -> DONE iff merged_at != null
#   2. else branch set -> GET /pulls?state=closed&head=O:branch -> DONE iff a merged_at != null
#   3. else git is-ancestor(origin/<branch>, origin/main)       -> DONE only if TRUE (merge/rebase)
#   4. otherwise: leave REVIEW and report why. NEVER mark DONE by assumption.
#
# Usage:
#   scripts/ai_queue_reconcile.sh           # report-only (dry run)
#   scripts/ai_queue_reconcile.sh --apply   # write status: REVIEW -> DONE (+ backfill pr_url)
#
# Network/auth: works unauthenticated against public repos (rate-limited). Honors
# GITHUB_TOKEN / GH_TOKEN if set. Network failure is non-fatal: the item is simply
# left REVIEW and reported. The script always exits 0 unless misused.
#
# NOTE: intentionally not `set -e` — a failed curl must not abort the whole sweep.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

items_dir="docs/ai/queue/items"
apply=0
[[ "${1:-}" == "--apply" ]] && apply=1

# Derive owner/repo from the origin remote (https or ssh form). POSIX sed has no
# lazy quantifier, so strip the prefix and the optional .git suffix in steps.
remote="$(git remote get-url origin 2>/dev/null || true)"
slug="$(printf '%s' "$remote" | sed -E 's#^.*github\.com[:/]+##; s#\.git$##; s#/+$##')"
owner="${slug%%/*}"
api="https://api.github.com/repos/${slug}"

curl_hdr=(-H "Accept: application/vnd.github+json")
token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
[[ -n "$token" ]] && curl_hdr+=(-H "Authorization: Bearer ${token}")

api_get() { curl -fsS -m 20 "${curl_hdr[@]}" "$1" 2>/dev/null; }

field() { sed -n "s/^$2:[[:space:]]*//p" "$1" | head -1 | tr -d '"' | sed 's/[[:space:]]*$//'; }

# json_first_value JSON KEY -> first value for "KEY": ... (string body or null), trimmed.
json_first_value() {
  printf '%s' "$1" | grep -m1 "\"$2\"" | sed -E "s/.*\"$2\"[[:space:]]*:[[:space:]]*//; s/,?[[:space:]]*$//"
}

# verify_by_number N -> echoes "merged" | "closed_unmerged" | "open" | "error"
verify_by_number() {
  local n="$1" body merged state
  body="$(api_get "${api}/pulls/${n}")" || { echo error; return; }
  [[ -z "$body" ]] && { echo error; return; }
  merged="$(json_first_value "$body" merged_at)"
  state="$(json_first_value "$body" state | tr -d '"')"
  if [[ -n "$merged" && "$merged" != "null" ]]; then echo "merged"; return; fi
  [[ "$state" == "closed" ]] && { echo "closed_unmerged"; return; }
  echo "open"
}

# verify_by_branch BRANCH -> echoes "merged <num>" | "closed_unmerged" | "none" | "error"
verify_by_branch() {
  local br="$1" body merged num
  body="$(api_get "${api}/pulls?state=closed&head=${owner}:${br}&per_page=20")" || { echo error; return; }
  [[ -z "$body" || "$body" == "[]" ]] && { echo none; return; }
  merged="$(json_first_value "$body" merged_at)"
  num="$(json_first_value "$body" number)"
  if [[ -n "$merged" && "$merged" != "null" ]]; then echo "merged ${num}"; return; fi
  echo "closed_unmerged"
}

# git fallback: positive-only. Echoes "merged" iff the remote branch tip is an
# ancestor of origin/main (merge-commit / rebase merge). Squash merges will NOT
# match here — that is the safe direction (no false DONE).
verify_by_git() {
  local br="$1"
  git rev-parse --verify --quiet "origin/${br}" >/dev/null 2>&1 || { echo "unknown"; return; }
  if git merge-base --is-ancestor "origin/${br}" origin/main 2>/dev/null; then
    echo "merged"
  else
    echo "ambiguous" # could be squash-merged or genuinely unmerged — cannot tell
  fi
}

set_frontmatter() { # set_frontmatter FILE KEY VALUE  (only if KEY exists)
  local file="$1" key="$2" val="$3"
  sed -i -E "s#^(${key}:[[:space:]]*).*#\1${val}#" "$file"
}

echo "== queue reconcile ($([[ $apply -eq 1 ]] && echo apply || echo dry-run)) =="
echo "repo: ${slug:-<unknown>}"
[[ -z "$slug" ]] && { echo "WARN no github remote — cannot verify; leaving all REVIEW items untouched"; }

mapfile -t items < <(find "$items_dir" -type f -name '*.md' | sort)
done_list=(); left_list=()

for f in "${items[@]}"; do
  [[ "$(field "$f" status)" == "REVIEW" ]] || continue
  id="$(field "$f" id)"
  branch="$(field "$f" branch)"
  pr_url="$(field "$f" pr_url)"
  verdict=""; prnum=""

  # Method 1: explicit pr_url.
  if [[ -n "$pr_url" && "$pr_url" =~ /pull/([0-9]+) ]]; then
    prnum="${BASH_REMATCH[1]}"
    case "$(verify_by_number "$prnum")" in
      merged) verdict="merged" ;;
      closed_unmerged) left_list+=("$id — PR #$prnum closed UNMERGED (needs attention)"); continue ;;
      open) left_list+=("$id — PR #$prnum still open"); continue ;;
      *) verdict="" ;; # error -> fall through to next method
    esac
  fi

  # Method 2: branch -> closed merged PR.
  if [[ -z "$verdict" && -n "$branch" && -n "$slug" ]]; then
    res="$(verify_by_branch "$branch")"
    case "$res" in
      merged\ *) verdict="merged"; prnum="${res#merged }" ;;
      closed_unmerged) left_list+=("$id — branch '$branch' has a closed UNMERGED PR"); continue ;;
      none) : ;; # no closed PR found yet -> try git fallback
      *) : ;;    # error -> try git fallback
    esac
  fi

  # Method 3: positive-only git fallback (merge-commit / rebase only).
  if [[ -z "$verdict" && -n "$branch" ]]; then
    case "$(verify_by_git "$branch")" in
      merged) verdict="merged-git" ;;
      ambiguous) left_list+=("$id — branch '$branch' tip not an ancestor of main (squash-merge unverifiable via git; set pr_url or check the PR)"); continue ;;
      unknown) left_list+=("$id — no pr_url, origin/$branch absent, API gave nothing (cannot verify)"); continue ;;
    esac
  fi

  if [[ "$verdict" == merged* ]]; then
    note="PR #${prnum:-?}"
    [[ "$verdict" == "merged-git" ]] && note="git-ancestor (merge/rebase)"
    if [[ $apply -eq 1 ]]; then
      set_frontmatter "$f" status DONE
      if [[ -n "$prnum" && ( -z "$pr_url" ) ]]; then
        set_frontmatter "$f" pr_url "https://github.com/${slug}/pull/${prnum}"
      fi
    fi
    done_list+=("$id — DONE ($note)")
  else
    left_list+=("$id — unverified, left REVIEW")
  fi
done

echo
echo "reconciled to DONE: ${#done_list[@]}"
for d in "${done_list[@]}"; do echo "  + $d"; done
echo "left REVIEW: ${#left_list[@]}"
for l in "${left_list[@]}"; do echo "  - $l"; done
[[ $apply -eq 0 && ${#done_list[@]} -gt 0 ]] && echo "(dry run — re-run with --apply to write these changes)"
exit 0
