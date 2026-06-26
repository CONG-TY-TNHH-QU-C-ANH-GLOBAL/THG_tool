package copilot

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/models"
)

func prepareBrainActionArgs(action BrainAction, gate BrainMarketSignalGate, prompt string, orgID, accountID int64) map[string]any {
	args := map[string]any{}
	for k, v := range action.Args {
		key := strings.ToLower(strings.TrimSpace(k))
		switch key {
		case "org_id", "account_id", "auto":
			continue
		default:
			args[k] = v
		}
	}
	if orgID > 0 {
		args["org_id"] = orgID
	}
	if accountID > 0 && brainToolNeedsAccount(action.Tool) {
		args["account_id"] = accountID
	}
	args["user_prompt"] = prompt
	if isCrawlerTool(action.Tool) && strings.TrimSpace(argStringFromMap(args, "keywords")) == "" {
		if kw := promptKeywords(prompt); kw != "" {
			args["keywords"] = kw
		}
	}
	if n := clampBrainMaxItems(args["max_items"]); n > 0 {
		args["max_items"] = n
	} else if n := extractMaxItemsFromPrompt(prompt); n > 0 {
		args["max_items"] = n
	}
	if action.Recurrence.Enabled && action.Recurrence.IntervalMinutes > 0 {
		args["interval_minutes"] = action.Recurrence.IntervalMinutes
	}
	if brainToolIsOutbound(action.Tool) {
		args["auto"] = wantsAutoOutbound(prompt)
	}
	if gate.TargetRole != "" || len(gate.PositiveSignals) > 0 || len(gate.NegativeSignals) > 0 || len(gate.RejectRules) > 0 {
		args["market_signal_gate"] = gate
	}
	return args
}

func brainAccounts(accounts []models.Account) []BrainAccount {
	out := make([]BrainAccount, 0, len(accounts))
	for _, acc := range accounts {
		out = append(out, BrainAccount{
			ID:            acc.ID,
			Name:          acc.Name,
			Platform:      string(acc.Platform),
			Status:        string(acc.Status),
			Email:         acc.Email,
			FBUserID:      acc.FBUserID,
			FBDisplayName: acc.FBDisplayName,
			FBUsername:    acc.FBUsername,
			Ready:         accountReadyForFacebookAutomation(acc),
		})
	}
	return out
}

func brainToolCapabilities() []string {
	names := make([]string, 0, len(brainAllowedTools()))
	for name := range brainAllowedTools() {
		names = append(names, name)
	}
	return names
}

func brainAllowedTools() map[string]bool {
	return map[string]bool{
		"set_context":        true,
		"describe_business":  true,
		"get_stats":          true,
		"add_group":          true,
		"scrape_group":       true,
		"scrape_comments":    true,
		"classify_leads":     true,
		"search_groups":      true,
		"auto_comment":       true,
		"comment_all_leads":  true,
		"auto_inbox":         true,
		"inbox_all_leads":    true,
		"create_job_post":    true,
		"scan_fanpage_inbox": true,
		"care_fanpage":       true,
		"post_to_profile":    true,
	}
}

func brainBrowserTools() []string {
	return []string{
		"scrape_group",
		"scrape_comments",
		"search_groups",
		"auto_comment",
		"comment_all_leads",
		"auto_inbox",
		"inbox_all_leads",
		"create_job_post",
		"scan_fanpage_inbox",
		"care_fanpage",
		"post_to_profile",
	}
}

func actionPlanNeedsBrowser(plan *BrainPlanResponse) bool {
	for _, action := range plan.Actions {
		if action.RequiresBrowser || brainToolNeedsBrowser(action.Tool) {
			return true
		}
	}
	return false
}

func actionPlanNeedsProfile(plan *BrainPlanResponse) bool {
	for _, action := range plan.Actions {
		if action.RequiresProfile || brainToolNeedsProfile(action.Tool) {
			return true
		}
	}
	return false
}

func brainToolNeedsBrowser(tool string) bool {
	switch tool {
	case "scrape_group", "scrape_comments", "search_groups", "auto_comment", "comment_all_leads", "auto_inbox", "inbox_all_leads", "create_job_post", "scan_fanpage_inbox", "care_fanpage", "post_to_profile":
		return true
	default:
		return false
	}
}

func brainToolNeedsProfile(tool string) bool {
	switch tool {
	case "scrape_group", "scrape_comments", "search_groups", "auto_comment", "comment_all_leads", "auto_inbox", "inbox_all_leads", "create_job_post", "scan_fanpage_inbox", "care_fanpage", "post_to_profile":
		return true
	default:
		return false
	}
}

func brainToolNeedsAccount(tool string) bool {
	return brainToolNeedsBrowser(tool)
}

func brainToolIsOutbound(tool string) bool {
	return outboundToolUsesPolicy(tool)
}

func clampBrainMaxItems(v any) int64 {
	n := brainInt64(v)
	if n <= 0 {
		return 0
	}
	if n > brainDefaultCap {
		n = brainDefaultCap
	}
	return n
}

func brainInt64(v any) int64 {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int64:
		return t
	case float64:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}
