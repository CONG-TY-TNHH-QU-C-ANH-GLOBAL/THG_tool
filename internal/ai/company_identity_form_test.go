package ai

import (
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapCompanyIdentityStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// The Company Identity form writes org-scoped user_context keys; this verifies the
// round-trip the PUT /org/company-identity endpoint relies on: the keys feed
// LoadProfileForOrg -> ResolveCompanyIdentity, and clearing a key (empty value)
// makes the agent stop citing it ("field rỗng thì AI không được nêu").
func TestCompanyIdentityForm_RoundTrip(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapCompanyIdentityStore, "company_identity_form")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set := func(k, v string) {
		if e := db.Leads().SetContext("org:5:"+k, v); e != nil {
			t.Fatalf("SetContext %s: %v", k, e)
		}
	}

	set("business_name", "THG Fulfill")
	set("business_website", "https://thgfulfill.com")
	set("official_contact", "t.me/thgfulfill")
	set("primary_cta", "Inbox mình nhé")
	set("services", "US fulfillment cho TikTok Shop")

	id := ResolveCompanyIdentity(LoadProfileForOrg(db, 5), nil)
	if id.CompanyName != "THG Fulfill" || id.Website != "https://thgfulfill.com" ||
		id.OfficialContact != "t.me/thgfulfill" || id.PrimaryCTA != "Inbox mình nhé" ||
		id.ServiceSummary != "US fulfillment cho TikTok Shop" {
		t.Fatalf("identity not resolved from form keys: %+v", id)
	}

	// Clearing the website must remove it so the agent cannot cite it.
	set("business_website", "")
	if cleared := ResolveCompanyIdentity(LoadProfileForOrg(db, 5), nil); cleared.Website != "" {
		t.Fatalf("cleared website should be empty, got %q", cleared.Website)
	}
}
