# Lead Management — Operator Experience

Domain: **facebook-sales-intelligence**. Layer: **experience** for the
`lead-management` experience. Every behavior below is shipped unless marked
planned (sources: lead-lifecycle technical contract PR-1..PR-5 evidence).

## Visible lifecycle states (shipped)

Each lead shows a derived `freshness_state`:

- `active` — untouched (fresh, eligible) or replied ("needs our response");
- `waiting_reply` — we acted, give them time;
- `followup_due` — re-engage now;
- `stale` — cold, archive candidate;
- `archived` — out of the queue, restorable.

The dashboard groups these as lifecycle tabs (shipped `LifecycleTabs.tsx`,
Vietnamese labels): **"Cần xử lý / Chờ phản hồi / Đến hạn follow-up / Đã lưu
trữ"**. The default view hides archived and stale leads.

## Work queue (shipped)

The queue is ordered by **score → freshness → `next_action_at`**; each lead
carries a suggested `next_action` (`comment`, `reply`, `wait`, `followup`,
`archive`, `none`). Coverage skip reasons from multi-actor-coverage
(`already_commented_by_this_actor`, `lead_replied`, `single_actor_policy`,
`coverage_full`, `coverage_gap_too_soon`) explain why a lead is not offered
for action.

## Archive and restore (shipped)

Auto-archive applies typed reason codes; the operator can archive/unarchive
via `POST /api/leads/:id/archive` / `:id/unarchive` and a restore button in
the archived tab.

## Copilot surfacing (shipped)

The sales copilot reports queue state in operator language
(`copilot_wording.go`): e.g. "Không tìm được lead hợp lệ để comment sau khi
quét N lead", inventory summaries ("N lead đang chờ phản hồi / N lead đến hạn
follow-up / N lead đã lưu trữ"), and suggestions ("cào thêm lead mới / bật
follow-up cho lead đến hạn / xem mục đã lưu trữ").

## Planned (not shipped)

- Evidence/raw-crawl retention **compaction** (`purged` sub-state) is
  config-staged only — it does not execute yet.
- The LeadsView UI decomposition is an implementation plan under
  workspace-ui, not a behavior change.

## Supporting technical features

- [lead-lifecycle](../../features/lead-lifecycle/technical.md)
- [lead-ingestion](../../features/lead-ingestion/technical.md)
- [multi-actor-coverage](../../features/multi-actor-coverage/technical.md)
