package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/skills"
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
	brain  *BrainClient
	// ActionHandler is the legacy execution path. Kept for backwards
	// compatibility — the Phase 6 skill registry, when present, is the
	// preferred path. The handler is still wired for code paths that
	// haven't migrated yet (deterministic fast-path, brain plan).
	ActionHandler func(action string, args map[string]any) (string, error)

	// registry, when set, drives the open-prompt resolver. Tools sent to
	// the LLM come from registry.EnabledFor(orgID); execution flows
	// through registry.Execute which handles per-org enablement,
	// validation, and audit logging.
	registry *skills.Registry
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

// SetBrainClient enables the schema-first planner sidecar. The sidecar only
// proposes a plan; Go still validates tenancy, account routing, and tool safety.
func (a *Agent) SetBrainClient(brain *BrainClient) {
	a.brain = brain
}

// SetSkillRegistry attaches the Phase 6 open-prompt skill catalog. Once
// set, the agent prefers registry.Execute over the legacy ActionHandler
// for skills the org has enabled, gaining per-org filtering, typed
// argument validation, and audit logging in skill_executions.
//
// The legacy path stays wired so existing deterministic actions (crawl
// fast-path, brain plan) keep working unchanged during the migration.
func (a *Agent) SetSkillRegistry(reg *skills.Registry) {
	a.registry = reg
}

// SkillRegistry returns the registered skill catalog or nil. Used by
// the API server to expose /api/skills without having to thread the
// registry through every handler.
func (a *Agent) SkillRegistry() *skills.Registry {
	return a.registry
}

// dispatchToolCall is the single execution point for an LLM-issued
// tool call. When the skill registry is wired, the call routes
// through registry.Execute (per-org enablement + typed validation +
// audit logging). When it isn't, or the skill ID is not registered,
// the call falls back to the legacy ActionHandler so unmigrated tools
// keep working unchanged. Returns the textual result the agent should
// surface back to chat / Telegram.
func (a *Agent) dispatchToolCall(ctx context.Context, fnName string, args map[string]any, orgID, accountID, userID int64, role, source, prompt string) (string, error) {
	if a.registry != nil {
		if skill := a.registry.Get(fnName); skill != nil {
			env := skills.Env{
				DB:        a.db,
				OrgID:     orgID,
				UserID:    userID,
				Role:      role,
				AccountID: accountID,
				Source:    source,
				Prompt:    prompt,
			}
			_, res, err := a.registry.Execute(ctx, env, fnName, args)
			if err != nil {
				return "", err
			}
			return res.Summary, nil
		}
	}
	if a.ActionHandler == nil {
		return "", fmt.Errorf("no executor available for %q", fnName)
	}
	return a.ActionHandler(fnName, args)
}

// runDeterministicFastPath dispatches a crawl/search/comment/inbox action
// when the prompt matches one of the deterministic patterns recognised by
// deterministicFacebookAction. Returns (response, handled): handled=false
// means no pattern matched and the caller should fall through to the
// next dispatch layer (brain planner or LLM tool-call fallback).
//
// Two callers (reasonCode disambiguates them on the routing dashboard):
//
//  1. The over-defensive-gating bypass — when promptIsSelfSufficient
//     returns true, we run this BEFORE the brain so a fully-specified
//     prompt never gets a "configure business profile" ask-back.
//     reasonCode = ReasonSelfSufficient.
//  2. The legacy late-path — after brain decides not to handle and the
//     preflights pass. Behaviour-identical to the inline code it replaced.
//     reasonCode = ReasonDeterministicMatched.
//
// accounts is consumed only to pick a ready account when selectedAccountID
// is 0 AND the matched action requires a browser — same picker the legacy
// late-path used.
func (a *Agent) runDeterministicFastPath(prompt, source string, orgID, selectedAccountID, userID int64, accounts []models.Account, reasonCode string) (string, bool) {
	action, args, ok := deterministicFacebookAction(prompt, orgID, selectedAccountID)
	if !ok || a.ActionHandler == nil {
		return "", false
	}
	// Pick an account when the action requires a browser and the caller
	// didn't already select one. Mirrors the brain path's behaviour.
	if selectedAccountID == 0 && brainToolNeedsAccount(action) {
		if picked := pickReadyFacebookAccountID(accounts); picked > 0 {
			selectedAccountID = picked
			args["account_id"] = picked
		}
	}
	if outboundToolUsesPolicy(action) && a.shouldAutoOutbound(prompt, orgID) {
		args["auto"] = true
	}
	args["user_prompt"] = prompt
	fnResult, err := a.ActionHandler(action, args)
	success := err == nil
	raw := fmt.Sprintf("✅ `%s` → %s", action, fnResult)
	if err != nil {
		raw = fmt.Sprintf("❌ Lỗi %s: %v", action, err)
	}
	responseText := polishActionResponse(action, raw, prompt)
	actionArgs := mustJSON(args)
	a.logPrompt(orgID, selectedAccountID, userID, source, prompt, responseText, action, actionArgs, success,
		NewDeterministicDecision(prompt, action, reasonCode))
	if success {
		a.updateMemory(prompt, action, actionArgs)
		if action == "scrape_group" {
			_ = a.db.Leads().SetContext("last_search_intent", prompt)
		}
	}
	return responseText, true
}

