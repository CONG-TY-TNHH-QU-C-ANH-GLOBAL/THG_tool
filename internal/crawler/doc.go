// Package crawler marks the target crawler / crawl-execution boundary: crawl jobs,
// scheduling, and posts/groups/comments ingestion for the distributed crawler role.
//
// Architecture role: APPLICATION/INFRASTRUCTURE — see
// docs/architecture/ARCHITECTURE_STANDARD.md §3 and MODULE_BOUNDARIES.md.
//
//   - Allowed imports (conceptual): domain/application packages, store/crawl + jobs
//     via approved boundaries, ports/adapters, models, stdlib.
//   - Forbidden imports (conceptual): HTTP/server transport handlers (internal/server*,
//     internal/drivers/{http,telegram,connector}) — enforced warn-only by
//     WORKER_NO_TRANSPORT; drivers/*.
//
// SCAFFOLD ONLY (Phase A): boundary marker; no runtime logic lives here. Crawler
// pieces currently live under internal/jobs, internal/jobhandlers,
// internal/store/crawl, and cmd/worker (see MODULE_OWNERSHIP.yml). Code migrates here
// only via a reviewed refactor — do not add or move runtime logic casually.
package crawler
