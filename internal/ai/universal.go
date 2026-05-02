package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// UniversalClassifyResult is the structured output of UniversalClassify.
type UniversalClassifyResult struct {
	Score    float64 `json:"score"`    // 0.0–1.0
	Intent   string  `json:"intent"`   // "potential_customer", "not_relevant", "spam", "candidate", "partner"
	Reason   string  `json:"reason"`   // one sentence
	Priority string  `json:"priority"` // "hot", "warm", "cold", "rejected"
}

// UniversalClassify evaluates ANY post against ANY business profile.
// No hardcoded industry logic — the profile drives everything.
// This replaces the old tuyen_dung/logistics branches in classifier.go.
func (mg *MessageGenerator) UniversalClassify(ctx context.Context, postContent, authorName string, profile *BusinessProfile) (*UniversalClassifyResult, error) {
	lang := detectLang(postContent)
	var langRule string
	if lang == "en" {
		langRule = "Respond in English."
	} else {
		langRule = "Trả lời bằng tiếng Việt."
	}

	prompt := fmt.Sprintf(`You are an expert lead qualification AI for a business.

%s

TASK: Evaluate this social media post. Is the author a potential customer/candidate for this business?

POST AUTHOR: %s
POST CONTENT:
"""%s"""

SCORING RULES:
- Score 0.8–1.0 (hot): Author clearly needs what we offer right now
- Score 0.5–0.7 (warm): Author might need us, indirect signal
- Score 0.2–0.4 (cold): Weak signal, worth low-priority outreach
- Score 0.0–0.1 (rejected): Irrelevant, competitor, spam, or violates IGNORE rules

MARKET SIGNAL RULES:
- Do not infer customer intent from industry keywords alone.
- First identify the author's role in this post.
- If the author is advertising/selling/providing a service, classify as rejected unless the business profile explicitly says partners, suppliers, or resellers are desired.
- A lead should normally contain a need/request/problem/question such as asking for quotes, suppliers, recommendations, help, buying, hiring, shipping, sourcing, or support.
- For broad industries, prefer precision over volume. Reject provider ads, generic promotions, spam, and self-promotion even when they contain matching keywords.

INTENT OPTIONS:
- "potential_customer": wants to buy/use our products/services
- "candidate": looking for a job (only for recruitment businesses)
- "partner": could be a business partner/reseller
- "provider_ad": author is promoting/selling their own service/product
- "not_relevant": post has nothing to do with our business
- "spam": MLM, mass recruitment ads, irrelevant promotions

%s

Respond with ONLY valid JSON:
{"score": 0.0, "intent": "", "reason": "", "priority": "hot|warm|cold|rejected"}`,
		profile.ToPromptBlock(), authorName, postContent, langRule)

	raw, err := mg.callOpenAI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("universal classify: %w", err)
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}") + 1
	if start < 0 || end <= start {
		return &UniversalClassifyResult{Score: 0, Intent: "not_relevant", Priority: "rejected"}, nil
	}

	var result UniversalClassifyResult
	if err := json.Unmarshal([]byte(raw[start:end]), &result); err != nil {
		return &UniversalClassifyResult{Score: 0, Intent: "not_relevant", Priority: "rejected"}, nil
	}

	// Clamp score
	if result.Score < 0 {
		result.Score = 0
	}
	if result.Score > 1 {
		result.Score = 1
	}

	return &result, nil
}

