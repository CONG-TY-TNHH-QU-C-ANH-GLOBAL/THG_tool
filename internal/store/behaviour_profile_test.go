package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func newBehaviourTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "behaviour.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Missing profile is not an error; the resolver returns the TrustWarming
// preset so a freshly-imported account still has working caps.
func TestResolveAccountCaps_DefaultsToWarmingWhenAbsent(t *testing.T) {
	db := newBehaviourTestStore(t)
	caps, trust, err := db.ResolveAccountCaps(context.Background(), 999)
	if err != nil {
		t.Fatalf("ResolveAccountCaps: %v", err)
	}
	if trust != models.TrustWarming {
		t.Errorf("missing profile must resolve to warming, got %s", trust)
	}
	want := models.ResolveBehaviourCaps(models.TrustWarming, "")
	if caps != want {
		t.Errorf("caps mismatch: got %+v want %+v", caps, want)
	}
}

// Upsert + read round trip: trust_level and workspace_role persist.
// CapsOverride is opaque JSON — we don't parse it on read.
func TestUpsertAndGetBehaviourProfile(t *testing.T) {
	db := newBehaviourTestStore(t)
	ctx := context.Background()
	p := &models.AccountBehaviourProfile{
		AccountID:      42,
		OrgID:          1,
		TrustLevel:     models.TrustTrusted,
		AccountAgeDays: 730,
		PersonaType:    "seller",
		WorkspaceRole:  "aggressive_outreach",
		Capabilities:   `{"posting_allowed":true}`,
		CapsOverride:   `{"comments_per_day": 50}`,
		Notes:          "warmed account",
	}
	if err := db.UpsertAccountBehaviourProfile(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := db.GetAccountBehaviourProfile(ctx, 42)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("get returned nil after upsert")
	}
	if got.TrustLevel != models.TrustTrusted {
		t.Errorf("trust mismatch: %s", got.TrustLevel)
	}
	if got.WorkspaceRole != "aggressive_outreach" {
		t.Errorf("role mismatch: %s", got.WorkspaceRole)
	}

	// Resolver must overlay caps_override on the trusted preset.
	caps, trust, err := db.ResolveAccountCaps(ctx, 42)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if trust != models.TrustTrusted {
		t.Errorf("resolve trust mismatch: %s", trust)
	}
	if caps.CommentsPerDay != 50 {
		t.Errorf("override must set comments cap to 50, got %d", caps.CommentsPerDay)
	}
}

// A fresh runtime state for an unseen account is zero-valued, not an error.
func TestGetRuntimeState_MissingReturnsZero(t *testing.T) {
	db := newBehaviourTestStore(t)
	r, err := db.GetAccountRuntimeState(context.Background(), 123)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.CommentsToday != 0 || r.InboxToday != 0 {
		t.Errorf("missing row must read as zero counters, got %+v", r)
	}
}

// Queueing a comment must increment comments_today by 1 for that account.
func TestQueueOutbound_IncrementsRuntimeCounter(t *testing.T) {
	db := newBehaviourTestStore(t)
	ctx := context.Background()

	// Cold preset has very low caps; use Trusted so the counter test
	// is independent of cap enforcement.
	if err := db.UpsertAccountBehaviourProfile(ctx, &models.AccountBehaviourProfile{
		AccountID: 7, OrgID: 1, TrustLevel: models.TrustTrusted,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	msg := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 7, TargetURL: "https://facebook.com/post/1", Content: "hi",
	}
	res, err := db.QueueOutboundForOrg(msg, false, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if !res.Decision.Allowed {
		t.Fatalf("queue must allow first comment: %+v", res.Decision)
	}

	r, err := db.GetAccountRuntimeState(ctx, 7)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	if r.CommentsToday != 1 {
		t.Errorf("comments_today should be 1 after one queue, got %d", r.CommentsToday)
	}
	if r.InboxToday != 0 {
		t.Errorf("inbox_today must stay 0, got %d", r.InboxToday)
	}
}

// Once the daily cap is reached, the queue must block with reason
// daily_limit_exceeded. This is the core PR-2 enforcement test.
func TestQueueOutbound_DailyCapBlocks(t *testing.T) {
	db := newBehaviourTestStore(t)
	ctx := context.Background()

	// Use the Cold preset; comments cap is small (3).
	if err := db.UpsertAccountBehaviourProfile(ctx, &models.AccountBehaviourProfile{
		AccountID: 9, OrgID: 1, TrustLevel: models.TrustCold,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	caps, _, _ := db.ResolveAccountCaps(ctx, 9)
	cap := caps.CommentsPerDay
	if cap <= 0 {
		t.Fatalf("expected positive comment cap for cold preset, got %d", cap)
	}

	// Fill up the cap. Each comment must target a different post so the
	// per-account dedup index does not fire.
	for i := 0; i < cap; i++ {
		res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
			OrgID: 1, Type: "comment", Platform: "facebook", AccountID: 9,
			TargetURL: postURL(i), Content: "n",
		}, false, 24*time.Hour)
		if err != nil {
			t.Fatalf("queue #%d: %v", i, err)
		}
		if !res.Decision.Allowed {
			t.Fatalf("queue #%d must succeed (cap=%d), got %s", i, cap, res.Decision.Reason)
		}
	}

	// The (cap+1)-th comment must be blocked.
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook", AccountID: 9,
		TargetURL: postURL(cap + 1), Content: "n",
	}, false, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue cap+1: %v", err)
	}
	if res.Decision.Allowed {
		t.Fatalf("queue past cap must be blocked, got allowed")
	}
	if res.Decision.Reason != "daily_limit_exceeded" {
		t.Errorf("expected daily_limit_exceeded, got %q", res.Decision.Reason)
	}
}

