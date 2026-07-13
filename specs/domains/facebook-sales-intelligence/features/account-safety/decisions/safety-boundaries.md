# Account Safety — Hard Boundaries and Data-Plane Ownership

Layer: **decision** for the `account-safety` feature.
Extracted from the PR-C0.5 spec (§2 hard boundaries + §7 data-plane ownership;
authority: [technical.md](../technical.md)). These boundaries are binding on
every runtime PR and are not tunable with telemetry evidence — they are
product/safety law, not thresholds.

The Coordinator designs **account safety, controlled concurrency, graceful
stop, cooldown, and operator visibility** — **not** anti-detection.

## Hard boundaries (the Coordinator must NOT own)

- Browser fingerprint spoofing / stealth / evasion of any kind.
- Checkpoint/CAPTCHA solving or auto-clicking challenges.
- Provider bypass, proxy rotation, or account rotation intended to dodge a checkpoint.
- The browser session lifecycle itself — that stays in `session.*`; the Coordinator only
  *reads* session state and *requests* leases.
- Cross data-plane persistence — it writes only to its owned plane (below); it must not
  reach into browser-secret or foreign-org state.

## Data-plane ownership

Per `docs/architecture/DATABASE_OWNERSHIP.md` §Data planes:
- **Local runtime / client (browser + connector)** owns **ephemeral** browser/session/crawl
  counters: `scroll_count`, `no_progress_rounds`, `duplicate_count`,
  `failed_extraction_count`, current `phase`, `scroll_moved_ever`, per-crawl dedup set.
  These live and die with the crawl; they are never a system of record.
- **SQLite local runtime** may hold local operational cache / outbox / session telemetry
  only (e.g. `browser_sessions` checkpoint/heartbeat/vnc state already lives here). No
  tenant system-of-record data.
- **PostgreSQL platform** owns **durable** org/account/workspace policy, queue/lease state,
  and any durable account-safety state (`recent_automation_window`, `cooldown_until`,
  `last_safe_stop_reason`, risk policy). `org_crawl_intents` (the recurring cursor) is
  already mid-migration to this plane (`platform/0108_platform_crawl`).
- **RAG / vector plane** is irrelevant here — the Coordinator neither reads nor writes it.
- **No browser secrets server-side.** Cookies/session/credential material never leave the
  local runtime.
- **No data-plane moves without a dedicated migration PR.** If PR-C3 needs durable safety
  state, that table/migration is its own RED-reviewed PR, not smuggled into a runtime change.

Cross-plane flow uses the existing explicit event/outbox path — never a hidden shared table.
