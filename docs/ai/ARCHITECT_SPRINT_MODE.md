---
doc_type: ai
status: active
owner: platform
last_reviewed: 2026-06-28
related_pr_or_issue: docs/architect-sprint-mode
---

# THG Architect Sprint Mode

**Status:** OFFICIAL PROTOCOL (process). Mission-based architecture execution mode.

Invoked by `/thg-architect` (or `/thg-next architect-sprint`). This protocol
**replaces the overly-serial micro-refactor cadence** (one tiny PR per file) with
a senior-system-architect-led flow that picks higher-leverage slices and finishes
coherent work — while preserving every existing safety guard.

It does **not** invent new rules. It is a *mode* over the existing authorities and
defers to them:

- Lanes / feasibility / boundary targets → [`../architecture/BOUNDARY_MIGRATION_PLAYBOOK.md`](../architecture/BOUNDARY_MIGRATION_PLAYBOOK.md) (the authority).
- Hard cases / decision records → [`ESCALATION_PLAYBOOK.md`](ESCALATION_PLAYBOOK.md).
- Queue policy / lifecycle / lockless rule → [`AUTOPILOT_QUEUE.md`](AUTOPILOT_QUEUE.md); per-item state in `queue/items/**`.
- Report shape → [`AGENT_REPORT_TEMPLATE.md`](AGENT_REPORT_TEMPLATE.md) + the report format below.
- Validation → `scripts/ai_preflight.sh`, `scripts/ai_validate.sh`, `scripts/go_cognitive_check.sh`, `scripts/check_file_size.py`.
- Engineering guardrails / discipline → `CLAUDE.md`.

Where this protocol and an authority above appear to disagree, the authority wins.
This doc only changes *how aggressively* Claude batches and decides **inside** the
GREEN/YELLOW lanes — never what RED is or how it is handled.

## Operate as a senior architect

Stop behaving like a mechanical file-splitter. The mandate:

- choose higher-leverage architecture slices, not the next alphabetical file;
- combine safe GREEN batches into one coherent PR;
- create enabling seams (same-package helpers/builders/ports-as-prep);
- finish a whole staged item when the remaining steps are same-package and safe;
- stop / re-scope only when the boundary is **genuinely** unsafe;
- keep New Code Sonar clean.

Laziness still applies (Ponytail): the least code that finishes the slice. Bigger
*scope* of a coherent move, not more *abstraction*.

## 1. Mission-based execution, not micro-control

- The user gives the **architecture goal**; Claude chooses the safest
  high-leverage implementation slice to get there.
- Do **not** ask permission for every small engineering judgment (split vs leave,
  rename for SRP, add a same-package helper). Decide, proceed, and **explain the
  tradeoffs in the final report**.
- Permission is required only for the stop/escalate triggers in §4.

## 2. Boundary-first architecture

Run the feasibility gate **before any code** (Boundary Migration Playbook §3).
Every architect-sprint PR must state up front:

