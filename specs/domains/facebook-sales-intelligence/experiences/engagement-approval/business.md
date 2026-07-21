# Engagement Approval — Business Contract

Domain: **facebook-sales-intelligence**. Layer: **business** for the
`engagement-approval` experience. Distilled from the shipped comment/outbound
contracts and the binding approval invariants formerly in the agent
entrypoints; nothing here is invented.

## Problem

Outbound engagement (comments, inbox messages, posts) on a customer's own
Facebook accounts is the highest-risk surface in the product: a wrong or
spammy action damages the tenant's brand and can cost the account. Automation
must therefore be approval-governed, deduplicated, verified, and attributable
— never fire-and-forget.

## Binding approval policy

- **Approval-required is the default.** Outbound comments, inbox messages, and
  posts default to draft or approval-required. `auto_execute` is an explicit
  per-org opt-in: a fresh org ships `auto_execute_enabled = false`; the Policy
  Gate is a safety floor, not an off-switch. Until amended, code ships
  default-off.
- **Auto-execution has hard preconditions.** Auto-execute is permitted only
  when every gate holds — including verified actor identity (`actor_verdict ==
  verified`; `unknown` → never auto; `mismatch` → account blocked) and
  grounded content (`knowledge_gap == true` → never auto-execute).
- **Even opted-in orgs pass guardrails.** Auto outbound still passes org-scoped
  dedup, cooldown, and conversation-thread guardrails before entering the
  approved outbox state.

## Channel discipline (binding)

- Inbox is customer-service-style, not one-shot blasting: if a lead replied,
  answer the latest reply with thread context; if they have not replied, do
  not re-send inside the cooldown window.
- First-touch outreach via the `inbox_all_leads` skill IS sales semantics — it
  defaults to `score_filter=hot` to target qualified, never-contacted leads.
  The customer-service discipline applies AFTER first-touch: the
  `awaiting_reply_cooldown` gate prevents repeat sends on the same thread.
  Reply-to-inbound uses a different path (a thread with `last_inbound_at >
  last_outbound_at` is gated as `lead_replied=Allowed` so the operator can
  respond); the bulk skill itself is not a CS triage tool.

## Ownership and verification value

- A Facebook WRITE runs only under the shipped control predicate (connector
  owned by the requester, live FB identity matches the account, assignment
  honored; `requester <= 0` fails closed; a member-owned account is never
  controllable by an admin).
- Sent actions are never assumed successful: verified-state-centric policy
  forbids promoting `submitted_unverified` to succeeded on a guess.

## Success measures

- Zero unapproved outbound from a default-configured org.
- Zero duplicate outbound to one target within policy windows.
- Every executed action ends in a verified, attributable outcome state.

## Supporting technical features

- [outbound-actions](../../features/outbound-actions/technical.md)
- [comment-automation](../../features/comment-automation/technical.md)
- [comment-intelligence](../../features/comment-intelligence/technical.md) (draft design)
- [direct-post-intake](../../features/direct-post-intake/technical.md)
- [multi-actor-coverage](../../features/multi-actor-coverage/technical.md)
- [account-safety](../../features/account-safety/technical.md)
