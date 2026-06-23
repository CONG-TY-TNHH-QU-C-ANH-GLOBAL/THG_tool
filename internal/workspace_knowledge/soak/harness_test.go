package soak

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/store"
)

// TestSoak_RealisticCatalog_RRF is the FULL soak: it runs the
// retrieval substrate end-to-end against the realistic POD catalog
// + realistic lead prompts and asserts the operator-trust verdict
// is at least DEGRADED (>= 60 score). READY would be best but
// substrate-quality varies with the clustered embedder's coverage.
//
// This test serves three roles simultaneously:
//
//  1. Regression guard — if a future refactor breaks retrieval, this
//     fails before it ships.
//  2. Living documentation — the printed Report shows what the soak
//     measures, in operator vocabulary.
//  3. Production-soak template — the operator runs the same harness
//     in production by swapping ClusteredEmbedder → OpenAIEmbedder
//     and pointing at a real PG.
//
// The test ALWAYS writes the report to specs/knowledge/RETRIEVAL_SOAK_REPORT.md
// so the latest run's results are committable artefact. CI can diff
// this file across PRs to spot quality regressions before merge.
func TestSoak_RealisticCatalog_RRF(t *testing.T) {
	db := newSoakTestStore(t)
	defer db.Close()

	h := &Harness{
		Store:           db,
		Embedder:        NewClusteredEmbedder(),
		Catalog:         RealisticCatalog(),
		Prompts:         RealisticLeads(),
		SearcherVariant: "rrf",
		TopK:            5,
	}

	report, err := h.Run(context.Background())
	if err != nil {
		t.Fatalf("soak run failed at infrastructure level: %v", err)
	}

	// Always log the Markdown report to test output. Operators read
	// this during PR review.
	t.Log("\n" + report.ToMarkdown())

	// Write to specs/ so the most recent soak result is a tracked
	// artefact in the repo. Writes go to a relative path; tests run
	// from package dir, so we use a relative parent walk.
	if err := writeSoakArtefact(report); err != nil {
		t.Logf("warning: could not write soak artefact: %v", err)
	}

	// --- ASSERTIONS — what makes this a useful test, not just a logger ---

	// 1. Compliance: NEVER acceptable to surface a banned claim.
	for _, p := range report.PromptOutcomes {
		if len(p.ComplianceLeaks) > 0 {
			t.Errorf("COMPLIANCE LEAK on prompt %q: %v", p.Prompt, p.ComplianceLeaks)
		}
		if len(p.HiddenLeaks) > 0 {
			t.Errorf("HIDDEN-STATE LEAK on prompt %q: %v", p.Prompt, p.HiddenLeaks)
		}
	}

	// 2. Replay completeness: every trace must carry SearcherImpl.
	if report.ReplayHealth.TracesProduced > 0 {
		if report.ReplayHealth.CompletenessRate < 0.95 {
			t.Errorf("replay completeness too low: %.2f (want >= 0.95)", report.ReplayHealth.CompletenessRate)
		}
	}

	// 3. Failure modes: all six MUST pass.
	for _, fm := range report.FailureModes {
		if fm.Verdict != "PASS" {
			t.Errorf("failure mode %s (%s) did not PASS: %s", fm.ID, fm.Name, fm.Behaviour)
		}
	}

	// 4. Operator trust: at minimum DEGRADED. NOT_READY = substrate
	//    has a blocking issue and is not safe to deploy.
	if report.OperatorTrust.Verdict == "NOT_READY" {
		t.Errorf("operator trust verdict NOT_READY (score=%d); blocking issues: %v",
			report.OperatorTrust.Score, report.OperatorTrust.BlockingIssues)
	}

	// 5. Embedding pipeline did its job — at least 80% of catalog
	//    has vectors after the soak's worker drain.
	approvedAssetsRoughly := report.CatalogSize - report.AssetsByType["banned_claim"] - 2 /* hidden + pending */
	if approvedAssetsRoughly > 0 && report.EmbeddingsGenerated < int(float64(approvedAssetsRoughly)*0.8) {
		t.Errorf("embedding coverage too low: %d generated / %d approved-ish",
			report.EmbeddingsGenerated, approvedAssetsRoughly)
	}
}

// TestSoak_HybridOnly_NoVectorCapability validates the SQLite path:
// when the runtime detects no pgvector, retrieval still functions
// via hybrid alone. This is the most-deployed path (every dev /
// CI box) and a foundational invariant: even without semantic
// retrieval, the substrate must be usable.
func TestSoak_HybridOnly_NoVectorCapability(t *testing.T) {
	db := newSoakTestStore(t)
	defer db.Close()

	h := &Harness{
		Store:           db,
		Embedder:        NewClusteredEmbedder(),
		Catalog:         RealisticCatalog(),
		Prompts:         RealisticLeads(),
		SearcherVariant: "hybrid",
		TopK:            5,
	}
	report, err := h.Run(context.Background())
	if err != nil {
		t.Fatalf("hybrid-only soak failed: %v", err)
	}

	// On the hybrid-only path the operator-trust score is LOWER (no
	// semantic signal) but the verdict should still be at least
	// DEGRADED — i.e. compliance clean and replay observable.
	if report.OperatorTrust.Verdict == "NOT_READY" {
		t.Errorf("hybrid-only NOT_READY: %v", report.OperatorTrust.BlockingIssues)
	}
	for _, p := range report.PromptOutcomes {
		if len(p.ComplianceLeaks) > 0 {
			t.Errorf("hybrid-only compliance leak on %q: %v", p.Prompt, p.ComplianceLeaks)
		}
	}
}

