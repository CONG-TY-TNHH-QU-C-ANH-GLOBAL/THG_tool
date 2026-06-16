package copilot

import "testing"

// P1.3D: every Facebook WRITE action must NOT first-ready-pick an account (they resolve from
// live connector identity downstream and fail closed). Broad read/crawl/search actions keep
// their existing auto-pick fallback.
func TestIsRiskyDirectAccountAction(t *testing.T) {
	writes := []string{"comment_single_post", "auto_comment", "comment_all_leads", "auto_inbox", "inbox_all_leads", "create_job_post", "post_to_profile"}
	for _, a := range writes {
		if !isRiskyDirectAccountAction(a) {
			t.Errorf("write action %s must be risky (no first-ready fallback)", a)
		}
	}
	reads := []string{"scrape_group", "scrape_comments", "search_groups", "add_group", "classify_leads"}
	for _, a := range reads {
		if isRiskyDirectAccountAction(a) {
			t.Errorf("read/crawl action %s must keep its existing fallback (not scoped as risky)", a)
		}
	}
}
