package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai/comment"
	"github.com/thg/scraper/internal/models"
)

// sliceStr safely returns s[:n] without panicking when len(s) < n.
func sliceStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// THG service URLs — map keyword → link
var thgServiceLinks = map[string]string{
	"fulfill":   "https://thgfulfill.com/thg-fulfill",
	"express":   "https://thgfulfill.com/thg-express",
	"warehouse": "https://thgfulfill.com/thg-warehouse",
	"order":     "https://thgfulfill.com/thg-order",
}

// PickServiceURL chọn link phù hợp dựa trên service_match của lead.
// Fulfillment keywords được check TRƯỚC warehouse vì nhiều keyword trùng ngữ cảnh
// (vd: "xưởng ff" = fulfillment, không phải warehouse).
func PickServiceURL(serviceMatch string) string {
	s := strings.ToLower(serviceMatch)
	switch {
	// Fulfillment / POD — check TRƯỚC warehouse vì "xưởng ff", "pod" thường bị nhầm
	case strings.Contains(s, "fulfill") || strings.Contains(s, "ff") ||
		strings.Contains(s, "pod") || strings.Contains(s, "print on demand") ||
		strings.Contains(s, "xưởng") || strings.Contains(s, "đóng gói") ||
		strings.Contains(s, "custom") || strings.Contains(s, "in ấn") ||
		strings.Contains(s, "sản xuất"):
		return thgServiceLinks["fulfill"]
	case strings.Contains(s, "express") || strings.Contains(s, "vận chuyển") ||
		strings.Contains(s, "shipping") || strings.Contains(s, "giao hàng") ||
		strings.Contains(s, "ship"):
		return thgServiceLinks["express"]
	case strings.Contains(s, "warehouse") || strings.Contains(s, "kho bãi") ||
		strings.Contains(s, "lưu kho") || strings.Contains(s, "kho hàng"):
		return thgServiceLinks["warehouse"]
	case strings.Contains(s, "order") || strings.Contains(s, "sourcing") ||
		strings.Contains(s, "đặt hàng") || strings.Contains(s, "mua hàng") ||
		strings.Contains(s, "nhập hàng"):
		return thgServiceLinks["order"]
	default:
		return thgServiceLinks["fulfill"]
	}
}

// PickCatalogURL trả về link catalog/trang sản phẩm phù hợp với service.
// Dùng khi không có ảnh match để buyer tự xem catalog thay vì nhận ảnh không liên quan.
func PickCatalogURL(serviceMatch string) string {
	// Catalog link = service page URL — buyer có thể xem portfolio/sản phẩm tại đây
	return PickServiceURL(serviceMatch)
}

// detectLang trả về "en" chỉ khi văn bản HOÀN TOÀN không có ký tự tiếng Việt.
// Mặc định là "vi" — bao gồm mọi bài có dấu thanh/dấu phụ tiếng Việt.
func detectLang(text string) string {
	for _, r := range text {
		// Latin Extended Additional (U+1E00–U+1EFF): chứa hầu hết ký tự có dấu tiếng Việt
		// Latin Extended-A/B (U+0100–U+024F): ắ ặ ề ế ộ ở...
		if (r >= 0x1E00 && r <= 0x1EFF) || (r >= 0x0100 && r <= 0x024F) {
			return "vi"
		}
	}
	// Không có ký tự có dấu → ASCII hoàn toàn → tiếng Anh
	return "en"
}

// MessageGenerator generates contextual messages for auto-commenting and auto-inbox.
type MessageGenerator struct {
	apiKey string
	model  string
	client *http.Client
}

