# Feature: direct-post-intake

Durable intake workflow turning an operator-submitted Facebook post link into
a lead + queued comment, with ownership guards and pre-submit verification
(`cmd/scraper/direct_post_intake*.go`, `internal/store/coordination/direct_post_*`,
`internal/directpost`, migration 0022). Supports the
[engagement-approval](../../experiences/engagement-approval/README.md)
experience.

- [technical.md](technical.md) — the workflow contract incl. the §8f control
  predicate (fail-closed writes, member-owned accounts never admin-controllable)
  and the pre-submit-verify oracle. Implementation state: **backed**
  (PR-1/PR-2 + hotfix train shipped with tests; PR-3 hardening and the
  extension pre-submit call remain planned).
