# Spec Governance

This document defines how specs in `specs/` are tracked, classified, and kept
trustworthy over time. It is the rulebook behind `specs/SPEC_REGISTRY.json` and
`scripts/check_spec_registry.py`.

## 1. Purpose

- Specs are **project memory and behavior contracts**, not throwaway notes.
- Without governance, specs rot into stale Markdown that nobody trusts and
  nobody can safely delete.
- Every spec therefore needs a **status**, an **owner/domain**, a **verification
  record**, and a place in a **lifecycle**.

This system exists so future PRs can review, import, and audit specs **one at a
time** instead of in a single unreliable mass audit.

## 2. Spec lifecycle statuses

```
active
draft
unreviewed
stale
deprecated
superseded
archived
```

- `active` — currently trusted; used to guide implementation/refactor.
- `draft` — useful but incomplete; not yet binding.
- `unreviewed` — existing document not yet audited under this governance system.
- `stale` — likely outdated or partially invalid; do not use as source of truth
  without review.
- `deprecated` — intentionally no longer used, kept for history.
- `superseded` — replaced by another spec (see `superseded_by`).
- `archived` — historical/report-only; not part of active architecture.

**Evidence rule:** only mark a spec `active`, `stale`, `deprecated`,
`superseded`, or `archived` when there is evidence. Default existing,
un-audited documents to `unreviewed`.

## 3. Spec types

```
behavior
architecture
contract
migration
investigation
runbook
report
roadmap
policy
unknown
```

Use `unknown` when the type is not obvious. Filename-based guesses are allowed
but must not be treated as confirmed classification.

## 4. Maturity levels

```
snapshot
reviewed
test_pinned
implementation_backed
superseded
unknown
```

- `snapshot` — describes current code at a point in time.
- `reviewed` — human-reviewed and considered accurate.
- `test_pinned` — backed by characterization/unit tests.
- `implementation_backed` — directly reflected in production code/design.
- `superseded` — replaced by another document.
- `unknown` — not reviewed yet.

## 5. Required registry fields

Every entry in `specs/SPEC_REGISTRY.json` must provide all of these fields:

```json
{
  "id": "lead-ingestion-behavior",
  "path": "specs/leadingest/lead_ingestion_behavior.md",
  "title": "Lead Ingestion Behavior Specification",
  "domain": "leadingest",
  "type": "behavior",
  "status": "active",
  "maturity": "test_pinned",
  "owner": "unassigned",
  "last_reviewed": "YYYY-MM-DD or null",
  "verified_against": ["code/tests/PR references"],
  "related_code": ["paths"],
  "related_tests": ["paths"],
  "supersedes": [],
  "superseded_by": null,
  "tags": [],
  "notes": "short note"
}
```

## 6. Review rules

- Do **not** delete specs without owner approval.
- Do **not** mark a spec `deprecated` without evidence.
- `active` specs must list related code/tests.
- `behavior` specs should be `test_pinned` whenever possible.
- `stale` specs must explain in `notes` why they are stale.
- `superseded` specs must point to their replacement via `superseded_by`.
- Future spec changes must update the registry in the **same PR**.

## 7. PR workflow

- Adding a new spec **requires** a registry entry.
- Modifying a spec **requires** a registry update when its status, maturity, or
  domain changes.
- A behavior spec used to drive a refactor should have tests/harness backing it.
- PRs should mention which registry entries they affect.

## 8. Future work

- PR24B+ will audit specs in **small batches**, not all at once.
- Existing `unreviewed` specs will be classified gradually as they are touched.
- Optional CI wiring for `scripts/check_spec_registry.py` can be added later,
  once the registry has stabilized. PR24A does not add CI.
