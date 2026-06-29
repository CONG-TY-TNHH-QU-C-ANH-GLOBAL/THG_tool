package copilot

import "github.com/thg/scraper/internal/drivers/copilot/intent"

// Thin package-local delegations to the copilot/intent subpackage (ARCHCP3). The intent
// classification cluster moved to internal/drivers/copilot/intent; these shims keep the
// existing agent_* / routing_* / test call sites unchanged (wrapper-first move — no
// caller churn, no import added to the near-200-line caller files). Callers migrate to
// intent.* directly in a follow-up, then these shims go away.

func deterministicFacebookAction(prompt string, orgID, accountID int64) (string, map[string]any, bool) {
	return intent.DeterministicFacebookAction(prompt, orgID, accountID)
}

func promptIsDirectPostComment(prompt string) bool {
	return intent.PromptIsDirectPostComment(prompt)
}

func firstFacebookURL(prompt string) string {
	return intent.FirstFacebookURL(prompt)
}

func extractMaxItemsFromPrompt(prompt string) int64 {
	return intent.ExtractMaxItemsFromPrompt(prompt)
}

func promptKeywords(prompt string) string {
	return intent.PromptKeywords(prompt)
}
