---
id: ARCHCM4a
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM-R1, ARCHCM-R2]
parallel_safe: false
branch: ""
pr_url: ""
boundary_target: prep-extraction
---

# ARCHCM4a — De-arg seam for the crawl entry points (enables ARCHCM4b)

## Goal
Introduce a typed crawl-request boundary in cmd/scraper so the crawl execution path no
longer threads `args map[string]any` through package-main `arg*`/prompt helpers.
cmd-internal, behavior-preserving prep that lets the typed core move to
`internal/crawler` in ARCHCM4b (internal/crawler cannot import package main).

## Component / domain
crawl plan-assembly / entry points (cmd/scraper, package main).

## What changes
- Split arg/prompt resolution (stays in cmd: `resolveCrawlMaxItems`,
  `resolveCrawlKeywords`, `buildCrawlExtras`, account resolution, and the `argInt64`/
  `argString`/`argBool` + `maxItemsFromPrompt`/`promptKeywordFallback`/`splitKeywords`
  reads — ~31 refs) from the typed execution entry points.
- `submitOpenCrawl` (and the scheduler's call into it) take typed parameters instead of
  `args map[string]any`; a thin cmd wrapper resolves args → typed and calls it.
- No package move. No connector-dispatch / command-creation change. No RED touch.

## Dependencies
ARCHCM-R1 (DONE), ARCHCM-R2 (DONE — Option A). NOT a blocker for anything else.

## Risk notes
YELLOW behavior-preserving signature refactor: touches `submitOpenCrawl` + ~5 callers
(`agent_actions.go` ×3, `direct_post_intake.go` ×1, `crawl_scheduler.go`). Must preserve
the founder semantics checklist (ARCHCM4 §10) exactly — this slice changes only how the
inputs are passed, not what the runtime does. Characterization tests for the
arg→typed resolution before the refactor.

## Validation
go build/vet/test ./cmd/scraper/... ; scripts/go_cognitive_check.sh ;
scripts/check_file_size.py ; ai_validate.sh ; git diff --check.

## Done criteria
Crawl entry points take typed params; arg/prompt parsing isolated in a cmd facade;
behavior identical (all ARCHCM4 §10 invariants hold); tests green; no package move yet.
