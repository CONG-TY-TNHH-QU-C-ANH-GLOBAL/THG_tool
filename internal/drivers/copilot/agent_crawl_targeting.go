package copilot

import (
	"regexp"
	"strings"
)

// Folded Vietnamese role-inference keywords matched in more than one place
// (here and in agent_business_context.go). Extracted so each literal is defined
// once; values are unchanged.
const (
	roleKwUngVien  = "ung vien"  // candidate (recruitment)
	roleKwKhachMua = "khach mua" // buyer (sales)
	roleKwDoiTac   = "doi tac"   // partner
)

// businessCalibrationPreflight is a no-op kept for the two legacy call sites.
// Crawl is no longer blocked by missing profile — instead, mergeEphemeralCrawlTargeting
// derives target_author_role, target_signals, and negative_signals from the user's
// prompt and feeds them into the in-memory userContext for the request scope only.
func businessCalibrationPreflight(_ map[string]string, _ string) (bool, string) {
	return true, ""
}

// mergeEphemeralCrawlTargeting fills userCtx with prompt-derived crawl targeting
// (target_author_role, target_signals, negative_signals) when the user's prompt
// contains crawl intent. The merged values are scoped to the current request only —
// they are NOT persisted to the database. Empty inferred values do not overwrite
// existing profile values, so a configured profile still falls through.
func mergeEphemeralCrawlTargeting(userCtx map[string]string, prompt string) {
	if userCtx == nil {
		return
	}
	inferred := inferCrawlTargetingFromPrompt(prompt)
	for key, value := range inferred {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		userCtx[key] = value
		userCtx["org_"+key] = value
	}
}

// inferCrawlTargetingFromPrompt extracts a minimal Market Signal Gate from the
// user's current prompt: target audience role plus a handful of positive and
// negative phrases. Returns an empty map when the prompt is empty.
func inferCrawlTargetingFromPrompt(prompt string) map[string]string {
	out := map[string]string{}
	clean := strings.TrimSpace(regexp.MustCompile(`https?://\S+`).ReplaceAllString(stripDashboardContext(prompt), ""))
	if clean == "" {
		return out
	}
	folded := foldVietnameseForMatch(clean)

	role := "customers"
	switch {
	case containsAnyFolded(folded, []string{"tuyen ", "tuyen dung", "nhan su", roleKwUngVien, "tim viec", "can viec", "san sang lam"}):
		role = "candidates"
	case containsAnyFolded(folded, []string{"supplier", "nha cung cap", "nguon hang", "factory"}) && !containsAnyFolded(folded, []string{"tim khach", roleKwKhachMua, "tim buyer"}):
		role = "suppliers"
	case containsAnyFolded(folded, []string{roleKwDoiTac, "partner", "reseller", "agency hop tac"}):
		role = "partners"
	}
	out["target_author_role"] = role

	if positives := extractCrawlPositiveSignals(folded, role); len(positives) > 0 {
		out["target_signals"] = strings.Join(positives, ", ")
	}

	if negatives := defaultCrawlNegativeSignals(role); len(negatives) > 0 {
		out["negative_signals"] = strings.Join(negatives, ", ")
	}

	return out
}

// extractCrawlPositiveSignals returns folded phrases the prompt mentions that
// buyer/candidate/supplier posts would also use. Output is intentionally small —
// the LLM classifier (UniversalClassify) does the heavy lifting; we only nudge
// the SignalGate toward the right intent.
func extractCrawlPositiveSignals(folded, role string) []string {
	var pool []string
	switch role {
	case "candidates":
		pool = []string{
			"tim viec", "can viec", roleKwUngVien, "ho so", "cv",
			"remote ok", "san sang lam", "co kinh nghiem", "freelance",
		}
	case "suppliers":
		pool = []string{
			"nhan in pod", "nhan order", "nhan lam", "studio", "factory",
			"san xuat", "in theo yeu cau", "fulfillment", "warehouse",
		}
	case "partners":
		pool = []string{
			"hop tac", roleKwDoiTac, "reseller", "agency", "share doanh thu",
		}
	default:
		pool = []string{
			"tim supplier", "can supplier", "tim nha cung cap", "tim nguon hang",
			"tim hang", "can bao gia", "looking for supplier", "need supplier",
			"can tu van", "can tim", "ai co", "ai biet",
			"pod", "dropship", "fulfillment",
			"ship my", "ship usa", "ship eu", "ship sang my",
			"vn sang my", "tq sang my", "viet nam sang my", "trung quoc sang my",
		}
	}
	seen := map[string]bool{}
	var result []string
	for _, kw := range pool {
		if seen[kw] {
			continue
		}
		if strings.Contains(folded, kw) {
			seen[kw] = true
			result = append(result, kw)
		}
	}
	return result
}

// defaultCrawlNegativeSignals returns a baseline list of phrases to reject when
// targeting buyers/candidates — these are the most common provider/spam markers.
func defaultCrawlNegativeSignals(role string) []string {
	switch role {
	case "candidates":
		return []string{
			"tuyen ctv mlm", "lam giau nhanh", "lam viec tai nha 0 von",
			"co hoi kiem tien khong gioi han", "spam link",
		}
	case "suppliers":
		return nil
	case "partners":
		return nil
	default:
		return []string{
			"nhan lam pod", "nhan order pod", "shop pod nhan",
			"studio nhan in", "agency nhan", "fulfillment service offered",
			"chuyen nhan in pod", "xuong in pod nhan",
		}
	}
}