// NewMessageGenerator creates a new AI message generator.
func NewMessageGenerator(apiKey, model string) *MessageGenerator {
	return &MessageGenerator{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Available returns true if the generator has a valid API key.
func (mg *MessageGenerator) Available() bool {
	return mg.apiKey != ""
}

// GenerateComment generates a contextual comment for any business.
// businessContext is a free-form description loaded from user_context (used as fallback profile).
func (mg *MessageGenerator) GenerateComment(ctx context.Context, postContent, authorName, businessContext string) (string, error) {
	return mg.GenerateCommentWithService(ctx, postContent, authorName, businessContext, "", models.CompanyIdentity{}, models.ActorPersona{})
}

// GenerateCommentWithService generates a comment using business profile for any industry.
// serviceMatch and niche are kept for backward compat but no longer drive hardcoded templates.
func (mg *MessageGenerator) GenerateCommentWithService(ctx context.Context, postContent, authorName, businessContext, serviceMatch string, identity models.CompanyIdentity, persona models.ActorPersona) (string, error) {
	lang := detectLang(postContent)
	var langRule string
	if lang == "en" {
		langRule = "MUST write in English."
	} else {
		langRule = "Viết bằng tiếng Việt."
	}

	// Build company context from businessContext param (caller injects profile.ToPromptBlock())
	// If serviceMatch has a URL, append it
	serviceNote := ""
	if url := PickServiceURL(serviceMatch); url != "" && serviceMatch != "" {
		serviceNote = fmt.Sprintf("\nMost relevant service link: %s", url)
	}

	// Brand-trust grounding (PR-3): the configured company identity (brand / website /
	// official contact / what-we-do) so the comment can include a real website +
	// contact for the lead to reach us — grounded ONLY in this block, never invented.
	companyBlock := buildCompanyBlock(identity)
	contactRule := buildContactRule(identity)
	// Multi-actor coverage: when a teammate already covered this lead, the persona
	// forces a different angle (no repeated website/CTA/phrasing). Empty for the first.
	personaRule := comment.BuildPersonaRule(persona)

	prompt := fmt.Sprintf(`You are a senior sales professional with 10+ years of experience. Write a natural, human-sounding comment on this post.

BUSINESS PROFILE:
%s%s

COMPANY IDENTITY (ground every brand / website / contact claim ONLY in this block — never invent one):
%s

POST AUTHOR: %s
POST CONTENT:
"""%s"""

RULES:
1. %s
2. Address the author by their EXACT name
3. 2–4 sentences. Natural tone — NOT a bot
4. Acknowledge their specific need or pain point
5. Introduce your most relevant offering naturally
6. End with a soft CTA, and if a Website and/or Official contact ARE listed in COMPANY IDENTITY, include them so the lead can reach you (each at most once).
7. NO EMOJIS. Professional but human.
%s
%s

RETURN ONLY THE COMMENT, NO EXPLANATION.`, businessContext, serviceNote, companyBlock, authorName, postContent, langRule, contactRule, personaRule)

	return mg.callOpenAI(ctx, prompt)
}

// GenerateCommentFromTemplate fills in a comment template with lead-specific details.
func (mg *MessageGenerator) GenerateCommentFromTemplate(ctx context.Context, template, postContent, authorName string) (string, error) {
	lang := detectLang(postContent)
	var langRule string
	if lang == "en" {
		langRule = "The post is in English — respond in English."
	} else {
		langRule = "Bài viết tiếng Việt — trả lời tiếng Việt."
	}

	prompt := fmt.Sprintf(`Adapt this sales comment template to fit the specific post below. Keep the spirit but personalize it. %s

Template:
"""%s"""

Post to comment on:
Author: %s
Content: """%s"""

RETURN ONLY THE ADAPTED COMMENT, NO EXPLANATION.`, langRule, template, authorName, postContent)

	return mg.callOpenAI(ctx, prompt)
}

// GenerateInboxMessage creates an AI-written inbox/DM message for any business and any lead.
// businessContext is the business profile block (profile.ToPromptBlock() or legacy free-form text).
// niche is kept for backward compat but no longer drives hardcoded templates.
func (mg *MessageGenerator) GenerateInboxMessage(ctx context.Context, leadContent, recipientName, businessContext, _ string) (string, error) {
	lang := detectLang(leadContent)
	var langRule string
	if lang == "en" {
		langRule = "Write in English."
	} else {
		langRule = "Viết bằng tiếng Việt, tự nhiên."
	}

	prompt := fmt.Sprintf(`You are a senior professional with 10+ years of experience. Write a personalized inbox message to this person.

BUSINESS PROFILE:
%s

RECIPIENT: %s
THEIR POST/CONTEXT:
"""%s"""

RULES:
1. %s
2. Warm, personal greeting using their name
3. Reference something SPECIFIC from their post — show you read it
4. Explain concisely how you can help their situation
5. 1–2 concrete benefits or differentiators
6. Clear CTA: ask to chat, DM, call, or visit
7. 3–5 sentences. Conversational, NOT corporate.
8. NO EMOJIS. Genuine and professional.

RETURN ONLY THE MESSAGE CONTENT.`, businessContext, recipientName, leadContent, langRule)

	return mg.callOpenAI(ctx, prompt)
}

// GenerateRecruitmentComment creates a personalized @mention reply to a job-seeker's comment.
// postContext is the original recruiter post topic (for JD matching context).
// jobsContext is a pre-formatted string of open positions from the DB.
// businessContext is the ToPromptBlock() output from BusinessProfile.
// The reply always starts with @candidateName so Facebook notifies the commenter.
func (mg *MessageGenerator) GenerateRecruitmentComment(ctx context.Context, postContext, candidateContent, candidateName, jobsContext, businessContext string) (string, error) {
	lang := detectLang(candidateContent)

	var prompt string
	if lang == "en" {
		prompt = fmt.Sprintf(`You are a Senior HR Professional representing this business:
%s

A candidate has commented on a job post expressing interest in finding work. Write a SHORT, natural reply to their comment inviting them to apply.

POST CONTEXT (what the original post was about): """%s"""
CANDIDATE NAME: %s
CANDIDATE COMMENT: """%s"""

OPEN POSITIONS:
%s

RULES:
1. Write in English (comment is in English)
2. START with "@%s" (exactly — so Facebook notifies them)
3. Acknowledge their job search (1 sentence)
4. Mention the most relevant open position matching the post topic (1 sentence, use exact job title)
5. End with a clear CTA to send CV or message us
6. Max 2-3 sentences total. NO EMOJIS. Professional and warm — NOT generic spam.
7. Do NOT mention salary or make promises.

RETURN ONLY THE COMMENT REPLY CONTENT.`, businessContext, postContext, candidateName, candidateContent, jobsContext, candidateName)
	} else {
		prompt = fmt.Sprintf(`Bạn là HR Senior đại diện cho doanh nghiệp sau:
%s

Một ứng viên đã COMMENT vào một bài tuyển dụng, thể hiện họ đang tìm việc. Viết 1 reply ngắn, chuyên nghiệp, tự nhiên để mời họ ứng tuyển.

BỐI CẢNH BÀI ĐĂNG GỐC: """%s"""
TÊN ỨNG VIÊN: %s
COMMENT CỦA ỨNG VIÊN: """%s"""

VỊ TRÍ ĐANG TUYỂN:
%s

QUY TẮC:
1. Viết tiếng Việt, tự nhiên như người thật (KHÔNG như bot/spam)
2. BẮT ĐẦU BẰNG "@%s" (chính xác — để Facebook notify đúng người)
3. Ghi nhận họ đang tìm việc (1 câu ngắn)
4. Đề cập VỊ TRÍ PHÙ HỢP NHẤT với nội dung bài đăng gốc (gọi đúng tên vị trí, 1 câu)
5. CTA rõ ràng để ứng viên gửi CV hoặc nhắn tin
6. Tổng cộng tối đa 2-3 câu. TUYỆT ĐỐI KHÔNG DÙNG EMOJI. Uy tín, chuyên nghiệp.
7. KHÔNG đề cập lương hoặc hứa hẹn cụ thể.

CHỈ TRẢ VỀ NỘI DUNG REPLY COMMENT.`, businessContext, postContext, candidateName, candidateContent, jobsContext, candidateName)
	}

	return mg.callOpenAI(ctx, prompt)
}

// GenerateFollowUp creates a contextual follow-up reply using the full conversation history.
// conversationHistory is a pre-formatted string of alternating lines: "Role: content".
func (mg *MessageGenerator) GenerateFollowUp(ctx context.Context, conversationHistory, recipientName, businessContext, _ string) (string, error) {
	lang := detectLang(conversationHistory)

	persona := "You are a Senior Sales & Customer Service Professional with over 10 years of experience."

	var prompt string
	if lang == "en" {
		prompt = fmt.Sprintf(`%s

You are continuing a conversation with a prospect named %s. Read the full conversation history below and write a natural, professional follow-up reply to their latest message. Stay on topic — do NOT start a new pitch.

CONVERSATION HISTORY:
%s

BUSINESS CONTEXT:
%s

RULES:
1. Write in English
2. Directly address their latest message
3. Be helpful, specific, and professional (NOT generic)
4. Max 3-4 sentences
5. ABSOLUTELY NO EMOJIS.
6. End with a soft CTA or open question to keep the dialogue going

RETURN ONLY THE REPLY CONTENT.`, persona, recipientName, conversationHistory, businessContext)
	} else {
		prompt = fmt.Sprintf(`%s

Bạn đang tiếp tục cuộc trò chuyện với khách hàng tên %s. Đọc toàn bộ lịch sử hội thoại bên dưới và viết phản hồi follow-up tự nhiên, chuyên nghiệp đáp lại tin nhắn mới nhất của họ. Đừng bắt đầu lại từ đầu — tiếp tục mạch hội thoại.

LỊCH SỬ HỘI THOẠI:
%s

THÔNG TIN DOANH NGHIỆP:
%s

QUY TẮC:
1. Viết tiếng Việt, tự nhiên, văn phong chuyên nghiệp
2. Đáp lại TRỰC TIẾP tin nhắn mới nhất của họ
3. Hữu ích, cụ thể và uy tín (KHÔNG generic)
4. Tối đa 3-4 câu
5. TUYỆT ĐỐI KHÔNG DÙNG EMOJI.
6. Kết bằng câu hỏi mở hoặc CTA nhẹ để duy trì hội thoại

CHỈ TRẢ VỀ NỘI DUNG TIN NHẮN.`, persona, recipientName, conversationHistory, businessContext)
	}

	return mg.callOpenAI(ctx, prompt)
}

// GenerateJobPost creates a polished Facebook job posting from structured inputs.
// Returns a ready-to-post text (no HTML, no emojis unless Vietnamese style calls for it).
func (mg *MessageGenerator) GenerateJobPost(ctx context.Context, title, description, requirements, benefits, salary, email string) (string, error) {
	prompt := fmt.Sprintf(`You are a Senior HR Professional. Write a professional, engaging Facebook job post for the following position. The post should attract quality candidates naturally — NOT look like a generic spam ad.

JOB DETAILS:
- Title: %s
- Description: %s
- Requirements: %s
- Benefits: %s
- Salary: %s
- Apply via: %s

RULES:
1. Write in Vietnamese (natural, professional tone)
2. Start with the job title prominently
3. Describe what the candidate will actually do (concrete, not vague)
4. List 3-5 key requirements (bullet points with -)
5. List 2-3 benefits (salary/bonus, growth, environment)
6. End with clear CTA: send CV to the provided email
7. Max 250 words. No excessive emojis. Professional but warm.
8. Do NOT include hashtags or links other than the email.

RETURN ONLY THE POST CONTENT.`, title, description, requirements, benefits, salary, email)

	return mg.callOpenAI(ctx, prompt)
}

// ExtractJobKeywords returns domain-aware search keywords for a JD — no API call.
// Used to find relevant Facebook groups and posts for each job position.
func (mg *MessageGenerator) ExtractJobKeywords(job models.CareerJob) []string {
	seen := make(map[string]bool)
	var kws []string
	add := func(s string) {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" && len(s) >= 3 && !seen[s] {
			seen[s] = true
			kws = append(kws, s)
		}
	}
	for _, w := range strings.Fields(job.Title) {
		add(w)
	}
	t := strings.ToLower(job.Title)
	switch {
	case strings.Contains(t, "sales") || strings.Contains(t, "kinh doanh"):
		add("tuyển dụng sales")
		add("nhân viên kinh doanh")
		add("kinh doanh")
	case strings.Contains(t, "accountant") || strings.Contains(t, "kế toán"):
		add("tuyển kế toán")
		add("nhân viên kế toán")
		add("kế toán")
	case strings.Contains(t, "ai") || strings.Contains(t, "research") || strings.Contains(t, "intern"):
		add("tuyển dụng AI")
		add("thực tập sinh")
		add("intern IT")
	case strings.Contains(t, "operations") || strings.Contains(t, "vận hành"):
		add("nhân viên vận hành")
		add("e-commerce operations")
		add("ecommerce")
	case strings.Contains(t, "warehouse") || strings.Contains(t, "kho"):
		add("kho vận")
		add("logistics")
		add("warehouse")
	case strings.Contains(t, "china") || strings.Contains(t, "trung quốc"):
		add("china desk")
		add("tiếng trung")
		add("nhập khẩu trung quốc")
	}
	add("tuyển dụng")
	if job.Location != "" {
		add(job.Location)
	}
	return kws
}

// ScoreCandidateMatch uses AI to score 0.0–1.0 how well a candidate matches a job.
// postContext is the topic of the post the candidate commented on — enforces strict domain matching.
func (mg *MessageGenerator) ScoreCandidateMatch(ctx context.Context, candidateContent, postContext string, job models.CareerJob) (float64, string, error) {
	prompt := fmt.Sprintf(`You are a strict senior HR recruiter scoring candidate-job fit.

JOB:
Title: %s
Requirements: %s
Location: %s

POST CONTEXT (topic of the post this candidate commented on):
%s

CANDIDATE COMMENT:
%s

STRICT DOMAIN RULE — domain mismatch MUST result in score 0.0:
- Tech/AI/dev post → ONLY tech roles (AI Intern, Developer, etc.)
- Finance/accounting post → ONLY accounting roles
- Sales/logistics post → sales/logistics roles
- DO NOT cross-match: dev≠sales, dev≠accountant, sales≠tech

Score 0.0–1.0:
1.0 = perfect match | 0.7+ = strong | 0.5–0.7 = moderate | <0.5 = weak | 0.0 = mismatch

Respond with ONLY valid JSON:
{"score": 0.XX, "reason": "one sentence"}`,
		job.Title, sliceStr(job.Requirements, 200), job.Location,
		sliceStr(postContext, 200), sliceStr(candidateContent, 300))

	raw, err := mg.callOpenAI(ctx, prompt)
	if err != nil {
		return 0, "", err
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}") + 1
	if start < 0 || end <= start {
		return 0, "", fmt.Errorf("bad score response: %s", sliceStr(raw, 80))
	}
	var resp struct {
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw[start:end]), &resp); err != nil {
		return 0, "", fmt.Errorf("parse score JSON: %w", err)
	}
	if resp.Score < 0 {
		resp.Score = 0
	}
	if resp.Score > 1 {
		resp.Score = 1
	}
	return resp.Score, resp.Reason, nil
}

