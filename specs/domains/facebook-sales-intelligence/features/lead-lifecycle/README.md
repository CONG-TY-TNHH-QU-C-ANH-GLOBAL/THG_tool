# Feature: lead-lifecycle

Derived lead lifecycle projection, work queue, and auto-archive
(`internal/models/lead_lifecycle.go`, `work_queue.go`, `lead_archive.go`,
`internal/store/leads/*`, `cmd/scraper/auto_archive_scheduler.go`). Supports
the [lead-management](../../experiences/lead-management/README.md) experience.

- [technical.md](technical.md) — the lifecycle/work-queue/auto-archive
  contract (PR-1..PR-5 shipped; retention compaction config-staged only).
- [decisions/organic-sales-network.md](decisions/organic-sales-network.md) —
  founder architecture directive (lead shared / execution owned / attribution
  derived / campaign orchestration). Proposed-partial: ledger and ownership
  foundations exist; campaign layer and event bus do not.

Related feature: [multi-actor-coverage](../multi-actor-coverage/README.md)
(standalone coverage projection the work queue surfaces).
