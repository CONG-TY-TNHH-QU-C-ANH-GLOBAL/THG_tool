package copilot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/identities"
)

func newAgentPolicyTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "agent-policy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDeterministicOutboundHonorsOrgAutoPolicy(t *testing.T) {
	db := newAgentPolicyTestStore(t)
	const orgID int64 = 42
	if err := db.Leads().SetContext("org:42:outbound_mode", "auto"); err != nil {
		t.Fatal(err)
	}
	accountID, err := db.Identities().AddAccount(&models.Account{
		OrgID:    orgID,
		Platform: models.PlatformFacebook,
		Name:     "Ready Facebook",
		Status:   models.AccountActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Identities().SetAccountFacebookIdentity(accountID, "1001", "", identities.FacebookIdentityMeta{DisplayName: "Ready Facebook"}); err != nil {
		t.Fatal(err)
	}

	agent := NewAgent("test-key", "test-model", db)
	var gotAction string
	var gotArgs map[string]any
	agent.ActionHandler = func(action string, args map[string]any) (string, error) {
		gotAction = action
		gotArgs = args
		return "queued_comment=1 mode=approved_auto", nil
	}

	if _, err := agent.ProcessPromptForOrgWithAccount(context.Background(), "comment all leads", "dashboard", orgID, accountID); err != nil {
		t.Fatal(err)
	}
	if gotAction != "comment_all_leads" {
		t.Fatalf("action = %q, want comment_all_leads", gotAction)
	}
	if got, ok := gotArgs["auto"].(bool); !ok || !got {
		t.Fatalf("auto = %#v, want true from org outbound policy", gotArgs["auto"])
	}
}
