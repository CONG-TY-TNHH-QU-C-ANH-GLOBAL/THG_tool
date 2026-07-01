Execute the THG **Accelerated Architecture Convergence Loop**.

Authority: `docs/ai/ACCELERATED_ARCHITECTURE_CONVERGENCE_LOOP.md` (the protocol).

Goal: move living production code toward the agreed modular-monolith
architecture (thin composition roots, bounded domain subpackages, store
subpackages via accessors, fewer god packages/files, fewer deprecated shims)
by running one coherent, larger convergence batch — not a tiny one-off diff —
and pushing it end to end without stopping for per-file approval.

Use:
- `docs/ai/ACCELERATED_ARCHITECTURE_CONVERGENCE_LOOP.md` (full protocol, §1–§11)
- `docs/architecture/BOUNDARY_MIGRATION_PLAYBOOK.md` (lane + feasibility authority)
- `docs/ai/ESCALATION_PLAYBOOK.md` (stop conditions → decision record, A/B/C)
- `internal/store/DOMAINS.md`, `specs/RUNTIME_TOPOLOGY.md` (truth ownership / boundaries)
- `scripts/ai_preflight.sh`, `scripts/ai_validate.sh`, `scripts/go_cognitive_check.sh`,
  `scripts/check_file_size.py`, `scripts/check_topology.sh`,
  `scripts/check_component_structure.py`

Steps:
1. **Sync (protocol §1):** fetch/start from latest `origin/main`; check branch
   cleanliness; run `scripts/ai_preflight.sh`. Never `git add -A`; never stage
   queue-reconcile `.md` files, soak artifacts, env files, or `.mcp.json`.
2. **Survey (§2):** scan hotspots (`specs/COMPONENT_HOTSPOTS.md`,
   `scripts/file_size_allowlist.txt`, remaining deprecated wrapper/alias
   files, `*AppStore`-style god objects). Identify the top 3 convergence
   candidates and score by leverage/risk/PR-size/Sonar-risk/behavior-risk.
   Check §10 of the protocol (current default backlog) as a starting point,
   not a fixed order — re-score against what survey actually finds.
3. **Select (§3):** pick the highest-leverage safe batch. Batch larger than
   the old one-file cadence; one coherent boundary per PR; combine low/medium
   risk mechanical caller migrations toward the same goal; don't over-split
   harmless mechanical change, but keep RED semantic cutovers staged.
4. **Implement (§4):** execute without per-file approval. Reuse existing
   patterns (subpackage + accessor, domain contract, projection). No new
   store-layer ports/interfaces unless `DOMAINS.md` explicitly allows it. No
   speculative abstraction, no unrelated cleanup, no formatting/import churn.
5. **Guardrails (§5):** preserve route paths/auth/order, wire shape, tenant
   filtering, queue/CAS/lease/outbox/action_ledger semantics, auth/session
   behavior. No schema/migration change unless explicitly selected as its own
   controlled item. Leave `installOutboundHooks`-equivalent hooks alone unless
   they are the explicit target. No Sonar suppressions or config changes.
6. **Acceleration rule (§6):** a cognitive-complexity or file-size guard trip
   on a touched file gets a minimal behavior-preserving helper extraction in
   the same PR — do not abandon a good batch over one fixable guard trip.
7. **Validate (§7):** targeted `go test` → `go test ./...` →
   `scripts/ai_validate.sh` → `scripts/check_topology.sh` (if a store/domain
   boundary moved) → `scripts/check_component_structure.py` (if files moved) →
   `git diff --check`. Fix all local failures before reporting.
8. **Report (§8):** selected boundary and why; candidates considered and
   scores; files moved/changed/deleted; wrappers/shims migrated or deleted;
   behavior-preservation proof; route/wire/auth/tenant/CAS/ledger/schema risk
   statement; tests/guards run; Sonar expectation; rollback plan; next
   recommended batch.

Stop-and-ask conditions (protocol §9) — produce a decision record instead of
coding through: product-visible behavior change; auth/security/session
semantics change; schema/migration design decision; queue/CAS/lease/outbox/
action_ledger semantic cutover; public DTO/wire-shape change; unclear tenant
isolation; deleting code with uncertain runtime usage; CI/Sonar policy/config
change; an import cycle forcing a package-ownership decision.

Do not stop after survey if a safe convergence batch exists — implement it.
Push only after `scripts/ai_validate.sh` passes. One branch, one PR. **Never
merge.**
