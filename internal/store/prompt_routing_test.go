// Domain: prompts (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func newRoutingTestStore(t *testing.T) *Store {
	return newSharedStore(t, "routing.db")
}

func seedRouting(t *testing.T, db *Store, orgID int64, action, decisionJSON, prompt string) {
	t.Helper()
	if err := db.InsertPromptLog(&models.PromptLog{
		OrgID:               orgID,
		Source:              "dashboard",
		UserPrompt:          prompt,
		ActionTaken:         action,
		Success:             true,
		RoutingDecisionJSON: decisionJSON,
	}); err != nil {
		t.Fatalf("seed InsertPromptLog: %v", err)
	}
}

// Distribution must group by (route, reason_code, action) and ignore
// rows outside the window. Legacy rows (empty routing_decision_json)
// collapse to route="legacy" so the dashboard renders historical data.
func TestPromptRoutingDistribution_GroupsAndBoundsTime(t *testing.T) {
	db := newRoutingTestStore(t)
	ctx := context.Background()

	// 3 deterministic self-sufficient + 2 brain ask-user + 1 legacy.
	det := `{"route":"deterministic","reason_code":"self_sufficient_prompt"}`
	ask := `{"route":"brain","reason_code":"brain_ask_user"}`
	for i := 0; i < 3; i++ {
		seedRouting(t, db, 1, "scrape_group", det, "crawl 50 in https://fb.com/groups/X")
	}
	for i := 0; i < 2; i++ {
		seedRouting(t, db, 1, "brain_ask_user", ask, "find me leads")
	}
	seedRouting(t, db, 1, "chat", "", "hello") // legacy → "{}"

	buckets, err := db.PromptRoutingDistribution(ctx, 1, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("Distribution: %v", err)
	}
	total := 0
	routes := map[string]int{}
	for _, b := range buckets {
		total += b.Count
		routes[b.Route] += b.Count
	}
	if total != 6 {
		t.Fatalf("total = %d; want 6", total)
	}
	if routes["deterministic"] != 3 {
		t.Errorf("deterministic = %d; want 3", routes["deterministic"])
	}
	if routes["brain"] != 2 {
		t.Errorf("brain = %d; want 2", routes["brain"])
	}
	if routes["legacy"] != 1 {
		t.Errorf("legacy = %d; want 1 (empty decision_json collapses to legacy)", routes["legacy"])
	}
}

// Distribution must be org-scoped. Org bleed on a dashboard endpoint is
// a security regression, not a UI bug.
func TestPromptRoutingDistribution_OrgScoped(t *testing.T) {
	db := newRoutingTestStore(t)
	ctx := context.Background()
	det := `{"route":"deterministic","reason_code":"self_sufficient_prompt"}`
	for _, org := range []int64{1, 1, 2} {
		seedRouting(t, db, org, "scrape_group", det, "crawl 50 in https://fb.com/groups/X")
	}
	org1, _ := db.PromptRoutingDistribution(ctx, 1, time.Now().Add(-time.Hour))
	org2, _ := db.PromptRoutingDistribution(ctx, 2, time.Now().Add(-time.Hour))
	sum := func(bs []PromptRoutingBucket) int {
		s := 0
		for _, b := range bs {
			s += b.Count
		}
		return s
	}
	if sum(org1) != 2 || sum(org2) != 1 {
		t.Errorf("org isolation: org1=%d org2=%d (want 2/1)", sum(org1), sum(org2))
	}
}

// Recent feed must parse the routing_decision_json into structured fields
// the dashboard renders without client-side JSON parsing.
func TestRecentPromptRouting_ParsesDecision(t *testing.T) {
	db := newRoutingTestStore(t)
	ctx := context.Background()
	det := `{"route":"deterministic","sufficiency_score":1.0,"inferred_signals":["pod","dropship"],"reason_code":"self_sufficient_prompt","reason":"because"}`
	seedRouting(t, db, 1, "scrape_group", det, "crawl 50 in https://fb.com/groups/X")

	rows, err := db.RecentPromptRouting(ctx, 1, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len rows = %d; want 1", len(rows))
	}
	r := rows[0]
	if r.Route != "deterministic" {
		t.Errorf("route parsed = %q", r.Route)
	}
	if r.ReasonCode != "self_sufficient_prompt" {
		t.Errorf("reason_code parsed = %q", r.ReasonCode)
	}
	if r.SufficiencyScore != 1.0 {
		t.Errorf("sufficiency_score parsed = %v", r.SufficiencyScore)
	}
	if !strings.Contains(strings.Join(r.InferredSignals, ","), "pod") {
		t.Errorf("inferred_signals lost: %v", r.InferredSignals)
	}
}

