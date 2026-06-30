package knowledge_test

import (
	"testing"

	"github.com/thg/scraper/internal/store/knowledge"
)

// Characterization for the data-source registry CRUD after the move from the
// top-level store into the knowledge subpackage (DOMAINS Candidate A). Pins the
// round-trip + org/tenant isolation that the relocated methods must preserve.

func TestDataSources_CRUDRoundTrip(t *testing.T) {
	ks := newKnowledgeStore(t, "data_sources_crud.db")
	const orgID = int64(1)

	id, err := ks.CreateDataSource(&knowledge.DataSource{
		OrgID: orgID, Type: "google_sheet", Name: "Catalog",
		SourceURL: "https://docs.google.com/x", Status: "pending", MetadataJSON: `{"k":"v"}`,
	})
	if err != nil || id <= 0 {
		t.Fatalf("CreateDataSource: id=%d err=%v", id, err)
	}

	got, err := ks.GetDataSourceForOrg(orgID, id)
	if err != nil || got == nil {
		t.Fatalf("GetDataSourceForOrg: %v", err)
	}
	if got.Name != "Catalog" || got.Type != "google_sheet" || got.Status != "pending" || got.MetadataJSON != `{"k":"v"}` {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.LastSyncAt != nil {
		t.Errorf("last_sync_at should be nil before any sync, got %v", got.LastSyncAt)
	}

	list, err := ks.ListDataSources(orgID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListDataSources = %d rows, err=%v", len(list), err)
	}

	if err := ks.UpdateDataSourceSyncResult(orgID, id, "synced", 7, "ok", `{"k":"v2"}`, "", true); err != nil {
		t.Fatalf("UpdateDataSourceSyncResult: %v", err)
	}
	synced, _ := ks.GetDataSourceForOrg(orgID, id)
	if synced.Status != "synced" || synced.ItemCount != 7 || synced.Summary != "ok" || synced.LastSyncAt == nil {
		t.Fatalf("sync result not persisted: %+v", synced)
	}

	if err := ks.DeleteDataSourceForOrg(orgID, id); err != nil {
		t.Fatalf("DeleteDataSourceForOrg: %v", err)
	}
	after, _ := ks.ListDataSources(orgID)
	if len(after) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(after))
	}
}

func TestDataSources_OrgScoping(t *testing.T) {
	ks := newKnowledgeStore(t, "data_sources_org.db")
	idA, _ := ks.CreateDataSource(&knowledge.DataSource{OrgID: 1, Type: "website", Name: "A", SourceURL: "https://a", Status: "pending"})
	_, _ = ks.CreateDataSource(&knowledge.DataSource{OrgID: 2, Type: "website", Name: "B", SourceURL: "https://b", Status: "pending"})

	list1, err := ks.ListDataSources(1)
	if err != nil || len(list1) != 1 || list1[0].Name != "A" {
		t.Fatalf("org 1 list = %+v err=%v, want only A", list1, err)
	}

	// Cross-org read must not return org 1's row (no-match yields nil + sql.ErrNoRows;
	// the isolation property is that org 2 never sees org 1's source).
	if leak, _ := ks.GetDataSourceForOrg(2, idA); leak != nil {
		t.Fatalf("tenant isolation breach: org 2 read org 1's source %+v", leak)
	}
}
