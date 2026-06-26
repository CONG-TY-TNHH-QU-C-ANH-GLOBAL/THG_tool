---
id: ARCHCM1
status: REVIEW
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: chore/arch-cm1-action-args-split
pr_url: ""
---

# ARCHCM1 — Split cmd/scraper/action_args.go (coercion vs domain)

## Goal
Separate pure typeless-arg coercion (argString/argInt64/argBool) from domain-aware helpers (businessContextForOrg, promptKeywordFallback, maxItemsFromPrompt, platform detection) in the composition root.

## Component / domain
cmd/scraper composition root — arg plumbing vs domain config.

## Files likely involved
cmd/scraper/action_args.go → keep pure coercion; NEW action_config.go (domain helpers, same package).

## Dependencies
None. Precedes the YELLOW move-out items (so outbound/crawl moves don't inherit business-context logic).

## Risk notes
GREEN — same package, no import-boundary change, code move only. Confirms callers (crawl_runtime, outbound_*, agent_actions, skills_register) still resolve.

## Validation
go build ./cmd/scraper/... ; go test ./... ; ai_validate.sh

## Done criteria
action_args.go is pure coercion only; domain helpers in action_config.go; no behavior change; all callers compile.
