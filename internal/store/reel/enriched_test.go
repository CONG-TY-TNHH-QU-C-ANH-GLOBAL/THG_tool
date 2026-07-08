// Postgres-gated behavior tests for the enriched-format accessors
// (migration 0112). Gated on POSTGRES_PLATFORM_TEST_DSN via reeltest; the
// dialect-guard proofs (no DB needed) live in reel_dialect_guard_test.go.
package reel_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/reel"
	"github.com/thg/scraper/internal/store/reel/reeltest"
)

func TestReelEnriched_RoundTrip(t *testing.T) {
	s := reeltest.OpenStore(t)
	const org int64 = 5001
	reeltest.CleanupOrgs(t, s, org)
	ctx := context.Background()

	reelID, err := s.Reel().CreateReel(ctx, org, "enriched reel", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}

	// A fresh reel defaults to reel_type 'enriched' and empty keys.
	e, err := s.Reel().GetEnriched(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetEnriched: %v", err)
	}
	if e.ReelType != "enriched" {
		t.Fatalf("default reel_type = %q, want enriched", e.ReelType)
	}

	if err := s.Reel().SetSource(ctx, org, reelID, "src/clip.mp4", "audio"); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	if err := s.Reel().SetAvatarKey(ctx, org, reelID, "avatar/av.webm"); err != nil {
		t.Fatalf("SetAvatarKey: %v", err)
	}
	if err := s.Reel().SetFinalOutput(ctx, org, reelID, "final/out.mp4"); err != nil {
		t.Fatalf("SetFinalOutput: %v", err)
	}
	if err := s.Reel().AddCost(ctx, org, reelID, 0.30); err != nil {
		t.Fatalf("AddCost#1: %v", err)
	}
	if err := s.Reel().AddCost(ctx, org, reelID, 0.05); err != nil {
		t.Fatalf("AddCost#2: %v", err)
	}

	e, err = s.Reel().GetEnriched(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetEnriched (after writes): %v", err)
	}
	if e.SourceKey != "src/clip.mp4" || e.InputBranch != "audio" {
		t.Fatalf("source/branch = %q/%q", e.SourceKey, e.InputBranch)
	}
	if e.AvatarKey != "avatar/av.webm" || e.FinalOutputKey != "final/out.mp4" {
		t.Fatalf("avatar/final = %q/%q", e.AvatarKey, e.FinalOutputKey)
	}
	if e.TotalCostUSD < 0.349 || e.TotalCostUSD > 0.351 {
		t.Fatalf("total_cost_usd = %v, want ~0.35", e.TotalCostUSD)
	}
}

// TestReelEnriched_ClaimRenderIdempotent proves the money invariant: only
// the first claim wins; a retry with any key finds the slot taken and
// returns claimed=false, so no second paid render is ever started.
func TestReelEnriched_ClaimRenderIdempotent(t *testing.T) {
	s := reeltest.OpenStore(t)
	const org int64 = 5002
	reeltest.CleanupOrgs(t, s, org)
	ctx := context.Background()

	reelID, err := s.Reel().CreateReel(ctx, org, "reel", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}
	lease := time.Now().Add(12 * time.Minute)

	claimed, err := s.Reel().ClaimRender(ctx, org, reelID, "idem-1", lease)
	if err != nil || !claimed {
		t.Fatalf("first ClaimRender = (%v, %v), want (true, nil)", claimed, err)
	}
	claimed, err = s.Reel().ClaimRender(ctx, org, reelID, "idem-2", lease)
	if err != nil {
		t.Fatalf("second ClaimRender err: %v", err)
	}
	if claimed {
		t.Fatalf("second ClaimRender = true, want false (slot already taken)")
	}
}

// TestReelEnriched_CrossOrgNoop proves the enriched setters are org-scoped:
// a foreign org cannot read or mutate another org's reel.
func TestReelEnriched_CrossOrgNoop(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgA, orgB int64 = 5101, 5102
	reeltest.CleanupOrgs(t, s, orgA, orgB)
	ctx := context.Background()

	reelA, err := s.Reel().CreateReel(ctx, orgA, "org A", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel(orgA): %v", err)
	}

	if _, err := s.Reel().GetEnriched(ctx, orgB, reelA); err != sql.ErrNoRows {
		t.Fatalf("GetEnriched(orgB, orgA's id) = %v, want sql.ErrNoRows", err)
	}
	if err := s.Reel().SetSource(ctx, orgB, reelA, "forged", "audio"); err != nil {
		t.Fatalf("cross-org SetSource returned error: %v", err)
	}
	claimed, err := s.Reel().ClaimRender(ctx, orgB, reelA, "forged", time.Now())
	if err != nil {
		t.Fatalf("cross-org ClaimRender err: %v", err)
	}
	if claimed {
		t.Fatalf("cross-org ClaimRender = true, want false")
	}
	e, err := s.Reel().GetEnriched(ctx, orgA, reelA)
	if err != nil {
		t.Fatalf("GetEnriched(orgA): %v", err)
	}
	if e.SourceKey != "" {
		t.Fatalf("cross-org SetSource mutated orgA's reel: source_key = %q", e.SourceKey)
	}
}

// TestReelTranscript_RoundTripAndCrossOrg covers transcript create/read plus
// the composite-FK tenant guard (same proof shape as reel_scripts).
func TestReelTranscript_RoundTripAndCrossOrg(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgA, orgB int64 = 5201, 5202
	reeltest.CleanupOrgs(t, s, orgA, orgB)
	ctx := context.Background()

	reelA, err := s.Reel().CreateReel(ctx, orgA, "org A", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel(orgA): %v", err)
	}
	seg := `[{"text":"xin chao","from_ms":0,"to_ms":800}]`
	in := reel.TranscriptInput{Segments: seg, LangSrc: "vi", LangTgt: "en", Source: "whisper", CostUSD: 0.006}
	if _, err := s.Reel().CreateTranscript(ctx, orgA, reelA, in); err != nil {
		t.Fatalf("CreateTranscript(orgA): %v", err)
	}

	tr, err := s.Reel().GetLatestTranscript(ctx, orgA, reelA)
	if err != nil {
		t.Fatalf("GetLatestTranscript(orgA): %v", err)
	}
	if tr.Segments != seg || tr.Source != "whisper" || tr.LangSrc != "vi" {
		t.Fatalf("transcript roundtrip mismatch: %+v", tr)
	}

	// orgB cannot attach a transcript to orgA's reel (composite FK).
	if _, err := s.Reel().CreateTranscript(ctx, orgB, reelA, in); err == nil {
		t.Fatalf("CreateTranscript(orgB, orgA's reel) succeeded, want FK violation")
	}
	if _, err := s.Reel().GetLatestTranscript(ctx, orgB, reelA); err != sql.ErrNoRows {
		t.Fatalf("GetLatestTranscript(orgB, orgA's reel) = %v, want sql.ErrNoRows", err)
	}
}
