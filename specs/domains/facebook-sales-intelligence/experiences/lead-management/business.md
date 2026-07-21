# Lead Management — Business Contract

Domain: **facebook-sales-intelligence**. Layer: **business** for the
`lead-management` experience. Distilled from the shipped lead-lifecycle,
lead-ingestion, and multi-actor-coverage contracts (see feature links below).

## Problem

The crawler produces a high volume of leads. Shown raw, the dashboard buries
the operator: fresh, actionable leads drown among old and dead ones, replies
go unanswered, and multiple actors risk spamming the same lead.

## Intended users

Tenant operators and their team members working leads from the workspace, with
multiple Facebook actor accounts per org.

## Business value and policy

- **Work the right lead at the right time.** Every lead carries a derived
  lifecycle state; the operator's queue is ordered by score → freshness →
  next action due, so effort lands on hot, fresh, or due leads first.
- **Leads are shared; execution is owned.** A lead belongs to the org, but
  each outreach action is attributed to the actor who performed it
  (multi-actor coverage policy). Coverage rules prevent one lead being
  blasted by several actors: coverage caps, per-actor dedup, and reply-aware
  gating are binding.
- **Never spam a waiting customer.** A lead that replied is answered with
  thread context; a lead that has not replied is not re-contacted inside the
  cooldown window.
- **Cold leads leave the queue honestly.** Auto-archive applies typed reason
  codes (`cold_no_activity`, `coverage_full_no_reply`, `invalid_target_url`,
  `thread_inactive`, `manual_not_relevant`); archived leads are restorable,
  never silently deleted.

## Success measures

- Operators act from the lifecycle queue instead of scanning raw lead lists.
- Zero duplicate outreach to one lead by multiple actors within policy windows.
- Replies surface as "needs response" and are answered before new first-touch.

## Business exclusions

- Not a CRM replacement; it is a battlefield queue over org-shared leads.
- No automatic outbound from this experience — engagement remains governed by
  the [engagement-approval](../engagement-approval/README.md) policy.

## Supporting technical features

- [lead-lifecycle](../../features/lead-lifecycle/technical.md)
- [lead-ingestion](../../features/lead-ingestion/technical.md)
- [multi-actor-coverage](../../features/multi-actor-coverage/technical.md)
- Platform dependency: workspace-ui (platform-foundation).
