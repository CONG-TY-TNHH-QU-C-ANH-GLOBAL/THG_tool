// Domain: identities (see internal/store/DOMAINS.md)
package identities_test

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/identities"
)

func TestSetAccountFacebookIdentityStoresMetaWithoutEmail(t *testing.T) {
	_, db := newIdentitiesStore(t, "identity.db")
	id, err := db.AddAccount(&models.Account{
		OrgID:    1,
		Platform: models.PlatformFacebook,
		Name:     "Facebook slot",
		Status:   models.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = db.SetAccountFacebookIdentity(id, "100014197607233", "", identities.FacebookIdentityMeta{
		DisplayName: "David Anh",
		Username:    "david.anh",
		ProfileURL:  "https://www.facebook.com/david.anh",
	})
	if err != nil {
		t.Fatal(err)
	}

	acc, err := db.GetAccount(id)
	if err != nil {
		t.Fatal(err)
	}
	if !acc.BrowserLoggedIn {
		t.Fatal("expected browser_logged_in=true")
	}
	if acc.FBUserID != "100014197607233" || acc.FBDisplayName != "David Anh" || acc.FBUsername != "david.anh" || acc.FBProfileURL == "" {
		t.Fatalf("identity meta was not stored correctly: %+v", acc)
	}
	if acc.Email != "" {
		t.Fatalf("empty email should not be faked, got %q", acc.Email)
	}
}

func TestSetAccountFacebookIdentityRejectsProfileMismatch(t *testing.T) {
	_, db := newIdentitiesStore(t, "identity-mismatch.db")

	id, err := db.AddAccount(&models.Account{
		OrgID:    1,
		Platform: models.PlatformFacebook,
		Name:     "Facebook slot",
		Status:   models.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountFacebookIdentity(id, "111", "", identities.FacebookIdentityMeta{DisplayName: "First"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountFacebookIdentity(id, "222", "", identities.FacebookIdentityMeta{DisplayName: "Second"}); err == nil {
		t.Fatal("expected profile mismatch error")
	}

	acc, err := db.GetAccount(id)
	if err != nil {
		t.Fatal(err)
	}
	if acc.FBUserID != "111" || acc.FBDisplayName != "First" {
		t.Fatalf("mismatch overwrote account identity: %+v", acc)
	}
}
