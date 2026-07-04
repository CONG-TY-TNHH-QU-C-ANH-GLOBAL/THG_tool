package leadingest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/app"
)

// DB-backed characterization for IngestPost's persist + notify side effects
// (specs/lead_ingestion_behavior.md §4/§5/§7). Each test uses a FRESH SQLite store
// under t.TempDir() (auto-cleaned, no shared state, no ordering dependency) and the
// existing public constructors store.New / db.App() — NO production seam is
// added. These pin CURRENT behavior before the PR23C refactor.

// newIngestStores opens an isolated *store.Store + *app.Store backed by one fresh
// on-disk SQLite file for the duration of a single test.
func newIngestStores(t *testing.T) (*store.Store, *app.Store) {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "leadingest.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, db.App()
}

// qualifyingInput is the proven deterministic hot/warm post (mirrors the content used by
// TestIngestPost_DeterministicHotLeadQualifies) so persistence is reached without AI.
func qualifyingInput(taskID, primaryURL string) Input {
	return Input{
		TaskID:           taskID,
		OrgID:            7,
		SourceType:       "post",
		PrimaryURL:       primaryURL,
		AuthorName:       "Khách Hàng",
		AuthorProfileURL: "https://facebook.com/khach.hang",
		Content:          "Mình cần tìm supplier POD/dropship, cần báo giá ship đi Mỹ. Ai biết bên nào uy tín không? Cần tư vấn gấp.",
		Reactions:        12,
		Comments:         5,
	}
}

func qualifyingDeps(db *store.Store, appStore *app.Store, onLead func(LeadEvent)) Deps {
	return Deps{
		AppStore: appStore,
		LegacyDB: db,
		Scorer:   scoring.New(scoring.DefaultConfig()),
		Guidance: scoring.Guidance{
			TargetAuthorRole: "customers",
			TargetSignals:    []string{"ship đi mỹ", "supplier"},
		},
		Keywords:      []string{"pod", "dropship", "supplier"},
		OnLeadCreated: onLead,
	}
}

// legacyLeadCount returns the number of non-archived legacy `leads` rows for orgID via the
// existing public read API — one row per persisted lead (source_id=0 rows are kept by the
// LEFT JOIN). Scoped to the test's unique org, so it never reads other tests' data.
func legacyLeadCount(t *testing.T, db *store.Store, orgID int64) int {
	t.Helper()
	rows, err := db.Leads().GetLeadsFiltered("", "", 1000, 0, orgID)
	if err != nil {
		t.Fatalf("GetLeadsFiltered: %v", err)
	}
	return len(rows)
}

// §4/§7: a qualifying lead is persisted to task_leads exactly once and the OnLeadCreated
// hook fires exactly once with the new lead's identity.
func TestIngestPost_PersistsLeadAndNotifiesOnce(t *testing.T) {
	db, appStore := newIngestStores(t)
	var events []LeadEvent
	deps := qualifyingDeps(db, appStore, func(e LeadEvent) { events = append(events, e) })
	in := qualifyingInput("task-persist", "https://facebook.com/groups/1/posts/300")

	out, err := IngestPost(context.Background(), deps, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Inserted || out.Skipped != "" {
		t.Fatalf("expected inserted lead, got inserted=%v skipped=%q", out.Inserted, out.Skipped)
	}

	counts, err := appStore.GetLeadCounts(context.Background(), in.OrgID)
	if err != nil {
		t.Fatalf("GetLeadCounts: %v", err)
	}
	if counts.Total != 1 {
		t.Fatalf("expected exactly 1 task_lead persisted, got %d", counts.Total)
	}

	rows, err := appStore.ListLeads(context.Background(), in.OrgID, "", "", 0, 10, 0)
	if err != nil {
		t.Fatalf("ListLeads: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 task_lead row, got %d", len(rows))
	}
	if rows[0].SourceURL != in.PrimaryURL {
		t.Fatalf("persisted source_url = %q, want %q", rows[0].SourceURL, in.PrimaryURL)
	}
	if rows[0].Category != out.Category {
		t.Fatalf("persisted category = %q, want Outcome category %q", rows[0].Category, out.Category)
	}

	if len(events) != 1 {
		t.Fatalf("expected OnLeadCreated to fire exactly once, fired %d times", len(events))
	}
	if events[0].OrgID != in.OrgID || events[0].PostURL != in.PrimaryURL {
		t.Fatalf("LeadEvent identity mismatch: %+v", events[0])
	}
	if events[0].LeadID <= 0 {
		t.Fatalf("expected a positive LeadID in the notification, got %d", events[0].LeadID)
	}
}

// §5: task_leads dedup is scoped by UNIQUE(task_id, source_url). Re-ingesting the SAME
// (TaskID, PrimaryURL) is silently IGNOREd (no new row, no error, treated as success) yet
// the best-effort OnLeadCreated hook still fires per call. A DIFFERENT TaskID with the same
// URL is a NEW row — pinning that the dedup key is task-scoped, not URL-global.
func TestIngestPost_TaskLeadDedupByTaskAndURL(t *testing.T) {
	db, appStore := newIngestStores(t)
	var fired int
	deps := qualifyingDeps(db, appStore, func(LeadEvent) { fired++ })
	const url = "https://facebook.com/groups/1/posts/301"

	first := qualifyingInput("task-A", url)
	if _, err := IngestPost(context.Background(), deps, first); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	// Same TaskID + same URL → deduped by UNIQUE(task_id, source_url).
	if _, err := IngestPost(context.Background(), deps, first); err != nil {
		t.Fatalf("second (duplicate) ingest must not error: %v", err)
	}

	counts, err := appStore.GetLeadCounts(context.Background(), first.OrgID)
	if err != nil {
		t.Fatalf("GetLeadCounts: %v", err)
	}
	if counts.Total != 1 {
		t.Fatalf("duplicate (task_id, source_url) must stay 1 task_lead, got %d", counts.Total)
	}
	// Best-effort notification fires once PER call, even when the row was deduped.
	if fired != 2 {
		t.Fatalf("OnLeadCreated must fire per call (twice), fired %d", fired)
	}
	// Legacy `leads` mirror is NOT deduped: IngestPost writes SourceID=0, so the partial
	// UNIQUE idx_leads_dedup(source_type, source_id) WHERE source_id > 0 never applies and
	// each call inserts a new legacy row (spec §5 quirk). PR23C must preserve SourceID=0 and
	// must NOT add a dedup pre-check or change this to silent-ignore.
	if n := legacyLeadCount(t, db, first.OrgID); n != 2 {
		t.Fatalf("legacy leads must grow per call (no dedup), got %d want 2", n)
	}

	// Different TaskID, same URL → a new row (dedup is task-scoped, not URL-global).
	second := qualifyingInput("task-B", url)
	if _, err := IngestPost(context.Background(), deps, second); err != nil {
		t.Fatalf("third ingest (new task): %v", err)
	}
	counts, err = appStore.GetLeadCounts(context.Background(), first.OrgID)
	if err != nil {
		t.Fatalf("GetLeadCounts: %v", err)
	}
	if counts.Total != 2 {
		t.Fatalf("different task_id with same URL must create a 2nd task_lead, got %d", counts.Total)
	}
	if fired != 3 {
		t.Fatalf("OnLeadCreated must have fired 3 times total, fired %d", fired)
	}
	// Third call adds a third un-deduped legacy row (still SourceID=0).
	if n := legacyLeadCount(t, db, first.OrgID); n != 3 {
		t.Fatalf("legacy leads must be 3 after the third ingest, got %d", n)
	}
}
