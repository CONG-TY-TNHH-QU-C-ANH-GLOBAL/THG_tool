// Domain: crawl (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/store/crawl"
	"time"
)

func newCrawlIntentTestStore(t *testing.T) *Store {
	return newSharedStore(t, "intents.db")
}

func seedIntent(t *testing.T, db *Store, orgID int64) crawl.Intent {
	t.Helper()
	ctx := context.Background()
	intent, err := db.Crawl().UpsertIntent(ctx, crawl.Intent{
		OrgID:           orgID,
		AccountID:       1,
		Prompt:          "find buyers",
		Intent:          "facebook_crawl",
		SourceType:      "facebook_group",
		SourceURL:       "https://facebook.com/groups/" + t.Name(),
		Keywords:        []string{"pod", "supplier"},
		IntervalMinutes: 30,
		MaxItems:        10,
		NextRunAt:       time.Now().UTC().Add(-time.Minute), // due immediately
	})
	if err != nil {
		t.Fatalf("UpsertCrawlIntent: %v", err)
	}
	return *intent
}

// New intents must land in 'active', not just enabled=1.
func TestUpsertCrawlIntent_NewIntentIsActive(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	intent := seedIntent(t, db, 1)
	if intent.Status != crawl.IntentStatusActive {
		t.Errorf("Status = %q, want %q", intent.Status, crawl.IntentStatusActive)
	}
	if !intent.Enabled {
		t.Errorf("Enabled = false, want true (legacy mirror of active)")
	}
}

// SetCrawlIntentStatus validates the status string + keeps Enabled mirrored.
func TestSetCrawlIntentStatus_Transitions(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()
	intent := seedIntent(t, db, 1)

	cases := []struct {
		status      string
		wantEnabled bool
	}{
		{crawl.IntentStatusPaused, false},
		{crawl.IntentStatusActive, true},
		{crawl.IntentStatusArchived, false},
		{crawl.IntentStatusFailed, false},
		{crawl.IntentStatusCooldown, false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			if err := db.Crawl().SetIntentStatus(ctx, intent.OrgID, intent.ID, tc.status); err != nil {
				t.Fatalf("SetCrawlIntentStatus(%q): %v", tc.status, err)
			}
			got, err := db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
			if err != nil {
				t.Fatalf("readback: %v", err)
			}
			if got.Status != tc.status {
				t.Errorf("Status = %q, want %q", got.Status, tc.status)
			}
			if got.Enabled != tc.wantEnabled {
				t.Errorf("Enabled = %v, want %v (must mirror status==active)", got.Enabled, tc.wantEnabled)
			}
		})
	}
}

func TestSetCrawlIntentStatus_RejectsInvalid(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	intent := seedIntent(t, db, 1)
	err := db.Crawl().SetIntentStatus(context.Background(), intent.OrgID, intent.ID, "nonsense")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestSetCrawlIntentStatus_UnknownIntent(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	err := db.Crawl().SetIntentStatus(context.Background(), 1, 999999, crawl.IntentStatusPaused)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("want sql.ErrNoRows for missing intent, got %v", err)
	}
}

// Only 'active' intents are claimed by the scheduler.
func TestClaimDueCrawlIntents_OnlyActive(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()

	active := seedIntent(t, db, 1)
	// Make a paused, archived, and failed intent each.
	for status, sourceTag := range map[string]string{
		crawl.IntentStatusPaused:   "paused-source",
		crawl.IntentStatusArchived: "archived-source",
		crawl.IntentStatusFailed:   "failed-source",
	} {
		intent, err := db.Crawl().UpsertIntent(ctx, crawl.Intent{
			OrgID:           1,
			AccountID:       1,
			Intent:          "facebook_crawl",
			SourceType:      "facebook_group",
			SourceURL:       "https://facebook.com/groups/" + sourceTag,
			Keywords:        []string{"pod"},
			IntervalMinutes: 30,
			MaxItems:        10,
			NextRunAt:       time.Now().UTC().Add(-time.Minute),
		})
		if err != nil {
			t.Fatalf("upsert %s: %v", status, err)
		}
		if err := db.Crawl().SetIntentStatus(ctx, intent.OrgID, intent.ID, status); err != nil {
			t.Fatalf("transition to %s: %v", status, err)
		}
	}

	claimed, err := db.Crawl().ClaimDueIntents(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("ClaimDueCrawlIntents: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d intents, want 1", len(claimed))
	}
	if claimed[0].ID != active.ID {
		t.Errorf("claimed intent ID = %d, want %d (the active one)", claimed[0].ID, active.ID)
	}
}

