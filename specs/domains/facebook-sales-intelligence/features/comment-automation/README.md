# Feature: comment-automation

Comment execution, forensics, and verification machinery: soft-touch
submission, forensics classification, async reverify, human-verify fallback
(`internal/models/comment_forensics.go`, `internal/store/coordination/comment_*`,
`cmd/scraper/comment_reverify_scheduler.go`, extension `reverify.js`). Supports
the [engagement-approval](../../experiences/engagement-approval/README.md)
experience.

- [technical.md](technical.md) — the verification lifecycle contract
  (forensics buckets, soft touch, async reverify, human-verify eligibility).
  Implementation state: **backed** (shipped with tests).
- [runbooks/direct-link-smoke.md](runbooks/direct-link-smoke.md) — operator
  live smoke for NL direct-link commenting.
- [evidence/](evidence/) — historical: the PR8 autocomment redirect
  investigation and its root-cause report (closed; archived).
