package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// Build a retrieval event with full trace + budget so the soak
// metric extractor has something to chew on.
func seedSoakRetrieval(t *testing.T, db *Store, orgID int64, retrievalID string, semanticScore, droppedByCap float64, hadFallback bool) {
	t.Helper()
	trace := retrieval.Trace{
		SearcherImpl: "rrf-v1",
		Selected: []retrieval.ScoredHit{
			{AssetID: 1, Title: "X", Score: 0.8, Breakdown: retrieval.ScoreBreakdown{Semantic: semanticScore}},
		},
		TotalByReason: map[retrieval.RejectionReason]int{},
	}
	if hadFallback {
		trace.TotalByReason["fallback_primary_timeout"] = 1
	}
	budget := retrieval.AssemblyBudget{
		DroppedByCap:    int(droppedByCap),
		EstimatedTokens: 200,
	}
	db.RecordKnowledgeRetrievalWithTrace(context.Background(), orgID, retrievalID, "q", "comment_drafted", trace, budget)
}

func TestGetKnowledgeSoakMetricsForOrg(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	seedSoakRetrieval(t, db, 7, "r1", 0.85, 1, false)
	seedSoakRetrieval(t, db, 7, "r2", 0.75, 2, false)
	seedSoakRetrieval(t, db, 7, "r3", 0.0, 0, true) // fallback case, no semantic

	m, err := db.GetKnowledgeSoakMetricsForOrg(ctx, 7, 24)
	if err != nil {
		t.Fatalf("GetKnowledgeSoakMetricsForOrg: %v", err)
	}
	if m.TotalRetrievals != 3 {
		t.Errorf("TotalRetrievals: got %d want 3", m.TotalRetrievals)
	}
	if m.HitRate != 1.0 {
		t.Errorf("HitRate: got %.2f want 1.0", m.HitRate)
	}
	// fallback rate = 1/3
	if m.FallbackRate < 0.32 || m.FallbackRate > 0.34 {
		t.Errorf("FallbackRate: got %.3f want ~0.333", m.FallbackRate)
	}
	// avg semantic = (0.85 + 0.75) / 2 = 0.80
	if m.AvgSemanticScore < 0.79 || m.AvgSemanticScore > 0.81 {
		t.Errorf("AvgSemanticScore: got %.3f want ~0.80", m.AvgSemanticScore)
	}
}

// Cross-org isolation: org-1's events invisible to org-2 metrics.
func TestSoakMetrics_TenantScope(t *testing.T) {
	db := newKnowledgeTestStore(t)
	seedSoakRetrieval(t, db, 1, "r1_org1", 0.8, 0, false)
	seedSoakRetrieval(t, db, 2, "r2_org2", 0.6, 0, false)

	m1, _ := db.GetKnowledgeSoakMetricsForOrg(context.Background(), 1, 24)
	if m1.TotalRetrievals != 1 {
		t.Errorf("org-1 should see 1 retrieval; got %d", m1.TotalRetrievals)
	}

	m2, _ := db.GetKnowledgeSoakMetricsForOrg(context.Background(), 2, 24)
	if m2.TotalRetrievals != 1 {
		t.Errorf("org-2 should see 1 retrieval; got %d", m2.TotalRetrievals)
	}
}

// Embedding model drift: distinct model versions are surfaced.
func TestSoakMetrics_EmbeddingModelDrift(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)

	// 2 assets with different embedding model versions → drift signal.
	a1, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_1", "X"))
	a2, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_2", "Y"))
	_, _ = db.ExecContext(ctx,
		`UPDATE knowledge_assets SET embedding_model_version = ? WHERE id = ?`, "openai:text-embedding-3-small:v1", a1.ID)
	_, _ = db.ExecContext(ctx,
		`UPDATE knowledge_assets SET embedding_model_version = ? WHERE id = ?`, "openai:text-embedding-3-large:v1", a2.ID)

	m, err := db.GetKnowledgeSoakMetricsForOrg(ctx, 7, 24)
	if err != nil {
		t.Fatalf("GetKnowledgeSoakMetricsForOrg: %v", err)
	}
	if m.DistinctEmbeddingModels != 2 {
		t.Errorf("DistinctEmbeddingModels: got %d want 2", m.DistinctEmbeddingModels)
	}
}

// Empty window: no retrievals → all-zero metrics, no panic.
func TestSoakMetrics_EmptyWindow(t *testing.T) {
	db := newKnowledgeTestStore(t)
	m, err := db.GetKnowledgeSoakMetricsForOrg(context.Background(), 7, 24)
	if err != nil {
		t.Fatalf("empty-window: %v", err)
	}
	if m.TotalRetrievals != 0 {
		t.Errorf("empty workspace: got %d retrievals", m.TotalRetrievals)
	}
	if m.HitRate != 0 || m.FallbackRate != 0 {
		t.Errorf("empty rates should be 0; got hit=%v fallback=%v", m.HitRate, m.FallbackRate)
	}
}

// Failure mode: malformed data_json must not crash the aggregator.
// (Production scenario E: stale/corrupted events from old schema.)
func TestSoakMetrics_MalformedEventsToleratedGracefully(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()

	// Insert one good event + one bad one (raw SQL bypass).
	seedSoakRetrieval(t, db, 7, "good", 0.8, 0, false)
	bad, _ := json.Marshal(map[string]any{"completely": "different shape"})
	_, _ = db.ExecContext(ctx, `INSERT INTO knowledge_events (org_id, event_type, retrieval_id, data_json) VALUES (?, ?, ?, ?)`,
		7, "retrieval", "bad", string(bad))

	m, err := db.GetKnowledgeSoakMetricsForOrg(ctx, 7, 24)
	if err != nil {
		t.Fatalf("malformed events crashed: %v", err)
	}
	if m.TotalRetrievals != 2 {
		t.Errorf("malformed event still counted; got %d, want 2", m.TotalRetrievals)
	}
}
