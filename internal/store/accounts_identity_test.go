package store

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestSetAccountFacebookIdentityStoresMetaWithoutEmail(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := db.AddAccount(&models.Account{
		OrgID:    1,
		Platform: models.PlatformFacebook,
		Name:     "Facebook slot",
		Status:   models.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = db.SetAccountFacebookIdentity(id, "100014197607233", "", FacebookIdentityMeta{
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
	db, err := New(filepath.Join(t.TempDir(), "identity-mismatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := db.AddAccount(&models.Account{
		OrgID:    1,
		Platform: models.PlatformFacebook,
		Name:     "Facebook slot",
		Status:   models.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountFacebookIdentity(id, "111", "", FacebookIdentityMeta{DisplayName: "First"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountFacebookIdentity(id, "222", "", FacebookIdentityMeta{DisplayName: "Second"}); err == nil {
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
