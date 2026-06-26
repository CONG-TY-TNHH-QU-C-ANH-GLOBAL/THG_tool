#!/usr/bin/env bash
# go_cognitive_check.sh — local approximation of Sonar rule S3776 (cognitive
# complexity) for the Go files CHANGED on this branch.
#
# WHY: architecture/refactor PRs often MOVE a function into a new file. To Sonar
# a relocated function counts as NEW CODE, so a function that was already over the
# complexity threshold gets flagged even though the move changed no behavior
# (PR129 moved brain validators; PR131 moved inferBusinessCalibrationFromPrompt and
# added a new _test.go). Move-only is not enough. This guard surfaces that BEFORE
# push instead of after the Sonar scan.
#
# SCOPE: only Go files changed vs origin/main (incl. _test.go and newly added or
# moved files). It deliberately does NOT scan the whole repo, so unrelated historical
# debt in untouched files never fails the build.
#
# TOOL: github.com/uudashr/gocognit, run via `go run ...@<pinned>`. No `go get`, no
# `go install`, no go.mod/go.sum edits, no reliance on $GOPATH/bin. gocognit measures
# COGNITIVE complexity (matches S3776) — NOT gocyclo, which is cyclomatic.
#
# EXIT: 0 = clean (or no Go files changed). 1 = a changed function is over the
# threshold (listing printed to stderr). 2 = the tool could not run, with an
# actionable message — this guard never passes silently when it cannot check.
#
# Not `set -e`: we inspect the tool's exit code ourselves to tell "violations" apart
# from "tool failed to run".
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Sonar S3776 default threshold. gocognit -over N reports complexity strictly > N,
# i.e. >= N+1, which matches the Sonar issue trigger.
readonly THRESHOLD=15
# Pinned tool version (NOT @latest, for reproducibility). Bump deliberately.
readonly GOCOGNIT="github.com/uudashr/gocognit/cmd/gocognit@v1.1.3"
readonly BASE_REF="origin/main"
readonly HEADER="== go cognitive-complexity guard (gocognit > ${THRESHOLD}) =="

# changed_go_files -> existing changed .go paths (one per line), or nothing.
# Union of three sources so new files are caught however far the PR has progressed:
#   - tracked changes vs base (ACMR = added/copied/modified/renamed, committed or not),
#   - untracked new files (validation may run before the commit, so new .go files are
#     still untracked at that point).
# Vanished/deleted files are filtered out; the list is de-duplicated.
changed_go_files() {
  local base
  base="$(git merge-base "$BASE_REF" HEAD 2>/dev/null)" || base="$BASE_REF"
  [[ -z "$base" ]] && base="$BASE_REF"
  {
    git diff --name-only --diff-filter=ACMR "$base" -- '*.go' 2>/dev/null
    git ls-files --others --exclude-standard -- '*.go' 2>/dev/null
  } | sort -u | while IFS= read -r path; do
        [[ -f "$path" ]] && printf '%s\n' "$path"
      done
  return 0
}

# report_violations OUT -> print the gocognit listing to stderr with guidance.
report_violations() {
  local out="$1" line
  echo "FAIL: cognitive complexity over ${THRESHOLD} in changed Go files:" >&2
  printf '%s\n' "$out" | while IFS= read -r line; do
    [[ -n "$line" ]] && printf '  %s\n' "$line" >&2
  done
  echo >&2
  echo "Format: <complexity> <package> <function> <file>:<line>:<col>" >&2
  echo "Reduce via pure helper extraction / flat-dispatch switch (no complexity moved" >&2
  echo "into a helper). See docs/ai/COGNITIVE_COMPLEXITY_GUARD.md." >&2
  return 0
}

# report_tool_failure STATUS ERRFILE -> actionable message; never a silent pass.
report_tool_failure() {
  local status="$1" err_file="$2"
  echo "ERROR: gocognit could not run (exit ${status}); failing loudly, not passing." >&2
  echo "Tool: go run ${GOCOGNIT}" >&2
  echo "Likely cause: no network to fetch the pinned module on first run, or a Go build error." >&2
  echo "Fix: run the guard once with network so the module caches, then re-run." >&2
  if [[ -s "$err_file" ]]; then
    echo "--- tool stderr ---" >&2
    cat "$err_file" >&2
  fi
  return 0
}

main() {
  local files
  mapfile -t files < <(changed_go_files)

  echo "$HEADER"
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "no changed Go files vs ${BASE_REF} — skipped"
    return 0
  fi
  echo "checking ${#files[@]} changed Go file(s) vs ${BASE_REF}"

  local err_file out status
  err_file="$(mktemp)"
  out="$(go run "$GOCOGNIT" -over "$THRESHOLD" "${files[@]}" 2>"$err_file")"
  status=$?

  # gocognit prints over-threshold functions to STDOUT (exit 1); a tool/build/fetch
  # failure prints to STDERR with empty STDOUT. So non-empty stdout == real violations.
  if [[ -n "$out" ]]; then
    report_violations "$out"
    rm -f "$err_file"
    return 1
  fi
  if [[ $status -ne 0 ]]; then
    report_tool_failure "$status" "$err_file"
    rm -f "$err_file"
    return 2
  fi

  rm -f "$err_file"
  echo "OK: all changed Go functions are <= ${THRESHOLD}"
  return 0
}

main "$@"