- **selected item(s)** (queue id),
- **lane** (GREEN / YELLOW / RED),
- **risk** (the item's `risk:`),
- **boundary_target** (`prep-extraction` / `leaf-move` / `transport-to-usecase` /
  `store-test-seam` / `blocked-decision`),
- **target boundary/layer** (which layer move it serves, per the Playbook §1 map),
- **feasibility-before-code result** (receiver/coupling, import-cycle,
  call-site + export count, transport-leakage, coverage — Playbook §3.1–3.6).

A file split is only worth doing as prep for a clean boundary; a boundary move is
only allowed once the seam is already clean.

## 3. Throughput rules

**GREEN (prep-extraction, same-package):**

- complete the **whole remaining staged batch** if it is same package/domain and
  safe — do not stop at one file;
- combine related GREEN batches when they are one coherent domain;
- avoid one-tiny-PR-per-file unless a file is genuinely risky;
- **no import-boundary change, no behavior change** (same-package extraction only).

**YELLOW (one real boundary move):**

- **one real seam per PR** (a single port/leaf move);
- **characterization tests required** before the move;
- include an **import-cycle / call-site / export-count report**;
- bounded, behavior-preserving changes only.

**RED / BLOCKED:**

- do **not** auto-code behavior changes;
- produce an audit / decision PR (Escalation decision record);
- provide **A / B / C options with a recommended default**;
- unblock the queue by making the decision *explicit*, not by guessing.

## 4. Autonomy rules

Claude **may** do autonomously (decide + report, no ask):

- split vs leave / take an exception, when justified;
- rename files for SRP accuracy;
- add same-package helpers / builders / fixture constructors to cut Sonar
  duplication;
- add direct tests for moved or pure helpers;
- update queue-item frontmatter metadata (its own item only);
- create enabling docs / new queue items;
- combine safe staged GREEN work into one PR.

Claude **must stop / escalate** before:

- schema / migration changes,
- auth / session / cookie changes,
- connector CAS / lease changes,
- queue / outbox / action_ledger / execution_attempts changes,
- DTO / wire contract changes,
- runtime store **semantics** changes,
- endpoint delete-vs-wire decisions,
- raw DB or raw `*Handler` crossing a boundary,
- public API / export explosion,
- import cycle or package-boundary uncertainty.

These map to Escalation classes E2/E3 — produce a decision record, do not force it.

## 5. Open PR policy (controlled parallelism)

- **max 2 open PRs at once**;
- only if their **package roots are disjoint**;
- **never** two PRs editing the same queue item / frontmatter;
- **never** parallel RED / controlled-zone work;
- **never** parallel migrations / schema / auth / CAS / ledger / outbox work.

Otherwise the lockless-queue rule (`AUTOPILOT_QUEUE.md`) and one-branch-one-PR
still hold: each PR updates only its own item file.

## 6. Sonar policy

- **New Code Sonar must be 0 before merge.**
- Do not suppress issues; do not change Sonar config.
- For **S3776**, run `scripts/go_cognitive_check.sh` before push; a moved function
  counts as New Code, so reduce any over-threshold moved function in the same PR.
- For **duplicated New Code**, prefer fixture builders / shared constructors over
  suppression.
- Sonar cleanup must **support the architecture migration**, not spawn random
  helper sprawl (an extracted helper must not itself become a new S3776).

## 7. PR size guidance (soft budgets, not handcuffs)

- A GREEN PR **may touch multiple files** if they are one coherent domain/package.
- Prefer reviewable slices.
- If a PR exceeds the normal size, **justify in the report why it is still safer
  than splitting**.
- Do **not** split purely to hit an arbitrary file count if splitting makes the
  architecture worse (e.g. half a move landing in two PRs).

## 8. Final report format

Every architect-sprint PR reports (superset of `AGENT_REPORT_TEMPLATE.md`):

```text
Architect Sprint:
- selected item(s):
- lane / risk / boundary_target:
- goal:
- feasibility result:        (receiver/coupling · import-cycle · call-sites · exports · coverage)
- files changed:
- architecture impact:       (which boundary/layer moved closer to target)
- Sonar / New Code risk:     (S3776 / duplication outcome; go_cognitive_check result)
- tests / validation:        (which guards/tests ran + result)
- skipped risky candidates:  (what was deliberately NOT done, and why)
- next recommended action:   (the highest-leverage next slice)
```

For a RED/BLOCKED audit PR, also include the Escalation decision record
(`class / trigger / options A·B·C / recommended default / why safe / remaining
risk`) from `ESCALATION_PLAYBOOK.md`.

## 9. Validation

Run as appropriate to the change:

- relevant `go test` for touched packages,
- `go build` / `go vet` / `go test` as the change warrants,
- `scripts/go_cognitive_check.sh` (S3776 guard on changed Go, incl. `_test.go`),
- `scripts/check_file_size.py` (200-line rule, baseline-aware),
- `scripts/ai_validate.sh` (wraps build/vet/test + boundary/file-size/complexity
  guards + `git diff --check`),
- `git diff --check`,
- queue/docs guards (`scripts/check_docs_governance.sh`,
  `scripts/ai_queue_check.sh`) for any docs/queue changes.

Push only after `ai_validate.sh` passes. **Never merge.**
