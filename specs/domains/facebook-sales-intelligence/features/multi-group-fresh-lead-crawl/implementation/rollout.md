# Multi-Group Fresh-Lead Crawl — Rollout PR Plan

Layer: **implementation** for the `multi-group-fresh-lead-crawl` feature.
Extracted from the PR-M0 spec (§11; authority: [technical.md](../technical.md)).
Status: **draft — docs only.**

One branch/PR each; behavior-changing PRs ship with tests protecting reason codes
and policy decisions. Sequenced to stay releasable at every step.

| PR | Scope | Behavior change? |
|---|---|---|
| **PR-M0** | The technical contract + registry entry. | Docs only. |
| **PR-M1** | Extension timestamp parser (`platforms/facebook/crawl_time.js` + `crawl_time.test.mjs`, pure + tested) emitting the per-item `TimestampParse` DTO (`posted_at`, `confidence`, `earliest_utc`/`latest_utc`) on the existing crawl wire; `content/crawl.js` gains wiring only. | Additive telemetry; fills the currently-empty `posted_at`. No stop-logic change. |
| **PR-M2** | Platform migration: campaign/run tables + partial unique indexes. **RED — own reviewed PR.** Shipped as PR-M2B, migrations 0112–0117 ([postgres-schema.md](./postgres-schema.md)). | Schema only; nothing reads it yet. |
| **PR-M3** | Pure policy package `freshlead` (freshness gate, frontier, scheduler decision, reason codes) + store CRUD. | No wiring; dead code with tests. |
| **PR-M4** | Scheduler wiring in composition root: campaign → queue → admit via Allocator lease + machine budget + DB constraint. | New orchestration path; existing intent path untouched. |
| **PR-M5** | Crawl task carries `fresh_cutoff_at`; crawl loop consumes frontier decision; ingest applies the fresh-lead gate + lead-identity dedup. | The fresh-lead-only behavior lands here, telemetry-visible. |
| **PR-M6** | Operator UI: campaign CRUD, run history, exclusion counters. | UI only. |

Dependency on the safety track: PR-M4/M5 assume PR-C2 (classifier stop) and
PR-C3 (Coordinator) are in place or land together — the campaign scheduler calls
the Coordinator, it does not reimplement it.

## Rollback

Docs-only spec train entry. Rollback of the docs = revert the affected files +
their `SPEC_REGISTRY.json` entries. Rollback of the whole runtime feature
(post-runtime): pause campaigns (`status='paused'`) — the tables are additive
and inert when no campaign is active.
