---
id: ARCHCM-R2b
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: connector-owner-investigation
boundary_target: blocked-decision
blocks: []
---

# ARCHCM-R2b — Connector command TTL/GC + CreateConnectorCommand idempotency (reliability follow-up)

## Goal (reliability follow-up — investigation, RED connector semantics)
Answer two open questions surfaced by the ARCHCM-R2 crawl-runtime audit
([`ARCHCM-R2`](ARCHCM-R2-crawl-runtime-semantics-audit.md) §Q1) about the durable
connector command queue:
1. Does `CreateConnectorCommand` (the row the Chrome Extension polls) have a **TTL /
   GC**? A command for a connector that never returns online appears to sit
   indefinitely — resumable but potentially stale.
2. Is `CreateConnectorCommand` **idempotent** on re-dispatch, or can a manual re-submit
   (same deterministic crawl `TaskID`) create a **duplicate** command row?

## NON-BLOCKING for ARCHCM4
This does **not** block ARCHCM4. ARCHCM4 is a behavior-preserving move that does NOT
change command creation/dispatch semantics, so it neither fixes nor worsens this. The
question is recorded here so it is not lost — it is a connector-reliability concern,
not an architecture-move gate.

## Component / domain
connector command queue / dispatch (`db.Connectors().CreateConnectorCommand`,
`appStore` task lifecycle). RED — connector runtime semantics; needs the connector/jobs
owners. Do NOT auto-change CAS/lease/command semantics.

## What to investigate
- Whether `connector_commands` rows expire / are GC'd (schema + any sweeper).
- Whether `CreateConnectorCommand` dedups on (org, account, agent, task_id) or appends.
- Operator impact: stale "queued" commands that never run; possible duplicate dispatch
  on re-submit.

## Risk notes
RED (connector command semantics). Investigation/decision only — any fix (TTL, dedup
key) changes connector-runtime behavior and needs owner sign-off + tests.

## Validation
N/A (investigation). A future fix needs connector-command characterization tests.

## Done criteria
The two questions answered (TTL/GC present? idempotent?), with a recommendation
(leave as-is / add TTL / add dedup key). Any change is a separate owner-approved PR.
Stays BLOCKED until investigated.
