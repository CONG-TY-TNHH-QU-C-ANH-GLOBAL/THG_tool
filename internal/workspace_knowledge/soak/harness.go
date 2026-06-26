package soak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/fallback"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/hybrid"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/rrf"
)

// Harness orchestrates one soak run end-to-end. It is the single
// entry point the test suite + operator CLI both use.
//
// Production-vs-test split:
//
//	test     : Harness{Store: tempSQLite, Embedder: ClusteredEmbedder, Catalog: RealisticCatalog()}
//	prod-soak: Harness{Store: realPG,    Embedder: OpenAIEmbedder,    Catalog: realLoadedCatalog}
//
// Same Run() method; same Report shape; same assertions.
type Harness struct {
	Store    *store.Store
	Embedder embedding.Embedder
	Catalog  []CatalogFixture
	Prompts  []LeadPrompt
	OrgID    int64

	// TopK retrieval cap per query. Default 5 (matches production
	// runtime config; matches the "top 5 candidates" assembly cap).
	TopK int

	// Search variant. "rrf" composes hybrid + simulated-vector
	// (since real pgvector requires PG which isn't available in
	// SQLite test runs); "hybrid" runs hybrid only. Operator soak
	// against real PG sets this to "pgvector-rrf".
	SearcherVariant string
}

// Run executes the full soak flow:
//
//  1. Build the test catalog (ingest into store, mark all pending).
//  2. Run embedding worker to generate vectors for the catalog.
//  3. Build the searcher (per SearcherVariant).
//  4. For each prompt: retrieve, measure outcome.
//  5. Inject failure modes A-F and validate graceful behaviour.
//  6. Compute verdict + return Report.
//
// Returns the Report alongside an error if the SOAK ITSELF failed
// (couldn't ingest, couldn't generate embeddings, etc.). A normal
// soak completion with FAIL verdicts inside the report returns
// (report, nil) — the operator decides what to do with the result.
func (h *Harness) Run(ctx context.Context) (*Report, error) {
	if h.Store == nil {
		return nil, errors.New("soak: store is required")
	}
	if h.Embedder == nil {
		return nil, errors.New("soak: embedder is required")
	}
	if len(h.Catalog) == 0 || len(h.Prompts) == 0 {
		return nil, errors.New("soak: catalog and prompts are required")
	}
	if h.OrgID == 0 {
		h.OrgID = 7777 // dedicated soak-only org so prod data is never touched
	}
	if h.TopK == 0 {
		h.TopK = 5
	}
	if h.SearcherVariant == "" {
		h.SearcherVariant = "rrf"
	}

	report := &Report{
		GeneratedAt: time.Now().UTC(),
		HarnessConfig: HarnessConfig{
			EmbedderModel:       h.Embedder.ModelVersion(),
			EmbeddingDimensions: h.Embedder.Dimensions(),
			SearcherDescription: h.SearcherVariant,
			TopK:                h.TopK,
			MinConfidence:       rrf.DefaultMinConfidence,
		},
		AssetsByType: map[string]int{},
		FallbackBehaviour: FallbackMetrics{
			ByReason: map[string]int{},
		},
	}

	// --- Step 1: Ingest catalog ---
	sourceID, err := h.setupSource(ctx)
	if err != nil {
		return report, fmt.Errorf("setup source: %w", err)
	}
	h.ingestCatalog(ctx, report, sourceID)

	// --- Step 2: Run embedding pipeline ---
	h.runEmbeddingPipeline(ctx, report)

	// --- Step 3: Build the searcher ---
	wrap, err := h.buildSearcher()
	if err != nil {
		return report, err
	}

	// --- Step 4: Run prompts, collect outcomes ---
	for _, prompt := range h.Prompts {
		outcome := h.runOnePrompt(ctx, wrap, prompt)
		report.PromptOutcomes = append(report.PromptOutcomes, outcome)
		if outcome.FellBackTo != "" {
			report.FallbackBehaviour.FallbackInvocations++
			report.FallbackBehaviour.ByReason[outcome.FellBackTo]++
		}
	}
	report.FallbackBehaviour.TotalQueries = len(h.Prompts)
	if report.FallbackBehaviour.TotalQueries > 0 {
		report.FallbackBehaviour.FallbackRate =
			float64(report.FallbackBehaviour.FallbackInvocations) / float64(report.FallbackBehaviour.TotalQueries)
	}

	// Quality aggregates.
	report.Quality = computeQualityAggregates(report.PromptOutcomes)

	// Replay health.
	report.ReplayHealth = h.measureReplayHealth(ctx)

	// Stale detection.
	report.StaleDetection = h.measureStale(ctx)

	// --- Step 5: Failure-mode scenarios ---
	report.FailureModes = h.runFailureModes(ctx, sourceID)

	// --- Step 6: Operator-trust verdict ---
	report.OperatorTrust = computeTrustVerdict(report)

	return report, nil
}

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

