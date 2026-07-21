# Feature: multi-actor-coverage

Standalone coverage policy over the engagement ledger: which actor may touch
which lead, when, and why not (`internal/models/coverage.go`,
`coverage_text.go`, `internal/store/leads/lead_coverage.go`,
`internal/ai/comment/persona.go`). Pure `EvaluateCoverage` projection —
independent of the lifecycle derivation; the work queue composes both.
Supports [lead-management](../../experiences/lead-management/README.md) and
[engagement-approval](../../experiences/engagement-approval/README.md).

- [technical.md](technical.md) — binding coverage rules, typed skip reasons,
  actor-persona derivation. Implementation state: **backed** (PR-0..PR-3
  code + tests).