// False-positive conflict heuristic: deterministic dispatch followed
// within 5 min by a retry/cancel prompt from the same account.
func TestPromptRoutingConflictCandidates_FalsePositive(t *testing.T) {
	db := newRoutingTestStore(t)
	ctx := context.Background()
	// Seed deterministic row, then a follow-up "wrong" prompt.
	det := `{"route":"deterministic","reason_code":"self_sufficient_prompt"}`
	seedRouting(t, db, 1, "scrape_group", det, "crawl 50 in https://fb.com/groups/X")
	// Sleep 1s so created_at strictly orders newer.
	time.Sleep(1100 * time.Millisecond)
	seedRouting(t, db, 1, "chat", "", "huỷ đi, sai rồi") // retry pattern

	conflicts, err := db.PromptRoutingConflictCandidates(ctx, 1, time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	fp := 0
	for _, c := range conflicts {
		if c.Kind == "false_positive_deterministic" {
			fp++
		}
	}
	if fp != 1 {
		t.Errorf("expected 1 false-positive conflict; got %d (all=%v)", fp, conflicts)
	}
}

// False-negative conflict heuristic: brain ask-back, but the predicate
// says the prompt was actually self-sufficient.
func TestPromptRoutingConflictCandidates_FalseNegative(t *testing.T) {
	db := newRoutingTestStore(t)
	ctx := context.Background()
	// Seed a brain ask-back on a prompt that IS self-sufficient (URL +
	// crawl verb + count). The injected predicate returns true.
	askDecision := `{"route":"brain","reason_code":"brain_ask_user","missing_signals":["target"]}`
	seedRouting(t, db, 1, "brain_ask_user", askDecision,
		"crawl 50 posts in https://facebook.com/groups/12345 looking for POD")

	selfSufficient := func(prompt string) bool {
		// Mock predicate: any prompt with "crawl" + "facebook.com/groups/"
		// is treated as self-sufficient for this test.
		return strings.Contains(prompt, "crawl") && strings.Contains(prompt, "facebook.com/groups/")
	}

	conflicts, err := db.PromptRoutingConflictCandidates(ctx, 1, time.Now().Add(-time.Hour), selfSufficient)
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	fn := 0
	for _, c := range conflicts {
		if c.Kind == "false_negative_deterministic" {
			fn++
		}
	}
	if fn != 1 {
		t.Errorf("expected 1 false-negative conflict; got %d", fn)
	}
}

// Without a self-sufficient predicate, false-negative detection is
// disabled (graceful — false-positive still works).
func TestPromptRoutingConflictCandidates_NilPredicate_FalseNegativeDisabled(t *testing.T) {
	db := newRoutingTestStore(t)
	ctx := context.Background()
	askDecision := `{"route":"brain","reason_code":"brain_ask_user"}`
	seedRouting(t, db, 1, "brain_ask_user", askDecision, "crawl 50 in https://fb.com/groups/X")

	conflicts, err := db.PromptRoutingConflictCandidates(ctx, 1, time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	for _, c := range conflicts {
		if c.Kind == "false_negative_deterministic" {
			t.Errorf("expected zero FN conflicts when predicate is nil; got %+v", c)
		}
	}
}

// MissingSignalDistribution should only count ask-back rows; deterministic
// dispatches don't count as ambiguity even if they happen to lack a
// market hint.
func TestMissingSignalDistribution_OnlyAskBacks(t *testing.T) {
	db := newRoutingTestStore(t)
	ctx := context.Background()
	// One ask-back with missing_signals=[source,target] — should count.
	askDecision := `{"route":"brain","reason_code":"brain_ask_user","missing_signals":["source","target"]}`
	seedRouting(t, db, 1, "brain_ask_user", askDecision, "tìm khách cho mình")
	// One deterministic dispatch — should NOT count even if MissingSignals exists.
	detDecision := `{"route":"deterministic","reason_code":"self_sufficient_prompt","missing_signals":["market"]}`
	seedRouting(t, db, 1, "scrape_group", detDecision, "crawl 50 in https://fb.com/groups/X")

	buckets, err := db.MissingSignalDistribution(ctx, 1, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("MissingSignalDistribution: %v", err)
	}
	got := map[string]int{}
	for _, b := range buckets {
		got[b.Signal] = b.Count
	}
	if got["source"] != 1 {
		t.Errorf("source count = %d; want 1", got["source"])
	}
	if got["target"] != 1 {
		t.Errorf("target count = %d; want 1", got["target"])
	}
	if got["market"] != 0 {
		t.Errorf("market should NOT count (it's on a deterministic row); got %d", got["market"])
	}
}
