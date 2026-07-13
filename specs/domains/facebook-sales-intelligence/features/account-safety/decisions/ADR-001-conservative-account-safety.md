# ADR-001: Conservative, Fail-Safe Account Safety

Layer: **decision** (supporting rationale) for the `account-safety` feature.
Status: accepted with the PR-C0.5 baseline.
Normative owner: [technical.md](../technical.md) — the binding invariants
(hard boundaries §2, state machine §3, concurrency §4, risk budgets §5,
data-plane ownership §7) live there, not here. This record explains *why*
those conclusions were selected.

## Context

A single operator machine hosts 5–10 signed-in Facebook accounts. Uncoordinated
parallel automation on one host/IP creates correlated risk: a checkpoint on one
account raises the odds for its neighbours, a single stalled checkpoint page can
burn the whole fleet's time budget unnoticed, and uncoordinated re-dispatch
after failures produces exactly the retry/rotation patterns platforms flag as
coordinated inauthentic behaviour. THG is a tenant-facing SaaS: account loss is
a customer-harm event, not an acceptable operating cost.

## Decision

Coordinate via **safety budgets, not evasion**, and fail safe in every
ambiguous situation: hard per-machine and per-account concurrency caps, typed
risk states that stop work gracefully, operator-only recovery from risk states,
and strict data-plane separation. The full, binding formulation is
[technical.md](../technical.md).

## Alternatives considered and rejected

- **Account rotation after a wall** (promote the next account into a freed slot
  or re-queue the source elsewhere): highest-throughput option, rejected because
  routing around a platform check is the rotation-to-dodge pattern — it converts
  a single-account risk signal into a fleet-level ban risk and is an evasion
  behavior the product forbids categorically.
- **Auto-clearing risk states on a timer** (resume after N minutes to keep
  campaigns moving): rejected because a cleared-but-unresolved checkpoint
  resumes automation on a compromised session; only an operator-verified path
  can prove the wall is actually gone. Throughput never overrides safety.
- **Higher default concurrency** (run several accounts per machine in
  parallel): rejected as the default because simultaneous automation on one
  host/IP is itself a risk signal; parallelism multiplies checkpoint exposure
  instead of adding usable throughput. Budgets may only be loosened later by
  explicit operator opt-in under telemetry evidence — never by pool size.
- **Solving/auto-clicking checkpoints and CAPTCHAs, stealth or fingerprint
  evasion**: rejected unconditionally; these are product boundaries
  (AGENTS.md invariants), not tunable engineering trade-offs.
- **Immediate automatic retries after failures**: rejected in favour of
  bounded, reason-tagged cooldown + backoff, because retry storms both hammer
  the flagged account and correlate activity across the fleet.

## Trade-offs accepted

Deliberately lower throughput: one active crawl per machine by default, sources
wait for their account to recover instead of failing over, and campaigns stall
visibly on `human_required` rather than proceeding silently. The exchange is
account longevity, tenant trust, and operator-legible behavior over crawl
volume — consistent with the fresh-lead product goal that lead *quality*, not
raw volume, is the outcome.

## Consequences

- The Coordinator stays a pure decision/bookkeeping layer above `session.*`;
  anything that would need browser control or secret access is structurally out
  of its reach (see the ownership boundaries in technical.md §2/§7).
- Recovery paths are operator workflows (checkpoint alert + VNC deep-link),
  so operator UX is a first-class part of the safety design.
- Durable safety state changes require their own RED-reviewed migration PR,
  keeping the data-plane doctrine enforceable at review time.
- Any future loosening (e.g. machine budget 2) is an explicit, evidence-backed
  policy change reviewed against the
  [review checklist](../runbooks/review-checklist.md), never a code-path default.
