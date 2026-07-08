// Orchestration tests for the enriched pipeline. These run FULLY LOCALLY —
// no Postgres, no Docker — against the in-memory store double and the fake
// provider ports, so `go test ./...` exercises the real orchestration logic
// everywhere. (Postgres SQL behavior is covered separately at the store
// layer in internal/store/reel, gated on POSTGRES_PLATFORM_TEST_DSN.)
package reel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/services/reel"
)

func newEnrichedFixture() (*reel.EnrichedService, *reel.FakeObjectStore, *reel.InMemoryStore) {
	store := reel.NewInMemoryStore()
	obj := reel.NewFakeObjectStore()
	deps := reel.EnrichedDeps{AvatarID: "av_test", VoiceID: "vo_test", AvatarPos: "bottom-right"}
	svc := reel.NewEnrichedService(store, obj, reel.FakeTranscriber{}, reel.FakeScriptEngine{}, reel.FakeAvatarRenderer{}, reel.FakeComposer{}, deps)
	return svc, obj, store
}

// prepareApproved: create reel, set source, prepare script, approve it —
// the "ready to render" starting state.
func prepareApproved(t *testing.T, svc *reel.EnrichedService, store *reel.InMemoryStore, orgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	reelID, err := store.CreateReel(ctx, orgID, "enriched", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}
	if err := store.SetSource(ctx, orgID, reelID, "src/clip.mp4", "audio"); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	script, err := svc.PrepareScript(ctx, orgID, reelID)
	if err != nil {
		t.Fatalf("PrepareScript: %v", err)
	}
	if err := store.ApproveScript(ctx, orgID, script.ID); err != nil {
		t.Fatalf("ApproveScript: %v", err)
	}
	return reelID
}

func TestEnriched_HappyPath(t *testing.T) {
	const org int64 = 6001
	svc, obj, store := newEnrichedFixture()
	ctx := context.Background()

	reelID := prepareApproved(t, svc, store, org)

	tr, err := store.GetLatestTranscript(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetLatestTranscript: %v", err)
	}
	if tr.Source != "whisper" {
		t.Fatalf("transcript source = %q, want whisper", tr.Source)
	}
	es, err := svc.GetEnrichedScript(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetEnrichedScript: %v", err)
	}
	if len(es.Subtitles) != 2 || es.AvatarScript == "" {
		t.Fatalf("enriched script = %+v, want 2 subtitles + avatar script", es)
	}

	if err := svc.RenderEnriched(ctx, org, reelID); err != nil {
		t.Fatalf("RenderEnriched: %v", err)
	}

	e, err := store.GetEnriched(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetEnriched: %v", err)
	}
	if e.AvatarKey == "" || e.FinalOutputKey == "" {
		t.Fatalf("keys not set: avatar=%q final=%q", e.AvatarKey, e.FinalOutputKey)
	}
	if !obj.Has(e.AvatarKey) {
		t.Fatalf("avatar not uploaded to object store: %q", e.AvatarKey)
	}
	got, err := store.GetReel(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetReel: %v", err)
	}
	if got.Status != reel.StatusDone {
		t.Fatalf("status = %q, want done", got.Status)
	}
}

func TestEnriched_RenderBeforeApproval_Fails(t *testing.T) {
	const org int64 = 6002
	svc, _, store := newEnrichedFixture()
	ctx := context.Background()

	reelID, err := store.CreateReel(ctx, org, "enriched", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}
	if err := store.SetSource(ctx, org, reelID, "src/clip.mp4", "audio"); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	if _, err := svc.PrepareScript(ctx, org, reelID); err != nil {
		t.Fatalf("PrepareScript: %v", err)
	}
	if err := svc.RenderEnriched(ctx, org, reelID); !errors.Is(err, reel.ErrScriptNotApproved) {
		t.Fatalf("RenderEnriched = %v, want ErrScriptNotApproved", err)
	}
}

func TestEnriched_PrepareWithoutSource_Fails(t *testing.T) {
	const org int64 = 6003
	svc, _, store := newEnrichedFixture()
	ctx := context.Background()

	reelID, err := store.CreateReel(ctx, org, "no source", "brief", 1)
	if err != nil {
		t.Fatalf("CreateReel: %v", err)
	}
	if _, err := svc.PrepareScript(ctx, org, reelID); !errors.Is(err, reel.ErrNoSource) {
		t.Fatalf("PrepareScript = %v, want ErrNoSource", err)
	}
}

// TestEnriched_RenderIdempotent proves the money invariant: a second
// RenderEnriched after the first claim is a no-op (ClaimRender returns
// false), never a second paid render.
func TestEnriched_RenderIdempotent(t *testing.T) {
	const org int64 = 6004
	svc, _, store := newEnrichedFixture()
	ctx := context.Background()

	reelID := prepareApproved(t, svc, store, org)
	if err := svc.RenderEnriched(ctx, org, reelID); err != nil {
		t.Fatalf("RenderEnriched #1: %v", err)
	}
	first, err := store.GetEnriched(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetEnriched #1: %v", err)
	}

	if err := svc.RenderEnriched(ctx, org, reelID); err != nil {
		t.Fatalf("RenderEnriched #2 (should be no-op): %v", err)
	}
	second, err := store.GetEnriched(ctx, org, reelID)
	if err != nil {
		t.Fatalf("GetEnriched #2: %v", err)
	}
	if first.TotalCostUSD != second.TotalCostUSD || first.FinalOutputKey != second.FinalOutputKey {
		t.Fatalf("second render mutated state: %+v -> %+v", first, second)
	}
}
