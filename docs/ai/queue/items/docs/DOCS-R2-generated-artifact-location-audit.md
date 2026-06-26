---
id: DOCS-R2
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# DOCS-R2 — AUDIT: generated artifact RETRIEVAL_SOAK_REPORT.md location

## Goal (audit-only — DO NOT move or change the test)
specs/knowledge/RETRIEVAL_SOAK_REPORT.md is a TEST-GENERATED artifact tracked inside the spec tree; the soak test rewrites it on every `go test ./...`, dirtying the working tree. Decide its correct home and the write-gating policy.

## Doc area
specs/knowledge/ (generated artifact) + the writer test.

## Why RED — code/test write path
internal/workspace_knowledge/soak/harness_test.go writes the report to specs/knowledge/RETRIEVAL_SOAK_REPORT.md (3 candidate relative paths). Relocating the file or stopping the rewrite is a CODE change to a test, not a docs move.

## Escalation decision record
```text
Escalation:
- class: E2 (controlled zone — test write path / generated artifact) + E3 (artifact ownership)
- trigger: a generated report lives in the curated specs/ tree and is rewritten every
  test run, repeatedly dirtying git (handled today by reverting the file before commit).
- options considered:
  1. Gate the write behind a `-update` flag (test only writes when explicitly asked),
     leaving the committed report stable. Smallest behavior change; test-code edit = RED-adjacent.
  2. Move the artifact to docs/artifacts/ (or gitignore it) and update the test's write path.
  3. Leave as-is; continue reverting before each commit (status quo, ongoing friction).
- decision: NEEDS HUMAN — recommend option 1 (gate behind -update); aligns with the prior
  project note on this exact friction. Requires a test-code change, so out of any docs lane.
- why safe: audit only; nothing changes until approved.
- files touched: none (audit only).
- validation: n/a until the chosen-option PR; that PR adds a test for the gated behavior.
- remaining risk: option 2 changes the path 3 call sites expect; must update all + CI.
```

## Done criteria
Human chooses gate-vs-relocate; follow-up PR (test track, not docs) implements it with a test. No change here.
