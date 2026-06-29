# PR Checklist

Tick every item before opening a PR. These mirror the **Engineering Guardrails**
in `CLAUDE.md` — read that section first.

- [ ] No new production file over 200 lines.
- [ ] No large legacy file was made worse (prefer extracting a small module).
- [ ] No duplicated logic introduced (extract a shared helper/service/policy).
- [ ] Business logic is not mixed with UI / transport / storage.
- [ ] Platform-specific logic is not placed in generic core.
- [ ] Typed reason codes are centralized.
- [ ] New behavior has tests.
- [ ] Refactor-only PR has no behavior change.
- [ ] Build / test / vet passed (`python scripts/check_file_size.py`, `go test ./...`, `go vet ./...`, `npm --prefix frontend run build`).
- [ ] Completion report lists file sizes and exceptions.

## Completion report (required)

1. Files changed.
2. Which files (if any) exceed 200 lines.
3. Which large legacy file(s) were touched, and why the edit could not be extracted now.
4. What logic was extracted or reused.
5. Any intentional exception used (and why).
6. Which build/test/vet checks were run (and which were skipped + why).
7. Whether the PR is behavior-changing or refactor-only.
