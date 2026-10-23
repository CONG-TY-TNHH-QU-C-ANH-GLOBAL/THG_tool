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

The probe also counts fixed `analysis_error` categories and the focused
`grounded_product_fulfillment` fallback. These counters distinguish provider
input/auth/rate errors from timeout/transport, bad response shape and invalid
context evidence without printing a provider body. An input rejection does not
alone prove that the model lacks vision; expired media, unsupported formats or
other invalid parameters can produce the same category. Older binaries/history
without this field cannot identify those causes retroactively.

Validation:

```text
python -B -m unittest discover -s scripts/tests -p test_lead_suggestion_diagnostic.py -v
```

Tests execute the actual inline Python without its main routine, verify invalid
inputs stop before commands, tenant-isolated selection, unchanged database bytes,
no creation of a missing DB and absence of raw log values from summaries.

This is diagnostic tooling only. Run it from an explicitly selected ref; source
changes or a successful diagnostic do not establish production reply quality.

## 2026-10-21 incident and correction

Check-only run `39536897217` on the diagnostic branch confirmed that both running
processes match deployed `58b6e606`, analysis is enabled for THGFulfill, and the
reported post has one persisted lead and one `runner_busy` outcome. It was not
an analyzed noncommercial verdict. Across its 24-hour window, 11 of 23 events
were busy; no history cap was reached. This corrects the earlier 20-minute
diagnostic, which could not explain the older notice.

The notifications component now has an opt-in bounded FIFO queue (32 pending,
60-second maximum wait), wired only for enabled analysis orgs in both roots.
The legacy constructor and outside-scope admission stay fail-fast. Provider
concurrency and active budgets are preserved. Expiry sends the safe operator
fallback once without starting the expired provider work. Expired callbacks
still occupy pending capacity until delivery returns. A busy/timeout fallback
always includes a review note, even for a discussion with no sales draft.

Tests: `go test ./internal/notifications ./internal/services/facebook -count=1
-timeout=120s`, including a 23-job burst, FIFO/active concurrency, scope isolation,
queue capacity, fresh active budget, hung-provider/expiry behavior, bounded hung
fallback delivery and discussion-note rendering. Existing panic/timeout tests
remain relevant. CI race detection is a release gate.

This queue is memory-only and does not guarantee delivery across a process
restart or sustained overload. Eligible accepted leads can consume more API
calls than the previously dropped jobs. No automatic old-lead replay, duplicate
Telegram notices, schema, provider credential or knowledge approval is added.
