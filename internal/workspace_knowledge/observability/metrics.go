// Package observability is the Layer-7 port for the Workspace
// Knowledge OS. It records sync outcomes, retrieval events, and final
// outcomes so the Operator Replay UI can show "exactly which assets
// were retrieved and why" for every AI action.
//
// This package contains the CONTRACT only. The concrete implementation
// lands in Phase D of the roadmap and writes to the knowledge_metrics
// table joined with prompt_logs. See [specs/WORKSPACE_KNOWLEDGE_OS.md §9].
//
// In tests and the early MVP, callers can wire [NoOp] as a stand-in.
package observability

import (
	"context"

	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Metrics is the recording surface. Implementations write to whatever
// backend the deployment uses (SQLite table, Prometheus, OpenTelemetry).
type Metrics interface {
	// RecordSync logs the outcome of one ingestor.Sync call.
	RecordSync(ctx context.Context, orgID int64, sourceType sources.SourceType, assetsSeen, assetsCreated, assetsUpdated, assetsRejected int, durationMs int64, errs int)

	// RecordRetrieval logs one retrieval event for an org. retrievalID
	// is a caller-supplied opaque token that ties this event to the
	// downstream RecordOutcome call (when the generated comment is
	// approved, sent, or rejected).
	RecordRetrieval(ctx context.Context, orgID int64, retrievalID, query string, hits []retrieval.Hit, generatedAction string)

	// RecordOutcome closes the loop. outcome is one of:
	// "approved" | "sent" | "rejected" | "failed" | "converted".
	// The Operator Replay UI joins this against the matching
	// RecordRetrieval entry.
	RecordOutcome(ctx context.Context, orgID int64, retrievalID, outcome string)
}

// NoOp is a Metrics implementation that does nothing. Used in tests
// and in the early MVP before the Phase-D backend lands. Callers
// inject this so they do not need to nil-check every recording call
// site.
type NoOp struct{}

func (NoOp) RecordSync(context.Context, int64, sources.SourceType, int, int, int, int, int64, int) {
}
func (NoOp) RecordRetrieval(context.Context, int64, string, string, []retrieval.Hit, string) {}
func (NoOp) RecordOutcome(context.Context, int64, string, string)                            {}
