// Domain: knowledge (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Operator-state setters MUST NOT mark embedding pending.
// Load-bearing invariant: pinning, hiding, or boosting an asset is
// an operator action that does NOT change the embedding-input text.
// If these flipped status to 'pending', every operator click would
// trigger a wasted re-embedding cycle.
func TestSetters_DoNotMarkEmbeddingPending(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)

	// Insert an asset and let the worker "process" it: simulate
	// completed embedding by writing status='generated' directly.
	a, err := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_1", "Cat Tee"))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE knowledge_assets SET embedding_status='generated', embedding_model_version='test:v1' WHERE id=? AND org_id=?`,
		a.ID, 7); err != nil {
		t.Fatalf("mark generated: %v", err)
	}

	// Operator actions: pin, boost, approve, hide. NONE should flip
	// embedding back to pending.
	if err := db.SetKnowledgeAssetPinned(ctx, a.ID, 7, true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if err := db.SetKnowledgeAssetBoost(ctx, a.ID, 7, 80); err != nil {
		t.Fatalf("SetBoost: %v", err)
	}
	if err := db.SetKnowledgeAssetState(ctx, a.ID, 7, assets.StateApproved); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	// Embedding must still be 'generated'.
	var status string
	if err := db.QueryRowContext(ctx,
		`SELECT embedding_status FROM knowledge_assets WHERE id=? AND org_id=?`, a.ID, 7,
	).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "generated" {
		t.Errorf("operator setters wrongly flipped status to %q; want 'generated'", status)
	}
}

// Re-Upserting the SAME content MUST NOT regress status from
// 'generated' back to 'pending'. The hash-comparison hook protects
// against re-embedding loops.
func TestUpsert_UnchangedContent_PreservesGeneratedStatus(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)

	a, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_1", "Cat Tee"))
	// Simulate worker completion.
	if _, err := db.ExecContext(ctx,
		`UPDATE knowledge_assets SET embedding_status='generated', embedding_model_version='test:v1' WHERE id=?`,
		a.ID); err != nil {
		t.Fatalf("seed generated: %v", err)
	}

	// Re-Upsert with same content.
	again := newTestAsset(7, sid, "ext_1", "Cat Tee")
	if _, err := db.UpsertKnowledgeAsset(ctx, again); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}

	var status string
	_ = db.QueryRowContext(ctx,
		`SELECT embedding_status FROM knowledge_assets WHERE id=?`, a.ID).Scan(&status)
	if status != "generated" {
		t.Errorf("unchanged content should not re-trigger pending; got %q", status)
	}
}

// CHANGED content MUST flip status back to pending. Without this the
// worker would never re-embed updated assets and the catalog would
// drift.
func TestUpsert_ChangedTitle_FlipsBackToPending(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)

	a, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_1", "Cat Tee Original"))
	if _, err := db.ExecContext(ctx,
		`UPDATE knowledge_assets SET embedding_status='generated' WHERE id=?`, a.ID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// New version with different title.
	changed := newTestAsset(7, sid, "ext_1", "Cat Tee Premium")
	if _, err := db.UpsertKnowledgeAsset(ctx, changed); err != nil {
		t.Fatalf("re-upsert changed: %v", err)
	}

	var status string
	_ = db.QueryRowContext(ctx,
		`SELECT embedding_status FROM knowledge_assets WHERE id=?`, a.ID).Scan(&status)
	if status != "pending" {
		t.Errorf("text change should flip status to pending; got %q", status)
	}
}

// RecordEmbeddingAttempt: incrementing past MaxAttempts flips status
// to 'failed'. Operator action is then required to reset.
func TestRecordEmbeddingAttempt_FailsAfterMaxAttempts(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)
	a, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_1", "Test"))

	// 3 attempts at MaxAttempts=3 → failed.
	for range 3 {
		if err := db.RecordEmbeddingAttempt(ctx, a.ID, 7, "transient err", 3); err != nil {
			t.Fatalf("RecordEmbeddingAttempt: %v", err)
		}
	}
	var status string
	var attempts int
	_ = db.QueryRowContext(ctx,
		`SELECT embedding_status, embedding_attempts FROM knowledge_assets WHERE id=?`, a.ID).
		Scan(&status, &attempts)
	if status != "failed" {
		t.Errorf("after MaxAttempts, status should be 'failed'; got %q", status)
	}
	if attempts != 3 {
		t.Errorf("attempts: got %d want 3", attempts)
	}
}

// ResetEmbeddingFailures clears failed rows back to pending.
func TestResetEmbeddingFailures(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)
	a, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_1", "Test"))
	// Force into failed state.
	for range 3 {
		_ = db.RecordEmbeddingAttempt(ctx, a.ID, 7, "x", 3)
	}

	n, err := db.ResetEmbeddingFailures(ctx, 7)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if n != 1 {
		t.Errorf("reset count: got %d want 1", n)
	}

	var status string
	_ = db.QueryRowContext(ctx, `SELECT embedding_status FROM knowledge_assets WHERE id=?`, a.ID).Scan(&status)
	if status != "pending" {
		t.Errorf("after reset, status should be 'pending'; got %q", status)
	}
}

// Cross-org isolation: RecordEmbeddingAttempt on another org's asset
// silently no-ops (row not found via WHERE org_id).
func TestRecordEmbeddingAttempt_ForeignOrgIsIgnored(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 1)
	a, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(1, sid, "ext_1", "Owned by org 1"))

	// Org 2 tries to fail org 1's asset.
	if err := db.RecordEmbeddingAttempt(ctx, a.ID, 2, "hijack", 3); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var attempts int
	_ = db.QueryRowContext(ctx, `SELECT embedding_attempts FROM knowledge_assets WHERE id=?`, a.ID).Scan(&attempts)
	if attempts != 0 {
		t.Errorf("foreign-org write leaked: attempts=%d", attempts)
	}
}

// EmbeddingStats: round-trips counts by status.
func TestGetEmbeddingStatsForOrg(t *testing.T) {
	db := newKnowledgeTestStore(t)
	ctx := context.Background()
	sid := mustSetupSource(t, db, 7)

	// 3 pending (default), 1 generated, 1 failed.
	for i, st := range []string{"pending", "pending", "pending", "generated", "failed"} {
		a, _ := db.UpsertKnowledgeAsset(ctx, newTestAsset(7, sid, "ext_"+itoa(i), "row "+itoa(i)))
		if st != "pending" {
			_, _ = db.ExecContext(ctx,
				`UPDATE knowledge_assets SET embedding_status=? WHERE id=?`, st, a.ID)
		}
	}
	stats, err := db.GetEmbeddingStatsForOrg(ctx, 7)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Pending != 3 {
		t.Errorf("Pending: got %d want 3", stats.Pending)
	}
	if stats.Generated != 1 {
		t.Errorf("Generated: got %d want 1", stats.Generated)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed: got %d want 1", stats.Failed)
	}
}

// pgVectorLiteral: format must be parseable by pgvector. The text
// representation is "[1.0,2.0,3.0]" — comma-separated, square-bracket-wrapped.
func TestPGVectorLiteral(t *testing.T) {
	cases := []struct {
		in   []float32
		want string
	}{
		{[]float32{}, "[]"},
		{[]float32{1.0}, "[1]"},
		{[]float32{1.0, 2.0, 3.0}, "[1,2,3]"},
		{[]float32{0.5, -0.25}, "[0.5,-0.25]"},
	}
	for _, c := range cases {
		if got := pgVectorLiteral(c.in); got != c.want {
			t.Errorf("pgVectorLiteral(%v) = %q; want %q", c.in, got, c.want)
		}
	}
}
