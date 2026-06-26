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
#   scripts/ai_queue_reconcile.sh --check    # report-only (dry run, explicit)
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

# MERGED is the "verified merged" verdict token shared by the verify_* helpers and
# the reconcile loop. Defined once (Sonar: no duplicated string literal).
readonly MERGED="merged"

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

# api_get URL -> body on stdout; PRESERVES curl's exit status so callers can detect
# network failure via `api_get ... || ...`. (bare `return` returns curl's $?.)
api_get() {
  local url="$1"
  curl -fsS -m 20 "${curl_hdr[@]}" "$url" 2>/dev/null
  return
}

# field FILE KEY -> first frontmatter value for KEY (trimmed, quotes stripped).
field() {
  local file="$1" key="$2"
  sed -n "s/^${key}:[[:space:]]*//p" "$file" | head -1 | tr -d '"' | sed 's/[[:space:]]*$//'
  return 0
}

# json_first_value JSON KEY -> first value for "KEY": ... (string body or null), trimmed.
json_first_value() {
  local json="$1" key="$2"
  printf '%s' "$json" | grep -m1 "\"${key}\"" | sed -E "s/.*\"${key}\"[[:space:]]*:[[:space:]]*//; s/,?[[:space:]]*$//"
  return 0
}

# verify_by_number N -> echoes "$MERGED" | "closed_unmerged" | "open" | "error"
verify_by_number() {
  local n="$1" body merged_at state
  body="$(api_get "${api}/pulls/${n}")" || { echo error; return 0; }
  [[ -z "$body" ]] && { echo error; return 0; }
  merged_at="$(json_first_value "$body" merged_at)"
  state="$(json_first_value "$body" state | tr -d '"')"
  if [[ -n "$merged_at" && "$merged_at" != "null" ]]; then echo "$MERGED"; return 0; fi
  [[ "$state" == "closed" ]] && { echo "closed_unmerged"; return 0; }
  echo "open"
  return 0
}

# verify_by_branch BRANCH -> echoes "$MERGED <num>" | "closed_unmerged" | "none" | "error"
verify_by_branch() {
  local br="$1" body merged_at num
  body="$(api_get "${api}/pulls?state=closed&head=${owner}:${br}&per_page=20")" || { echo error; return 0; }
  [[ -z "$body" || "$body" == "[]" ]] && { echo none; return 0; }
  merged_at="$(json_first_value "$body" merged_at)"
  num="$(json_first_value "$body" number)"
  if [[ -n "$merged_at" && "$merged_at" != "null" ]]; then echo "$MERGED ${num}"; return 0; fi
  echo "closed_unmerged"
  return 0
}

# git fallback: positive-only. Echoes "$MERGED" iff the remote branch tip is an
# ancestor of origin/main (merge-commit / rebase merge). Squash merges will NOT
# match here — that is the safe direction (no false DONE).
verify_by_git() {
  local br="$1"
  git rev-parse --verify --quiet "origin/${br}" >/dev/null 2>&1 || { echo "unknown"; return 0; }
  if git merge-base --is-ancestor "origin/${br}" origin/main 2>/dev/null; then
    echo "$MERGED"
  else
    echo "ambiguous" # could be squash-merged or genuinely unmerged — cannot tell
  fi
  return 0
}

# set_frontmatter FILE KEY VALUE -> rewrite an existing KEY: line in place.
set_frontmatter() {
  local file="$1" key="$2" val="$3"
  sed -i -E "s#^(${key}:[[:space:]]*).*#\1${val}#" "$file"
  return 0
}

echo "== queue reconcile ($([[ $apply -eq 1 ]] && echo apply || echo dry-run)) =="
echo "repo: ${slug:-<unknown>}"
[[ -z "$slug" ]] && echo "WARN no github remote — cannot verify; leaving all REVIEW items untouched" >&2

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
      "$MERGED") verdict="$MERGED" ;;
      closed_unmerged) left_list+=("$id — PR #$prnum closed UNMERGED (needs attention)"); continue ;;
      open) left_list+=("$id — PR #$prnum still open"); continue ;;
      *) verdict="" ;; # error -> fall through to next method
    esac
  fi

  # Method 2: branch -> closed merged PR.
  if [[ -z "$verdict" && -n "$branch" && -n "$slug" ]]; then
    res="$(verify_by_branch "$branch")"
    case "$res" in
      "$MERGED "*) verdict="$MERGED"; prnum="${res#"$MERGED" }" ;;
      closed_unmerged) left_list+=("$id — branch '$branch' has a closed UNMERGED PR"); continue ;;
      none) : ;; # no closed PR found yet -> try git fallback
      *) : ;;    # error -> try git fallback
    esac
  fi

  # Method 3: positive-only git fallback (merge-commit / rebase only).
  if [[ -z "$verdict" && -n "$branch" ]]; then
    case "$(verify_by_git "$branch")" in
      "$MERGED") verdict="${MERGED}-git" ;;
      ambiguous) left_list+=("$id — branch '$branch' tip not an ancestor of main (squash-merge unverifiable via git; set pr_url or check the PR)"); continue ;;
      unknown) left_list+=("$id — no pr_url, origin/$branch absent, API gave nothing (cannot verify)"); continue ;;
      *) : ;; # unexpected token -> leave unverified below
    esac
  fi

  if [[ "$verdict" == "$MERGED"* ]]; then
    note="PR #${prnum:-?}"
    [[ "$verdict" == "${MERGED}-git" ]] && note="git-ancestor (merge/rebase)"
    if [[ $apply -eq 1 ]]; then
      set_frontmatter "$f" status DONE
      if [[ -n "$prnum" && -z "$pr_url" ]]; then
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
