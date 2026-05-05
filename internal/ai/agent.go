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
func (a *Agent) dispatchToolCall(ctx context.Context, fnName string, args map[string]any, orgID, accountID int64, source, prompt string) (string, error) {
	if a.registry != nil {
		if skill := a.registry.Get(fnName); skill != nil {
			env := skills.Env{
				DB:        a.db,
				OrgID:     orgID,
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
		a.captureBusinessCalibrationFromPrompt(orgID, userContext, prompt)
	}
	// Load accounts for AI account mapping
	accounts, _ := a.db.GetAllAccounts(orgID)
	if response, handled := a.processBrainPlan(ctx, prompt, source, orgID, selectedAccountID, userContext, accounts); handled {
		return response, nil
	}
	if ok, msg := facebookScopePreflight(prompt); !ok {
		a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "facebook_scope_guard", "", true)
		return msg, nil
	}
	if requiresFacebookBrowser(prompt) {
		if ok, msg := businessCalibrationPreflight(userContext, prompt); !ok {
			a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "business_preflight", "", false)
			return msg, nil
		}
		if ok, msg := facebookBrowserPreflight(accounts, selectedAccountID); !ok {
			a.logPrompt(orgID, selectedAccountID, source, prompt, msg, "browser_preflight", "", false)
			return msg, nil
		}
		if selectedAccountID <= 0 {
			selectedAccountID = pickReadyFacebookAccountID(accounts)
		}
	}
	if action, args, ok := deterministicFacebookAction(prompt, orgID, selectedAccountID); ok && a.ActionHandler != nil {
		args["user_prompt"] = prompt
		fnResult, err := a.ActionHandler(action, args)
		success := err == nil
		raw := fmt.Sprintf("✅ `%s` → %s", action, fnResult)
		if err != nil {
			raw = fmt.Sprintf("❌ Lỗi %s: %v", action, err)
		}
		responseText := polishActionResponse(action, raw, prompt)
		actionArgs := mustJSON(args)
		a.logPrompt(orgID, selectedAccountID, source, prompt, responseText, action, actionArgs, success)
		if success {
			a.updateMemory(prompt, action, actionArgs)
			if action == "scrape_group" {
				_ = a.db.SetContext("last_search_intent", prompt)
			}
		}
		return responseText, nil
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
			if isCrawlerTool(fnName) && argStringFromMap(args, "keywords") == "" {
				if kw := promptKeywords(prompt); kw != "" {
					args["keywords"] = kw
				}
			}
			if wantsAutoOutbound(prompt) {
				args["auto"] = true
			}

			// Execute through the skill registry when wired (Phase 6)
			// — the registry handles per-org enablement, typed
			// validation, and audit logging in skill_executions. Fall
			// back to the legacy ActionHandler for skills that have
			// not been registered yet (e.g. tests, partial boot).
			fnResult, err := a.dispatchToolCall(ctx, fnName, args, orgID, selectedAccountID, source, prompt)
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
			_ = a.db.SetContext("last_search_intent", prompt)
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

	// Log prompt for learning
	a.logPrompt(orgID, selectedAccountID, source, prompt, responseText, actionTaken, actionArgs, success)

	// Update memory for learning
	if success && actionTaken != "chat" {
		a.updateMemory(prompt, actionTaken, actionArgs)
	}

	return responseText, nil
}
