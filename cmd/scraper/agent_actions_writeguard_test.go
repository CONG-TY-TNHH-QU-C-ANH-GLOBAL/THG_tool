package main

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
)

// Item 1: every Facebook WRITE action fails closed when the requester identity is missing
// (user_id=0) — no org-wide resolution, no workflow/outbound/post job. The broad read action
// (scrape_group) is NOT account-guarded and proceeds even without a requester.
func TestAgentActionHandler_WriteActionsRequireRequester(t *testing.T) {
	db, js := newIntakeEnv(t)
	handler := makeAgentActionHandler(db, js, ai.NewMessageGenerator("", ""), nil)

	// No user_id → requester required (identity), for both pool and single-task write actions.
	for _, action := range []string{"comment_all_leads", "auto_comment", "inbox_all_leads", "auto_inbox", "create_job_post", "post_to_profile"} {
		out, err := handler(action, map[string]any{"org_id": int64(5), "nl_prompt": "x", "content": "x"})
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if out != msgDPRequesterRequired {
			t.Errorf("write action %s with no requester must fail closed (identity required), got %q", action, out)
		}
	}

	// Broad read/crawl is unchanged — it must NOT hit the account guard (requester not required).
	out, err := handler("scrape_group", map[string]any{"org_id": int64(5), "url": "https://www.facebook.com/groups/123456789"})
	if err != nil {
		t.Fatalf("scrape_group: %v", err)
	}
	if out == msgDPRequesterRequired || strings.Contains(out, "đăng nhập trong Chrome") {
		t.Errorf("broad crawl must NOT be account-guarded, got %q", out)
	}
}

// With a PROVEN requester but no controllable live account, pool actions fail closed with the
// pool message and single-task actions with the live-account guard message.
func TestAgentActionHandler_WriteActionsFailClosedNoLive(t *testing.T) {
	db, js := newIntakeEnv(t)
	handler := makeAgentActionHandler(db, js, ai.NewMessageGenerator("", ""), nil)
	base := func() map[string]any {
		return map[string]any{"org_id": int64(5), "user_id": int64(7), "nl_prompt": "x", "content": "x"}
	}

	for _, action := range []string{"comment_all_leads", "auto_comment", "inbox_all_leads", "auto_inbox"} {
		out, err := handler(action, base())
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if out != msgDPNoControllableLive {
			t.Errorf("pool action %s must fail closed (empty pool), got %q", action, out)
		}
	}
	for _, action := range []string{"create_job_post", "post_to_profile"} {
		out, err := handler(action, base())
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !strings.Contains(out, "đăng nhập trong Chrome") {
			t.Errorf("single-task action %s must fail closed on the live-account guard, got %q", action, out)
		}
	}
}
