package soak

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/fallback"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/hybrid"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/rrf"
)

// Per-stage soak pipeline steps (ingest, embed, build searcher, run one prompt).
// Split from harness.go; same package, behavior unchanged.

// ingestCatalog upserts the fixture catalog into the soak source and tallies catalog
// stats on the report. Verbatim extraction of Run's former Step-1 loop.
func (h *Harness) ingestCatalog(ctx context.Context, report *Report, sourceID int64) {
	for _, fx := range h.Catalog {
		a := &assets.Asset{
			OrgID:       h.OrgID,
			SourceID:    sourceID,
			ExternalID:  fx.ExternalID,
			Type:        fx.Type,
			Title:       fx.Title,
			Description: fx.Description,
			Tags:        fx.Tags,
			Payload:     fx.PayloadJSON(),
			State:       fx.State,
			Pinned:      fx.Pinned,
			Boost:       fx.Boost,
		}
		if a.State == "" {
			a.State = assets.StateApproved
		}
		saved, err := h.Store.Knowledge().UpsertAsset(ctx, a)
		if err != nil {
			report.Notes = append(report.Notes, fmt.Sprintf("ingest %s: %v", fx.ExternalID, err))
			continue
		}
		// Operator-state setters bypass the embedding hook — but
		// UpsertKnowledgeAsset DID mark embedding_status='pending'
		// because the asset is fresh. Confirm via the typed asset.
		_ = saved
		report.CatalogSize++
		report.AssetsByType[string(fx.Type)]++
	}
}

// runEmbeddingPipeline drains the pending embedding queue (bounded) and records the
// resulting embedding stats on the report. Verbatim extraction of Run's former Step 2.
func (h *Harness) runEmbeddingPipeline(ctx context.Context, report *Report) {
	worker := embedding.NewWorker(h.Store.Knowledge(), h.Embedder)
	worker.BatchSize = 32
	// Drain the pending queue. Loop until idle (or safety cap).
	for range 50 {
		n, err := worker.Tick(ctx)
		if err != nil {
			report.Notes = append(report.Notes, fmt.Sprintf("embed tick: %v", err))
			break
		}
		if n == 0 {
			break
		}
	}
	stats, err := h.Store.Knowledge().GetEmbeddingStatsForOrg(ctx, h.OrgID)
	if err == nil {
		report.EmbeddingsGenerated = stats.Generated
		report.EmbeddingsPending = stats.Pending
		report.EmbeddingsFailed = stats.Failed
	}
}

// buildSearcher composes the soak searcher for the configured variant and wraps it in the
// fallback observer. Real pgvector is unavailable in SQLite; the rrf variant simulates the
// pgvector pathway via the deterministic mock semantic searcher (full pipeline behaviour,
// no external dependency). Verbatim extraction of Run's former Step 3.
func (h *Harness) buildSearcher() (retrieval.Searcher, error) {
	hybridSearcher := hybrid.New(h.Store.Knowledge())
	var primarySearcher retrieval.Searcher
	switch h.SearcherVariant {
	case "hybrid":
		primarySearcher = hybridSearcher
	case "rrf":
		// Compose RRF over hybrid + the mock semantic searcher.
		semantic := newMockSemanticSearcher(h.Store.Knowledge(), h.Embedder)
		primarySearcher = rrf.New(hybridSearcher, semantic)
	default:
		return nil, fmt.Errorf("soak: unknown SearcherVariant %q", h.SearcherVariant)
	}
	// Wrap in fallback so we can observe whether it ever fires under
	// healthy conditions (it shouldn't).
	return fallback.New(primarySearcher, hybridSearcher), nil
}

// setupSource creates the soak source row that owns the fixture assets.
func (h *Harness) setupSource(ctx context.Context) (int64, error) {
	// Lazy import of sources types to avoid bloating the imports
	// list; use json.RawMessage for the config blob.
	cfgJSON := json.RawMessage(`{"description":"soak harness source"}`)
	src, err := h.Store.Knowledge().UpsertSource(ctx, mustValidSoakSource(h.OrgID, cfgJSON))
	if err != nil {
		return 0, err
	}
	return src.ID, nil
}

// runOnePrompt executes one query and computes the outcome.
func (h *Harness) runOnePrompt(ctx context.Context, searcher retrieval.Searcher, prompt LeadPrompt) PromptOutcome {
	started := time.Now()
	hits, trace, err := searcher.TopKWithTrace(ctx, h.OrgID, prompt.Text, retrieval.SearchFilter{}, h.TopK)
	latency := time.Since(started)

	// Persist the trace through the same path the production runtime
	// uses — RecordKnowledgeRetrievalWithTrace — so the soak's Replay
	// Health measurement actually exercises the knowledge_events
	// substrate. Without this, the soak validates retrieval but NOT
	// observability, which defeats the goal directive's "operator
	// trust" criterion. The retrieval_id is generated per call so
	// the events table sees realistic identifiers.
	retrievalID := newSoakRetrievalID()
	h.Store.Knowledge().RecordRetrievalWithTrace(ctx, h.OrgID, retrievalID, prompt.Text, "soak_query", trace, retrieval.AssemblyBudget{
		AssembledProducts: len(hits),
	})

	out := PromptOutcome{
		Prompt:         prompt.Text,
		Language:       prompt.Lang,
		ExpectedIntent: prompt.IntentTags,
		LatencyMs:      latency.Milliseconds(),
		SearcherImpl:   trace.SearcherImpl,
		TraceComplete:  isTraceComplete(trace),
	}

	if err != nil {
		out.Verdict = "FAIL"
		out.RetrievedTitles = []string{fmt.Sprintf("error: %v", err)}
		return out
	}

	out.FellBackTo = detectFallback(trace)
	out.RetrievedTitles, out.ComplianceLeaks, out.HiddenLeaks = scanHitSignals(hits)
	if len(hits) > 0 {
		out.TopScore = hits[0].Score
	}
	out.PrecisionAtK = h.computePrecisionAtK(hits, prompt.IntentTags)
	out.BelowMinScore = out.TopScore < prompt.MinScore
	out.Verdict = soakVerdict(out, prompt.IntentTags, len(hits))
	return out
}
