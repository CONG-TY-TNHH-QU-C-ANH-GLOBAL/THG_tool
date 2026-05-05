package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

const (
	brainPlanPath       = "/v1/plan"
	brainDefaultCap     = 200
	brainDefaultTimeout = 1500 * time.Millisecond
)

// BrainClient talks to the local Python planner sidecar. The sidecar is not a
// tool executor; it only returns a schema-first action plan for Go to validate.
type BrainClient struct {
	baseURL string
	client  *http.Client
}

func NewBrainClient(baseURL string, timeout time.Duration) *BrainClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if timeout <= 0 {
		timeout = brainDefaultTimeout
	}
	return &BrainClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *BrainClient) Available() bool {
	return c != nil && strings.TrimSpace(c.baseURL) != ""
}

func (c *BrainClient) Plan(ctx context.Context, req BrainPlanRequest) (*BrainPlanResponse, error) {
	if !c.Available() {
		return nil, errors.New("agent brain url is empty")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+brainPlanPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("agent brain HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out BrainPlanResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

type BrainPlanRequest struct {
	OrgID             int64              `json:"org_id"`
	Source            string             `json:"source"`
	Prompt            string             `json:"prompt"`
	BusinessProfile   *BusinessProfile   `json:"business_profile,omitempty"`
	SelectedAccountID int64              `json:"selected_account_id,omitempty"`
	Accounts          []BrainAccount     `json:"accounts"`
	DataSummaries     BrainDataSummaries `json:"data_summaries"`
	ToolCapabilities  []string           `json:"tool_capabilities"`
	Policy            BrainPolicy        `json:"policy"`
}

type BrainAccount struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	Status        string `json:"status"`
	Email         string `json:"email,omitempty"`
	FBUserID      string `json:"fb_user_id,omitempty"`
	FBDisplayName string `json:"fb_display_name,omitempty"`
	FBUsername    string `json:"fb_username,omitempty"`
	Ready         bool   `json:"ready"`
}

type BrainDataSummaries struct {
	PrivateFilesSummary string `json:"private_files_summary,omitempty"`
	DataSourcesSummary  string `json:"data_sources_summary,omitempty"`
}

type BrainPolicy struct {
	DefaultOutboundMode string   `json:"default_outbound_mode"`
	MaxItemsCap         int      `json:"max_items_cap"`
	BrowserRequiredFor  []string `json:"browser_required_for"`
}

type BrainPlanResponse struct {
	DomainScope      string                `json:"domain_scope"`
	Intent           string                `json:"intent"`
	Decision         string                `json:"decision"`
	Confidence       float64               `json:"confidence"`
	ResponseSummary  string                `json:"response_summary"`
	MarketSignalGate BrainMarketSignalGate `json:"market_signal_gate"`
	Actions          []BrainAction         `json:"actions"`
}

type BrainMarketSignalGate struct {
	TargetRole      string   `json:"target_role"`
	PositiveSignals []string `json:"positive_signals"`
	NegativeSignals []string `json:"negative_signals"`
	RejectRules     []string `json:"reject_rules"`
	MinConfidence   float64  `json:"min_confidence"`
}

type BrainAction struct {
	Tool            string          `json:"tool"`
	Args            map[string]any  `json:"args"`
	Reason          string          `json:"reason"`
	Evidence        []string        `json:"evidence"`
	RequiresBrowser bool            `json:"requires_browser"`
	RequiresProfile bool            `json:"requires_profile"`
	Recurrence      BrainRecurrence `json:"recurrence"`
}

type BrainRecurrence struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

func (a *Agent) processBrainPlan(ctx context.Context, prompt, source string, orgID, selectedAccountID int64, userContext map[string]string, accounts []models.Account) (string, bool) {
	if a == nil || a.brain == nil || !a.brain.Available() {
		return "", false
	}
	if ok, msg := facebookScopePreflight(prompt); !ok {
		a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "facebook_scope_guard", "", true)
		return msg, true
	}

	profile := ProfileFromContext(userContext)
	req := BrainPlanRequest{
		OrgID:             orgID,
		Source:            source,
		Prompt:            prompt,
		BusinessProfile:   profile,
		SelectedAccountID: selectedAccountID,
		Accounts:          brainAccounts(accounts),
		DataSummaries: BrainDataSummaries{
			PrivateFilesSummary: firstNonEmptyBrain(userContext["org_private_files_summary"], userContext["private_files_summary"]),
			DataSourcesSummary:  firstNonEmptyBrain(userContext["org_data_sources_summary"], userContext["data_sources_summary"]),
		},
		ToolCapabilities: brainToolCapabilities(),
		Policy: BrainPolicy{
			DefaultOutboundMode: firstNonEmptyBrain(userContext["org_outbound_mode"], userContext["outbound_mode"], "draft"),
			MaxItemsCap:         brainDefaultCap,
			BrowserRequiredFor:  brainBrowserTools(),
		},
	}

	plan, err := a.brain.Plan(ctx, req)
	if err != nil {
		log.Printf("[AgentBrain] planner unavailable, falling back to legacy router: %v", err)
		return "", false
	}
	if err := validateBrainPlan(plan); err != nil {
		msg := "Agent Brain returned an unsafe or invalid plan, so no automation was executed.\n\nDetail: " + err.Error()
		a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "brain_invalid_plan", mustJSON(plan), false)
		return msg, true
	}

	if strings.EqualFold(plan.DomainScope, "out_of_scope") || strings.EqualFold(plan.Decision, "refuse") {
		msg := strings.TrimSpace(plan.ResponseSummary)
		if msg == "" {
			msg = facebookScopeGuardMessage()
		}
		a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "brain_refuse", mustJSON(plan), true)
		return msg, true
	}

	if !strings.EqualFold(plan.Decision, "execute") || len(plan.Actions) == 0 {
		msg := strings.TrimSpace(plan.ResponseSummary)
		if msg == "" {
			msg = facebookActionNotExecutedMessage()
		}
		a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "brain_"+strings.ToLower(plan.Decision), mustJSON(plan), true)
		if strings.EqualFold(plan.Decision, "chat") {
			a.learnFromPrompt(prompt)
		}
		return msg, true
	}

	if actionPlanNeedsProfile(plan) {
		if ok, msg := businessCalibrationPreflight(userContext, prompt); !ok {
			a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "brain_business_preflight", mustJSON(plan), false)
			return msg, true
		}
	}
	if actionPlanNeedsBrowser(plan) {
		if ok, msg := facebookBrowserPreflight(accounts, selectedAccountID); !ok {
			a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "brain_browser_preflight", mustJSON(plan), false)
			return msg, true
		}
		if selectedAccountID <= 0 {
			selectedAccountID = pickReadyFacebookAccountID(accounts)
		}
	}
	if a.ActionHandler == nil {
		msg := "Action handler is not configured; Agent Brain plan was validated but not executed."
		a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "brain_no_action_handler", mustJSON(plan), false)
		return msg, true
	}

	var results []string
	var firstAction, firstArgs string
	success := false
	for _, action := range plan.Actions {
		args := prepareBrainActionArgs(action, plan.MarketSignalGate, prompt, orgID, selectedAccountID)
		fnResult, err := a.ActionHandler(action.Tool, args)
		if firstAction == "" {
			firstAction = action.Tool
			firstArgs = mustJSON(args)
		}
		if err != nil {
			results = append(results, fmt.Sprintf("ERROR `%s` -> %v", action.Tool, err))
			continue
		}
		success = true
		results = append(results, fmt.Sprintf("OK `%s` -> %s", action.Tool, fnResult))
	}

	raw := strings.Join(results, "\n\n")
	responseText := raw
	if firstAction != "" {
		responseText = polishActionResponse(firstAction, raw, prompt)
	}
	if !success && strings.TrimSpace(responseText) == "" {
		responseText = "Agent Brain plan was validated, but no action completed successfully."
	}
	a.logPrompt(orgID, selectedAccountID, source, prompt, responseText, "brain_"+firstAction, firstArgs, success)
	if success && firstAction != "" {
		a.updateMemory(prompt, firstAction, firstArgs)
		if isCrawlerTool(firstAction) {
			_ = a.db.SetContext("last_search_intent", prompt)
		}
	}
	return responseText, true
}

