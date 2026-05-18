package ai

import (
	"encoding/json"
	"strings"
)

// RoutingDecision captures WHY the orchestrator picked a particular route
// for a given prompt. Persisted alongside every prompt_logs row in
// routing_decision_json so dashboards / replay surfaces / heuristic-drift
// analyses can reconstruct the orchestrator's reasoning without re-running
// the prompt.
//
// Stable wire shape — adding optional fields is safe (downstream tools
// must tolerate unknown keys). Removing or renaming requires a migration
// because historical rows reference the old names.
type RoutingDecision struct {
	// Route names the layer that handled the prompt.
	// Aggregation key for the "Routing Distribution" panel.
	Route string `json:"route"`
	// SufficiencyScore is 0..1: 1 = prompt is fully self-describing
	// (URL + crawl verb + count or signals), 0 = pure free-form chat.
	// Today the boolean gate emits 1.0 or 0.0; a future scored resolver
	// fills in the middle. Stored as float64 so the upgrade is lossless.
	SufficiencyScore float64 `json:"sufficiency_score"`
	// MissingSignals enumerates the operational signals (source, action,
	// target persona, market, quantity) the prompt did NOT carry. Used by
	// the "Ambiguous Prompt Surface" panel to spot recurring gaps.
	MissingSignals []string `json:"missing_signals,omitempty"`
	// InferredSignals enumerates signals the orchestrator derived from
	// the prompt body (e.g. ["pod", "dropship", "fulfillment"]). Used by
	// the "Routing Reason Breakdown" panel to verify the gate is seeing
	// what the user intended.
	InferredSignals []string `json:"inferred_signals,omitempty"`
	// ReasonCode is the machine-readable label the dashboard aggregates
	// on. Free-form `Reason` is operator-readable but not aggregatable.
	// See the Reason* constants below for the canonical set.
	ReasonCode string `json:"reason_code"`
	// Reason is the operator-readable explanation. Used in row drill-down,
	// NOT for aggregation. Empty string is acceptable; ReasonCode is the
	// load-bearing field.
	Reason string `json:"reason,omitempty"`
}

// Routes — the four orchestration layers.
const (
	RouteDeterministic = "deterministic"  // self-sufficient prompt → direct dispatch
	RouteBrain         = "brain"          // brain planner handled (execute / ask_user / chat / refuse)
	RouteLLMFallback   = "llm_fallback"   // OpenAI tool-call last-resort
	RouteScopeGuard    = "scope_guard"    // facebook scope guard blocked (out-of-domain prompt)
	RoutePreflight     = "preflight"      // business / browser preflight blocked
)

// Reason codes — machine-readable. Dashboards aggregate on these; free
// text in `Reason` is just for human drill-down. Adding new codes is fine
// (dashboards render unknowns under "other"); changing an existing code
// breaks historical rows aggregating against it.
const (
	// Deterministic-route reasons.
	ReasonSelfSufficient           = "self_sufficient_prompt"
	ReasonSelfSufficientLeadAction = "self_sufficient_lead_action"
	ReasonExplicitUserCommand      = "explicit_user_command"
	ReasonDeterministicMatched     = "deterministic_match"
	ReasonDeterministicNoMatch     = "deterministic_no_match"

	// Brain decisions.
	ReasonBrainExecute             = "brain_execute"
	ReasonBrainAskUser             = "brain_ask_user"
	ReasonBrainChat                = "brain_chat"
	ReasonBrainRefuse              = "brain_refuse"
	ReasonBrainInvalidPlan         = "brain_invalid_plan"
	ReasonBrainNoHandler           = "brain_no_handler"
	ReasonBrainClarificationNeeded = "brain_clarification_required"

	// Missing signals — these surface in MissingSignals and as the
	// dominant ReasonCode for ask-backs.
	ReasonMissingSource            = "missing_source"
	ReasonMissingAction            = "missing_action"
	ReasonMissingTarget            = "missing_target"
	ReasonMissingMarket            = "missing_market"
	ReasonMissingQuantity          = "missing_quantity"
	ReasonAmbiguousTarget          = "ambiguous_target"

	// Preflight blockers.
	ReasonScopeGuardBlocked        = "scope_guard_blocked"
	ReasonBusinessPreflightBlocked = "business_preflight_blocked"
	ReasonBrowserPreflightBlocked  = "browser_preflight_blocked"

	// LLM fallback.
	ReasonLLMFallback              = "llm_fallback_dispatch"
	ReasonFallbackNoMatch          = "fallback_no_match"
)

// MissingSignal enumerates the operational signals a fully-specified
// prompt should carry. Used by analyseMissingSignals to build a
// stable list for the dashboard.
const (
	SignalSource   = "source"   // Facebook URL (group / post / profile)
	SignalAction   = "action"   // crawl verb / outbound verb
	SignalTarget   = "target"   // target audience (buyer / candidate / supplier)
	SignalMarket   = "market"   // market/geo hint
	SignalQuantity = "quantity" // explicit count
)