// Setting cooldown_until in the future must block any queue regardless
// of the daily cap.
func TestQueueOutbound_AccountCooldownBlocks(t *testing.T) {
	db := newBehaviourTestStore(t)
	ctx := context.Background()
	if err := db.UpsertAccountBehaviourProfile(ctx, &models.AccountBehaviourProfile{
		AccountID: 11, OrgID: 1, TrustLevel: models.TrustTrusted,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.SetAccountCooldown(ctx, 1, 11, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("set cooldown: %v", err)
	}
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook", AccountID: 11,
		TargetURL: "https://facebook.com/post/cooldown", Content: "x",
	}, false, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if res.Decision.Allowed {
		t.Fatalf("cooldown must block queue, got allowed")
	}
	if res.Decision.Reason != "account_cooldown_active" {
		t.Errorf("expected account_cooldown_active, got %q", res.Decision.Reason)
	}
}

// risk_score above the trust-level ceiling blocks the queue.
func TestQueueOutbound_RiskCeilingBlocks(t *testing.T) {
	db := newBehaviourTestStore(t)
	ctx := context.Background()
	// Cold preset has ceiling 0.40; bump risk above it via the writer API.
	if err := db.UpsertAccountBehaviourProfile(ctx, &models.AccountBehaviourProfile{
		AccountID: 13, OrgID: 1, TrustLevel: models.TrustCold,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Failure default weight is 0.05; nine failures push past 0.40.
	for i := 0; i < 9; i++ {
		if err := db.ApplyRiskSignal(ctx, 1, 13, models.RiskSignalFailure, 0); err != nil {
			t.Fatalf("signal %d: %v", i, err)
		}
	}
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook", AccountID: 13,
		TargetURL: "https://facebook.com/post/risky", Content: "x",
	}, false, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if res.Decision.Allowed {
		t.Fatalf("risk ceiling must block, got allowed")
	}
	if res.Decision.Reason != "risk_ceiling_exceeded" {
		t.Errorf("expected risk_ceiling_exceeded, got %q", res.Decision.Reason)
	}
}

// success signal lowers risk_score (clamped at 0.0).
func TestApplyRiskSignal_SuccessLowersScore(t *testing.T) {
	db := newBehaviourTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := db.ApplyRiskSignal(ctx, 1, 20, models.RiskSignalFailure, 0); err != nil {
			t.Fatalf("fail %d: %v", i, err)
		}
	}
	r, _ := db.GetAccountRuntimeState(ctx, 20)
	if r.RiskScore <= 0 {
		t.Fatalf("expected raised risk after failures, got %.3f", r.RiskScore)
	}
	if err := db.ApplyRiskSignal(ctx, 1, 20, models.RiskSignalSuccess, 0); err != nil {
		t.Fatalf("success: %v", err)
	}
	r2, _ := db.GetAccountRuntimeState(ctx, 20)
	if r2.RiskScore >= r.RiskScore {
		t.Errorf("success signal must lower risk score, before=%.3f after=%.3f", r.RiskScore, r2.RiskScore)
	}
}

// Counters reset to zero on day rollover. We simulate this by writing
// a stale counters_day directly, then asking the read API.
func TestRuntimeState_DayRollover(t *testing.T) {
	db := newBehaviourTestStore(t)
	ctx := context.Background()

	// Seed a row dated yesterday with non-zero counters.
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	if _, err := db.db.Exec(
		`INSERT INTO account_runtime_state
			(account_id, org_id, counters_day, comments_today, inbox_today,
			 group_posts_today, profile_posts_today, risk_score, recent_failures,
			 updated_at)
		 VALUES (?, ?, ?, 99, 99, 99, 99, 0.5, 7, CURRENT_TIMESTAMP)`,
		15, 1, yesterday,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r, err := db.GetAccountRuntimeState(ctx, 15)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if r.CommentsToday != 0 || r.InboxToday != 0 || r.GroupPostsToday != 0 || r.ProfilePostsToday != 0 {
		t.Errorf("stale counters must roll to 0, got %+v", r)
	}
	if r.RiskScore != 0.5 {
		t.Errorf("risk_score must persist across day rollover, got %.3f", r.RiskScore)
	}
	if r.RecentFailures != 7 {
		t.Errorf("recent_failures must persist across day rollover, got %d", r.RecentFailures)
	}
}

// helper to build distinct post URLs without bringing in fmt repeatedly.
func postURL(i int) string {
	const base = "https://facebook.com/post/"
	digits := "0123456789"
	if i == 0 {
		return base + "0"
	}
	out := ""
	for i > 0 {
		out = string(digits[i%10]) + out
		i /= 10
	}
	return base + out
}
