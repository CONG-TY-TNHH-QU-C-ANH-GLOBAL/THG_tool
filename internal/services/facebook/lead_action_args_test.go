package facebook

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Pins the extracted Facebook synthetic-lead seam (Phase C). These assertions mirror
// the behavior characterized end-to-end in cmd/scraper/leads_from_action_args_test.go;
// they keep the contract stable now that the shaping lives in this FB-owned package.
func TestSyntheticLeadFromActionArgs(t *testing.T) {
	const org = int64(7)

	t.Run("comment prefers post_url over target_url and sets FB conventions", func(t *testing.T) {
		lead, ok := SyntheticLeadFromActionArgs(org, "comment", "https://fb.com/post", "https://fb.com/other", "Jane", "https://fb.com/jane", "hi")
		if !ok {
			t.Fatal("want ok=true")
		}
		if lead.SourceURL != "https://fb.com/post" {
			t.Errorf("SourceURL=%q want post_url", lead.SourceURL)
		}
		if lead.AuthorURL != "https://fb.com/jane" || lead.Author != "Jane" || lead.Content != "hi" {
			t.Errorf("fields=%+v", lead)
		}
		if lead.OrgID != org || lead.Score != models.LeadHot || lead.Platform != models.PlatformFacebook || lead.SourceType != "prompt_target" {
			t.Errorf("conventions=%+v", lead)
		}
	})

	t.Run("comment falls back to target_url", func(t *testing.T) {
		lead, ok := SyntheticLeadFromActionArgs(org, "comment", "", "https://fb.com/tgt", "", "", "")
		if !ok || lead.SourceURL != "https://fb.com/tgt" {
			t.Fatalf("ok=%v lead=%+v", ok, lead)
		}
	})

	t.Run("comment with no target → no synthetic lead", func(t *testing.T) {
		if _, ok := SyntheticLeadFromActionArgs(org, "comment", "", "", "", "", ""); ok {
			t.Fatal("want ok=false (falls through to work queue)")
		}
	})

	t.Run("non-comment puts target_url in AuthorURL, ignores author_url", func(t *testing.T) {
		lead, ok := SyntheticLeadFromActionArgs(org, "inbox", "https://fb.com/post", "https://fb.com/tgt", "Bob", "https://fb.com/ignored", "ctx")
		if !ok {
			t.Fatal("want ok=true")
		}
		if lead.AuthorURL != "https://fb.com/tgt" {
			t.Errorf("AuthorURL=%q want target_url", lead.AuthorURL)
		}
		if lead.SourceURL != "" {
			t.Errorf("SourceURL=%q want empty for non-comment", lead.SourceURL)
		}
		if lead.SourceType != "prompt_target" || lead.Platform != models.PlatformFacebook {
			t.Errorf("conventions=%+v", lead)
		}
	})

	t.Run("non-comment with no target_url → no synthetic lead", func(t *testing.T) {
		if _, ok := SyntheticLeadFromActionArgs(org, "inbox", "https://fb.com/post", "", "", "", ""); ok {
			t.Fatal("want ok=false")
		}
	})
}
