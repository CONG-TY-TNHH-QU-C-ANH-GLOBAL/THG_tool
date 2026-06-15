package copilot

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

func (a *Agent) loadUserContext() map[string]string {
	ctx, err := a.db.Leads().GetAllContext()
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
			_ = a.db.Leads().SetContext("last_search_intent", prompt)
			break
		}
	}

	// If the prompt describes a niche/business, save it
	nicheKeywords := []string{"lĩnh vực", "ngành", "niche", "chuyên về", "kinh doanh", "bán hàng", "dịch vụ"}
	for _, kw := range nicheKeywords {
		if strings.Contains(lower, kw) {
			_ = a.db.Leads().SetContext("last_niche_prompt", prompt)
			break
		}
	}
}

// buildDynamicSystemPrompt creates the AI Operator system prompt.
// Fully driven by BusinessProfile — no hardcoded niche strings.

func (a *Agent) getFewShotExamples(prompt string) []models.AIMemory {
	memories, err := a.db.Prompts().GetRelevantMemories(20)
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

// logPrompt persists a single orchestrator decision to prompt_logs.
//
// Watchpoint B: every call site MUST supply a RoutingDecision so the
// dashboard can aggregate on ReasonCode + Route. Zero-value decision is
// tolerated (serialises to "{}") but renders as "legacy/unknown" on the
// routing dashboard — prefer the constructor helpers in routing_decision.go
// (NewDeterministicDecision / NewBrainDecision / NewPreflightDecision /
// NewScopeGuardDecision / NewLLMFallbackDecision).
func (a *Agent) logPrompt(orgID, accountID, userID int64, source, prompt, response, action, args string, success bool, decision RoutingDecision) {
	pl := &models.PromptLog{
		OrgID:               orgID,
		AccountID:           accountID,
		UserID:              userID, // PR-M1: per-user chat privacy key
		Source:              source,
		UserPrompt:          prompt,
		AIResponse:          response,
		ActionTaken:         action,
		ActionArgs:          args,
		Success:             success,
		RoutingDecisionJSON: decision.ToJSON(),
	}
	_ = a.db.Prompts().InsertPromptLog(pl)
}

func (a *Agent) updateMemory(prompt, action, args string) {
	hash := promptHash(prompt)
	existing, err := a.db.Prompts().GetMemoryByHash(hash)
	if err == nil && existing != nil {
		_ = a.db.Prompts().UpdateMemoryUsage(existing.ID, true)
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
		_ = a.db.Prompts().InsertMemory(mem)
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
