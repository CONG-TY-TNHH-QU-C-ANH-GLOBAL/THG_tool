---
id: DOCS-R1
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# DOCS-R1 — AUDIT: spec source-of-truth (root specs/ vs governed docs/specs/)

## Goal (audit-only — DO NOT move files)
Reconcile the two spec locations: the authoritative root `specs/` tree (54 files, 13 domain subdirs, README + SPEC_GOVERNANCE.md + SPEC_REGISTRY.json) vs the governance taxonomy's intended `docs/specs/` (currently empty/absent). Decide whether specs stay at root or migrate — and how tooling follows.

## Doc area
specs/ (root) and docs/specs/ (governance taxonomy).

## Files likely involved (DO NOT change yet)
specs/** (all), specs/SPEC_REGISTRY.json, docs/INDEX.md, docs/DOCS_GOVERNANCE.md, CLAUDE.md "Read First", AGENTS.md.

## Why RED — script/tooling references
`specs/` paths are consumed by: scripts/check_spec_registry.py, scripts/check_component_structure.py, scripts/check_topology.sh, scripts/check_tenant_isolation.sh, scripts/rootcause_query.py, plus CLAUDE.md Read-First and SPEC_REGISTRY.json. A bulk move breaks all of these.

## Escalation decision record
```text
Escalation:
- class: E3 (architecture boundary decision — docs/spec ownership)
- trigger: governance (docs/DOCS_GOVERNANCE.md) says specs live under docs/specs/,
  but the authoritative, tool-referenced spec tree is root specs/. Two homes =
  source-of-truth ambiguity; docs/INDEX.md already hedges ("authoritative specs
  currently live under specs/").
- options considered:
  1. KEEP specs/ at root as canonical; update docs/DOCS_GOVERNANCE.md + docs/INDEX.md
     to declare root specs/ the spec home (docs-only, reconciles governance to reality).
  2. MIGRATE specs/ -> docs/specs/ (git mv all 54 files; rewrite SPEC_REGISTRY.json paths,
     5+ guard scripts, CLAUDE.md, AGENTS.md). Large, RED, high blast radius.
  3. HYBRID: keep specs/ canonical; add a thin docs/specs/INDEX.md pointer only.
- decision: NEEDS HUMAN — recommend option 1 (cheapest, preserves tooling), but it
  changes a governance contract, so a human must confirm the canonical home.
- why safe: this item changes NOTHING until approved; it is an audit + recommendation.
- files touched: none (audit only).
- validation: n/a until a chosen-option PR; that PR must keep check_spec_registry.py green.
- remaining risk: option 2 would touch scripts/registry (out of scope for any GREEN/YELLOW lane).
```

## Done criteria
Human picks the canonical spec home; if option 1, a follow-up docs-only PR reconciles DOCS_GOVERNANCE.md + docs/INDEX.md wording. No specs/ move without sign-off.