// analyseMissingSignals inspects the prompt and returns which operational
// signals are absent. Used by both the brain path (to label why brain was
// invoked) and the deterministic path (to enumerate what the prompt DID
// carry — those are simply "all signals present").
func analyseMissingSignals(prompt string) []string {
	var missing []string
	if firstFacebookURL(prompt) == "" {
		missing = append(missing, SignalSource)
	}
	folded := foldVietnameseForMatch(strings.ToLower(stripDashboardContext(prompt)))
	hasCrawlVerb := containsAnyFolded(folded, []string{"cao", "crawl", "scrape", "quet", "tim"})
	hasOutboundVerb := containsAnyFolded(folded, []string{"comment", "binh luan", "inbox", "nhan tin", "dang bai", "posting", "post len"})
	if !hasCrawlVerb && !hasOutboundVerb {
		missing = append(missing, SignalAction)
	}
	if extractMaxItemsFromPrompt(prompt) <= 0 {
		missing = append(missing, SignalQuantity)
	}
	inferred := inferCrawlTargetingFromPrompt(prompt)
	if strings.TrimSpace(inferred["target_signals"]) == "" {
		missing = append(missing, SignalTarget)
	}
	if !containsAnyFolded(folded, []string{"my", "usa", "us ", "viet nam", "vn ", "trung quoc", "tq ", "eu ", "japan"}) {
		missing = append(missing, SignalMarket)
	}
	return missing
}

// inferredSignalsFromPrompt returns the matched signals the orchestrator
// extracted from the prompt body. Mirrors what the SignalGate sees so
// dashboards can show "the system understood X, Y, Z" without re-running
// the inference.
func inferredSignalsFromPrompt(prompt string) []string {
	inferred := inferCrawlTargetingFromPrompt(prompt)
	raw := strings.TrimSpace(inferred["target_signals"])
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// NewDeterministicDecision builds the decision for a self-sufficient
// prompt that bypassed the brain. Called from the early short-circuit
// AND from the legacy late-path (after brain bailed) — same shape both
// times, only ReasonCode differs.
func NewDeterministicDecision(prompt, action, reasonCode string) RoutingDecision {
	if reasonCode == "" {
		reasonCode = ReasonDeterministicMatched
	}
	return RoutingDecision{
		Route:            RouteDeterministic,
		SufficiencyScore: 1.0,
		InferredSignals:  inferredSignalsFromPrompt(prompt),
		ReasonCode:       reasonCode,
		Reason:           "deterministic dispatch → " + action,
	}
}

// NewBrainDecision wraps a brain-planner outcome. brainDecision is the
// raw "execute" | "ask_user" | "chat" | "refuse" string from brain.py.
func NewBrainDecision(brainDecision, reason string) RoutingDecision {
	code := ReasonBrainExecute
	switch strings.ToLower(strings.TrimSpace(brainDecision)) {
	case "ask_user":
		code = ReasonBrainAskUser
	case "chat":
		code = ReasonBrainChat
	case "refuse":
		code = ReasonBrainRefuse
	case "execute":
		code = ReasonBrainExecute
	}
	return RoutingDecision{
		Route:      RouteBrain,
		ReasonCode: code,
		Reason:     reason,
	}
}

// NewPreflightDecision labels a row that was blocked by a preflight check.
func NewPreflightDecision(reasonCode, reason string) RoutingDecision {
	return RoutingDecision{
		Route:      RoutePreflight,
		ReasonCode: reasonCode,
		Reason:     reason,
	}
}

// NewScopeGuardDecision labels a row blocked by the facebook scope guard.
func NewScopeGuardDecision(reason string) RoutingDecision {
	return RoutingDecision{
		Route:      RouteScopeGuard,
		ReasonCode: ReasonScopeGuardBlocked,
		Reason:     reason,
	}
}

// NewLLMFallbackDecision labels a row that fell through to the OpenAI
// tool-call layer because neither deterministic nor brain matched.
func NewLLMFallbackDecision(action, reason string) RoutingDecision {
	code := ReasonLLMFallback
	if action == "" {
		code = ReasonFallbackNoMatch
	}
	return RoutingDecision{
		Route:      RouteLLMFallback,
		ReasonCode: code,
		Reason:     reason,
	}
}

// ToJSON serialises the decision for storage. Returns "{}" on error so
// the column never holds malformed JSON — dashboards can json.Unmarshal
// without error-handling per row.
func (d RoutingDecision) ToJSON() string {
	if d.Route == "" {
		return "{}"
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "{}"
	}
	return string(b)
}