// GeneratePersonalizedInbox creates a warm, personalized DM for a matched candidate.
// More detailed than a comment: references their specific background and the JD highlights.
func (mg *MessageGenerator) GeneratePersonalizedInbox(ctx context.Context, candidateContent, candidateName string, job models.CareerJob) (string, error) {
	lang := detectLang(candidateContent)
	salary := job.Salary
	if salary == "" {
		if lang == "en" {
			salary = "Negotiable"
		} else {
			salary = "Thoả thuận"
		}
	}

	var prompt string
	if lang == "en" {
		prompt = fmt.Sprintf(`You are a Senior HR Professional. Write a warm, personalized direct message to a candidate actively looking for a job.

CANDIDATE: %s
THEIR COMMENT: """%s"""
ROLE: %s (%s)
DESCRIPTION: %s
REQUIREMENTS: %s
SALARY: %s
APPLY TO: %s

RULES:
1. English only
2. Open with "Hi %s,"
3. Acknowledge their job search (1 sentence referencing their comment specifically)
4. Explain why they could fit this role (2 sentences — use their actual background)
5. Give 1–2 concrete role highlights (not generic)
6. Mention salary: %s
7. CTA: send CV to %s
8. Close warmly
9. Max 5–6 sentences. NO EMOJIS. Professional and personal.

RETURN ONLY THE MESSAGE.`,
			candidateName, sliceStr(candidateContent, 300), job.Title, job.Location,
			sliceStr(job.Description, 250), sliceStr(job.Requirements, 200), salary, job.Email,
			candidateName, salary, job.Email)
	} else {
		prompt = fmt.Sprintf(`Bạn là HR Senior chuyên nghiệp. Viết 1 tin nhắn riêng tư ấm áp, cá nhân hóa cho ứng viên đang tìm việc.

ỨNG VIÊN: %s
COMMENT CỦA HỌ: """%s"""
VỊ TRÍ: %s (%s)
MÔ TẢ: %s
YÊU CẦU: %s
LƯƠNG: %s
GỬI CV VỀ: %s

QUY TẮC:
1. Tiếng Việt, thân thiện và chuyên nghiệp
2. Mở đầu: "Chào %s,"
3. Nhận thấy họ đang tìm việc (1 câu, cụ thể từ comment của họ)
4. Giải thích vì sao họ có thể phù hợp với vị trí này (2 câu, dùng background thực của họ)
5. Nêu 1–2 điểm nổi bật thực sự của vị trí (không generic)
6. Đề cập lương: %s
7. CTA: gửi CV về %s
8. Kết thân thiện
9. Tổng cộng 5–6 câu. TUYỆT ĐỐI KHÔNG DÙNG EMOJI. Chuyên nghiệp và cá nhân hóa.

CHỈ TRẢ VỀ NỘI DUNG TIN NHẮN.`,
			candidateName, sliceStr(candidateContent, 300), job.Title, job.Location,
			sliceStr(job.Description, 250), sliceStr(job.Requirements, 200), salary, job.Email,
			candidateName, salary, job.Email)
	}
	return mg.callOpenAI(ctx, prompt)
}

