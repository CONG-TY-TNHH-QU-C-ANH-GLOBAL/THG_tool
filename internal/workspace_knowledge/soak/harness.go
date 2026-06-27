package soak

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
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
