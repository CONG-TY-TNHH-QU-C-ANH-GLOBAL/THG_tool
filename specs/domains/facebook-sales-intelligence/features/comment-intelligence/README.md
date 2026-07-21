# Feature: comment-intelligence

Knowledge-first comment decisioning: lead-candidate assembly, comment decision
model, policy gate, actor verification (`internal/ai/comment_decision.go`,
`policy_gate.go`, `internal/store/coordination/actor_verification.go`).
Supports the
[engagement-approval](../../experiences/engagement-approval/README.md)
experience.

- [technical.md](technical.md) — the P1–P4 design (STATUS: DESIGN). P1a/b,
  P2b, P2c shipped; P2d Policy-Gate enforcement, P2e Inspector UI, P3 media,
  P4 vision remain planned. Implementation state: **partial**. The binding
  default-off auto-execute invariant is restated in the engagement-approval
  business contract.
- [evidence/comment-decision-dryrun-report.md](evidence/comment-decision-dryrun-report.md)
  — historical one-time dry-run gate report (archived).
