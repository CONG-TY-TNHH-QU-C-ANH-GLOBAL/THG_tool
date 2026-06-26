---
id: ARCHCM-R1
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
---

# ARCHCM-R1 — AUDIT: consolidate duplicated account-control RBAC

## Goal (audit-only — security critical)
"Can requester control this account?" / "pick a safe account" logic is duplicated across facebook_account_scope.go, direct_post_account_guard.go, outbound_action_context.go, and inline in crawl_runtime.go. Decide the single canonical home + API.

## Component / domain
account ownership / RBAC authorization. RED (auth/security).

## Files likely involved
cmd/scraper/facebook_account_scope.go, direct_post_account_guard.go, outbound_action_context.go, crawl_runtime.go; target internal/connectors or internal/identity.

## Dependencies
Blocks ARCHCM2 and ARCHCM4 (they call this RBAC).

## Risk notes
RED — security/RBAC. Divergent copies are a real risk; but consolidation changes an authorization surface → human decision + RBAC-1 spec verification required. No autonomous change.

## Validation
N/A (audit). The eventual consolidation PR needs RBAC characterization tests.

## Done criteria
Decision record naming the canonical implementation + facade (CanControl/PickSafe), call-site map, and RBAC-1 conformance plan — approved by a human.
