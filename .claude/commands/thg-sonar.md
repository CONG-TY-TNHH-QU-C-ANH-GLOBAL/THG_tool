Execute the THG `/thg-sonar` workflow defined in `CLAUDE.md` (Custom Workflow Commands).

If a Sonar export JSON is provided, run `scripts/sonar_triage_from_export.py <export.json>`.
Work only on true OPEN issues.
Prefer GREEN mechanical cleanup; YELLOW S3776/S107 via pure extraction + direct tests (≤3–5 per PR).
S3776: extracted helpers must not become new S3776 — do not move complexity from the original function into a new helper; verify each new helper is itself under threshold.
S3776 (move-only splits): a function relocated to a new file counts as New Code, so any moved function already over the S3776 threshold gets flagged even with no behavior change — reduce it in the same PR; move-only is not enough.
Shell scripts under scripts/ are New Code too: local vars for positional params (no bare $1/$2), explicit returns (preserve exit status callers rely on), errors to stderr (>&2), constants for repeated literals, default *) case branches.
Use Ponytail discipline. Never touch RED zones without approval (`/thg-red-audit`).

Use `scripts/ai_preflight.sh` then `scripts/ai_validate.sh`.
Push one bounded PR. Do not merge.