func validateBrainPlan(plan *BrainPlanResponse) error {
	if plan == nil {
		return errors.New("missing plan")
	}
	domain := strings.ToLower(strings.TrimSpace(plan.DomainScope))
	if domain != "facebook" && domain != "out_of_scope" {
		return fmt.Errorf("invalid domain_scope %q", plan.DomainScope)
	}
	decision := strings.ToLower(strings.TrimSpace(plan.Decision))
	switch decision {
	case "execute", "ask_user", "chat", "refuse":
	default:
		return fmt.Errorf("invalid decision %q", plan.Decision)
	}
	if plan.Confidence < 0 || plan.Confidence > 1 {
		return fmt.Errorf("confidence out of range: %v", plan.Confidence)
	}
	for _, action := range plan.Actions {
		if err := validateBrainAction(action); err != nil {
			return err
		}
	}
	return nil
}

func validateBrainAction(action BrainAction) error {
	tool := strings.TrimSpace(action.Tool)
	if !allowedBrainTool(tool) {
		return fmt.Errorf("tool %q is not allowed", action.Tool)
	}
	switch tool {
	case "scrape_group":
		if !isFacebookURL(argStringFromMap(action.Args, "url")) {
			return errors.New("scrape_group requires a concrete Facebook url")
		}
	case "scrape_comments":
		if !isFacebookURL(argStringFromMap(action.Args, "post_url")) {
			return errors.New("scrape_comments requires a concrete Facebook post_url")
		}
	case "search_groups":
		query := strings.TrimSpace(argStringFromMap(action.Args, "query"))
		if query == "" || tooBroadBrainQuery(query) {
			return errors.New("search_groups requires a specific query")
		}
	case "add_group":
		if !isFacebookURL(argStringFromMap(action.Args, "url")) {
			return errors.New("add_group requires a Facebook url")
		}
	case "auto_comment":
		if !isFacebookURL(argStringFromMap(action.Args, "post_url")) {
			return errors.New("auto_comment requires a Facebook post_url")
		}
	case "auto_inbox":
		if !isFacebookURL(argStringFromMap(action.Args, "target_url")) {
			return errors.New("auto_inbox requires a Facebook target_url")
		}
	case "create_job_post":
		if strings.TrimSpace(firstNonEmptyBrain(argStringFromMap(action.Args, "content"), argStringFromMap(action.Args, "description"), argStringFromMap(action.Args, "title"))) == "" {
			return errors.New("create_job_post requires title, description, or content")
		}
	case "scan_fanpage_inbox":
		if !isFacebookURL(argStringFromMap(action.Args, "page_url")) {
			return errors.New("scan_fanpage_inbox requires a concrete Facebook page_url")
		}
	case "care_fanpage":
		if !isFacebookURL(argStringFromMap(action.Args, "page_url")) || strings.TrimSpace(argStringFromMap(action.Args, "action")) == "" {
			return errors.New("care_fanpage requires page_url and action")
		}
	case "post_to_profile":
		if strings.TrimSpace(argStringFromMap(action.Args, "content")) == "" {
			return errors.New("post_to_profile requires content")
		}
	case "set_context":
		if strings.TrimSpace(argStringFromMap(action.Args, "key")) == "" || strings.TrimSpace(argStringFromMap(action.Args, "value")) == "" {
			return errors.New("set_context requires key and value")
		}
	case "describe_business":
		if strings.TrimSpace(argStringFromMap(action.Args, "description")) == "" {
			return errors.New("describe_business requires description")
		}
	}
	return nil
}

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

func allowedBrainTool(name string) bool {
	return brainAllowedTools()[strings.TrimSpace(name)]
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
	switch tool {
	case "auto_comment", "comment_all_leads", "auto_inbox", "inbox_all_leads", "create_job_post", "care_fanpage", "post_to_profile":
		return true
	default:
		return false
	}
}

func isFacebookURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "facebook.com") || strings.Contains(host, "fb.com")
}

func tooBroadBrainQuery(query string) bool {
	folded := foldVietnameseForMatch(query)
	parts := strings.Fields(folded)
	if len(parts) < 2 {
		switch strings.TrimSpace(folded) {
		case "facebook", "fb", "group", "groups", "lead", "leads", "khach", "khach hang":
			return true
		}
	}
	return false
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

func firstNonEmptyBrain(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
