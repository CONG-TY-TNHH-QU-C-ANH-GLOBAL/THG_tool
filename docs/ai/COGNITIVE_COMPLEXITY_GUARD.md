---
doc_type: ai
status: active
owner: platform
related_pr_or_issue: chore/go-cognitive-guard
---

# Go Cognitive-Complexity Guard (local S3776 approximation)

`scripts/go_cognitive_check.sh` fails local validation when a Go function in a
**changed** file has cognitive complexity above **15**. It is wired into
`scripts/ai_validate.sh` and the `/thg-next` push gate so a PR is caught before
it reaches Sonar, not after.

## Why this exists

Sonar rule **S3776** measures *cognitive* complexity. Architecture/refactor PRs
repeatedly tripped it even when they changed no behavior:

- **PR129** moved brain validators/actions into new files.
- **PR131** moved `inferBusinessCalibrationFromPrompt` into a new file and added
  a new `_test.go`.

The reason: **a function relocated into a new file counts as New Code to Sonar.**
A function that was already over the threshold is flagged on the move alone, so a
move-only split is not enough — the moved function must be reduced in the same PR.
**`_test.go` files are New Code too**: a characterization test added to support a
refactor must itself be S3776-clean (extract assertion helpers / split focused
tests rather than nesting loops and conditionals).

## What it checks

- Only Go files **changed vs `origin/main`** — tracked changes (added / copied /
  modified / renamed, committed or not) **and** untracked new files. It does
  **not** scan the whole repo, so unrelated historical debt in untouched files
  never fails the build.
- Includes `_test.go`.
- Threshold: complexity **> 15** (matches the S3776 default trigger).

## The tool

It uses [`gocognit`](https://github.com/uudashr/gocognit), run via
`go run github.com/uudashr/gocognit/cmd/gocognit@v1.1.3` — a **pinned** version
(never `@latest`). No `go get`, no `go install`, no `go.mod`/`go.sum` edits, no
reliance on `$GOPATH/bin`.

`gocognit` (cognitive complexity), **not** `gocyclo` (cyclomatic complexity): the
Sonar issue is cognitive complexity, which penalizes nesting and mixed boolean
operators the way S3776 does. Cyclomatic complexity would measure a different
thing and miss/over-report the cases that actually fail Sonar.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Clean, or no Go files changed (skipped). |
| 1 | A changed function is over the threshold — listing printed to stderr as `<complexity> <package> <function> <file>:<line>:<col>`. |
| 2 | The tool could not run (e.g. no network to fetch the pinned module on first use). It fails loudly with an actionable message — it never passes silently when it cannot check. The module caches after one successful online run. |

## Fixing a failure

Reduce the reported function by **pure helper extraction** or a **flat-dispatch
switch** — and verify each extracted helper is itself under the threshold. Do
**not** move complexity from the original function into a new helper. This is a
local approximation; the authoritative check is still the Sonar New-Code scan on
the PR. Do not suppress Sonar or change Sonar config to dodge it.
