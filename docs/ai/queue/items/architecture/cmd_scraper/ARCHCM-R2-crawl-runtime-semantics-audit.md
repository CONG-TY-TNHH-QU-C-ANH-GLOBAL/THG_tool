---
id: ARCHCM-R2
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHCM-R2 — AUDIT: crawl runtime / dispatch semantics

## Goal (audit-only)
Document the crawl_runtime.go fallback chain (open crawl → account resolve → connector dispatch → jobStore fallback): resumability, race conditions on account-offline mid-submit, RBAC of pickReadyFacebookAccountIDForCrawl.

## Component / domain
crawler/jobhandler runtime + connector dispatch. RED.

## Files likely involved
cmd/scraper/crawl_runtime.go, internal/jobs, internal/connectors.

## Dependencies
Blocks ARCHCM4.

## Risk notes
RED — runtime/job/connector semantics. Human review with crawl/jobs owners before any move.

## Validation
N/A (audit).

## Done criteria
Semantics documented + the three audit questions answered; ARCHCM4 unblocked only after sign-off.
