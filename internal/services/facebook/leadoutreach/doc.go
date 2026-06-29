// Package leadoutreach is the store-free Facebook lead-outreach execution usecase:
// the per-run, per-lead spine (target → coverage → generate → reason → quality
// screen → queue) plus its outcome formatters and lifecycle-aware copilot wording.
//
// It consumes narrow, consumer-owned ports (OutboundRecorder, LeadCoverageReader,
// LeadLifecycleReader) — the composition root (cmd/scraper) builds the store-backed
// adapters and the Config, then calls New. This package never imports internal/store,
// internal/server, or cmd; the concrete store still executes the queue write, so
// queue/dedup/policy semantics are unchanged.
package leadoutreach
