package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "outbound.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestQueueOutboundForOrgDefaultsToDraft(t *testing.T) {
	db := newTestStore(t)
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/1",
		Content:   "hello",
		AIModel:   "agent",
	}, true /* requestedAuto */, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	if !res.Decision.Allowed || res.ID == 0 {
		t.Fatalf("expected allowed insert, got %+v", res)
	}
	// Org has no outbound_mode set → must downgrade to draft even though
	// the caller asked for auto.
	if res.Status != models.OutboundDraft {
		t.Fatalf("expected draft when org not opted-in, got %q", res.Status)
	}
}

func TestQueueOutboundForOrgRespectsOptInAuto(t *testing.T) {
	db := newTestStore(t)
	if err := db.SetContext("org:1:outbound_mode", "auto"); err != nil {
		t.Fatal(err)
	}
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "inbox",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/u/1",
		Content:   "hi",
		AIModel:   "agent",
	}, true, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	if res.Status != models.OutboundApproved {
		t.Fatalf("expected approved when org opted-in, got %q", res.Status)
	}
}

func TestClaimApprovedOutboundForOrgMovesToSendingOnce(t *testing.T) {
	db := newTestStore(t)
	if err := db.SetContext("org:1:outbound_mode", "auto"); err != nil {
		t.Fatal(err)
	}
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/claim",
		Content:   "hi",
		AIModel:   "agent",
	}, true, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	if err := db.ClaimApprovedOutboundForOrg(1, res.ID, "worker-a"); err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	msg, err := db.GetOutboundForOrg(1, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Status != models.OutboundSending {
		t.Fatalf("expected sending after claim, got %q", msg.Status)
	}
	if err := db.ClaimApprovedOutboundForOrg(1, res.ID, "worker-b"); err == nil {
		t.Fatal("expected second claim to fail")
	}
	dup, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/claim",
		Content:   "duplicate",
		AIModel:   "agent",
	}, true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if dup.Decision.Allowed {
		t.Fatal("sending rows must still block duplicate outbound targets")
	}
}

func TestQueueOutboundForOrgIgnoresGlobalAutoCommentMode(t *testing.T) {
	db := newTestStore(t)
	// Pre-attack: a stale global key from the legacy code path.
	_ = db.SetContext("auto_comment_mode", "true")
	// And the AI tries to flip the org via the same global key.
	_ = db.SetContext("org:1:auto_comment_mode", "true")

	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/2",
		Content:   "hi",
	}, true, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	// Only `org:1:outbound_mode == auto` should grant auto execution.
	if res.Status != models.OutboundDraft {
		t.Fatalf("legacy auto_comment_mode must NOT enable auto: got %q", res.Status)
	}
}

func TestQueueOutboundForOrgBlocksDuplicateActiveTarget(t *testing.T) {
	db := newTestStore(t)
	first := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: models.PlatformFacebook,
		AccountID: 7, TargetURL: "https://facebook.com/p/dup", Content: "x",
	}
	if _, err := db.QueueOutboundForOrg(first, false, time.Hour); err != nil {
		t.Fatal(err)
	}

	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: models.PlatformFacebook,
		AccountID: 7, TargetURL: "https://facebook.com/p/dup", Content: "y",
	}, false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision.Allowed {
		t.Fatal("expected duplicate to be blocked")
	}
	if res.Decision.Reason != "duplicate_outbound_target" {
		t.Fatalf("expected duplicate_outbound_target, got %q", res.Decision.Reason)
	}
}

func TestQueueOutboundForOrgConcurrentRaceLastResortUnique(t *testing.T) {
	// The application-level guard plus the partial UNIQUE index together
	// must ensure two concurrent QueueOutboundForOrg calls with the same
	// (org_id, type, target_url) result in exactly one row in the DB.
	db := newTestStore(t)
	target := "https://facebook.com/p/race"
	const goroutines = 8

	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]OutboundQueueResult, goroutines)
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = db.QueueOutboundForOrg(&models.OutboundMessage{
				OrgID: 2, Type: "comment", Platform: models.PlatformFacebook,
				AccountID: 9, TargetURL: target, Content: "race",
			}, false, time.Hour)
		}(i)
	}
	wg.Wait()

	allowed := 0
	for i := 0; i < goroutines; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d errored: %v", i, errs[i])
		}
		if results[i].Decision.Allowed {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("expected exactly one allowed insert, got %d", allowed)
	}
}