// callOpenAIStrictJSON is the JSON-schema-locked counterpart to callOpenAI.
// The model is forced to emit a payload that conforms exactly to `schema`,
// so callers no longer have to scan the response for balanced braces or
// strip stray prose. `out` must be a pointer whose Go shape matches the
// schema.
//
// Schema rules (per OpenAI strict mode):
//   - every object must have "additionalProperties": false
//   - every property must appear under "required"
//   - "enum" is honoured for closed sets (e.g. classifier intents)
//
// Temperature and max_tokens are intentionally omitted so this helper
// works on both classic chat models (gpt-4o*) and reasoning models
// (gpt-5*) without callers having to special-case the model family.
func (mg *MessageGenerator) callOpenAIStrictJSON(ctx context.Context, prompt, schemaName string, schema map[string]any, out any) error {
	body := map[string]any{
		"model": mg.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   schemaName,
				"strict": true,
				"schema": schema,
			},
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mg.apiKey)

	resp, err := mg.client.Do(req)
	if err != nil {
		return fmt.Errorf("OpenAI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if len(result.Choices) == 0 {
		return fmt.Errorf("no response from OpenAI")
	}
	if refusal := strings.TrimSpace(result.Choices[0].Message.Refusal); refusal != "" {
		return fmt.Errorf("OpenAI refused: %s", refusal)
	}
	content := result.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("empty content from OpenAI")
	}
	return json.Unmarshal([]byte(content), out)
}

