---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-10-21
related_pr_or_issue: Add Taobao sourcing to replies
---

# Historical lead suggestion diagnostic

Component: release/operations diagnostics, owned by the existing
`lead-suggestion-diagnostic.yml` workflow. No service API or customer-facing
contract changes. Facebook services and Telegram rendering do not own this probe.

The old 20-minute/unfiltered-last-2000-lines check cannot explain a notice sent
earlier today. The workflow now accepts `lookback_minutes` (20, 1440 or 2880), an
optional numeric `post_fbid`, and an optional full `expected_revision` SHA.

The Python probe checks effective context/image flags and scope, looks for the
audit guard and requested VCS revision in the running executable, selects only
org-scoped lead IDs through read-only SQLite, and correlates those IDs with
fixed journal result codes. It never selects lead content or returns IDs, URLs,
authors, secrets, raw journals or executable bytes. It performs no restart,
configuration update, data mutation, source search or Telegram delivery.

History is filtered for `[LeadSuggestion]` before the 20,000-line cap. A reached
cap is reported. Missing/rotated journal history is not evidence that processing
never occurred. A missing SQLite match can mean another store/instance; this probe
does not automatically connect to PostgreSQL or discover other hosts. A missing
VCS string is reported as `not_confirmed`, not as proof of an old binary.

The public Facebook post ID is passed as an input; no customer sample is baked
into the workflow. Raw errors are not printed by the historical probe.

Validation:

```text
python -B -m unittest discover -s scripts/tests -p test_lead_suggestion_diagnostic.py -v
```

Tests execute the actual inline Python without its main routine, verify invalid
inputs stop before commands, tenant-isolated selection, unchanged database bytes,
no creation of a missing DB and absence of raw log values from summaries.

This is diagnostic tooling only. Run it from an explicitly selected ref; source
changes or a successful diagnostic do not establish production reply quality.
