package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// spamKeywords are Vietnamese low-skill / mass recruitment signals that penalize a group.
var spamKeywords = []string{
	"việc làm phổ thông", "tuyển gấp", "bao ăn ở", "phục vụ",
	"tạp vụ", "lao động phổ thông", "công nhân", "xuất khẩu lao động",
	"bảo vệ", "giúp việc", "bán hàng online tại nhà", "thu nhập 100",
	"tuyển 50 người", "tuyển mass", "daily wage",
}

// ScoreGroupQuality uses GPT to evaluate a Facebook group on 4 dimensions.
// groupName, groupDesc: the group metadata.
// samplePosts: concatenated text of recent posts (max ~1500 chars).
// jobDomain: the target domain to match against ("tech", "sales", "ops", "finance").
func (mg *MessageGenerator) ScoreGroupQuality(ctx context.Context, groupName, groupDesc, samplePosts, jobDomain string) (*models.GroupQuality, error) {
	// Pre-compute local spam penalty from keyword detection
	localSpam := 0.0
	combined := strings.ToLower(groupName + " " + groupDesc + " " + samplePosts)
	for _, kw := range spamKeywords {
		if strings.Contains(combined, kw) {
			localSpam += 0.10
		}
	}
	if localSpam > 0.5 {
		localSpam = 0.5
	}

	prompt := fmt.Sprintf(`You are a senior OSINT analyst specializing in Vietnamese social media recruitment.
Evaluate this Facebook group's quality for sourcing **%s** professionals.

GROUP NAME: %s
GROUP DESCRIPTION: %s
SAMPLE POSTS (recent):
%s

Score each dimension 0.0–1.0:
1. relevance: how well does this group match the "%s" job domain?
2. professionalism: presence of skilled professionals, industry-level discussions
3. content_quality: depth of posts, career discussions vs generic content
4. spam_penalty: 0.0=clean, 0.1–0.3=some spam, 0.4+=heavy spam (detect: "tuyển gấp", "việc làm phổ thông", "bao ăn ở", daily-wage labor posts, MLM)

Also determine:
- category: one of "tech", "sales", "ops", "finance", "low_quality"
- decision: "use" if final_score>=0.7, "monitor" if 0.4-0.69, "reject" if <0.4
- reason: one sentence explaining your decision

Formula: final = 0.4*relevance + 0.3*professionalism + 0.3*content_quality - spam_penalty

Respond with ONLY valid JSON:
{"relevance":0.0,"professionalism":0.0,"content_quality":0.0,"spam_penalty":0.0,"final_score":0.0,"category":"","decision":"","reason":""}`,
		jobDomain, sliceStr(groupName, 200), sliceStr(groupDesc, 500), sliceStr(samplePosts, 1500), jobDomain)

	raw, err := mg.callOpenAI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("score group: %w", err)
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}") + 1
	if start < 0 || end <= start {
		return nil, fmt.Errorf("bad score response: %s", sliceStr(raw, 100))
	}

	var resp struct {
		Relevance       float64 `json:"relevance"`
		Professionalism float64 `json:"professionalism"`
		ContentQuality  float64 `json:"content_quality"`
		SpamPenalty     float64 `json:"spam_penalty"`
		FinalScore      float64 `json:"final_score"`
		Category        string  `json:"category"`
		Decision        string  `json:"decision"`
		Reason          string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw[start:end]), &resp); err != nil {
		return nil, fmt.Errorf("parse score JSON: %w", err)
	}

	// Merge local spam detection with AI's assessment
	totalSpam := resp.SpamPenalty + localSpam
	if totalSpam > 1.0 {
		totalSpam = 1.0
	}

	// Recalculate final to ensure consistency
	finalScore := 0.4*resp.Relevance + 0.3*resp.Professionalism + 0.3*resp.ContentQuality - totalSpam
	if finalScore < 0 {
		finalScore = 0
	}

	decision := "monitor"
	if finalScore >= 0.7 {
		decision = "use"
	} else if finalScore < 0.4 {
		decision = "reject"
	}

	return &models.GroupQuality{
		Category:             resp.Category,
		RelevanceScore:       resp.Relevance,
		ProfessionalismScore: resp.Professionalism,
		ContentQualityScore:  resp.ContentQuality,
		SpamPenalty:          totalSpam,
		FinalScore:           finalScore,
		Decision:             decision,
		Reason:               resp.Reason,
	}, nil
}

// JobDomainCategory maps a career job to a group domain category.
// ONLY uses the Title to avoid false positives from descriptions mentioning unrelated keywords.
func JobDomainCategory(job models.CareerJob) string {
	t := " " + strings.ToLower(job.Title) + " "

	// Finance — check BEFORE tech to prevent "account" overlap
	if containsWord(t, "accountant", "accounting", "finance", "kế toán", "tài chính") {
		return "finance"
	}

	// Tech / Engineering
	if containsWord(t, "developer", "engineer", "software", "frontend", "backend",
		"devops", "fullstack", "data scientist", "machine learning", "research intern") {
		return "tech"
	}
	// "AI" only as exact word (not substring of "chair", "aims", etc.)
	if containsWord(t, " ai ") {
		return "tech"
	}

	// Sales / Business — checked BEFORE ops so "Shipping Sales Executive" → sales not ops
	if containsWord(t, "sales", "kinh doanh", "marketing", "business development",
		"account executive", "china desk", "dropship", "pod") {
		return "sales"
	}

	// Operations / Logistics / Warehouse — only when no "sales" keyword in title
	if containsWord(t, "operations", "logistics", "warehouse", "shipping",
		"supply chain", "kho", "vận hành", "e-commerce operations") {
		return "ops"
	}

	return "sales" // default to broadest category
}

// containsWord checks if padded text contains any of the keywords.
// Text should be pre-padded with spaces: " title here "
func containsWord(paddedText string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(paddedText, kw) {
			return true
		}
	}
	return false
}
