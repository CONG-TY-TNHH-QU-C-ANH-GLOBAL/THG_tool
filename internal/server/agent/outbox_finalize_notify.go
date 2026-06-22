package agent

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/telegram/control"
)

// Terminal output for outbound finalization: build the /sent|/failed HTTP
// response and dispatch the operator notifications (global-admin notifier +
// per-org Telegram channel). Best-effort emission only.

// buildResponse is the FIRST-WIN terminal response: the (state, outcome) pair the
// dashboard consumes, plus the attempt id (0 when BeginExecutionAttempt failed).
func (f *outboundFinalizer) buildResponse() *finalizeResolution {
	return &finalizeResolution{
		HTTPStatus: 200,
		Body: fiber.Map{
			"execution_state":      string(f.terminalState),
			"verification_outcome": string(f.outcome),
			"attempt_id":           f.attemptID,
		},
	}
}

// notifyOutboundFinalized emits the global-admin notifier event (verified vs
// detailed-failure fork) and the per-org Telegram channel event (when tgEvents is
// configured). Best-effort; extracted verbatim.
func (f *outboundFinalizer) notifyOutboundFinalized() {
	if models.IsVerifiedSuccess(f.terminalState, f.terminalOutcome) {
		system.NotifyOutboundStatus(f.h.db, f.h.notifier, f.orgID, f.id, f.terminalState, f.terminalOutcome)
	} else {
		// Surface the extension's GRANULAR diagnostic note directly in the
		// operator's chat (see notificationDetail for the overwrite-free contract).
		system.NotifyOutboundStatusDetail(f.h.db, f.h.notifier, f.orgID, f.id, f.terminalState, f.terminalOutcome, notificationDetail(f.proof, f.report, f.outcome))
	}

	// Per-ORG Telegram CHANNEL notification (distinct from the global-admin notifier
	// above): uses the org's own bot + channel destinations. Best-effort.
	if f.h.tgEvents != nil {
		verified := models.IsVerifiedSuccess(f.terminalState, f.terminalOutcome)
		if ev := agentEventType(f.msg.Type, verified, models.IsSuccessOutcome(f.outcome)); ev != "" {
			channel := strings.ToLower(string(f.msg.Platform))
			if channel == "" {
				channel = "facebook"
			}
			f.h.tgEvents.NotifyAction(control.ActionNotice{
				OrgID: f.orgID, OutboundID: f.id, EventType: ev, Channel: channel,
				Workspace:   f.h.orgName(f.orgID),
				Agent:       f.h.agentName(f.msg.AccountID),
				Author:      f.msg.TargetName,
				CommentText: f.msg.Content,
				PostURL:     f.msg.TargetURL,
				Reason:      failureReasonText(f.report, string(f.outcome)),
				BaseURL:     f.h.baseURL,
			})
		}
	}
}

// notificationDetail selects the most diagnostic string to surface in the
// operator's failure notification (the `Chi tiet:` line in chat/Telegram).
//
// proof.Notes carries the extension's GRANULAR path/gate prefix
// (path2.article_not_found_in_feed, path2.group_home_nav_failed,
// outbox.crawler_nav_failed, …) plus landed_url — enough to bucket a failure
// from a single run. We prefer it so the failure is self-explanatory in chat
// without opening the diagnostic endpoint. The data path that feeds it is
// overwrite-free: ClassifyExtensionReport copies report.Notes verbatim (it
// only fills Notes on success-WITHOUT-proof, never on a failure), and
// EnforceTargetIdentity only APPENDS (" · entity-drift …"), never replaces.
//
// report.FailureReason then string(outcome) are last-resort fallbacks only —
// coarse labels that cannot distinguish path from gate (redirected_feed vs
// context_drift), so they are used solely when no note survived.
func notificationDetail(proof runtime.VerifierProof, report runtime.ExtensionExecutionReport, outcome models.ExecutionOutcome) string {
	if d := strings.TrimSpace(proof.Notes); d != "" {
		return d
	}
	if d := strings.TrimSpace(report.FailureReason); d != "" {
		return d
	}
	return string(outcome)
}

// agentEventType maps an outbound action type + outcome to a Telegram destination event key.
// "" means "no notification for this action type". comment submitted-but-not-verified is
// comment_unverified (pending manual verification).
func agentEventType(actionType string, verified, success bool) string {
	switch actionType {
	case "comment":
		if verified {
			return "comment_verified"
		}
		if success {
			return "comment_unverified"
		}
		return "comment_failed"
	case "inbox":
		if success {
			return "inbox_sent"
		}
		return "inbox_failed"
	case "group_post", "post":
		if success {
			return "post_submitted"
		}
		return "post_failed"
	}
	return ""
}
