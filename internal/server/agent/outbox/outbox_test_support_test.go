package outbox

import (
	"strconv"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Test-support copied alongside the outbound-execution tests when this cluster
// moved out of package agent. Small enough that a shared exported test package
// isn't worth it (see the ARCHST1 shared-test-seam decision).

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

func recordingNotifier() (func(string), *[]string) {
	var msgs []string
	return func(s string) { msgs = append(msgs, s) }, &msgs
}

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