// UniversalComment generates a contextual comment for ANY business, for ANY post.
// No hardcoded company names or industry templates.
// This replaces GenerateCommentWithService for the universal use case.
func (mg *MessageGenerator) UniversalComment(ctx context.Context, postContent, authorName string, profile *BusinessProfile) (string, error) {
	lang := detectLang(postContent)
	var langRule string
	if lang == "en" {
		langRule = "MUST write in English. Match the author's language."
	} else {
		langRule = "Viết bằng tiếng Việt. Khớp ngôn ngữ với tác giả."
	}

	prompt := fmt.Sprintf(`You are a senior sales professional with 10+ years of experience. Write a natural, human-sounding comment on this social media post on behalf of the business below.

%s

POST AUTHOR: %s
POST CONTENT:
"""%s"""

RULES:
1. %s
2. Address the author by their exact name
3. 2–3 sentences MAX. Sound like a real person, NOT a bot
4. Directly address their specific need or pain point from the post
5. Introduce your most relevant service/product naturally — do not be pushy
6. End with a soft CTA ("Inbox mình nhé!" / "Feel free to DM us!")
7. NO EMOJIS. Professional but warm tone.
8. If the post is in a group context, be relevant to that context

RETURN ONLY THE COMMENT TEXT, NO EXPLANATION, NO QUOTES.`, profile.ToPromptBlock(), authorName, postContent, langRule)

	return mg.callOpenAI(ctx, prompt)
}

// UniversalInbox generates a personalized inbox/DM message for ANY business.
// Replaces GenerateInboxMessage for the universal use case.
func (mg *MessageGenerator) UniversalInbox(ctx context.Context, postContent, recipientName string, profile *BusinessProfile) (string, error) {
	lang := detectLang(postContent)
	var langRule string
	if lang == "en" {
		langRule = "Write in English."
	} else {
		langRule = "Viết bằng tiếng Việt."
	}

	prompt := fmt.Sprintf(`You are a senior sales professional. Write a personalized inbox/DM message to a potential customer.

%s

RECIPIENT: %s
THEIR POST/CONTEXT:
"""%s"""

REQUIREMENTS:
1. %s
2. Open with a warm, personal greeting using their name
3. Reference something specific from their post to show you read it (not generic)
4. Explain clearly how your business can help their specific situation
5. Mention 1–2 concrete benefits or differentiators
6. Clear call-to-action: ask to chat, schedule a call, or visit your page
7. 3–5 sentences. Conversational, not corporate.
8. NO EMOJIS. Professional, genuine tone.

RETURN ONLY THE MESSAGE CONTENT, NO SUBJECT LINE, NO EXPLANATION.`, profile.ToPromptBlock(), recipientName, postContent, langRule)

	return mg.callOpenAI(ctx, prompt)
}

// UniversalGroupQueries generates Facebook group search queries for ANY business.
// Replaces hardcoded domain-specific queries.
func (mg *MessageGenerator) UniversalGroupQueries(ctx context.Context, profile *BusinessProfile) ([]string, error) {
	if !profile.IsConfigured() {
		return []string{"tuyển dụng việc làm HCM"}, nil
	}

	prompt := fmt.Sprintf(`You are a social media marketing expert. Generate Facebook group search queries to find the most relevant communities for this business.

%s

Generate 6–8 Facebook group search queries that will find groups where the business's IDEAL CUSTOMERS are active.
Think about: what problems do their customers have? What groups do they join? What topics do they discuss?

Rules:
- Mix Vietnamese and English queries
- Include location if known
- Target the CUSTOMER communities, not the business's own industry peers
- Include both broad and niche queries
- Make them sound natural (how a user would search for a FB group)

Respond with ONLY a JSON array of strings:
["query 1", "query 2", "query 3", ...]`, profile.ToPromptBlock())

	raw, err := mg.callOpenAI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("group queries: %w", err)
	}

	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]") + 1
	if start < 0 || end <= start {
		return nil, fmt.Errorf("bad group queries response")
	}

	var queries []string
	if err := json.Unmarshal([]byte(raw[start:end]), &queries); err != nil {
		return nil, fmt.Errorf("parse queries: %w", err)
	}

	// Deduplicate and cap at 8
	seen := make(map[string]bool)
	var clean []string
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q != "" && !seen[q] {
			seen[q] = true
			clean = append(clean, q)
			if len(clean) >= 8 {
				break
			}
		}
	}
	return clean, nil
}
