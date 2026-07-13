# Multi-Group Fresh-Lead Crawl — Rejected Designs

Layer: **decision** for the `multi-group-fresh-lead-crawl` feature.
Extracted from the PR-M0 spec (§13; authority: [technical.md](../technical.md)).
Each rejection is binding until explicitly revisited with new evidence.

| Design | Why rejected |
|---|---|
| **Client-computed freshness cutoff** (extension subtracts 24h from its own clock) | Client clocks skew and are user-controlled; lead eligibility would differ per machine. Cutoff is a server contract (technical.md §3). |
| **Trusting ambiguous timestamps** ("hôm qua" counts as fresh) | Fresh-lead-only means provably fresh; plausible-but-unproven posts pollute the lead queue with stale contacts. Excluded with a typed reason instead. |
| **Freshest-interpretation-wins at the margin** (`derived_relative` eligible when `latest_utc ≥ fresh_cutoff_at`) | Admits posts whose oldest possible age is already past the cutoff ("24 giờ" would pass) — plausibly fresh, not provably fresh. Replaced by the strict whole-window rule `earliest_utc ≥ fresh_cutoff_at` (technical.md §4). |
| **Timestamp-based dedup** (replace content dedup with posted_at identity) | Already rejected in `crawl.js` (in-code mandate): FB reorders/pins/async-injects too aggressively; timestamps are also the *least* reliable extracted field. |
| **First-stale-post stop** (stop the moment one old post appears) | Pinned/re-injected posts make single-post evidence worthless; would truncate runs at the first pin. Consecutive-streak frontier instead (technical.md §5). |
| **Round-robin account rotation per source** (spread each group over many accounts for throughput) | Rotation is the coordinated-inauthentic-behaviour pattern; also destroys the membership/affinity model. Sticky affinity + single machine slot instead (technical.md §2). |
| **Auto-handoff of a source to the next account after a checkpoint** | Rotation-to-dodge; explicitly forbidden by PR-C0.5. Source waits for operator/recovery. |
| **Relying on Facebook's "sort by new" feed parameter as the freshness guarantee** | FB changes/ignores the parameter unpredictably; it may be used as a *hint* in the task URL but never replaces per-post timestamp proof or the frontier. |
| **Shipping raw page HTML server-side to parse timestamps centrally** | Violates the privacy rule (no full-page DOM off the browser); parser runs client-side, typed fields only. |
| **A separate queue service / generic scheduler framework** | A status column + two partial unique indexes on `facebook_crawl_runs` *is* the queue; no second use case justifies a framework (PR-C0.5 §8 discipline). |
| **Extending `org_crawl_intents` in place** (add campaign columns to the intents table) | Conflates two lifecycles (a personal recurring intent vs an org campaign plan with pool + runs); would force RED changes to a live table for a new feature. Additive tables instead (technical.md §7). |
| **Auto-clearing `human_required` after a timer to keep the campaign moving** | Forbidden by the PR-C0.5 invariant; a campaign's throughput never overrides account safety. |
