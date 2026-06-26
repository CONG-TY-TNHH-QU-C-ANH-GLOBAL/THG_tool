# AUTOPILOT_QUEUE

Claude must pick the first READY item unless the user names a different item.

## Rules

- One PR per queue item.
- Do not merge.
- Push only after validation passes.
- Stop on RED ambiguity.
- Do not chase unrelated Sonar backlog.
- Update the item status in the final report only; do not self-edit this file unless explicitly asked.

## Queue

### READY — PR31D: Facebook crawl session fake seam
Risk: YELLOW
Goal: Introduce the smallest fakeable seam for the session-acquire branch if needed.
Scope:
- internal/jobhandlers/facebook_crawl
- internal/runtime
- internal/livesession
Constraints:
- no real Chrome/browser/CDP/network in tests
- no broad Browser framework
- no package moves
Validation:
- go test ./internal/jobhandlers/... -run 'Facebook|Crawl|Session|Runtime|Human|Offline' -v
- go test ./...
- go build ./...
- go vet ./...
- bash scripts/check_import_boundaries.sh
- python scripts/check_file_size.py

### READY — PR31E: Facebook crawl readiness/runtime edge coverage
Risk: GREEN/YELLOW
Goal: Add missing characterization tests around not-ready/offline/human_required/failure mapping if existing seams allow.
Scope:
- internal/jobhandlers/facebook_crawl
- internal/readiness
- cmd/scraper crawl/readiness tests
Constraints:
- test-only preferred
- no runtime semantics change

### READY — PR32A: Product-path audit for Facebook automation operator UX
Risk: YELLOW
Goal: Audit and harden operator-visible status flow: readiness reason -> queue status -> execution result.
Scope:
- backend API/status payloads
- dashboard-facing response DTOs only if already existing
Constraints:
- no DTO/wire contract change unless explicitly reported
- characterization first

### BACKLOG — Sonar Ponytail cleanup batch
Risk: GREEN
Goal: Fix low-risk Sonar New Code issues only when explicitly requested.
Scope:
- issue-specific files only
