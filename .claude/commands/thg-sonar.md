Execute the THG `/thg-sonar` workflow defined in `CLAUDE.md` (Custom Workflow Commands).

If a Sonar export JSON is provided, run `scripts/sonar_triage_from_export.py <export.json>`.
Work only on true OPEN issues.
Prefer GREEN mechanical cleanup; YELLOW S3776/S107 via pure extraction + direct tests (≤3–5 per PR).
Use Ponytail discipline. Never touch RED zones without approval (`/thg-red-audit`).

Use `scripts/ai_preflight.sh` then `scripts/ai_validate.sh`.
Push one bounded PR. Do not merge.
