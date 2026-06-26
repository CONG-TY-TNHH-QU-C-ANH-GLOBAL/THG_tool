---
id: PR32A
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [PR31E]
parallel_safe: false
branch: ""
pr_url: ""
---

# PR32A — Product-path audit for Facebook automation operator UX

## Goal

Audit and harden operator-visible status flow: readiness reason -> queue status -> execution result.

## Scope

- backend API/status payloads
- dashboard-facing response DTOs only if already existing

## Constraints

- no DTO/wire contract change unless explicitly reported
- characterization first
- no schema/migration
- no auth/session/queue/ledger/CAS behavior change

## Validation

- scripts/ai_preflight.sh
- scripts/ai_validate.sh
