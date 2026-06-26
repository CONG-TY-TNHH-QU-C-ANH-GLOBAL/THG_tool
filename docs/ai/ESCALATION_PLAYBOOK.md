# ESCALATION_PLAYBOOK

Use this when a normal AUTOPILOT_QUEUE task hits RED ambiguity, hard Sonar, architecture uncertainty, or missing test seams.

## Core rule

Do not ask for a new prompt by default. Classify, choose the safest bounded option, document the decision, validate, and push. Stop only when human/business/credential/destructive decisions are required.

## Escalation classes

### E1 — Hard Sonar issue

Examples:
- cognitive complexity in production code
- duplication requiring test helper extraction
- shell rule touching guard scripts
- S107 in tests

Protocol:
1. Confirm the issue is in New Code or touched files.
2. Fix only the issue scope.
3. Apply Ponytail:
   - no broad rewrite
   - no suppressions
   - no Sonar config change
   - no unrelated cleanup
4. If production logic is touched, add/confirm characterization tests.
5. Validate and report.

Allowed:
- private helper extraction
- table-driven test conversion
- small guard-clause refactor
- tiny shell cleanup

Forbidden:
- changing behavior to satisfy Sonar
- suppressions
- config changes
- chasing unrelated backlog

### E2 — RED ambiguity

Examples:
- queue/outbox write
- action_ledger / execution_attempts
- connector CAS/lease
- crawler/jobhandler runtime
- auth/session/cookie
- schema/migration
- DTO/wire contract

Protocol:
1. Classify exact controlled zone.
2. Prefer characterization tests over production changes.
3. If behavior is already covered, report “already covered”.
4. If missing coverage is practical, add tests only.
5. If production fix is needed, only do it if it is a tiny obvious bug fix and does not change semantics.
6. Otherwise stop with a minimal decision record.

### E3 — Architecture boundary decision

Examples:
- whether to move code into `internal/services/facebook`
- whether a package is shared leaf vs service-owned
- whether to introduce a port
- reverse dependency uncertainty

Protocol:
1. Run import/reverse-import audit.
2. Identify current import graph.
3. Choose based on ownership:
   - pure shared leaf stays neutral
   - service-specific logic may move to service
   - DB/store reads require narrow consumer-owned port
   - runtime orchestration stays composition root
4. Prefer no move if ownership is ambiguous.
5. Add/extend boundary guard if the mistake can recur.
6. Document decision in final report.

### E4 — Missing fake/test seam

Examples:
- browser/CDP/session requires real Chrome
- crawler runtime not fakeable
- DB fixture too large
- external I/O required for unit tests

Protocol:
1. Search existing fakes/helpers first.
2. If seam exists, write tests using it.
3. If seam is missing, do not invent a framework.
4. If a tiny seam is obvious and local, implement it.
5. If seam requires broad production signature changes, stop and propose the smallest next PR.

### E5 — Conflicting behavior / product ambiguity

Protocol:
1. Do not guess product behavior.
2. Preserve existing behavior.
3. Add characterization tests if useful.
4. Stop and ask for product decision only if two valid product outcomes exist.

## Decision record format

Every hard-case PR report must include:

```text
Escalation:
- class: E1/E2/E3/E4/E5
- trigger:
- options considered:
- decision:
- why safe:
- files touched:
- validation:
- remaining risk:
```

## Autonomy rules

Claude may autonomously implement when:

* change is bounded,
* diff is reviewable,
* no forbidden semantics change,
* tests/guards pass,
* decision record is included.

Claude must stop when:

* destructive operation is needed,
* secret/access/credential is needed,
* production schema or wire contract changes,
* auth/security policy changes,
* queue/lease/ledger semantics would change,
* business/product behavior is ambiguous.
