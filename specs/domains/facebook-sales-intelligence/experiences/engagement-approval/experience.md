# Engagement Approval — Operator Experience

Domain: **facebook-sales-intelligence**. Layer: **experience** for the
`engagement-approval` experience. Shipped behavior unless marked planned.

## Submitting engagement (shipped)

- The operator drops a Facebook post link into chat: the system acknowledges
  asynchronously ("Đã nhận bài viết này. Mình sẽ đưa bài viết vào leads…") and
  runs the durable direct-post intake workflow; honest terminal reasons (e.g.
  `lead_not_observed_after_retries`, identity-mismatch codes) are surfaced
  instead of silent failure.
- Natural-language direct-link commenting queues only when the lead already
  exists; each outcome case has a typed operator response (see the
  [direct-link smoke runbook](../../features/comment-automation/runbooks/direct-link-smoke.md)).

## Verification states the operator sees (shipped)

- A submitted comment shows **"Đã gửi, đang chờ xác minh"**
  (`waiting_verification`); "Mở post" is always available, "Xác minh lại"
  where eligible.
- Manual human-verify ("Xác nhận đã đăng" with confirm dialog) is offered ONLY
  for `submitted_unverified` outcomes; retry is offered only for pre-submit
  failures — never manual-confirm for those.
- Comment metrics gate the workflow: an unverified rate under ~10% keeps
  manual fallback; above 10–15% reopens async reverify.

## Approval and blocking states

- Shipped: coverage/dedup blocks explain themselves with typed reasons
  (`coverage_full`, `already_commented_by_this_actor`, cooldown gates).
- Shipped: actor-mismatch blocks an account and notifies
  (`NotifyActorMismatch`); the operator clears it explicitly via
  `POST /accounts/:id/clear-actor-block`.
- Planned (comment-intelligence P2d/P2e): full Policy Gate enforcement UI and
  the Agent Decision Inspector with verdict chips (auto_execute ✅ /
  require_review ⏸ / reject ⛔) and Verified-Actor chips (✅/⚠️/❔). Do not
  treat these as shipped.

## Human-required behavior (binding, shipped)

Login walls/checkpoints during engagement return `human_required` and stop the
action; recovery is operator-driven (account-safety state machine). Human
review is a fallback for gated decisions, not a mandatory queue for every
action.

## Supporting technical features

- [outbound-actions](../../features/outbound-actions/technical.md)
- [comment-automation](../../features/comment-automation/technical.md)
- [comment-intelligence](../../features/comment-intelligence/technical.md)
- [direct-post-intake](../../features/direct-post-intake/technical.md)
- [account-safety](../../features/account-safety/technical.md)
