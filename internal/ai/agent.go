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

// Agent is an AI-powered agent that interprets natural language prompts
// and executes scraper actions using OpenAI Function Calling.
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
		for _, key := range []string{"business_profile", "private_files_summary", "data_sources_summary", "outbound_mode"} {
			if v, err := a.db.GetContext(fmt.Sprintf("org:%d:%s", orgID, key)); err == nil && strings.TrimSpace(v) != "" {
				userContext["org_"+key] = strings.TrimSpace(v)
			}
		}
		if userContext["org_business_profile"] != "" {
			userContext["business_desc"] = userContext["org_business_profile"]
		}
	}

	// Load accounts for AI account mapping
	accounts, _ := a.db.GetAllAccounts(orgID)
	if requiresFacebookBrowser(prompt) {
		if ok, msg := facebookBrowserPreflight(accounts, selectedAccountID); !ok {
			a.logPrompt(source, prompt, msg, "browser_preflight", "", false)
			return msg, nil
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
		"tools":       agentTools,
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

func browserNotReadyMessage(acc *models.Account) string {
	target := "Workspace chưa có Facebook session sẵn sàng."
	if acc != nil {
		target = fmt.Sprintf("Facebook account %q chưa sẵn sàng để chạy automation.", acc.Name)
	}
	return target + `

Để bảo toàn dữ liệu và tránh chạy sai tài khoản, THG chỉ bắt đầu crawl khi Browser đã xác nhận Facebook session thật.

Bạn hãy hoàn tất kết nối trong tab Browser:
1. Mở tab Browser của workspace.
2. Chạy THG Local Kit trên thiết bị đã ghép.
3. Mở đúng Facebook account và đăng nhập nếu hệ thống yêu cầu.
4. Chờ trạng thái chuyển sang Facebook local ready.

Sau khi Browser sẵn sàng, gửi lại lệnh này. Agent sẽ bắt đầu crawl ngay với đúng account và dữ liệu thật.`
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
	jobID := ""
	if m := regexp.MustCompile(`job #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		jobID = m[1]
	}
	taskID := ""
	if m := regexp.MustCompile(`task=([a-zA-Z0-9_-]+)`).FindStringSubmatch(raw); len(m) == 2 {
		taskID = m[1]
	}
	var sb strings.Builder
	sb.WriteString("Đã nhận lệnh crawl và đưa vào hàng đợi xử lý.\n\n")
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
	if taskID != "" {
		sb.WriteString("Task: ")
		sb.WriteString(taskID)
		sb.WriteString("\n")
	}
	sb.WriteString("\nHệ thống sẽ dùng Facebook session đã kết nối để thu thập dữ liệu thật, lọc tín hiệu theo nhu cầu trong prompt, phân loại leads hot/warm/cold và lưu kết quả về Leads.")
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

	// ── FUNCTION MAPPING ─────────────────────────────────────────────────────
	sb.WriteString(`## PHÂN CÔNG CHỨC NĂNG (FUNCTION MAPPING)

**FIND_CANDIDATES → run_full_recruitment_pipeline**
Triggers: "cào nhân sự", "tìm ứng viên", "tìm dev gấp", "recruit người", "chạy pipeline"
→ GỌI NGAY, không hỏi thêm

**POST_CONTENT → post_jds_to_groups**
Triggers: "đăng bài tuyển dụng", "post JD vào groups", "đăng tin tuyển dụng"
→ positions cụ thể → truyền vào param | không có → đăng tất cả

**scan_own_jd_posts**: "quét bài đã đăng", "kiểm tra comments bài JD"

**FIND_CUSTOMERS -> prompt-scoped open crawler**
Triggers: nếu user có URL group/post Facebook cụ thể thì dùng scrape_group/scrape_comments. Nếu user chỉ mô tả tệp khách/ngách/nhu cầu mà không đưa URL, KHÔNG hỏi lại; dùng search_groups(query=<target/query suy luận từ prompt>) để tìm nguồn phù hợp trước, rồi crawler sẽ lọc/classify theo prompt.

**scrape_group**: user gửi URL group Facebook cụ thể

**OUTREACH comment → auto_comment**
Triggers: URL bài viết + "comment lên", "bình luận vào bài này"

**OUTREACH batch → comment_all_leads**
Triggers: "comment leads", "bình luận hết", "comment tất cả" | "kèm ảnh" → with_image=true

**OUTREACH inbox → auto_inbox**: user chỉ rõ 1 người cụ thể

**OUTREACH batch inbox → inbox_all_leads**
Triggers: "inbox tất cả", "nhắn tin hết", "inbox all"

**list_career_jobs**: "xem jobs", "vị trí đang tuyển" (chỉ xem, không execute)

**crawl_careers**: URL /careers → cào JD
**crawl_careers_images**: "lấy ảnh JD", "chụp ảnh tuyển dụng"
**crawl_catalog**: link website + "crawl ảnh"
**update_price_list**: "học bảng giá", "giá dịch vụ"
**search_groups**: "tìm group", "tìm nhóm Facebook"
**score_groups**: "score groups", "đánh giá groups"
**discover_groups_for_jobs**: "khám phá groups cho jobs"
**seed_quality_groups**: "seed groups", "khởi tạo groups mặc định"

**UPDATE_BUSINESS_CONTEXT → describe_business**
Triggers: "mình kinh doanh X", "cập nhật thông tin công ty", "chúng tôi vừa thêm dịch vụ..."
→ describe_business(description=<nguyên văn>) — KHÔNG tóm tắt, copy nguyên văn

**set_context**: "bật auto comment" → auto_comment_mode=true | "tắt" → false

**check_inbox_replies**: "check reply", "xem ai nhắn lại", "follow up inbox"

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
"tìm dev gấp" → FIND_CANDIDATES → run_full_recruitment_pipeline (tech domain)
"đăng bài tuyển dụng luôn" → POST_CONTENT → post_jds_to_groups`)

	sb.WriteString(`## PRODUCTION OPEN CRAWLER OVERRIDE

Triggers: broad scan requests should become source discovery jobs, not dead-end clarification.
- Only call scrape_group when the user provides a concrete Facebook group/post URL.
- If the user asks to find customers without a target URL/search query but gives a target description, call search_groups with a concise query derived from the target.
- Ask a follow-up only when there is no business goal, no target description, and no usable source.
- Open crawler jobs must be prompt-scoped, classified against the business context, and attached to the selected visible workspace account.

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

var agentTools = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "scrape_group",
			"description": "Cào bài viết từ 1 Facebook group cụ thể",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":        map[string]string{"type": "string", "description": "Facebook group URL"},
					"account_id": map[string]string{"type": "integer", "description": "ID account dùng để cào group (từ ACCOUNT MAPPING). Bỏ trống = tự chọn account"},
				},
				"required": []string{"url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "scrape_comments",
			"description": "Cào/đọc các comments ĐÃ CÓ SẴN từ 1 bài viết để phân tích. KHÔNG dùng khi user muốn ĐĂNG comment mới lên bài.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_url": map[string]string{"type": "string", "description": "URL bài viết"},
				},
				"required": []string{"post_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "check_inbox",
			"description": "Kiểm tra inbox Messenger",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"account_id": map[string]string{"type": "integer", "description": "ID account"},
				},
				"required": []string{"account_id"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "add_group",
			"description": "Thêm group mới vào danh sách theo dõi",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":  map[string]string{"type": "string", "description": "URL group"},
					"name": map[string]string{"type": "string", "description": "Tên group"},
				},
				"required": []string{"url", "name"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "get_stats",
			"description": "Xem thống kê hệ thống",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "classify_leads",
			"description": "Phân loại bài viết thành leads (hot/warm/cold) theo tiêu chí user đã cấu hình",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"count": map[string]string{"type": "integer", "description": "Số bài cần classify (mặc định 20)"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "auto_comment",
			"description": "ĐĂNG comment mới lên 1 bài viết cụ thể theo URL. Dùng khi user gửi link bài viết + nói 'comment lên đây', 'bình luận lên post này', 'comment bài này cho tôi', 'comment vào link này'. AI soạn nội dung phù hợp.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_url":    map[string]string{"type": "string", "description": "URL bài viết cần comment"},
					"account_id":  map[string]string{"type": "integer", "description": "ID account dùng để comment"},
					"context":     map[string]string{"type": "string", "description": "Nội dung bài viết gốc"},
					"target_name": map[string]string{"type": "string", "description": "Tên tác giả bài viết"},
				},
				"required": []string{"post_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "comment_all_leads",
			"description": "Comment tất cả leads hiện tại. AI soạn comment riêng cho từng lead, hoặc dùng template user cung cấp. Nếu auto_comment_mode=true thì comment ngay không cần duyệt. Dùng khi user nói 'comment tất cả leads', 'bình luận hết', 'comment all', 'comment leads kèm ảnh'. Nếu user chỉ định account, truyền account_id.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template":     map[string]string{"type": "string", "description": "Mẫu comment (nếu có). Để trống = AI tự soạn phù hợp từng lead"},
					"score_filter": map[string]string{"type": "string", "description": "Lọc theo score: hot, warm, cold, hoặc all (mặc định: all)"},
					"with_image":   map[string]string{"type": "boolean", "description": "Kèm ảnh thực tế từ database khi comment. true/false"},
					"account_id":   map[string]string{"type": "integer", "description": "ID account dùng để comment (từ ACCOUNT MAPPING). Bỏ trống = tự chọn account"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "auto_inbox",
			"description": "Tạo draft tin nhắn inbox cho 1 lead cụ thể. AI soạn nội dung tư vấn. Cần duyệt trước khi gửi.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_url":  map[string]string{"type": "string", "description": "URL profile hoặc Messenger"},
					"account_id":  map[string]string{"type": "integer", "description": "ID account dùng để gửi"},
					"context":     map[string]string{"type": "string", "description": "Nội dung bài viết/comment của lead"},
					"target_name": map[string]string{"type": "string", "description": "Tên người nhận"},
				},
				"required": []string{"target_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "inbox_all_leads",
			"description": "Inbox tất cả leads hiện tại. AI soạn tin nhắn riêng cho từng lead và gửi luôn qua Messenger. Dùng khi user nói 'inbox tất cả leads', 'nhắn tin hết leads', 'inbox all', 'gửi tin nhắn tất cả'. Nếu user chỉ định account, truyền account_id.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"score_filter": map[string]string{"type": "string", "description": "Lọc theo score: hot, warm, cold, hoặc all (mặc định: hot)"},
					"skip_sent":    map[string]string{"type": "boolean", "description": "Bỏ qua leads đã inbox rồi. true/false (mặc định: true)"},
					"account_id":   map[string]string{"type": "integer", "description": "ID account dùng để inbox (từ ACCOUNT MAPPING). Bỏ trống = tự chọn account"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "recruit_from_database",
			"description": "TỰ ĐỘNG cào nhân sự dựa trên các vị trí tuyển dụng đã lưu trong database. Tool này làm TẤT CẢ trong 1 bước: đọc danh sách jobs → set niche tuyển dụng → tự sinh keywords → tìm groups Facebook → tự động cào. Dùng khi user nói 'cào nhân sự liên quan đến vị trí trong database', 'tìm ứng viên cho jobs đã cào', 'cào tuyển dụng theo database'.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "list_career_jobs",
			"description": "Xem danh sách tất cả vị trí tuyển dụng đang mở đã lưu trong database. Dùng khi user hỏi 'xem jobs đã cào', 'vị trí trong database', 'danh sách tuyển dụng', hoặc muốn cào nhân sự theo vị trí đã lưu. Trả về tiêu đề + mô tả ngắn của từng job.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "recruit_all_candidates",
			"description": "Comment outreach tất cả ứng viên (candidates) đang tìm việc. AI soạn comment cá nhân hóa dựa trên JD đang có, gửi đến từng bài của ứng viên. Dùng khi user nói 'comment ứng viên', 'outreach candidates', 'tiếp cận ứng viên', 'comment tất cả ứng viên'. Nếu user chỉ định account, truyền account_id.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"score_filter": map[string]string{"type": "string", "description": "Lọc theo score: hot, warm, cold, hoặc all (mặc định: hot)"},
					"account_id":   map[string]string{"type": "integer", "description": "ID account dùng để comment (từ ACCOUNT MAPPING). Bỏ trống = tự chọn account"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "create_job_post",
			"description": "Tạo bài đăng tuyển dụng chuyên nghiệp cho Facebook. AI soạn nội dung hấp dẫn thu hút ứng viên. Dùng khi user muốn 'đăng tuyển dụng', 'tạo bài tuyển dụng', 'tạo JD', 'viết job post'.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":        map[string]string{"type": "string", "description": "Tên vị trí tuyển dụng (vd: Nhân viên Kho, Sales Executive)"},
					"description":  map[string]string{"type": "string", "description": "Mô tả công việc ngắn gọn"},
					"requirements": map[string]string{"type": "string", "description": "Yêu cầu ứng viên (kinh nghiệm, kỹ năng)"},
					"benefits":     map[string]string{"type": "string", "description": "Quyền lợi (lương, thưởng, môi trường làm việc)"},
					"salary":       map[string]string{"type": "string", "description": "Mức lương (vd: 12-15 triệu, thỏa thuận)"},
					"email":        map[string]string{"type": "string", "description": "Email nhận CV (mặc định: career@thgfulfill.com)"},
				},
				"required": []string{"title"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "check_inbox_replies",
			"description": "Kiểm tra tất cả conversations đang mở xem có khách hàng reply chưa. Nếu có → AI tự soạn và gửi follow-up ngay dựa trên toàn bộ lịch sử hội thoại. Dùng khi user nói 'check reply', 'xem ai nhắn lại chưa', 'follow up inbox', 'kiểm tra tin nhắn mới'.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "crawl_catalog",
			"description": "Tự động vào website catalog/sản phẩm và tải tất cả ảnh về database để dùng cho auto-comment. Dùng khi user gửi link website và muốn AI lấy ảnh từ đó. Trigger: 'crawl ảnh', 'lấy ảnh từ web', 'import ảnh', link website catalog.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]string{"type": "string", "description": "URL trang catalog/website cần crawl ảnh"},
				},
				"required": []string{"url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "update_price_list",
			"description": "Học bảng giá dịch vụ/sản phẩm từ text. AI sẽ trích xuất và lưu để tư vấn đúng giá khi comment/inbox khách. Trigger: 'học bảng giá', 'bảng giá:', 'giá dịch vụ là', 'update giá', user paste danh sách giá.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text":    map[string]string{"type": "string", "description": "Nội dung bảng giá dạng text cần học"},
					"replace": map[string]string{"type": "boolean", "description": "true = xóa bảng giá cũ và thay mới, false = thêm vào (mặc định false)"},
				},
				"required": []string{"text"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "set_context",
			"description": "Lưu thông tin cấu hình kinh doanh và chế độ hoạt động. Dùng khi user mô tả lĩnh vực, dịch vụ, khách hàng mục tiêu, quy tắc lọc bài, hoặc chuyển lĩnh vực.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":   map[string]string{"type": "string", "description": "Loại thông tin cần lưu: business_name, business_industry, business_desc, services, target_customers, business_location, business_usp, reject_rules, last_search_intent, auto_comment_mode (true/false)"},
					"value": map[string]string{"type": "string", "description": "Giá trị cần lưu"},
				},
				"required": []string{"key", "value"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "search_groups",
			"description": "Tìm kiếm nhóm Facebook theo keywords. AI tự sinh keywords từ prompt của user. Tool tự add groups vào danh sách theo dõi và tự submit cào ngay. Nếu user chỉ định account, truyền account_id để tìm kiếm bằng session của account đó.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]string{"type": "string", "description": "Keywords để tìm FB groups (VD: 'tuyển dụng kho vận', 'việc làm logistics')"},
					"niche":      map[string]string{"type": "string", "description": "Lĩnh vực đang làm (VD: 'tuyen_dung', 'logistics'). Tool này sẽ auto set_context active_niche luôn."},
					"account_id": map[string]string{"type": "integer", "description": "ID account dùng để tìm kiếm (từ ACCOUNT MAPPING). Bỏ trống = dùng session mặc định."},
				},
				"required": []string{"query"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "crawl_careers",
			"description": "Tự động truy cập trang tuyển dụng (careers page) của công ty và trích xuất mọi tin tuyển dụng đang mở. Lưu danh sách vào CSDL để dùng cho việc comment HR. Trigger: khi user gửi URL có /careers hoặc nhắc nhở cào trang tuyển dụng.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]string{"type": "string", "description": "Đường link trang careers (vd: https://www.thgfulfill.com/careers)"},
				},
				"required": []string{"url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "crawl_careers_images",
			"description": "Chụp ảnh screenshot từng thẻ JD (job card modal) trên trang careers và lưu vào DB dưới dạng ảnh có category=career_job. Sau đó HR Agent sẽ tự đính kèm đúng ảnh JD khi comment reply ứng viên. Trigger: khi user gửi link careers kèm yêu cầu 'chụp ảnh JD' / 'lấy ảnh JD' / 'attach ảnh vào comment'. Nên gọi SAU crawl_careers() để đảm bảo danh sách jobs đã có.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]string{"type": "string", "description": "Đường link trang careers để chụp ảnh JD (vd: https://www.thgfulfill.com/careers)"},
				},
				"required": []string{"url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "run_full_recruitment_pipeline",
			"description": "Chạy toàn bộ pipeline tuyển dụng end-to-end: load tất cả jobs theo priority → extract keywords → tìm/cào groups → scrape comments ứng viên → AI score + domain-match → dedup → queue comment_reply + inbox DM → tạo bài đăng JD draft. TRIGGERS (gọi ngay khi user nói bất kỳ): 'cào nhân sự liên quan đến', 'cào ứng viên', 'tìm ứng viên cho jobs', 'tìm nhân sự theo database', 'crawl candidates', 'chạy pipeline tuyển dụng', 'recruit từ database'. Yêu cầu: DB phải có career jobs (chạy crawl_careers trước). Trả về báo cáo đầy đủ.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "post_jds_to_groups",
			"description": "Tạo bài viết tuyển dụng chuyên nghiệp cho các vị trí trong database và đăng trực tiếp vào Facebook groups phù hợp. AI sẽ: (1) load jobs từ DB, (2) filter theo positions nếu user chỉ định, (3) tìm groups theo domain, (4) soạn bài chuyên nghiệp, (5) đăng vào groups. QUAN TRỌNG: Nếu user liệt kê CỤ THỂ vị trí, bạn PHẢI truyền vào positions.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"positions": map[string]any{
						"type":        "string",
						"description": "Danh sách vị trí cần đăng, phân cách bởi dấu phẩy. Ví dụ: 'Accountant, Sales Executive, E-Commerce Operations'. Để trống nếu đăng tất cả vị trí trong DB.",
					},
				},
				"required": []string{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "scan_own_jd_posts",
			"description": "Quét các bài JD đã đăng lên Facebook groups để tìm ứng viên comments. Khi có người comment lên bài, họ sẽ trở thành leads trong tab Tuyển dụng và HR Agent sẽ tự động @reply. TRIGGERS: 'quét bài đã đăng', 'kiểm tra comments', 'tìm ứng viên từ bài JD', 'scan bài tuyển dụng', 'xem ai comment bài tuyển dụng'.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "score_groups",
			"description": "Chạy NLP scoring tất cả groups chưa được đánh giá: relevance, professionalism, content quality, spam penalty → final_score → decision (use/monitor/reject). Dùng khi user nói 'đánh giá groups', 'score groups', 'chất lượng groups', 'lọc groups xấu', 'tìm groups tốt'.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "discover_groups_for_jobs",
			"description": "AI tự sinh search queries theo từng job domain → tìm groups Facebook phù hợp → score chất lượng → lưu vào DB. Dùng khi user nói 'tìm groups cho jobs', 'khám phá groups tuyển dụng', 'discover groups', 'tìm thêm groups chất lượng'.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "seed_quality_groups",
			"description": "Nạp danh sách groups chất lượng cao đã được tuyển chọn (tech, sales, ops, finance) vào hệ thống và tự động score chúng. Dùng lần đầu hoặc khi muốn khởi tạo lại bộ groups chuẩn. Trigger: 'seed groups', 'khởi tạo groups', 'nạp groups mặc định', 'bootstrap groups'.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "describe_business",
			"description": "Lưu thông tin doanh nghiệp từ mô tả tự do của user. AI sẽ tự trích xuất: tên, ngành, dịch vụ, khách hàng mục tiêu, địa điểm, USP, quy tắc lọc bài. Không cần format cứng — user chỉ cần mô tả bằng lời thường. Trigger: khi user giới thiệu về doanh nghiệp, thay đổi lĩnh vực kinh doanh, bổ sung dịch vụ mới, hoặc nói 'cấu hình lại', 'cập nhật thông tin công ty', 'mình kinh doanh X'.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]string{"type": "string", "description": "Mô tả tự do về doanh nghiệp của user (copy nguyên văn, không tóm tắt)"},
				},
				"required": []string{"description"},
			},
		},
	},
}