// Re-prompting reactivates a paused/failed/cooldown intent, but keeps archived sticky.
func TestUpsertCrawlIntent_ArchivedStickyOnReprompt(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()
	intent := seedIntent(t, db, 1)

	// User archives the intent.
	if err := db.Crawl().SetIntentStatus(ctx, intent.OrgID, intent.ID, crawl.IntentStatusArchived); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// Re-prompting (same dedup_hash) should NOT un-archive.
	reprompted, err := db.Crawl().UpsertIntent(ctx, crawl.Intent{
		OrgID:           1,
		AccountID:       1,
		Prompt:          "find buyers",
		Intent:          "facebook_crawl",
		SourceType:      "facebook_group",
		SourceURL:       "https://facebook.com/groups/" + t.Name(),
		Keywords:        []string{"pod", "supplier"},
		IntervalMinutes: 30,
		MaxItems:        10,
		NextRunAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if reprompted.Status != crawl.IntentStatusArchived {
		t.Errorf("Status = %q after re-prompt, want %q (archived must be sticky)",
			reprompted.Status, crawl.IntentStatusArchived)
	}

	// Re-prompting a PAUSED intent reactivates it.
	if err := db.Crawl().SetIntentStatus(ctx, intent.OrgID, intent.ID, crawl.IntentStatusPaused); err != nil {
		t.Fatalf("transition to paused: %v", err)
	}
	reprompted2, err := db.Crawl().UpsertIntent(ctx, crawl.Intent{
		OrgID:           1,
		AccountID:       1,
		Prompt:          "find buyers",
		Intent:          "facebook_crawl",
		SourceType:      "facebook_group",
		SourceURL:       "https://facebook.com/groups/" + t.Name(),
		Keywords:        []string{"pod", "supplier"},
		IntervalMinutes: 30,
		MaxItems:        10,
		NextRunAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("re-upsert paused: %v", err)
	}
	if reprompted2.Status != crawl.IntentStatusActive {
		t.Errorf("Status = %q after re-prompt of paused, want %q",
			reprompted2.Status, crawl.IntentStatusActive)
	}
}

// 2 consecutive errors transition the intent to 'failed'. enabled stays mirrored.
func TestMarkCrawlIntentRunResult_SecondErrorMarksFailed(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()
	intent := seedIntent(t, db, 1)

	// First error — still active (no auto-disable on first failure).
	if err := db.Crawl().MarkIntentRunResult(ctx, intent.ID, "task-1", "transient: timeout"); err != nil {
		t.Fatalf("first error: %v", err)
	}
	got, _ := db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if got.Status != crawl.IntentStatusActive {
		t.Errorf("after 1st error: status = %q, want active (no auto-disable on first failure)", got.Status)
	}

	// Second consecutive error — transitions to failed.
	if err := db.Crawl().MarkIntentRunResult(ctx, intent.ID, "task-2", "still failing"); err != nil {
		t.Fatalf("second error: %v", err)
	}
	got, _ = db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if got.Status != crawl.IntentStatusFailed {
		t.Errorf("after 2nd error: status = %q, want failed", got.Status)
	}
	if got.Enabled {
		t.Errorf("after 2nd error: Enabled = true, want false (mirror of status!=active)")
	}
}

// AdvanceCrawlIntentCursor: with timestamp present, only newer posts advance.
func TestAdvanceCrawlIntentCursor_OnlyMovesForward(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()
	intent := seedIntent(t, db, 1)

	old := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	newer := old.Add(30 * time.Minute)

	// First advance — sets cursor (was empty).
	if err := db.Crawl().AdvanceIntentCursor(ctx, intent.ID, "fb_post_1", old); err != nil {
		t.Fatalf("first advance: %v", err)
	}
	got, _ := db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if got.CursorLastPostID != "fb_post_1" || !got.CursorLastPostAt.Equal(old) {
		t.Fatalf("after 1st advance: got id=%q at=%v", got.CursorLastPostID, got.CursorLastPostAt)
	}

	// Older post — must NOT regress the cursor.
	older := old.Add(-30 * time.Minute)
	if err := db.Crawl().AdvanceIntentCursor(ctx, intent.ID, "fb_post_old", older); err != nil {
		t.Fatalf("advance with older: %v", err)
	}
	got, _ = db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if got.CursorLastPostID != "fb_post_1" {
		t.Errorf("cursor regressed: id=%q, want fb_post_1 (older post must not win)", got.CursorLastPostID)
	}

	// Newer post — must advance.
	if err := db.Crawl().AdvanceIntentCursor(ctx, intent.ID, "fb_post_2", newer); err != nil {
		t.Fatalf("advance with newer: %v", err)
	}
	got, _ = db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if got.CursorLastPostID != "fb_post_2" || !got.CursorLastPostAt.Equal(newer) {
		t.Errorf("after newer advance: id=%q at=%v, want fb_post_2 / %v", got.CursorLastPostID, got.CursorLastPostAt, newer)
	}
}

// AdvanceCrawlIntentCursor: zero timestamp = degraded last-call-wins.
func TestAdvanceCrawlIntentCursor_DegradedNoTimestamp(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()
	intent := seedIntent(t, db, 1)

	if err := db.Crawl().AdvanceIntentCursor(ctx, intent.ID, "first", time.Time{}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := db.Crawl().AdvanceIntentCursor(ctx, intent.ID, "second", time.Time{}); err != nil {
		t.Fatalf("second: %v", err)
	}
	got, _ := db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if got.CursorLastPostID != "second" {
		t.Errorf("degraded last-call-wins: cursor = %q, want second", got.CursorLastPostID)
	}
}

// Empty post id is a no-op (avoid clobbering the cursor with empty).
func TestAdvanceCrawlIntentCursor_EmptyPostIDNoop(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()
	intent := seedIntent(t, db, 1)

	now := time.Now().UTC().Truncate(time.Second)
	if err := db.Crawl().AdvanceIntentCursor(ctx, intent.ID, "real_id", now); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
	if err := db.Crawl().AdvanceIntentCursor(ctx, intent.ID, "   ", now.Add(time.Hour)); err != nil {
		t.Fatalf("empty advance: %v", err)
	}
	got, _ := db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if got.CursorLastPostID != "real_id" {
		t.Errorf("empty post id must not clobber cursor; got %q", got.CursorLastPostID)
	}
}

// Cursor advances; ClaimDueCrawlIntents surfaces it for the scheduler.
func TestUpdateCrawlIntentCursor_Advances(t *testing.T) {
	db := newCrawlIntentTestStore(t)
	ctx := context.Background()
	intent := seedIntent(t, db, 1)

	postID := "fb_post_12345"
	postAt := time.Now().UTC().Truncate(time.Second)
	if err := db.Crawl().UpdateIntentCursor(ctx, intent.ID, postID, postAt); err != nil {
		t.Fatalf("UpdateCrawlIntentCursor: %v", err)
	}
	got, err := db.Crawl().GetIntentByHash(ctx, intent.OrgID, intent.DedupHash)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.CursorLastPostID != postID {
		t.Errorf("CursorLastPostID = %q, want %q", got.CursorLastPostID, postID)
	}
	if !got.CursorLastPostAt.Equal(postAt) {
		t.Errorf("CursorLastPostAt = %v, want %v", got.CursorLastPostAt, postAt)
	}
	if got.CursorUpdatedAt.IsZero() {
		t.Errorf("CursorUpdatedAt must be set after an advance")
	}
}
