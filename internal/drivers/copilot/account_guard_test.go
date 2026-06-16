package copilot

import "testing"

// P1.3D: the direct-post single-comment action must NOT first-ready-pick an account (it
// resolves from live connector identity downstream and fails closed). Broad crawl / bulk /
// post actions keep their existing auto-pick fallback — they are not scoped here.
func TestIsRiskyDirectAccountAction(t *testing.T) {
	if !isRiskyDirectAccountAction("comment_single_post") {
		t.Error("comment_single_post must be risky (no first-ready fallback)")
	}
	for _, a := range []string{"scrape_group", "scrape_comments", "comment_all_leads", "create_job_post", "post_to_profile", "search_groups"} {
		if isRiskyDirectAccountAction(a) {
			t.Errorf("%s must keep its existing fallback (not scoped as risky)", a)
		}
	}
}
