package ai

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

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
		"   ":                          "",
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
		{"Website: www.thgfulfill.com/vi", "Website: https://thgfulfill.com/vi"}, // www variant → no-www canonical
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

// Case 4: empty website → no website mention ever (identity carries no
// URL; the contact rule forbids any URL; repair drops them).
func TestEmptyWebsite_NoMention(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill"} // no website
	if rule := buildContactRule(id); !strings.Contains(rule, "do NOT include any URL") {
		t.Errorf("contact rule must forbid URLs when website empty: %q", rule)
	}
	repaired, changed := RepairCommentContacts("Ghé thgfulfill.com/vi nhé", id)
	if !changed || strings.Contains(repaired, "thgfulfill") {
		t.Errorf("non-grounded URL must be stripped when no website configured: %q", repaired)
	}
}

// Case 5: the generated-comment pipeline (identity → repair → screen)
// ends with the official website EXACTLY as the normalized canonical
// value, and the result passes the contact guard.
func TestCommentPipeline_UsesCanonicalWebsiteExactly(t *testing.T) {
	profile := &BusinessProfile{Name: "THG Fulfill", Website: "thgfulfill.com/vi"}
	id := ResolveCompanyIdentity(profile, nil)
	if id.Website != "https://thgfulfill.com/vi" {
		t.Fatalf("identity website not canonical: %q", id.Website)
	}
	raw := "Bên mình hỗ trợ fulfill — xem thgfulfill. com/vi để biết thêm."
	repaired, changed := RepairCommentContacts(raw, id)
	if !changed || !strings.Contains(repaired, "https://thgfulfill.com/vi") {
		t.Fatalf("spaced mention not repaired to canonical: %q", repaired)
	}
	if strings.Contains(repaired, ". com") || strings.Contains(strings.ToLower(repaired), "thgfulfill com") {
		t.Fatalf("spaced domain survived repair: %q", repaired)
	}
	if ok, reason := ScreenCommentContacts(repaired, id); !ok {
		t.Fatalf("repaired canonical comment must pass screening, got %s", reason)
	}
}