// isReasoningModel reports whether model is an OpenAI reasoning-class model
// (o-series or the GPT-5 family). These models reject sampling controls such
// as temperature != 1 (HTTP 400) and spend hidden reasoning tokens before
// emitting any output, so callers must omit temperature and budget extra
// completion tokens.
func isReasoningModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") ||
		strings.HasPrefix(m, "gpt-5")
}

// chatCompletionBody builds the /chat/completions request body for a freeform
// (non-JSON) generation. It is model-aware so the SAME call works on both
// classic chat models (gpt-4o / gpt-4.1) and reasoning models (gpt-5*, o*):
//
//   - temperature is sent ONLY for non-reasoning models. gpt-5* / o* reject
//     any temperature other than the default 1 with HTTP 400 — that single
//     line silently failed EVERY comment/inbox/post generation (skip reason
//     generation_failed) once OPENAI_COMMENT_MODEL defaulted to gpt-5.4,
//     while classification kept working because it uses the no-temperature
//     callOpenAIStrictJSON path.
//   - max_completion_tokens is generous (2000) because reasoning models spend
//     hidden reasoning tokens first; the old 400 cap could be fully consumed
//     by reasoning, returning empty content (finish_reason=length).
func chatCompletionBody(model, prompt string) map[string]any {
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_completion_tokens": 2000,
	}
	if !isReasoningModel(model) {
		body["temperature"] = 0.7
	}
	return body
}

