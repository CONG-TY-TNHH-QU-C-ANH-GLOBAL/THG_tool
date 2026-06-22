package agent

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/server/testsupport"
	"github.com/thg/scraper/internal/store"
)

// seedDedupeLead pre-inserts a task_lead so processConnectorCrawlItem's source-URL
// dedupe short-circuits (fetched=true, inserted=false) WITHOUT running the lead
// ingest pipeline — so the multi-item path stays deterministic and never reaches
// the OnLeadCreated / tgEvents notification (the lead-creating branch PR19A
// leaves uncovered, which would need a notification seam to characterize).
func seedDedupeLead(t *testing.T, as *store.AppStore, orgID int64, sourceURL string) {
	t.Helper()
	ctx := context.Background()
	if err := as.CreateTask(ctx, "seed-task", orgID, "seed"); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := as.InsertLead(ctx, "seed-task", orgID, store.TaskLead{
		OrgID: orgID, SourceURL: sourceURL, AuthorName: "seed", Content: "seed", Category: "warm",
	}); err != nil {
		t.Fatalf("seed lead %q: %v", sourceURL, err)
	}
}

// TestProcessConnectorCrawlResult_MultipleItemsFold proves the extracted item
// loop folds EACH item independently (no item-collapse / last-item duplication):
// two DISTINCT items whose source URLs are already-seen are each counted fetched
// (→ 2, not 1), neither is re-inserted (→ 0), and the app_task totals persist 2/0.
// Deterministic because the source-URL dedupe short-circuits before the lead
// pipeline, so no lead is created and tgEvents is never invoked.
func TestProcessConnectorCrawlResult_MultipleItemsFold(t *testing.T) {
	db := testsupport.NewTestStore(t, "crawl_multi")
	const orgID = int64(1)
	accID := seedCrawlAccount(t, db, orgID)
	connID := seedOwningConnector(t, db, orgID, accID)

	as := crawlAppStore(t, db)
	const url1 = "https://www.facebook.com/groups/1/posts/11/"
	const url2 = "https://www.facebook.com/groups/2/posts/22/"
	seedDedupeLead(t, as, orgID, url1)
	seedDedupeLead(t, as, orgID, url2)

	notify, notes := recordingNotifier()
	h := &Handler{db: db, notifier: notify}
	app := newCrawlApp(h, connID, orgID)

	// Two DISTINCT items, each with >= 20 runes of content and an already-seen
	// source_url, so each is counted fetched then deduped (no lead pipeline).
	body := `{
		"task_id": "t-multi",
		"account_id": ` + itoa64(accID) + `,
		"items": [
			{"id":"obs-1","source_url":"` + url1 + `","author_name":"A Mua","content":"cần tìm xưởng áo thun số lượng lớn giá tốt","post_fbid":"11","group_fbid":"1"},
			{"id":"obs-2","source_url":"` + url2 + `","author_name":"B Mua","content":"tìm nhà cung cấp ốp lưng điện thoại số lượng lớn","post_fbid":"22","group_fbid":"2"}
		]
	}`
	code, out := postCrawl(t, app, body)

	if code != 200 || out["status"] != "stored" {
		t.Fatalf("multi-item result = %d %v, want 200 stored", code, out)
	}
	// Both items iterated and folded independently: fetched scales to 2, not 1.
	if out["fetched"] != float64(2) {
		t.Fatalf("fetched = %v, want 2 (item loop must not collapse to a single item)", out["fetched"])
	}
	if out["inserted"] != float64(0) {
		t.Fatalf("inserted = %v, want 0 (both items deduped)", out["inserted"])
	}
	task, err := as.GetTask(context.Background(), "t-multi")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != "completed" || task.TotalFetched != 2 || task.TotalReturned != 0 {
		t.Fatalf("task = %+v, want completed 2/0", task)
	}
	// No new lead beyond the two pre-seeded dedupe rows.
	requireLeadCount(t, as, orgID, 2)
	if len(*notes) != 1 {
		t.Fatalf("summary notifier = %v, want exactly 1", *notes)
	}
}