// ProcessPrompt takes a user prompt, sends it to OpenAI with function definitions,
// and executes the appropriate action. Returns the AI response text.
func (a *Agent) ProcessPrompt(ctx context.Context, prompt, source string) (string, error) {
	return a.ProcessPromptForOrg(ctx, prompt, source, 0)
}

// ProcessPromptForOrg runs a prompt with tenant-scoped business context and
// injects org_id into production tool calls. UserID / Role default to zero
// (unauthenticated / Telegram path). Dashboard callers should use
// ProcessPromptForOrgWithUser so skill executors can enforce account
// ownership (RBAC-1 skill-path enforcement).
func (a *Agent) ProcessPromptForOrg(ctx context.Context, prompt, source string, orgID int64) (string, error) {
	return a.ProcessPromptForOrgWithUser(ctx, prompt, source, orgID, 0, 0, "")
}

// ProcessPromptForOrgWithAccount is the legacy entry-point with a selected
// account but no user context. Kept for back-compat; new callers should use
// ProcessPromptForOrgWithUser so RBAC-1 skill-path enforcement applies.
func (a *Agent) ProcessPromptForOrgWithAccount(ctx context.Context, prompt, source string, orgID int64, selectedAccountID int64) (string, error) {
	return a.ProcessPromptForOrgWithUser(ctx, prompt, source, orgID, selectedAccountID, 0, "")
}

