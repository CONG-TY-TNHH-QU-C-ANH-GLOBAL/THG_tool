package comment

import "testing"

// Case 1 (PR-6, review-updated): EVERY variant of the domain — with or
// without scheme, with or without www — normalizes to exactly
// https://thgfulfill.com (+path). The canonical host has NO www.
func TestCanonicalWebsite_NormalizesToClickable(t *testing.T) {
	cases := map[string]string{
		"thgfulfill.com":                "https://thgfulfill.com",
		"www.thgfulfill.com":            "https://thgfulfill.com",
		"https://thgfulfill.com":        "https://thgfulfill.com",
		"https://www.thgfulfill.com":    "https://thgfulfill.com",
		"http://www.thgfulfill.com":     "https://thgfulfill.com",
		"www.thgfulfill.com/vi":         "https://thgfulfill.com/vi",
		"https://www.thgfulfill.com/vi": "https://thgfulfill.com/vi",
		"THGFulfill.com/Vi":             "https://thgfulfill.com/Vi", // host lowercased, path kept
		"thgfulfill. com/vi":            "https://thgfulfill.com/vi", // healed spacing at storage
		"https://thgfulfill.com/":       "https://thgfulfill.com",
		"":                              "",
		"   ":                           "",
	}
	for in, want := range cases {
		if got := CanonicalWebsite(in); got != want {
			t.Errorf("CanonicalWebsite(%q) = %q, want %q", in, got, want)
		}
	}
}

// Cases 2+3: spaced/malformed domain output is repaired to the
// canonical URL; well-formed but non-canonical variants are rewritten.
func TestRepairWebsiteMentions_SpacedAndVariants(t *testing.T) {
	canonical := "https://thgfulfill.com/vi"
	cases := []struct {
		in   string
		want string
	}{
		{"Xem thêm tại thgfulfill. com/vi nhé", "Xem thêm tại https://thgfulfill.com/vi nhé"},
		{"Xem thêm tại thgfulfill com nhé", "Xem thêm tại https://thgfulfill.com/vi nhé"},
		{"Website: http://thgfulfill.com/vi", "Website: https://thgfulfill.com/vi"},
		{"Website: www.thgfulfill.com/vi", "Website: https://thgfulfill.com/vi"},             // www variant → no-www canonical
		{"Đã có https://thgfulfill.com/vi rồi", "Đã có https://thgfulfill.com/vi rồi"},       // canonical untouched
		{"Trang khác example.com không liên quan", "Trang khác example.com không liên quan"}, // other domains untouched
	}
	for _, c := range cases {
		got, _ := RepairWebsiteMentions(c.in, canonical)
		if got != c.want {
			t.Errorf("RepairWebsiteMentions(%q):\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

// NOTE: the identity->repair->screen pipeline tests (TestEmptyWebsite_NoMention,
// TestCommentPipeline_UsesCanonicalWebsiteExactly) live at root in
// internal/ai/comment_brand_test.go — they exercise root symbols
// (ResolveCompanyIdentity, buildContactRule) alongside the moved comment.* calls.
