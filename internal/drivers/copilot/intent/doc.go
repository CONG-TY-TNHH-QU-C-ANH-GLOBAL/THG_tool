// Package intent is the copilot deterministic intent-classification cluster: it maps a
// normalized Copilot prompt + extracted entities to an action name + args, with no DB /
// outbound / session access (downstream handlers own every readiness/coverage/quality/
// dedup/outbound gate). Pure classification.
//
// It depends only on neutral leaves — internal/drivers/copilot/textnorm (Vietnamese
// folding/matching), internal/drivers/copilot/promptprep (prompt preprocessing), and
// internal/fburl — plus stdlib; it imports nothing else from package copilot, so the
// dependency is one-way (copilot → intent). Public surface: DeterministicFacebookAction,
// PromptIsDirectPostComment, FirstFacebookURL, ExtractMaxItemsFromPrompt, PromptKeywords,
// RouteDecisionFor, and the RouteDecision / IntentEntities / IntentDecision / Confidence
// types. Moved out of package copilot under ARCHCP3 (Option B, incremental); thin
// package-copilot shims (intent_shims.go) keep existing call sites unchanged for now.
package intent
