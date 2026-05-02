package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Agent is an AI-powered operator that interprets natural language prompts
// and executes production workspace actions using OpenAI Function Calling.
// It is fully prompt-driven: no hardcoded industry logic. The user's prompts
// define what to scrape, what qualifies as a "match", and how to engage.
type Agent struct {
	apiKey string
	model  string
	db     *store.Store
	client *http.Client
	// ActionHandler is set by the orchestrator to execute actions
	ActionHandler func(action string, args map[string]any) (string, error)
}

// NewAgent creates a new AI Agent powered by OpenAI.
func NewAgent(apiKey, model string, db *store.Store) *Agent {
	return &Agent{
		apiKey: apiKey,
		model:  model,
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Available returns true if the agent has a valid API key.
func (a *Agent) Available() bool {
	return a.apiKey != ""
}

// ProcessPrompt takes a user prompt, sends it to OpenAI with function definitions,
// and executes the appropriate action. Returns the AI response text.
func (a *Agent) ProcessPrompt(ctx context.Context, prompt, source string) (string, error) {
	return a.ProcessPromptForOrg(ctx, prompt, source, 0)
}

// ProcessPromptForOrg runs a prompt with tenant-scoped business context and
// injects org_id into production tool calls.
func (a *Agent) ProcessPromptForOrg(ctx context.Context, prompt, source string, orgID int64) (string, error) {
	return a.ProcessPromptForOrgWithAccount(ctx, prompt, source, orgID, 0)
}

// ProcessPromptForOrgWithAccount runs a prompt with tenant scope plus an
// optional dashboard-selected Facebook account. The selected account is kept
// out of user-visible prompt text and injected directly into tool args.
func (a *Agent) ProcessPromptForOrgWithAccount(ctx context.Context, prompt, source string, orgID int64, selectedAccountID int64) (string, error) {
	if !a.Available() {
		return "", fmt.Errorf("OpenAI API key not configured")
	}
	if selectedAccountID <= 0 {
		selectedAccountID = extractDashboardAccountID(prompt)
	}
	prompt = stripDashboardContext(prompt)

	// Load dynamic user context (business rules, niche, etc.)
	userContext := a.loadUserContext()
	if orgID > 0 {
		for _, key := range orgContextKeysForPrompt() {
			if v, err := a.db.GetContext(fmt.Sprintf("org:%d:%s", orgID, key)); err == nil && strings.TrimSpace(v) != "" {
				userContext["org_"+key] = strings.TrimSpace(v)
				userContext[key] = strings.TrimSpace(v)
			}
		}
		if userContext["org_business_profile"] != "" {
			userContext["business_desc"] = userContext["org_business_profile"]
		}
	}

	// Load accounts for AI account mapping
	accounts, _ := a.db.GetAllAccounts(orgID)
	if requiresFacebookBrowser(prompt) {
		if ok, msg := businessCalibrationPreflight(userContext, prompt); !ok {
			a.logPrompt(source, prompt, msg, "business_preflight", "", false)
			return msg, nil
		}
		if ok, msg := facebookBrowserPreflight(accounts, selectedAccountID); !ok {
			a.logPrompt(source, prompt, msg, "browser_preflight", "", false)
			return msg, nil
		}
		if selectedAccountID <= 0 {
			selectedAccountID = pickReadyFacebookAccountID(accounts)
		}
	}

	// Get semantically relevant few-shot examples
	fewShots := a.getFewShotExamples(prompt)

	// Build system prompt with dynamic context injected
	sysPrompt := buildDynamicSystemPrompt(userContext, accounts)

	// Build messages
	messages := []map[string]string{
		{"role": "system", "content": sysPrompt},
	}
	for _, fs := range fewShots {
		messages = append(messages,
			map[string]string{"role": "user", "content": fs.UserPrompt},
			map[string]string{"role": "assistant", "content": fmt.Sprintf(`Đã thực thi: %s(%s)`, fs.BestAction, fs.BestArgs)},
		)
	}
	messages = append(messages, map[string]string{"role": "user", "content": prompt})

	// Call OpenAI with function definitions
	body := map[string]any{
		"model":       a.model,
		"messages":    messages,
		"tools":       productionAgentTools(),
		"tool_choice": "auto",
		"temperature": 0.05,
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	choice := result.Choices[0]
	var responseText string
	var actionTaken, actionArgs string
	var success bool

	if len(choice.Message.ToolCalls) > 0 {
		// Process ALL tool calls (not just the first)
		var allResults []string
		for _, tc := range choice.Message.ToolCalls {
			fnName := tc.Function.Name
			fnArgs := tc.Function.Arguments

			log.Printf("[Agent] Function call: %s(%s)", fnName, fnArgs)

			var args map[string]any
			_ = json.Unmarshal([]byte(fnArgs), &args)
			if args == nil {
				args = map[string]any{}
			}
			if orgID > 0 {
				args["org_id"] = orgID
			}
			if selectedAccountID > 0 && argMissing(args, "account_id") {
				args["account_id"] = selectedAccountID
			}
			args["user_prompt"] = prompt
			if isCrawlerTool(fnName) && argStringFromMap(args, "keywords") == "" {
				if kw := promptKeywords(prompt); kw != "" {
					args["keywords"] = kw
				}
			}
			if wantsAutoOutbound(prompt) {
				args["auto"] = true
			}

			if a.ActionHandler != nil {
				fnResult, err := a.ActionHandler(fnName, args)
				if err != nil {
					allResults = append(allResults, fmt.Sprintf("❌ Lỗi %s: %v", fnName, err))
				} else {
					allResults = append(allResults, fmt.Sprintf("✅ `%s` → %s", fnName, fnResult))
					success = true
				}
			} else {
				allResults = append(allResults, "⚠️ Action handler chưa được cấu hình")
			}

			// Track first action for logging
			if actionTaken == "" {
				actionTaken = fnName
				actionArgs = fnArgs
			}
		}

		responseText = polishActionResponse(actionTaken, strings.Join(allResults, "\n\n"), prompt)

		// If user is setting context via prompt, learn it
		if actionTaken == "set_context" && success {
			a.learnFromPrompt(prompt)
		}
		// Save user's search intent when scraping
		if actionTaken == "scrape_group" && success {
			_ = a.db.SetContext("last_search_intent", prompt)
			log.Printf("[Agent] Saved search intent: %s", prompt)
		}
	} else {
		responseText = choice.Message.Content
		actionTaken = "chat"
		success = true
		// Always try to learn business context from conversational prompts
		a.learnFromPrompt(prompt)
	}

	// Log prompt for learning
	a.logPrompt(source, prompt, responseText, actionTaken, actionArgs, success)

	// Update memory for learning
	if success && actionTaken != "chat" {
		a.updateMemory(prompt, actionTaken, actionArgs)
	}

	return responseText, nil
}

func wantsAutoOutbound(prompt string) bool {
	lower := strings.ToLower(prompt)
	triggers := []string{
		"gửi luôn", "gui luon", "chạy luôn", "chay luon", "tự động", "tu dong",
		"không cần duyệt", "khong can duyet", "auto", "automation hết", "automation het",
		"comment lên", "comment len", "inbox leads", "inbox tất cả", "inbox tat ca",
		"post lên", "post len", "đăng lên", "dang len", "posting",
	}
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

func stripDashboardContext(prompt string) string {
	marker := "\n\nDashboard context:"
	if idx := strings.Index(prompt, marker); idx >= 0 {
		return strings.TrimSpace(prompt[:idx])
	}
	return strings.TrimSpace(prompt)
}

func extractDashboardAccountID(prompt string) int64 {
	re := regexp.MustCompile(`account_id\s*=\s*(\d+)`)
	m := re.FindStringSubmatch(prompt)
	if len(m) < 2 {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

func requiresFacebookBrowser(prompt string) bool {
	lower := strings.ToLower(stripDashboardContext(prompt))
	if strings.Contains(lower, "facebook.com") || strings.Contains(lower, "fb.com") {
		return true
	}
	triggers := []string{
		"cào", "cao ", "crawl", "scrape", "quét", "quet ",
		"tìm tệp", "tim tep", "tệp khách", "tep khach", "tìm khách", "tim khach",
		"lead", "leads", "group", "nhóm", "nhom",
		"comment", "bình luận", "binh luan", "inbox", "messenger",
		"đăng bài", "dang bai", "posting", "post lên", "post len",
	}
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

func orgContextKeysForPrompt() []string {
	return []string{
		"business_profile",
		"business_name",
		"business_industry",
		"services",
		"target_customers",
		"target_author_role",
		"target_signals",
		"negative_signals",
		"business_location",
		"markets",
		"business_usp",
		"tone",
		"approval_policy",
		"reject_rules",
		"private_files_summary",
		"data_sources_summary",
		"outbound_mode",
	}
}

func facebookBrowserPreflight(accounts []models.Account, selectedAccountID int64) (bool, string) {
	if selectedAccountID > 0 {
		for _, acc := range accounts {
			if acc.ID != selectedAccountID {
				continue
			}
			if accountReadyForFacebookAutomation(acc) {
				return true, ""
			}
			return false, browserNotReadyMessage(&acc)
		}
		return false, browserNotReadyMessage(nil)
	}
	for _, acc := range accounts {
		if accountReadyForFacebookAutomation(acc) {
			return true, ""
		}
	}
	return false, browserNotReadyMessage(nil)
}

func accountReadyForFacebookAutomation(acc models.Account) bool {
	return acc.Platform == models.PlatformFacebook &&
		acc.BrowserLoggedIn &&
		acc.Status == models.AccountActive &&
		strings.TrimSpace(acc.FBUserID) != ""
}

func pickReadyFacebookAccountID(accounts []models.Account) int64 {
	for _, acc := range accounts {
		if accountReadyForFacebookAutomation(acc) {
			return acc.ID
		}
	}
	return 0
}

func businessCalibrationPreflight(userCtx map[string]string, prompt string) (bool, string) {
	profile := ProfileFromContext(userCtx)
	if profile.IsConfigured() {
		return true, ""
	}
	if isBusinessContextPrompt(prompt) {
		return true, ""
	}
	return false, `Mình chưa chạy crawl ngay vì workspace chưa có định vị doanh nghiệp đủ rõ.

Để Market Signal Gate lọc đúng tệp và không đổ dữ liệu rác vào dashboard, hãy cấu hình trước phần Định vị doanh nghiệp trong Data Private, hoặc trả lời trực tiếp theo 5 ý ngắn:

1. Doanh nghiệp/brand của bạn là ai?
2. Bạn đang bán sản phẩm, dịch vụ hoặc offer gì?
3. Tệp cần tìm là ai: khách mua dịch vụ, supplier, partner, ứng viên hay nhóm khác?
4. Những tín hiệu nào phải giữ lại? Ví dụ: “cần báo giá”, “looking for supplier”, “tìm fulfillment”.
5. Những tín hiệu nào phải loại bỏ? Ví dụ: bài quảng cáo dịch vụ, tuyển CTV, spam link, đối thủ tự bán.

Sau khi lưu định vị, gửi lại prompt. Lúc đó agent sẽ dùng đúng Facebook session của workspace để crawl, lọc theo ngữ cảnh doanh nghiệp, phân loại hot/warm/cold và chỉ lưu leads đủ điều kiện.`
}

func isBusinessContextPrompt(prompt string) bool {
	lower := strings.ToLower(stripDashboardContext(prompt))
	triggers := []string{
		"định vị doanh nghiệp", "dinh vi doanh nghiep", "business profile", "business context",
		"mình là", "minh la", "chúng tôi là", "chung toi la", "công ty", "cong ty",
		"doanh nghiệp", "doanh nghiep", "brand", "thương hiệu", "thuong hieu",
		"dịch vụ của tôi", "dich vu cua toi", "chúng tôi bán", "chung toi ban",
	}
	for _, trigger := range triggers {
		if strings.Contains(lower, trigger) {
			return true
		}
	}
	return false
}

func browserNotReadyMessage(acc *models.Account) string {
	target := "Workspace chưa có Facebook session sẵn sàng."
	if acc != nil {
		target = fmt.Sprintf("Facebook account %q chưa sẵn sàng để chạy automation.", acc.Name)
	}
	return target + `

THG chỉ chạy crawl/comment/inbox khi Browser đã xác nhận đúng Facebook session thật của workspace. Cách này tránh chạy nhầm tài khoản và giữ dữ liệu theo đúng organization.

Vào tab Browser và hoàn tất 3 bước:
1. Chạy THG Local Kit trên thiết bị đã ghép với workspace.
2. Đăng nhập Facebook trong Chrome Runtime nếu hệ thống yêu cầu.
3. Chờ trạng thái chuyển sang Facebook local ready.

Khi Browser đã sẵn sàng, gửi lại prompt này. Agent sẽ dùng đúng account đã xác thực để thu dữ liệu thật, phân loại leads và lưu kết quả về workspace.`
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

func promptKeywords(prompt string) string {
	prompt = stripDashboardContext(prompt)
	prompt = regexp.MustCompile(`https?://\S+`).ReplaceAllString(prompt, " ")
	cleaner := strings.NewReplacer(
		"\n", " ", "\t", " ", ".", " ", ",", ",", ";", ",", ":", " ",
		"(", " ", ")", " ", "[", " ", "]", " ", "\"", " ", "'", " ",
	)
	prompt = cleaner.Replace(prompt)
	fields := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '/'
	})
	stop := map[string]bool{
		"cào": true, "cao": true, "crawl": true, "scrape": true, "tôi": true, "toi": true,
		"cần": true, "can": true, "tìm": true, "tim": true, "tệp": true, "tep": true,
		"khách": true, "khach": true, "có": true, "co": true, "nhu": true, "cầu": true,
		"cau": true, "hoặc": true, "hoac": true, "từ": true, "tu": true, "đi": true,
		"di": true, "và": true, "va": true, "the": true, "a": true, "an": true,
	}
	out := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, raw := range fields {
		for _, token := range strings.Fields(raw) {
			token = strings.Trim(token, " -_")
			if len([]rune(token)) < 3 || stop[token] || seen[token] {
				continue
			}
			seen[token] = true
			out = append(out, token)
			if len(out) >= 8 {
				return strings.Join(out, ", ")
			}
		}
	}
	return strings.Join(out, ", ")
}

func polishActionResponse(action, raw, prompt string) string {
	switch action {
	case "scrape_group", "scrape_comments":
		return crawlerQueuedMessage(raw, prompt, "group/post Facebook đã chọn")
	case "search_groups":
		return crawlerQueuedMessage(raw, prompt, "tìm nguồn Facebook phù hợp")
	default:
		return raw
	}
}

func crawlerQueuedMessage(raw, prompt, sourceLabel string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "❌") || strings.Contains(strings.ToLower(raw), "lỗi") || strings.Contains(strings.ToLower(raw), "error") {
		return "Chưa thể khởi động crawler cho lệnh này.\n\n" +
			"Lý do kỹ thuật: backend chưa trả về mã thực thi hợp lệ từ hàng đợi crawl.\n\n" +
			"Bạn giữ THG Local Kit đang online ở tab Browser, sau đó gửi lại lệnh. Nếu vẫn lặp lại, kiểm tra terminal Runtime xem có dòng `[Input] received ... command(s)` hoặc lỗi `crawl command` để xác định Runtime có nhận lệnh chưa."
	}
	jobID := ""
	if m := regexp.MustCompile(`job #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		jobID = m[1]
	}
	localCommandID := ""
	if m := regexp.MustCompile(`local crawler command #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		localCommandID = m[1]
	}
	taskID := ""
	if m := regexp.MustCompile(`task=([a-zA-Z0-9_-]+)`).FindStringSubmatch(raw); len(m) == 2 {
		taskID = m[1]
	}
	var sb strings.Builder
	if localCommandID != "" {
		sb.WriteString("Đã gửi lệnh crawl xuống THG Local Runtime đang online.\n\n")
	} else {
		sb.WriteString("Đã nhận lệnh crawl và đưa vào hàng đợi xử lý.\n\n")
	}
	sb.WriteString("Mục tiêu: ")
	sb.WriteString(strings.TrimSpace(stripDashboardContext(prompt)))
	sb.WriteString("\n")
	sb.WriteString("Nguồn: ")
	sb.WriteString(sourceLabel)
	sb.WriteString("\n")
	if jobID != "" {
		sb.WriteString("Job: #")
		sb.WriteString(jobID)
		sb.WriteString("\n")
	}
	if localCommandID != "" {
		sb.WriteString("Local command: #")
		sb.WriteString(localCommandID)
		sb.WriteString("\n")
	}
	if taskID != "" {
		sb.WriteString("Task: ")
		sb.WriteString(taskID)
		sb.WriteString("\n")
	}
	sb.WriteString("\nAutomation 24/7: hệ thống sẽ ghi nhớ nhu cầu này thành lịch crawl định kỳ 30 phút cho workspace. Các vòng sau dùng scheduler và THG Local Runtime, không gọi AI lại nếu không cần phân tích/ngôn ngữ.")
	if localCommandID != "" {
		sb.WriteString("\nRuntime sẽ điều khiển Chrome Facebook thật trên thiết bị đã ghép, thu dữ liệu từ nguồn bạn đưa, lọc tín hiệu theo prompt và lưu leads đủ điều kiện về Leads. Bạn có thể quan sát luồng chạy trong tab Browser.")
	} else {
		sb.WriteString("\nHệ thống đã tạo job nền. Nếu bạn đang dùng THG Local Runtime, phản hồi chuẩn phải có `Local command`. Khi không thấy `Local command`, nghĩa là lệnh chưa được dispatch xuống Chrome local và cần kiểm tra account/session routing.")
	}
	return sb.String()
}

// --- Dynamic Context ---

// loadUserContext retrieves stored business rules from the database.
func (a *Agent) loadUserContext() map[string]string {
	ctx, err := a.db.GetAllContext()
	if err != nil {
		return map[string]string{}
	}
	return ctx
}

// learnFromPrompt extracts business intent keywords from user prompts
// and stores them as context for future use.
func (a *Agent) learnFromPrompt(prompt string) {
	lower := strings.ToLower(prompt)

	// Save search-related prompts as search intent
	searchKeywords := []string{"tìm", "cào", "quét", "scan", "scrape", "tệp khách", "lead"}
	for _, kw := range searchKeywords {
		if strings.Contains(lower, kw) {
			_ = a.db.SetContext("last_search_intent", prompt)
			break
		}
	}

	// If the prompt describes a niche/business, save it
	nicheKeywords := []string{"lĩnh vực", "ngành", "niche", "chuyên về", "kinh doanh", "bán hàng", "dịch vụ"}
	for _, kw := range nicheKeywords {
		if strings.Contains(lower, kw) {
			_ = a.db.SetContext("last_niche_prompt", prompt)
			break
		}
	}
}

// buildDynamicSystemPrompt creates the AI Operator system prompt.
// Fully driven by BusinessProfile — no hardcoded niche strings.
func buildDynamicSystemPrompt(userCtx map[string]string, accounts []models.Account) string {
	profile := ProfileFromContext(userCtx)
	var sb strings.Builder

	// ── 1. IDENTITY ─────────────────────────────────────────────────────────
	sb.WriteString(`# INTENT → BUSINESS ACTION ENGINE

Bạn là AI Operator của doanh nghiệp. Bạn KHÔNG phải chatbot.
Bạn là senior production system với 10+ năm kinh nghiệm:
AI systems · recruitment automation · growth & sales · real-world operations.

Công thức: User input → Intent → Business Action → Execution
Bạn là brain (engine ra quyết định). Codebase chỉ là execution layer.

`)

	// ── 2. MULTILINGUAL ──────────────────────────────────────────────────────
	sb.WriteString(`## NGÔN NGỮ (MULTILINGUAL)
- User dùng Tiếng Việt / Tiếng Anh / trộn lẫn → Bạn hiểu tất cả
- Normalize internally, trả lời đúng ngôn ngữ user đang dùng
- "cào nhân sự" = "find candidates" = "recruit people" → cùng action
- KHÔNG bao giờ yêu cầu user viết lại bằng tiếng Anh

`)

	// ── 3. BUSINESS ANCHOR (CRITICAL) ────────────────────────────────────────
	sb.WriteString("## BUSINESS ANCHOR (MỌI QUYẾT ĐỊNH DỰA TRÊN ĐÂY)\n")
	if profile.IsConfigured() {
		sb.WriteString(profile.ToPromptBlock())
	} else {
		sb.WriteString("(Chưa có hồ sơ doanh nghiệp — dùng describe_business để cấu hình)\n")
	}
	sb.WriteString(`
RULE: Nếu user input không rõ → suy luận từ business context trên.
KHÔNG hành xử như generic assistant. Mọi action phải phục vụ business goal.

`)

	// ── OPERATING MODE ───────────────────────────────────────────────────────
	if autoMode := userCtx["auto_comment_mode"]; autoMode == "true" || autoMode == "1" {
		sb.WriteString("CHẾ ĐỘ: AUTO — execute ngay, không chờ duyệt\n\n")
	} else {
		sb.WriteString("CHẾ ĐỘ: DRAFT — tạo draft, user duyệt trong Dashboard\n\n")
	}
	if userCtx["last_image_upload"] != "" {
		sb.WriteString("Có ảnh doanh nghiệp trong DB để đính kèm comment/inbox.\n\n")
	}

	// ── ACCOUNTS ─────────────────────────────────────────────────────────────
	if files := strings.TrimSpace(userCtx["org_private_files_summary"]); files != "" {
		sb.WriteString("## PRIVATE FILES SUMMARY\n")
		sb.WriteString(files)
		sb.WriteString("\n\n")
	}
	if sources := strings.TrimSpace(userCtx["org_data_sources_summary"]); sources != "" {
		sb.WriteString("## CONNECTED DATA SOURCES SUMMARY\n")
		sb.WriteString(sources)
		sb.WriteString("\n\n")
	}

	if len(accounts) > 0 {
		sb.WriteString("## TÀI KHOẢN FACEBOOK (ACCOUNT MAPPING)\n")
		sb.WriteString("Danh sách accounts hiện có. Khi user đề cập đến account, map về account_id và LUÔN truyền vào tool call:\n\n")
		for i, acc := range accounts {
			letter := string(rune('A' + i))
			sb.WriteString(fmt.Sprintf("- Account %s / Tài khoản %d → ID=%d | Tên: %s | Trạng thái: %s\n",
				letter, i+1, acc.ID, acc.Name, string(acc.Status)))
		}
		sb.WriteString(`
Ví dụ mapping (QUAN TRỌNG — luôn truyền account_id đúng vào tool call):
- "account A tìm khách" → search_groups(query="...", account_id=<ID account A>)
- "dùng account B inbox hết" → inbox_all_leads(account_id=<ID account B>)
- "account 2 comment tất cả leads" → comment_all_leads(account_id=<ID account B>)
- "tài khoản Nguyen Van A outreach ứng viên" → recruit_all_candidates(account_id=<ID của Nguyen Van A>)

`)
	}

	// ── 4. INTENT CLASSIFICATION ─────────────────────────────────────────────
	sb.WriteString(`## PHÂN LOẠI Ý ĐỊNH (INTENT CLASSIFICATION)

Với mỗi input, classify ngay vào một trong:

| INTENT | Ví dụ trigger |
|---|---|
| FIND_CUSTOMERS | "kiếm khách", "tìm đơn hàng", "cào khách logistics", "tìm người cần dịch vụ" |
| FIND_CANDIDATES | "tìm ứng viên", "cào nhân sự", "recruit dev", "tìm người đi làm" |
| POST_CONTENT | "đăng bài", "post JD", "tạo content tuyển dụng" |
| OUTREACH | "comment leads", "inbox hết", "nhắn tin ứng viên" |
| UPDATE_BUSINESS_CONTEXT | "mình kinh doanh X", "cập nhật dịch vụ", "thêm sản phẩm mới" |
| OTHER | suy luận thông minh từ context |

`)

	// ── 5. INTENT → ACTION MAPPING ───────────────────────────────────────────
	sb.WriteString(`## INTENT → ACTION MAPPING

Sau khi classify intent, xác định:

5.1 DOMAIN (từ business profile — KHÔNG hardcode):
- Infer từ industry/services trong business profile
- Ví dụ: logistics → tìm khách vận chuyển | recruitment → tìm ứng viên | bakery → tìm người mua bánh

5.2 TARGET:
- FIND_CUSTOMERS → buyers / service seekers / partners
- FIND_CANDIDATES → job seekers matching open JDs
- OUTREACH → warm leads (đã replied) trước → hot leads → cold leads

5.3 ACTIONS (tuỳ intent):
crawl → filter → match → comment → inbox → post → report

`)

	// ── 6. DECISION ENGINE ───────────────────────────────────────────────────
	sb.WriteString(`## DECISION ENGINE (TRƯỚC MỌI ACTION)

Trước khi execute BẤT KỲ action nào, tự hỏi:
- WHY: Tại sao action này phù hợp với intent của user?
- WHO: Đây có phải target đúng (domain, role, need) không?
- WHAT: Kết quả kỳ vọng là gì? Có đo được không?

Với mỗi lead/ứng viên, quyết định MỘT trong:
- COMMENT: phù hợp → reply vào post/comment của họ
- INBOX: score cao + đúng nhu cầu → DM cá nhân hóa
- SKIP: không đủ điều kiện, sai domain, đã liên hệ rồi
- DELAY: đủ điều kiện nhưng chưa đúng timing

KHÔNG hành động 100% leads. Chọn lọc thông minh.

`)

	// ── 7. CONTEXT-DRIVEN STRATEGY ───────────────────────────────────────────
	sb.WriteString(`## CHIẾN LƯỢC THEO CONTEXT (CONTEXT-DRIVEN STRATEGY)

Chiến lược thay đổi hoàn toàn theo business profile:

- Logistics/fulfillment → tìm posts về vận chuyển, ecom, xuất nhập khẩu; target sellers
- Recruitment → tìm posts "tìm việc", "thất nghiệp", "muốn chuyển việc"; match với JD
- Local service (bakery, spa, gym...) → target groups theo địa điểm; adjust timing giờ cao điểm
- B2B → target decision makers, business owners; tone formal hơn
- Bất kỳ ngành nào khác → đọc business profile → tự suy luận strategy phù hợp

`)

	// ── 8. NO HARDCODING ─────────────────────────────────────────────────────
	sb.WriteString(`## KHÔNG HARDCODE (NO HARDCODING)

KHÔNG dựa vào:
- Fixed niche strings ("tuyen_dung", "logistics")
- Predefined workflows
- Template cứng nhắc

TẤT CẢ phải được infer từ: business context + user intent
Nếu business thay đổi → strategy thay đổi tự động.

`)

	// ── 9. ACTION EXECUTION MODEL ────────────────────────────────────────────
	sb.WriteString(`## MÔ HÌNH THỰC THI (ACTION EXECUTION MODEL)

Với mỗi task: PLAN → EXECUTE → ADAPT

KHÔNG làm: if A → do B (script cứng nhắc)
PHẢI làm: hiểu context → xây plan → thực thi → học từ kết quả

`)

	// ── 10. OUTPUT REQUIREMENT ───────────────────────────────────────────────
	sb.WriteString(`## YÊU CẦU OUTPUT

Responses phải:
- Actionable: có action cụ thể, không nói chung chung
- Business-focused: gắn với mục tiêu kinh doanh
- Aligned với THG goals

KHÔNG:
- Giải thích generic
- Hành xử như chatbot trả lời câu hỏi
- Hỏi lại khi đã đủ context để execute

`)

	// ── 11. MEMORY AWARENESS ─────────────────────────────────────────────────
	sb.WriteString(`## BỘ NHỚ & TRÁNH TRÙNG LẶP (MEMORY)

- Không outreach người đã liên hệ (check outbound_messages)
- Warm leads (đã reply) → ưu tiên follow-up trước cold leads mới
- Lead status: new → contacted → replied → converted/closed
- Group cooldown: dynamic (last_post_at + quality score), KHÔNG dùng weekly_post_count

`)

	// ── 12. SELF-ADAPTATION ──────────────────────────────────────────────────
	sb.WriteString(`## TỰ HỌC (SELF-ADAPTATION)

Liên tục điều chỉnh:
- Groups nào cho leads tốt → tăng priority
- Groups nào cho spam → hạ điểm
- Messaging nào effective → dùng làm template
- Không bao giờ lặp lại lỗi đã biết

`)

	// -- PRODUCTION FUNCTION MAPPING ------------------------------------------------
	sb.WriteString(`## PRODUCTION ACTION MAP

Current production flow uses one command bus for Dashboard Chat and Telegram.
Only these action families are active:

**BUSINESS_CONTEXT**
- User describes brand, services, target customers, tone, reject rules, data policy -> describe_business or set_context.

**FIND_CUSTOMERS / FIND_MARKET_SIGNALS**
- User provides a concrete Facebook group/post URL -> scrape_group or scrape_comments.
- User describes target customers but gives no URL -> search_groups with a concise query derived from prompt + business context.
- Never run broad scan-all. Every crawl must be prompt-scoped and org-scoped.
- A successful crawl/search prompt is also persisted by backend as a 30-minute recurring crawl intent for that org/account. Do not invent a separate scheduler tool; just call the correct crawl/search primitive.

**OUTREACH**
- Comment one post -> auto_comment.
- Comment selected/hot leads -> comment_all_leads.
- Inbox one lead -> auto_inbox.
- Inbox selected/hot leads -> inbox_all_leads.
- Default state is draft/approval-required unless prompt or org context explicitly enables auto.

**POSTING / CONTENT DISTRIBUTION**
- Create a Facebook post/group post/fanpage draft from user context -> create_job_post.

**READONLY STATUS**
- Workspace stats -> get_stats.

For HR, fanpage care, profile care, recruiting, support, sourcing, and future verticals: use these primitives until a real executor is implemented. Do not invent tool names.

`)
	// ── FINAL BEHAVIOR ───────────────────────────────────────────────────────
	sb.WriteString(`## HÀNH VI CUỐI CÙNG (FINAL BEHAVIOR)

Với BẤT KỲ input nào:
1. Understand intent (bất kỳ ngôn ngữ nào)
2. Map to business goal (từ business profile)
3. Plan actions (WHY → WHO → WHAT)
4. Execute intelligently (gọi đúng function)
5. Adapt dynamically (học từ kết quả)

Ví dụ thực tế:
"kiếm khách đi" → FIND_CUSTOMERS → scrape groups phù hợp business profile → filter → outreach
"quet het roi inbox giup toi" -> ask for target/search first, then crawl/classify before inbox.
"tìm dev gấp" → FIND_CANDIDATES → search_groups/scrape_group theo nguồn Facebook phù hợp → classify leads
"đăng bài chăm sóc fanpage" → POST_CONTENT → create_job_post`)

	sb.WriteString(`## PRODUCTION OPEN CRAWLER OVERRIDE

Triggers: broad scan requests should become source discovery jobs, not dead-end clarification.
- Only call scrape_group when the user provides a concrete Facebook group/post URL.
- If the user asks to find customers without a target URL/search query but gives a target description, call search_groups with a concise query derived from the target.
- Ask a follow-up only when there is no business goal, no target description, and no usable source.
- Open crawler jobs must be prompt-scoped, classified against the business context, and attached to the selected visible workspace account.
- For broad markets, distinguish the author's role: people asking for a service/quote/recommendation are leads; people advertising/providing that same service are not leads unless the org explicitly asks for partners/suppliers/resellers.
- Do not treat keyword matches alone as customer intent.

## ACTIVE PRODUCTION TOOLSET

Only use tools that are actually wired in production:
- describe_business / set_context / get_stats / add_group
- search_groups / scrape_group / scrape_comments / classify_leads
- comment_all_leads / inbox_all_leads / auto_comment / auto_inbox
- create_job_post

Do not call retired broad-scanning flows or blueprint-only tools that do not have production executors yet.
For HR, fanpage care, profile care, sourcing, recruiting, sales, support, or any future workflow, express the plan through the current primitives above unless a dedicated executor is added later.

`)

	return sb.String()
}

// --- Memory & Learning ---

func (a *Agent) getFewShotExamples(prompt string) []models.AIMemory {
	memories, err := a.db.GetRelevantMemories(20)
	if err != nil || len(memories) == 0 {
		return nil
	}

	promptWords := extractKeywords(strings.ToLower(prompt))

	type scored struct {
		mem   models.AIMemory
		score float64
	}

	var results []scored
	for _, m := range memories {
		memWords := extractKeywords(strings.ToLower(m.UserPrompt))
		overlap := keywordOverlap(promptWords, memWords)
		finalScore := overlap * m.SuccessRate * (1.0 + float64(m.UseCount)*0.1)
		if finalScore > 0.1 {
			results = append(results, scored{m, finalScore})
		}
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	limit := 3
	if len(results) < limit {
		limit = len(results)
	}
	out := make([]models.AIMemory, limit)
	for i := 0; i < limit; i++ {
		out[i] = results[i].mem
	}
	return out
}

func extractKeywords(text string) []string {
	words := strings.Fields(text)
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		if len(w) > 2 {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

func keywordOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool)
	for _, w := range a {
		setA[w] = true
	}
	var intersection int
	setB := make(map[string]bool)
	for _, w := range b {
		setB[w] = true
		if setA[w] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func (a *Agent) logPrompt(source, prompt, response, action, args string, success bool) {
	pl := &models.PromptLog{
		Source:      source,
		UserPrompt:  prompt,
		AIResponse:  response,
		ActionTaken: action,
		ActionArgs:  args,
		Success:     success,
	}
	_ = a.db.InsertPromptLog(pl)
}

func (a *Agent) updateMemory(prompt, action, args string) {
	hash := promptHash(prompt)
	existing, err := a.db.GetMemoryByHash(hash)
	if err == nil && existing != nil {
		_ = a.db.UpdateMemoryUsage(existing.ID, true)
	} else {
		mem := &models.AIMemory{
			PromptHash:  hash,
			Category:    categorizeAction(action),
			UserPrompt:  prompt,
			BestAction:  action,
			BestArgs:    args,
			UseCount:    1,
			SuccessRate: 1.0,
		}
		_ = a.db.InsertMemory(mem)
	}
}

func promptHash(prompt string) string {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h[:8])
}

func categorizeAction(action string) string {
	switch {
	case strings.Contains(action, "scrape"):
		return "scrape"
	case strings.Contains(action, "group"):
		return "manage"
	case strings.Contains(action, "classify"):
		return "classify"
	case strings.Contains(action, "comment") || strings.Contains(action, "inbox"):
		return "engage"
	case strings.Contains(action, "context"):
		return "config"
	case strings.Contains(action, "stats"):
		return "query"
	default:
		return "other"
	}
}

// --- OpenAI Types ---

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

// --- Function Definitions for OpenAI ---
// Tools: prompt-scoped crawl, manage, query, engage, and configure.

func productionAgentTools() []map[string]any {
	allowed := map[string]bool{
		"set_context":       true,
		"describe_business": true,
		"get_stats":         true,
		"add_group":         true,
		"scrape_group":      true,
		"scrape_comments":   true,
		"classify_leads":    true,
		"search_groups":     true,
		"auto_comment":      true,
		"comment_all_leads": true,
		"auto_inbox":        true,
		"inbox_all_leads":   true,
		"create_job_post":   true,
	}
	out := make([]map[string]any, 0, len(allowed))
	for _, tool := range agentTools {
		fn, _ := tool["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if allowed[name] {
			out = append(out, tool)
		}
	}
	return out
}

var agentTools = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "scrape_group",
			"description": "Crawl a concrete Facebook group or post URL through the authenticated workspace browser session.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":        map[string]string{"type": "string", "description": "Facebook group or post URL"},
					"account_id": map[string]string{"type": "integer", "description": "Workspace Facebook account ID. Empty means auto-pick a ready account."},
				},
				"required": []string{"url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "scrape_comments",
			"description": "Read existing comments from one Facebook post for lead analysis. Do not use this to publish a new comment.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_url":   map[string]string{"type": "string", "description": "Facebook post URL"},
					"account_id": map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
				},
				"required": []string{"post_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "search_groups",
			"description": "Discover suitable Facebook sources when the user describes a target audience but does not provide a source URL.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]string{"type": "string", "description": "Search query derived from prompt and business context"},
					"account_id": map[string]string{"type": "integer", "description": "Workspace Facebook account ID. Empty means auto-pick a ready account."},
				},
				"required": []string{"query"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "auto_comment",
			"description": "Queue a comment for one concrete Facebook post. Default is draft unless auto mode is explicit.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_url":    map[string]string{"type": "string", "description": "Target post URL"},
					"account_id":  map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
					"context":     map[string]string{"type": "string", "description": "Post context if available"},
					"target_name": map[string]string{"type": "string", "description": "Author name if available"},
				},
				"required": []string{"post_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "comment_all_leads",
			"description": "Queue comments for qualified leads with dedup, cooldown, and approval guardrails.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template":     map[string]string{"type": "string", "description": "Optional user-provided comment template"},
					"score_filter": map[string]string{"type": "string", "description": "hot, warm, cold, or all"},
					"account_id":   map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "auto_inbox",
			"description": "Queue an inbox message for one concrete lead. Default is draft unless auto mode is explicit.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_url":  map[string]string{"type": "string", "description": "Profile or Messenger target URL"},
					"account_id":  map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
					"context":     map[string]string{"type": "string", "description": "Lead context"},
					"target_name": map[string]string{"type": "string", "description": "Lead name if available"},
				},
				"required": []string{"target_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "inbox_all_leads",
			"description": "Queue inbox outreach for qualified leads with conversation, dedup, cooldown, and approval guardrails.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"score_filter": map[string]string{"type": "string", "description": "hot, warm, cold, or all"},
					"account_id":   map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "create_job_post",
			"description": "Queue a Facebook post/group post draft from the user request and business context.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]string{"type": "string", "description": "Post title or topic"},
					"description": map[string]string{"type": "string", "description": "Post brief"},
					"content":     map[string]string{"type": "string", "description": "Full content if provided"},
					"group_url":   map[string]string{"type": "string", "description": "Target group URL if specified"},
					"account_id":  map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "describe_business",
			"description": "Store org-scoped business context: brand, services, target customers, tone, reject rules, and approval policy.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]string{"type": "string", "description": "Free-form business/workspace description"},
				},
				"required": []string{"description"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "set_context",
			"description": "Store org-scoped configuration such as business_profile, private_files_summary, data_sources_summary, or outbound_mode.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":   map[string]string{"type": "string", "description": "business_profile, private_files_summary, data_sources_summary, outbound_mode"},
					"value": map[string]string{"type": "string", "description": "Value to store"},
				},
				"required": []string{"key", "value"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "get_stats",
			"description": "Read workspace stats.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "add_group",
			"description": "Register a Facebook source for the current organization.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":  map[string]string{"type": "string", "description": "Facebook source URL"},
					"name": map[string]string{"type": "string", "description": "Source name"},
				},
				"required": []string{"url", "name"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "classify_leads",
			"description": "Confirm that classification is handled inline by prompt-scoped crawl results and current business context.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
}
