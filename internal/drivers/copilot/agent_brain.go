package copilot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/textutil"
)

// processBrainPlan is the Agent-Brain orchestrator: it builds the planner
// request, calls the sidecar, validates the returned plan, runs the
// readiness/scope preflights, executes the validated actions, and logs the
// outcome. The collaborators live in sibling files:
//   - brain_client.go         — HTTP transport (BrainClient.Plan)
//   - brain_types.go          — Brain* wire DTOs
//   - brain_plan_validator.go — validateBrainPlan / validateBrainAction
//   - brain_action_prep.go    — prepareBrainActionArgs + tool metadata
func (a *Agent) processBrainPlan(ctx context.Context, prompt, source string, orgID, selectedAccountID, userID int64, userContext map[string]string, accounts []models.Account) (string, bool) {
	if a == nil || a.brain == nil || !a.brain.Available() {
		return "", false
	}
	if ok, msg := facebookScopePreflight(prompt); !ok {
		a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "facebook_scope_guard", "", true, NewScopeGuardDecision(msg))
		return msg, true
	}

	profile := ai.ProfileFromContext(userContext)
	req := BrainPlanRequest{
		OrgID:             orgID,
		Source:            source,
		Prompt:            prompt,
		BusinessProfile:   profile,
		SelectedAccountID: selectedAccountID,
		Accounts:          brainAccounts(accounts),
		DataSummaries: BrainDataSummaries{
			PrivateFilesSummary: textutil.FirstNonEmpty(userContext["org_private_files_summary"], userContext["private_files_summary"]),
			DataSourcesSummary:  textutil.FirstNonEmpty(userContext["org_data_sources_summary"], userContext["data_sources_summary"]),
		},
		ToolCapabilities: brainToolCapabilities(),
		Policy: BrainPolicy{
			DefaultOutboundMode: textutil.FirstNonEmpty(userContext["org_outbound_mode"], userContext["outbound_mode"], "draft"),
			MaxItemsCap:         brainDefaultCap,
			BrowserRequiredFor:  brainBrowserTools(),
		},
	}

	plan, err := a.brain.Plan(ctx, req)
	if err != nil {
		log.Printf("[AgentBrain] planner unavailable, falling back to legacy router: %v", err)
		return "", false
	}
	if err := validateBrainPlan(plan); err != nil {
		msg := "Agent Brain returned an unsafe or invalid plan, so no automation was executed.\n\nDetail: " + err.Error()
		a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "brain_invalid_plan", mustJSON(plan), false,
			RoutingDecision{Route: RouteBrain, ReasonCode: ReasonBrainInvalidPlan, Reason: err.Error()})
		return msg, true
	}

	if strings.EqualFold(plan.DomainScope, "out_of_scope") || strings.EqualFold(plan.Decision, "refuse") {
		msg := strings.TrimSpace(plan.ResponseSummary)
		if msg == "" {
			msg = facebookScopeGuardMessage()
		}
		a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "brain_refuse", mustJSON(plan), true,
			NewBrainDecision("refuse", msg))
		return msg, true
	}

	if !strings.EqualFold(plan.Decision, "execute") || len(plan.Actions) == 0 {
		msg := strings.TrimSpace(plan.ResponseSummary)
		if msg == "" {
			msg = facebookActionNotExecutedMessage()
		}
		// Brain returned ask_user/chat — capture the missing signals so
		// the dashboard "Ambiguous Prompt Surface" panel can show what
		// users keep forgetting to specify.
		dec := NewBrainDecision(plan.Decision, msg)
		dec.MissingSignals = analyseMissingSignals(prompt)
		dec.InferredSignals = inferredSignalsFromPrompt(prompt)
		a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "brain_"+strings.ToLower(plan.Decision), mustJSON(plan), true, dec)
		if strings.EqualFold(plan.Decision, "chat") {
			a.learnFromPrompt(prompt)
		}
		return msg, true
	}

	if actionPlanNeedsProfile(plan) {
		if ok, msg := businessCalibrationPreflight(userContext, prompt); !ok {
			a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "brain_business_preflight", mustJSON(plan), false,
				NewPreflightDecision(ReasonBusinessPreflightBlocked, msg))
			return msg, true
		}
	}
	if actionPlanNeedsBrowser(plan) {
		if ok, msg := facebookBrowserPreflight(accounts, selectedAccountID); !ok {
			a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "brain_browser_preflight", mustJSON(plan), false,
				NewPreflightDecision(ReasonBrowserPreflightBlocked, msg))
			return msg, true
		}
		if selectedAccountID <= 0 {
			selectedAccountID = pickReadyFacebookAccountID(accounts)
		}
	}
	if a.ActionHandler == nil {
		msg := "Action handler is not configured; Agent Brain plan was validated but not executed."
		a.logPrompt(orgID, selectedAccountID, userID, source, prompt, msg, "brain_no_action_handler", mustJSON(plan), false,
			RoutingDecision{Route: RouteBrain, ReasonCode: ReasonBrainNoHandler, Reason: msg})
		return msg, true
	}

	var results []string
	var firstAction, firstArgs string
	success := false
	for _, action := range plan.Actions {
		args := prepareBrainActionArgs(action, plan.MarketSignalGate, prompt, orgID, selectedAccountID)
		// Upgrade auto when the org's outbound_mode policy is set to auto.
		// prepareBrainActionArgs only inspects the prompt text; the store
		// layer is the final guard that downgrades when the org is not opted in.
		if brainToolIsOutbound(action.Tool) && a.shouldAutoOutbound(prompt, orgID) {
			args["auto"] = true
		}
		fnResult, err := a.ActionHandler(action.Tool, args)
		if firstAction == "" {
			firstAction = action.Tool
			firstArgs = mustJSON(args)
		}
		if err != nil {
			results = append(results, fmt.Sprintf("ERROR `%s` -> %v", action.Tool, err))
			continue
		}
		success = true
		results = append(results, fmt.Sprintf("OK `%s` -> %s", action.Tool, fnResult))
	}

	raw := strings.Join(results, "\n\n")
	responseText := raw
	if firstAction != "" {
		responseText = polishActionResponse(firstAction, raw, prompt)
	}
	if !success && strings.TrimSpace(responseText) == "" {
		responseText = "Agent Brain plan was validated, but no action completed successfully."
	}
	a.logPrompt(orgID, selectedAccountID, userID, source, prompt, responseText, "brain_"+firstAction, firstArgs, success,
		NewBrainDecision("execute", "brain plan executed → "+firstAction))
	if success && firstAction != "" {
		a.updateMemory(prompt, firstAction, firstArgs)
		if isCrawlerTool(firstAction) {
			_ = a.db.Leads().SetContext("last_search_intent", prompt)
		}
	}
	return responseText, true
}
