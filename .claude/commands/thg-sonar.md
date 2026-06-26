Execute the THG `/thg-sonar` workflow defined in `CLAUDE.md` (Custom Workflow Commands).

If a Sonar export JSON is provided, run `scripts/sonar_triage_from_export.py <export.json>`.
Work only on true OPEN issues.
Prefer GREEN mechanical cleanup; YELLOW S3776/S107 via pure extraction + direct tests (≤3–5 per PR).
S3776: extracted helpers must not become new S3776 — do not move complexity from the original function into a new helper; verify each new helper is itself under threshold.
Use Ponytail discipline. Never touch RED zones without approval (`/thg-red-audit`).

Use `scripts/ai_preflight.sh` then `scripts/ai_validate.sh`.
Push one bounded PR. Do not merge.
