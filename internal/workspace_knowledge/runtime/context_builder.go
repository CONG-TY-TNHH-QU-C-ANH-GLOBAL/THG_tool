// Package runtime is the agent-runtime entry point into the
// Workspace Knowledge OS. It wires the Searcher (Layer 4) and the
// Assembler (Layer 5) into a single function the comment generator
// can call once per lead.
//
// The agent runtime imports THIS package, not the lower layers — so
// the lower-layer surface can evolve (swap LIKE-based naive search
// for pgvector, change the assembly format) without touching the
// generator. The contract is one function: [BuildForLead].
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assembly"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/fallback"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/hybrid"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/pgvector"
)

// AssetLister is the store surface the searcher needs. *store.Store
// satisfies this; tests use a fake. We re-declare it here (rather
// than import retrieval/naive's version) so this package compiles
// against any Searcher implementation.
type AssetLister interface {
	ListKnowledgeAssetsForOrg(ctx context.Context, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error)
}

// VectorCapableStore is the EXTENDED store surface needed for the
// pgvector path. *store.Store satisfies this when the dialect is
// Postgres AND pgvector is installed; otherwise HasPGVector returns
// false and the runtime sticks with hybrid-only.
type VectorCapableStore interface {
	AssetLister
	HasPGVector(ctx context.Context) bool
	QueryNearestVectors(ctx context.Context, orgID int64, queryVector []float32, modelVersion string, filter retrieval.VectorFilter, k int) ([]retrieval.VectorHit, error)
}

// RetrievalRecorder is the optional Phase-D observability surface.
// Provided by *store.Store when metrics are wired; tests pass nil
// (the builder is no-op when nil).
type RetrievalRecorder interface {
	IncrementKnowledgeAssetRetrieval(ctx context.Context, assetID, orgID int64) error
}

// TraceRecorder persists the explainability trace of a retrieval.
// The Operator Replay UI reads back through ListKnowledgeRetrievals.
// *store.Store satisfies this via RecordKnowledgeRetrievalWithTrace.
type TraceRecorder interface {
	RecordKnowledgeRetrievalWithTrace(
		ctx context.Context,
		orgID int64,
		retrievalID, query, generatedAction string,
		trace retrieval.Trace,
		budget retrieval.AssemblyBudget,
	)
}

// Builder is the runtime entry point. Construct once at startup and
// share — it is concurrency-safe and stateless beyond its
// dependencies.
type Builder struct {
	Searcher retrieval.Searcher
	// Recorder is optional. When non-nil, every asset returned in a
	// TopK result has its retrieval counter incremented. Failures are
	// logged and swallowed — a metric write should never break a sales
	// action.
	Recorder RetrievalRecorder
	// TraceRec is optional. When non-nil, the full explainability
	// trace (selected + rejected + score breakdowns + budget) is
	// recorded for the Operator Replay UI. Independent of Recorder
	// so callers can disable replay logging on hot paths without
	// losing the asset-level counters.
	TraceRec TraceRecorder
	// K is the number of hits to retrieve per call. Default 6 — enough
	// for one CTA + two products + two policies after assembly groups
	// them, with one slot of headroom for re-ranking.
	K int
}

// NewBuilder returns a Builder using the hybrid searcher.
//
// To opt into pgvector semantic retrieval, call [NewBuilderWithVector]
// instead — it inspects the store's HasPGVector capability and
// composes pgvector → hybrid via the fallback wrapper.
func NewBuilder(lister AssetLister) *Builder {
	return &Builder{
		Searcher: hybrid.New(lister),
		K:        6,
	}
}

// NewBuilderWithVector is the capability-driven constructor (goal
// directive PR-2 §4). When the store reports pgvector capability
// (Postgres + extension + embedding column), it wires:
//
//   primary  = pgvector.Searcher(store, embedder)
//   fallback = hybrid.Searcher(store)
//   wrap     = fallback.Searcher{Primary, Fallback}
//
// When pgvector capability is NOT present (SQLite, fresh PG without
// extension), it returns the plain hybrid searcher — same behavior
// as NewBuilder. The runtime caller does not branch on dialect; the
// builder does.
//
// NO feature flag. Detection happens here at boot, then the chosen
// Searcher is immutable for the Builder's lifetime.
func NewBuilderWithVector(ctx context.Context, store VectorCapableStore, embedder embedding.Embedder) *Builder {
	hybridSearcher := hybrid.New(store)
	if !store.HasPGVector(ctx) || embedder == nil {
		// No vector capability — hybrid is the whole show. The
		// fallback wrapper still goes around it so any future
		// addition of a "tertiary" path doesn't need to retouch
		// every call site. Primary=nil + Secondary=hybrid is the
		// "no primary configured" delegation path documented in
		// fallback.Searcher.TopKWithTrace.
		return &Builder{
			Searcher: fallback.New(nil, hybridSearcher),
			K:        6,
		}
	}
	pgv := pgvector.New(store, embedder)
	wrapped := fallback.New(pgv, hybridSearcher)
	wrapped.EmptinessTester = pgv
	return &Builder{
		Searcher: wrapped,
		K:        6,
	}
}