// newSoakTestStore returns a fresh SQLite store rooted in t.TempDir
// so soak runs are isolated and self-cleaning. The store has the
// full Knowledge OS schema thanks to store.New running migrations
// at boot.
func newSoakTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "soak.db"))
	if err != nil {
		t.Fatalf("soak store init: %v", err)
	}
	return db
}

// writeSoakArtefact persists the latest soak Markdown report under
// specs/ so reviewers can diff it across PRs. Best-effort — failure
// is logged at the call site, never fails the test.
//
// The path is resolved relative to the package directory because Go
// tests run from there. We walk up to find specs/ to keep this
// robust when the harness moves.
func writeSoakArtefact(r *Report) error {
	// Best-guess specs dir relative to this package's runtime location.
	// We're in internal/workspace_knowledge/soak; specs/ is 3 levels up.
	candidates := []string{
		"../../../specs/knowledge/RETRIEVAL_SOAK_REPORT.md",
		"../../specs/knowledge/RETRIEVAL_SOAK_REPORT.md",
		"specs/knowledge/RETRIEVAL_SOAK_REPORT.md",
	}
	body := []byte(r.ToMarkdown())
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Dir(p)); err == nil {
			return os.WriteFile(p, body, 0o644)
		}
	}
	return nil // no specs dir found — silent skip is fine for ad-hoc runs
}

// Smoke test on the mock embedder: similar texts produce similar
// vectors. Sanity check that the clustering math actually works —
// if this fails, the soak's semantic-similarity proxy is broken.
func TestClusteredEmbedder_SimilarTextsCluster(t *testing.T) {
	emb := NewClusteredEmbedder()
	ctx := context.Background()

	pairs := []struct {
		a, b string
		sameCluster bool
	}{
		{"cat tee POD", "custom cat shirt", true},     // share cat + shirt + pod
		{"cat tee POD", "dog mug", false},             // disjoint clusters
		{"oversized anime gothic shirt", "edgy alt tee", true}, // share gothic + shirt
		{"shipping policy US", "fulfillment to America", true},  // share shipping/fulfillment + us
	}
	for _, p := range pairs {
		vecs, err := emb.Embed(ctx, []string{p.a, p.b})
		if err != nil {
			t.Fatalf("embed: %v", err)
		}
		sim := cosineSimilarity(vecs[0], vecs[1])
		if p.sameCluster && sim < 0.20 {
			t.Errorf("expected high similarity for %q vs %q; got %.3f", p.a, p.b, sim)
		}
		if !p.sameCluster && sim > 0.30 {
			t.Errorf("expected low similarity for %q vs %q; got %.3f", p.a, p.b, sim)
		}
	}
}

// Determinism: same input → same vector across calls. Failing this
// breaks the InputHash contract (worker would re-embed forever).
func TestClusteredEmbedder_Deterministic(t *testing.T) {
	emb := NewClusteredEmbedder()
	ctx := context.Background()
	v1, _ := emb.Embed(ctx, []string{"cat tee POD"})
	v2, _ := emb.Embed(ctx, []string{"cat tee POD"})
	if len(v1) != 1 || len(v2) != 1 {
		t.Fatal("embedder did not return expected count")
	}
	for i := range v1[0] {
		if v1[0][i] != v2[0][i] {
			t.Errorf("non-deterministic at dim %d: %v vs %v", i, v1[0][i], v2[0][i])
			break
		}
	}
}

// Verify the report's Markdown renders without truncation issues.
func TestReport_MarkdownRendering(t *testing.T) {
	r := &Report{
		HarnessConfig: HarnessConfig{
			SearcherDescription: "test",
			EmbedderModel:       "test:v1",
			EmbeddingDimensions: 8,
			TopK:                5,
		},
		Quality: QualityMetrics{
			MeanPrecisionAtK: 0.65,
			PassedPrompts:    8,
			FailedPrompts:    1,
		},
		PromptOutcomes: []PromptOutcome{
			{Prompt: "test prompt", Language: "en", Verdict: "PASS", TopScore: 0.9, PrecisionAtK: 0.8, LatencyMs: 12},
		},
		FailureModes: []FailureModeOutcome{
			{ID: "A", Name: "test", Verdict: "PASS", Behaviour: "ok"},
		},
		OperatorTrust: TrustVerdict{Score: 75, Verdict: "DEGRADED"},
	}
	md := r.ToMarkdown()
	mustContain := []string{
		"Soak Report",
		"Operator Trust",
		"Catalog",
		"Per-Prompt Outcomes",
		"Compliance",
		"Failure Mode",
	}
	for _, s := range mustContain {
		if !strings.Contains(md, s) {
			t.Errorf("markdown missing %q", s)
		}
	}
}
