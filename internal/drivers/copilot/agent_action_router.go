package copilot

import (
	"encoding/json"
	"fmt"
	"strings"
)

// deterministicFacebookAction + the entity/normalize helpers moved to the Copilot
// intent layer: intent_router.go, intent_entities.go, intent_normalize.go,
// intent_lexicon.go, intent_types.go. See
// specs/domains/facebook-sales-intelligence/features/sales-copilot/technical.md.

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func argMissing(args map[string]any, key string) bool {
	if args == nil {
		return true
	}
	v, ok := args[key]
	if !ok || v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case float64:
		return t == 0
	case int:
		return t == 0
	case int64:
		return t == 0
	default:
		return false
	}
}

func argStringFromMap(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func isCrawlerTool(name string) bool {
	switch name {
	case "scrape_group", "scrape_comments", "search_groups":
		return true
	default:
		return false
	}
}

// promptIsSelfSufficient reports whether the user's prompt already carries
// enough specificity to dispatch a crawl WITHOUT consulting the brain
// planner. Three signals together:
//
//  1. A Facebook URL (the source).
//  2. A crawl verb (cào/crawl/scrape/quét) — distinguishes execution
//     intent from a "what does this group look like?" research question.
//  3. EITHER an explicit max_items count OR inferred buyer/candidate/
//     supplier signals from the prompt body. Either is enough; together
//     they triangulate "the user already specified target + source + qty".
//
// When true, the orchestrator skips the brain planner (which can produce
// over-defensive ask_user responses for self-sufficient prompts) and goes
// straight to deterministic dispatch. This is the over-defensive-gating
// bug fix — the brain is for ambiguous prompts, not these.
//
// Returning false is the SAFE default: anything ambiguous falls through
// to the brain so it can still ask for clarification when truly needed.
func promptIsSelfSufficient(prompt string) bool {
	if firstFacebookURL(prompt) == "" {
		return false
	}
	folded := foldVietnameseForMatch(strings.ToLower(stripDashboardContext(prompt)))
	if !containsAnyFolded(folded, []string{"cao", "crawl", "scrape", "quet"}) {
		return false
	}
	// Either an explicit count OR inferred target signals make the prompt
	// self-describing. We do NOT count target_author_role alone — that
	// helper defaults to "customers" for empty prompts, so it'd always
	// trip and the gate would become meaningless.
	if extractMaxItemsFromPrompt(prompt) > 0 {
		return true
	}
	inferred := inferCrawlTargetingFromPrompt(prompt)
	return strings.TrimSpace(inferred["target_signals"]) != ""
}

// PromptIsSelfSufficient is the exported wrapper around the package-
// private gate. Exported because the observability handler needs the
// predicate for its false-negative conflict heuristic, and keeping the
// gate unexported elsewhere prevents accidental use by non-orchestrator
// code paths. There is exactly one decision logic — both call sites
// reach it through this name.
func PromptIsSelfSufficient(prompt string) bool {
	return promptIsSelfSufficient(prompt)
}

// promptIsLeadActionSelfSufficient reports whether the prompt is an
// outbound action on already-stored leads (comment / inbox / DM all
// leads). These prompts do not need pre-crawl business positioning —
// the leads already exist in the workspace, scored by the gate that
// captured them. The user is asking for action, not for clarification.
//
// Signals:
//  1. An outbound-action verb (comment / bình luận / inbox / messenger /
//     nhắn tin / DM).
//  2. A scope phrase indicating "all qualified leads" (lead / leads /
//     tệp khách / khách hàng / tất cả / all).
//  3. NO Facebook URL — a URL means "act on this specific source," which
//     is the crawl / scrape_comments path, not the leads-pool path.
//
// Mirrors promptIsSelfSufficient's intent: bypass the brain planner for
// fully-specified prompts so the user is not asked to re-position their
// business before each outbound run. brain.py's ask_user gate at
// services/agent-brain/brain.py:278 false-positives on the word "lead"
// for empty-profile orgs; this Go gate prevents the round-trip entirely.
func promptIsLeadActionSelfSufficient(prompt string) bool {
	if firstFacebookURL(prompt) != "" {
		return false
	}
	folded := foldVietnameseForMatch(strings.ToLower(stripDashboardContext(prompt)))
	if !containsAnyFolded(folded, []string{"comment", "binh luan", "inbox", "messenger", "nhan tin", "dm "}) {
		return false
	}
	return containsAnyFolded(folded, []string{"lead", "leads", "tep khach", "khach hang", "tat ca", "all"})
}

// PromptIsLeadActionSelfSufficient is the exported wrapper, mirroring
// PromptIsSelfSufficient. The observability handler uses it to label
// rows that bypassed the brain because of the outbound-on-leads gate
// rather than the URL+crawl gate.
func PromptIsLeadActionSelfSufficient(prompt string) bool {
	return promptIsLeadActionSelfSufficient(prompt)
}

// inferredTargetingSummary builds the human-readable "Target recognized:"
// line surfaced in the response when the orchestrator chose to infer the
// audience instead of asking. Empty string when no signal was inferred —
// in which case the response simply omits the line.
//
// The summary is operator-trust UX, not classifier input. Its job is to
// answer the user's implicit question "did you understand what I asked
// for, or did you just blindly crawl?" without making them open another
// panel to find out.
func inferredTargetingSummary(prompt string) string {
	inferred := inferCrawlTargetingFromPrompt(prompt)
	role := strings.TrimSpace(inferred["target_author_role"])
	signals := strings.TrimSpace(inferred["target_signals"])
	if role == "" && signals == "" {
		return ""
	}
	roleLabel := map[string]string{
		"customers":  "khách hàng tiềm năng (buyer-intent)",
		"candidates": "ứng viên / nhân sự",
		"suppliers":  "nhà cung cấp / nguồn hàng",
		"partners":   "đối tác / reseller",
	}[role]
	if roleLabel == "" {
		roleLabel = role
	}
	var parts []string
	parts = append(parts, "Đối tượng nhận diện: "+roleLabel)
	if signals != "" {
		parts = append(parts, "Tín hiệu khớp: "+signals)
	}
	if neg := strings.TrimSpace(inferred["negative_signals"]); neg != "" {
		parts = append(parts, "Lọc bỏ: "+neg)
	}
	return strings.Join(parts, "\n")
}

// promptKeywords moved to intent_entities.go (Copilot intent layer).