// ProcessPromptForOrgWithUser is the canonical entry-point. It threads the
// caller's identity (userID + role) into skill-execution args so skill
// handlers can enforce execution-layer ownership. Sales staff cannot queue
// outbound through accounts they do not own; admin / platform passes.
func (a *Agent) ProcessPromptForOrgWithUser(ctx context.Context, prompt, source string, orgID int64, selectedAccountID int64, userID int64, role string) (string, error) {
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
			if v, err := a.db.Leads().GetContext(fmt.Sprintf("org:%d:%s", orgID, key)); err == nil && strings.TrimSpace(v) != "" {
				userContext["org_"+key] = strings.TrimSpace(v)
				userContext[key] = strings.TrimSpace(v)
			}
		}
		if userContext["org_business_profile"] != "" {
			userContext["business_desc"] = userContext["org_business_profile"]
		}
		a.captureBusinessCalibrationFromPrompt(orgID, userContext, prompt)
	}
	// Derive ephemeral crawl targeting (target_role / signals) from this prompt
	// so brain.py builds a SignalGate from the user's actual ask, not just the
	// stored profile. Empty inferred values do not overwrite existing profile data.
	if requiresFacebookBrowser(prompt) {
		mergeEphemeralCrawlTargeting(userContext, prompt)
	}
	// Load accounts for AI account mapping
	accounts, _ := a.db.Identities().GetAllAccounts(orgID)

	// Over-defensive-gating bug fix: when the prompt is self-sufficient
	// (URL + crawl verb + max_items OR inferred target signals), bypass the
	// brain planner entirely and go straight to deterministic dispatch.
	// The brain is for AMBIGUOUS prompts; using it on fully-specified ones
	// produces "please position your business" ask-backs that the user
	// already answered in the prompt itself. See promptIsSelfSufficient.
	if promptIsSelfSufficient(prompt) {
		if response, handled := a.runDeterministicFastPath(prompt, source, orgID, selectedAccountID, userID, accounts, ReasonSelfSufficient); handled {
			return response, nil
		}
	}

	// Outbound-on-stored-leads bypass: comment_all_leads / inbox_all_leads
	// already have deterministic patterns and do not depend on a positioning
	// re-ask. brain.py's "needs context" gate false-positives on the bare
	// word "lead", so routing these through the planner can ask the user to
	// position their business even when they have qualified leads ready to
	// act on. Skip the brain when the prompt clearly says act-on-leads.
	if promptIsLeadActionSelfSufficient(prompt) {
		if response, handled := a.runDeterministicFastPath(prompt, source, orgID, selectedAccountID, userID, accounts, ReasonSelfSufficientLeadAction); handled {
			return response, nil
		}
	}

	if response, handled := a.processBrainPlan(ctx, prompt, source, orgID, selectedAccountID, userID, userContext, accounts); handled {
		return response, nil
	}
	if ok, msg := facebookScopePreflight(prompt); !ok {
		a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "facebook_scope_guard", "", true, NewScopeGuardDecision(msg))
		return msg, nil
	}
	if requiresFacebookBrowser(prompt) {
		if ok, msg := businessCalibrationPreflight(userContext, prompt); !ok {
			a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "business_preflight", "", false,
				NewPreflightDecision(ReasonBusinessPreflightBlocked, msg))
			return msg, nil
		}
		if ok, msg := facebookBrowserPreflight(accounts, selectedAccountID); !ok {
			a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "browser_preflight", "", false,
				NewPreflightDecision(ReasonBrowserPreflightBlocked, msg))
			return msg, nil
		}
		if selectedAccountID <= 0 {
			selectedAccountID = pickReadyFacebookAccountID(accounts)
		}
	}
	if response, handled := a.runDeterministicFastPath(prompt, source, orgID, selectedAccountID, userID, accounts, ReasonDeterministicMatched); handled {
		return response, nil
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

	// Build tool list. Phase 6: when the open-prompt skill registry is
	// wired, the LLM only sees skills the org has enabled — this is the
	// per-tenant catalog filter that prevents an HR-only org from
	// seeing POD-only tooling, etc. When the registry is absent (legacy
	// boot path / tests) we fall back to the static tool list so
	// behaviour is unchanged.
	var tools []map[string]any
	if a.registry != nil {
		tools = skills.OpenAITools(a.registry.EnabledFor(ctx, a.db, orgID))
	}
	if len(tools) == 0 {
		tools = productionAgentTools()
	}

	// Call OpenAI with function definitions
	body := map[string]any{
		"model":       a.model,
		"messages":    messages,
		"tools":       tools,
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
			// LLM tool calls sometimes omit url/post_url even when the user
			// provided a Facebook URL inline. Without this rescue, the crawl
			// dispatch failed silently and earlier code paths fell back to
			// the newsfeed. Inject from the prompt regex so the action
			// handler receives a concrete target.
			if fnName == "scrape_group" && argStringFromMap(args, "url") == "" {
				if u := firstFacebookURL(prompt); u != "" {
					args["url"] = u
				}
			}
			if fnName == "scrape_comments" && argStringFromMap(args, "post_url") == "" {
				if u := firstFacebookURL(prompt); u != "" {
					args["post_url"] = u
				}
			}
			if isCrawlerTool(fnName) && argStringFromMap(args, "keywords") == "" {
				if kw := promptKeywords(prompt); kw != "" {
					args["keywords"] = kw
				}
			}
			if a.shouldAutoOutbound(prompt, orgID) {
				args["auto"] = true
			}

			// Execute through the skill registry when wired (Phase 6)
			// — the registry handles per-org enablement, typed
			// validation, and audit logging in skill_executions. Fall
			// back to the legacy ActionHandler for skills that have
			// not been registered yet (e.g. tests, partial boot).
			fnResult, err := a.dispatchToolCall(ctx, fnName, args, orgID, selectedAccountID, userID, role, source, prompt)
			if err != nil {
				allResults = append(allResults, fmt.Sprintf("❌ Lỗi %s: %v", fnName, err))
			} else {
				allResults = append(allResults, fmt.Sprintf("✅ `%s` → %s", fnName, fnResult))
				success = true
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
			_ = a.db.Leads().SetContext("last_search_intent", prompt)
			log.Printf("[Agent] Saved search intent: %s", prompt)
		}
	} else {
		responseText = choice.Message.Content
		actionTaken = "chat"
		success = true
		if requiresFacebookBrowser(prompt) {
			responseText = facebookActionNotExecutedMessage()
			actionTaken = "action_not_executed"
			success = false
		}
		// Always try to learn business context from conversational prompts
		a.learnFromPrompt(prompt)
	}

	// Log prompt for learning. LLM fallback path — the OpenAI tool-call
	// resolved (or didn't), so this is the catch-all routing surface.
	a.logPrompt(orgID, selectedAccountID, userID, source, prompt, responseText, actionTaken, actionArgs, success,
		NewLLMFallbackDecision(actionTaken, "OpenAI tool-call fallback"))

	// Update memory for learning
	if success && actionTaken != "chat" {
		a.updateMemory(prompt, actionTaken, actionArgs)
	}

	return responseText, nil
}