// callOpenAI runs callOpenAIOnce with a single retry on TRANSIENT failures
// (network blip, HTTP 429/5xx, empty completion). It deliberately does NOT retry
// once the caller's context is cancelled/expired — a retry would have no time
// budget left and just fail again ("context deadline exceeded"). A short backoff
// separates the two attempts.
func (mg *MessageGenerator) callOpenAI(ctx context.Context, prompt string) (string, error) {
	const maxAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out, err := mg.callOpenAIOnce(ctx, prompt)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == maxAttempts || !isRetryableOpenAIError(err) {
			break
		}
		select {
		case <-ctx.Done():
			return "", lastErr
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", lastErr
}

// isRetryableOpenAIError reports whether a callOpenAIOnce failure is worth one
// retry. Context cancellation/deadline is NOT retryable (no time remains).
func isRetryableOpenAIError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "openai request failed") || // transport/network blip
		strings.Contains(s, "http 429") ||
		strings.Contains(s, "http 500") || strings.Contains(s, "http 502") ||
		strings.Contains(s, "http 503") || strings.Contains(s, "http 504") ||
		strings.Contains(s, "empty content")
}

func (mg *MessageGenerator) callOpenAIOnce(ctx context.Context, prompt string) (string, error) {
	jsonBody, _ := json.Marshal(chatCompletionBody(mg.model, prompt))
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mg.apiKey)

	resp, err := mg.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}
	content := result.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		// Surface empty output as an explicit error instead of returning ""
		// (which the caller would silently treat as empty_content and skip).
		// On reasoning models this usually means max_completion_tokens was
		// consumed by hidden reasoning before any visible text was emitted.
		return "", fmt.Errorf("empty content from OpenAI (model=%s)", mg.model)
	}
	return content, nil
}
