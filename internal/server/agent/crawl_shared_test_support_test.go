package agent

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Test-support shared by the finalize_outbound tests. These helpers used to live
// in the crawl-ingest characterization test before that cluster moved to the
// crawlingest subpackage; the moved package keeps its own copies.
// ponytail: 2 tiny test seeders duplicated across two test packages on purpose —
// cheaper than an exported test-support package; revisit if a third consumer appears
// (see the ARCHST1 shared-test-seam decision).

// recordingNotifier returns a notifier func plus a pointer to the messages it
// captured (synchronous, so reads after the call are race-free).
func recordingNotifier() (func(string), *[]string) {
	var msgs []string
	return func(s string) { msgs = append(msgs, s) }, &msgs
}

// seedCrawlAccount inserts an active Facebook account in orgID and returns its id.
func seedCrawlAccount(t *testing.T, db *store.Store, orgID int64) int64 {
	t.Helper()
	id, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "crawl-acc", Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount(org=%d): %v", orgID, err)
	}
	return id
}