// BuildForLead returns the prompt block to inject for one lead.
//
// baseContext is the freeform business profile loaded by the legacy
// path (businessContextForOrg in cmd/scraper) — kept as the org-wide
// constant. The knowledge block is appended below it so the LLM
// sees:
//
//	BASE PROFILE (org-wide, freeform)
//	---
//	PRODUCTS:  ... retrieved for THIS lead ...
//	POLICIES:  ...
//	CTAs:      ...
//
// If retrieval returns nothing (org has no Knowledge OS configured),
// the function returns baseContext unchanged — the system stays
// backwards-compatible with workspaces that have not migrated yet.
func (b *Builder) BuildForLead(ctx context.Context, orgID int64, leadText, baseContext string) string {
	out, _ := b.BuildForLeadWithTrace(ctx, orgID, leadText, baseContext, "")
	return out
}

// BuildForLeadWithTrace is the explainability-aware variant. It does
// the same work as BuildForLead but returns the assembled prompt
// block AND the retrieval_id under which the trace was recorded. The
// caller threads retrievalID into the eventual RecordKnowledgeOutcome
// call so the Operator Replay UI can join the retrieval and outcome.
//
// generatedAction is a short label ("comment_drafted", "inbox_drafted")
// recorded alongside the trace. It is operator vocabulary — not the
// generated text itself.
func (b *Builder) BuildForLeadWithTrace(ctx context.Context, orgID int64, leadText, baseContext, generatedAction string) (string, string) {
	if b == nil || b.Searcher == nil || orgID <= 0 {
		return baseContext, ""
	}
	k := b.K
	if k <= 0 {
		k = 6
	}

	hits, trace, err := b.Searcher.TopKWithTrace(ctx, orgID, leadText, retrieval.SearchFilter{}, k)
	if err != nil {
		log.Printf("[knowledge.BuildForLead] org=%d retrieval failed: %v", orgID, err)
		return baseContext, ""
	}
	if len(hits) == 0 {
		return baseContext, ""
	}

	knowledgeBlock, budget := assembly.AssembleWithBudget(hits, assembly.AssembleOptions{
		MaxProducts:    2,
		IncludeMetrics: false,
	})
	if strings.TrimSpace(knowledgeBlock) == "" {
		return baseContext, ""
	}

	retrievalID := newRetrievalID()

	// Best-effort metrics. Failures are logged + swallowed — neither
	// counter writes nor trace persistence should ever abort a sales
	// action.
	if b.Recorder != nil {
		for _, h := range hits {
			if h.Asset == nil {
				continue
			}
			if err := b.Recorder.IncrementKnowledgeAssetRetrieval(ctx, h.Asset.ID, orgID); err != nil {
				log.Printf("[knowledge.BuildForLead] increment retrieval id=%d: %v", h.Asset.ID, err)
			}
		}
	}
	if b.TraceRec != nil {
		b.TraceRec.RecordKnowledgeRetrievalWithTrace(ctx, orgID, retrievalID, leadText, generatedAction, trace, budget)
	}

	if strings.TrimSpace(baseContext) == "" {
		return knowledgeBlock, retrievalID
	}
	return strings.TrimRight(baseContext, "\n") + "\n\n---\n\n" + knowledgeBlock, retrievalID
}

// newRetrievalID generates an opaque, sortable-enough identifier for
// joining a retrieval event with its eventual outcome event. 12 bytes
// of crypto-random hex = collision-free across realistic traffic.
// Prefix matches the convention in project_service_descriptor_and_access.md.
func newRetrievalID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read never fails on modern platforms, but if it does we
		// still need an ID; fall back to a stable empty-prefixed id
		// rather than panicking.
		return "ret_unknown"
	}
	return "ret_" + hex.EncodeToString(b[:])
}