// detectFallback returns the first fallback_primary_* reason recorded in the trace,
// or "" if the primary searcher served the query.
func detectFallback(trace retrieval.Trace) string {
	for r := range trace.TotalByReason {
		if strings.HasPrefix(string(r), "fallback_primary_") {
			return string(r)
		}
	}
	return ""
}

// scanHitSignals collects the retrieved titles plus the non-negotiable compliance/hidden
// leak lists (banned-claim / hidden assets must never surface). Verbatim extraction.
func scanHitSignals(hits []retrieval.Hit) (titles, complianceLeaks, hiddenLeaks []string) {
	for _, hit := range hits {
		if hit.Asset == nil {
			continue
		}
		titles = append(titles, hit.Asset.Title)
		// SAFETY CHECKS — these are the non-negotiables.
		if hit.Asset.Type == assets.AssetBannedClaim {
			complianceLeaks = append(complianceLeaks, hit.Asset.Title)
		}
		if hit.Asset.State == assets.StateHidden {
			hiddenLeaks = append(hiddenLeaks, hit.Asset.Title)
		}
	}
	return titles, complianceLeaks, hiddenLeaks
}

// soakVerdict applies the precision-dominant verdict policy. PRECISION is the dominant
// quality signal, NOT raw score: different searchers produce wildly different score scales
// (hybrid in [0,1], RRF in [0,~0.05] from 1/(60+rank)), so cross-scale comparison would be
// apples-to-oranges. BelowMinScore stays as report observability metadata, not a gate.
//
//	FAIL      any compliance / hidden leak (non-negotiable)
//	FAIL      expected intent + hits returned but precision == 0 (wrong stuff surfaced)
//	DEGRADED  expected intent + no hits (orchestrator should ask for clarification)
//	DEGRADED  precision < 0.4 (weak relevance)
//	PASS      otherwise — including "no intent → no hits expected"
func soakVerdict(out PromptOutcome, intentTags []string, hitCount int) string {
	switch {
	case len(out.ComplianceLeaks) > 0 || len(out.HiddenLeaks) > 0:
		return "FAIL"
	case len(intentTags) > 0 && hitCount > 0 && out.PrecisionAtK == 0:
		return "FAIL"
	case len(intentTags) > 0 && hitCount == 0:
		return "DEGRADED"
	case len(intentTags) > 0 && out.PrecisionAtK < 0.4:
		return "DEGRADED"
	default:
		return "PASS"
	}
}

// measureReplayHealth: verify every recent retrieval event has a
// well-formed trace. Reads from knowledge_events directly via the
// existing ListKnowledgeReplayEventsForOrg path.
func (h *Harness) measureReplayHealth(ctx context.Context) ReplayHealth {
	rh := ReplayHealth{}
	events, err := h.Store.Knowledge().ListReplayEventsForOrg(ctx, h.OrgID, "", 100)
	if err != nil {
		return rh
	}
	for _, ev := range events {
		rh.TracesProduced++
		var parsed retrieval.Trace
		if len(ev.Trace) > 0 {
			_ = json.Unmarshal(ev.Trace, &parsed)
		}
		complete := true
		if parsed.SearcherImpl == "" {
			rh.MissingSearcherImpl++
			complete = false
		}
		if len(parsed.Selected) == 0 && parsed.CandidatesConsidered > 0 {
			rh.MissingSelected++
			complete = false
		}
		if complete {
			rh.TracesComplete++
		}
	}
	if rh.TracesProduced > 0 {
		rh.CompletenessRate = float64(rh.TracesComplete) / float64(rh.TracesProduced)
	}
	return rh
}

// measureStale: stale asset detection using the existing
// CountStaleKnowledgeAssetsForOrg query.
func (h *Harness) measureStale(ctx context.Context) StaleMetrics {
	s := StaleMetrics{
		TotalAssets: len(h.Catalog),
	}
	if stale, err := h.Store.Knowledge().CountStaleAssetsForOrg(ctx, h.OrgID, 30); err == nil {
		s.StalePast30d = stale
	}
	// Never-retrieved vs. fresh: derive from Stats (which counts
	// retrieval_count_30d > 0 vs == 0).
	if ks, err := h.Store.Knowledge().GetStatsForOrg(ctx, h.OrgID); err == nil {
		s.NeverRetrieved = ks.TotalAssets - len(ks.TopRetrieved)
	}
	return s
}

// isTraceComplete is the schema check the soak applies to every
// trace it observes. Goal directive PR-2 §3 — additive-compatible
// means OLD events with missing fields are tolerated, but NEW
// events MUST be complete.
func isTraceComplete(t retrieval.Trace) bool {
	if t.SearcherImpl == "" {
		return false
	}
	// CandidatesConsidered may be 0 for empty-result queries; not a
	// completeness signal.
	return true
}
