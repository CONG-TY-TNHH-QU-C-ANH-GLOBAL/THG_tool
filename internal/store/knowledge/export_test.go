package knowledge_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Export must return assets in EVERY state (approved/pending/hidden) and
// walk them all via keyset paging with no duplicates or omissions, in
// (updated_at, id) ascending order. Rows here share an insert second, so
// the test also exercises the id tiebreak across a page boundary.
func TestExportAssetsForOrg_PaginatesEveryState(t *testing.T) {
	db := newKnowledgeStore(t, "export_states.db")
	sid := mustSetupSource(t, db, 1)

	states := []assets.AssetState{
		assets.StateApproved, assets.StateHidden, assets.StatePending,
		assets.StateApproved, assets.StateHidden,
	}
	wantIDs := seedAssets(t, db, sid, states)

	got := drainExport(t, db, 1, 2) // page size 2 forces multiple pages
	assertIDsEqual(t, got, wantIDs)
}

// Org isolation: an export for org 1 never returns org 2's assets, even
// when both own byte-identical rows.
func TestExportAssetsForOrg_TenantIsolation(t *testing.T) {
	db := newKnowledgeStore(t, "export_tenant.db")
	sid1 := mustSetupSource(t, db, 1)
	sid2 := mustSetupSource(t, db, 2)

	want := seedAssets(t, db, sid1, []assets.AssetState{assets.StateApproved})
	_ = seedAssetsForOrg(t, db, 2, sid2, []assets.AssetState{assets.StateApproved})

	got := drainExport(t, db, 1, 500)
	assertIDsEqual(t, got, want)
}

// --- helpers (kept flat so each stays Sonar-clean) ---

func seedAssets(t *testing.T, db *knowledge.Store, sourceID int64, states []assets.AssetState) []int64 {
	return seedAssetsForOrg(t, db, 1, sourceID, states)
}

func seedAssetsForOrg(t *testing.T, db *knowledge.Store, orgID, sourceID int64, states []assets.AssetState) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, len(states))
	for i, st := range states {
		a := newTestAsset(orgID, sourceID, "ext_"+strconv.Itoa(int(orgID))+"_"+strconv.Itoa(i), "asset")
		a.State = st
		saved, err := db.UpsertAsset(ctx, a)
		if err != nil {
			t.Fatalf("seed asset %d: %v", i, err)
		}
		ids = append(ids, saved.ID)
	}
	return ids
}

func drainExport(t *testing.T, db *knowledge.Store, orgID int64, pageSize int) []int64 {
	t.Helper()
	ctx := context.Background()
	var got []int64
	var cur knowledge.ExportCursor
	for {
		page, err := db.ExportAssetsForOrg(ctx, orgID, cur, pageSize)
		if err != nil {
			t.Fatalf("export page: %v", err)
		}
		for _, a := range page {
			got = append(got, a.ID)
			cur = knowledge.ExportCursor{UpdatedAfter: a.UpdatedAt, AfterID: a.ID}
		}
		if len(page) < pageSize {
			return got
		}
	}
}

func assertIDsEqual(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("export returned %d ids, want %d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("id[%d] = %d, want %d (got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}
