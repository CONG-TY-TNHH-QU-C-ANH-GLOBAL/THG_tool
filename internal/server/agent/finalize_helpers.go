package agent

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/telegram/control"
)

// Supporting helpers for outbound finalization: the terminal HTTP response,
// the operator notifications (global-admin notifier + per-org Telegram channel),
// and the evidence/proof adapters (failure screenshot persistence + proof→evidence
// translation). Pure/stateless helpers around the core state machine
// (finalize_outbound.go) and the FIRST-WIN writers (finalize_side_effects.go).

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
	case "group_post", "profile_post", "post":
		// profile_post is the same "post" action class as group_post (queueProfilePost
		// stores Type="profile_post"); it must surface the same operator channel events.
		// Omitting it silently dropped all profile-post success/failure notifications.
		if success {
			return "post_submitted"
		}
		return "post_failed"
	}
	return ""
}

// maxEvidenceScreenshotBytes bounds a decoded evidence screenshot before it
// touches disk. A q40 JPEG of a 1080p tab is ~80–160 KB; 1 MB is a generous
// ceiling that still rejects an extension shipping a runaway payload.
const maxEvidenceScreenshotBytes = 1 << 20 // 1 MiB

// persistEvidenceScreenshot decodes the extension's out-of-band base64 JPEG to
// an ORG-SCOPED file under data/evidence/<orgID>/ and returns the relative path
// to record in NavDiagnostic.ScreenshotPath. The bytes never enter evidence_json.
//
// Tenant safety: every path component is server-derived (orgID, outboundID,
// attemptID are internal ids issued in org-scoped txs) — no extension-supplied
// string reaches the filename, so this cannot be steered out of the org dir.
// Best-effort: any failure returns "" with a logged warning; evidence capture
// must never block the terminal callback.
func persistEvidenceScreenshot(orgID, outboundID, attemptID int64, b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	// Accept a data: URL prefix or raw base64.
	if i := strings.Index(b64, ","); strings.HasPrefix(b64, "data:") && i >= 0 {
		b64 = b64[i+1:]
	}
	if b64 == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("decode evidence screenshot: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxEvidenceScreenshotBytes {
		return "", fmt.Errorf("evidence screenshot size out of bounds: %d bytes", len(raw))
	}
	dir := filepath.Join("data", "evidence", strconv.FormatInt(orgID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir evidence dir: %w", err)
	}
	name := fmt.Sprintf("ob%d-att%d-%d.jpg", outboundID, attemptID, time.Now().UTC().Unix())
	rel := filepath.ToSlash(filepath.Join(dir, name))
	if err := os.WriteFile(filepath.FromSlash(rel), raw, 0o644); err != nil {
		return "", fmt.Errorf("write evidence screenshot: %w", err)
	}
	return rel, nil
}

// proofToEvidence adapts the runtime verifier's proof shape onto the
// coordination domain's evidence shape. Two types exist (instead of
// one shared) to avoid an import cycle: runtime cannot import store,
// and coordination cannot import runtime. The fields are 1:1 today;
// if they diverge, this is the seam to translate.
func proofToEvidence(p runtime.VerifierProof) coordination.VerificationEvidence {
	return coordination.VerificationEvidence{
		CommentPermalink: p.CommentPermalink,
		MessageBubbleID:  p.MessageBubbleID,
		DOMSnippet:       p.DOMSnippet,
		PageURLAfter:     p.PageURLAfter,
		ObservedAt:       p.ObservedAt,
		Notes:            p.Notes,
		NavDiagnostic:    p.NavDiagnostic, // PR8A: structured landing telemetry → evidence_json
	}
}
