# Fresh-Lead Discovery — Business Contract

Domain: **facebook-sales-intelligence**. Layer: **business** for the
`fresh-lead-discovery` experience. Extracted from the PR-M0 spec
(now [multi-group-fresh-lead-crawl/technical.md](../../features/multi-group-fresh-lead-crawl/technical.md)).
Status: **draft — docs only.**

## Problem

An org wants "leads from the last 24 hours across my 20 groups, every few hours".
Today the system can only express that as 20 independent `org_crawl_intents`, each
crawling until `max_items` regardless of post age, each dispatched with no shared
plan. Consequences:

- **Stale waste.** A group whose feed surfaces week-old posts burns the whole
  `max_items` budget on posts that will never become leads. Crawl time is the
  scarce, risk-carrying resource; spending it on stale content is pure loss.
- **No freshness contract.** `posted_at` is empty on the wire, so "only fresh posts
  become leads" cannot be enforced anywhere — not in the extension, not at ingest.
- **No cross-group orchestration.** Nothing sequences 20 groups over a pool of 5
  accounts under the machine budget; nothing records which group was covered when,
  or which account is mid-crawl.
- **Duplicate leads across runs.** Re-crawling a group re-sees recent posts; only
  the `dedup_hash` insert guard stands between a re-seen post and a duplicate lead.

## Intended users

Tenant operators running multi-group Facebook lead campaigns from their own
logged-in account workspaces.

## Business value and success measures

- Leads delivered are **provably fresh** (post age inside the campaign's
  freshness window, default 24h) — a stale contact never enters the lead queue.
- **Lead volume becomes intentionally lower** per crawl: stale posts stop minting
  leads. This is the product goal, and the exclusion counters make the delta
  visible to the operator instead of mysterious (see
  [experience.md](./experience.md)).
- Crawl time (the scarce, risk-carrying resource) is spent on fresh content:
  runs stop early at the temporal frontier instead of grinding to `max_items`.
- One campaign covers many groups with an account pool: an account in cooldown
  or `human_required` does not stall the whole campaign.

## Business exclusions

- Not a generic "scan all Facebook groups" scraper — a campaign is an explicit,
  org-scoped plan (groups + freshness window + account pool).
- Account safety is never traded for throughput: no fingerprint spoofing,
  stealth/evasion, proxy/account rotation to dodge checkpoints,
  CAPTCHA/checkpoint solving, or speed increases
  ([account-safety technical contract](../../features/account-safety/technical.md)).
- A campaign's throughput never overrides account safety: `human_required`
  states clear only through the operator path, never on a timer.

## Supporting technical features

- [multi-group-fresh-lead-crawl](../../features/multi-group-fresh-lead-crawl/technical.md)
- [account-safety](../../features/account-safety/technical.md)
