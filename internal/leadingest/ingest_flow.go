package leadingest

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/store/leads"
)

// Classification phase for IngestPost — behaviour-preserving extraction of the score →
// market-gate → AI → cold-gate state machine (specs/domains/facebook-sales-intelligence/features/lead-ingestion/technical.md §6/§9).

// classifyLead runs the full classification state machine and returns the Outcome plus
// whether the lead should be SKIPPED. Each veto short-circuits with skip=true UNLESS
// Deps.ForceLead downgrades it to annotation (overrideVeto).
func classifyLead(ctx context.Context, deps Deps, in Input, content, urlSignal string) (Outcome, bool) {
	out := scoreLead(deps, in, content, urlSignal)

	if out.Category == "rejected" {
		out.Skipped = "rejected"
		if !deps.overrideVeto(&out, "deterministic_rejected") {
			return out, true
		}
	}
	if applyMarketGate(deps, content, &out) {
		return out, true
	}
	if runAIClassifier(ctx, deps, in, content, &out) {
		return out, true
	}
	if out.Category == "cold" {
		out.Skipped = "cold"
		if !deps.overrideVeto(&out, "cold") {
			return out, true
		}
	}
	return out, false
}

// scoreLead runs the deterministic scorer (defaulting when deps.Scorer is nil) and seeds
// the Outcome with score, category, and base signals (scorer + url telemetry + ExtraSignals).
func scoreLead(deps Deps, in Input, content, urlSignal string) Outcome {
	scorer := deps.Scorer
	if scorer == nil {
		scorer = scoring.New(scoring.DefaultConfig())
	}
	sr := scorer.ScoreWithGuidance(content, deps.Keywords, in.Reactions, in.Comments, in.AuthorProfileURL, deps.Guidance)
	signals := append([]string{}, sr.Signals...)
	if urlSignal != "" {
		signals = append(signals, urlSignal)
	}
	if len(deps.ExtraSignals) > 0 {
		signals = append(signals, deps.ExtraSignals...)
	}
	return Outcome{Score: sr.Score, Category: sr.Category, Signals: signals}
}

// applyMarketGate applies the market signal gate: a RejectRules hit emits gate_reject:<p>,
// a NegativeSignals hit emits gate_negative:<p>; both set Skipped="gate_negative"/
// Category="rejected". Returns true when a veto stands; ForceLead downgrades and falls through.
func applyMarketGate(deps Deps, content string, out *Outcome) bool {
	if hit := matchAny(content, deps.SignalGate.RejectRules); hit != "" {
		out.Skipped = "gate_negative"
		out.Category = "rejected"
		out.Signals = append(out.Signals, "gate_reject:"+hit)
		if !deps.overrideVeto(out, "gate_reject:"+hit) {
			return true
		}
	}
	if hit := matchAny(content, deps.SignalGate.NegativeSignals); hit != "" {
		out.Skipped = "gate_negative"
		out.Category = "rejected"
		out.Signals = append(out.Signals, "gate_negative:"+hit)
		if !deps.overrideVeto(out, "gate_negative:"+hit) {
			return true
		}
	}
	return false
}

// runAIClassifier runs the best-effort AI override when configured (spec §6): an error logs
// and falls back to deterministic. Returns true only on hard-reject without ForceLead override.
func runAIClassifier(ctx context.Context, deps Deps, in Input, content string, out *Outcome) bool {
	hasAIContext := (deps.BusinessProfile != nil && deps.BusinessProfile.IsConfigured()) || strings.TrimSpace(deps.UserPrompt) != ""
	if !hasAIContext || deps.AIClass == nil || !deps.AIClass.Available() {
		return false
	}
	timeout := deps.ClassifyTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	classifyCtx, cancel := context.WithTimeout(ctx, timeout)
	intent := ai.ClassifyIntent{
		UserPrompt:      deps.UserPrompt,
		Keywords:        deps.Keywords,
		TargetRole:      deps.SignalGate.TargetRole,
		PositiveSignals: deps.SignalGate.PositiveSignals,
	}
	// Brain sidecar fills SignalGate.TargetRole but can be offline; fall back to Go-side inference.
	if intent.TargetRole == "" {
		intent.TargetRole = ai.InferTargetRoleFromPrompt(deps.UserPrompt)
	}
	aiResult, err := deps.AIClass.UniversalClassify(classifyCtx, content, in.AuthorName, deps.BusinessProfile, intent)
	cancel()

	// Observability: a classification_log row for EVERY outcome (kept/rejected/errored).
	logEntry := leads.ClassificationLogEntry{
		OrgID:          in.OrgID,
		TaskID:         in.TaskID,
		SourceURL:      in.PrimaryURL,
		AuthorName:     in.AuthorName,
		ContentSnippet: content,
		TargetRole:     intent.TargetRole,
		UserPrompt:     deps.UserPrompt,
	}
	if err != nil {
		slog.WarnContext(ctx, "universal classify failed; using deterministic score",
			"task_id", in.TaskID, "org_id", in.OrgID, "error", err)
		logEntry.Decision = leads.ClassificationError
		logEntry.AIReason = err.Error()
		recordClassification(ctx, deps, logEntry)
		return false
	}
	if aiResult == nil {
		return false
	}
	return applyAIVerdict(ctx, deps, aiResult, intent, logEntry, out)
}

// applyTargetRoleGuard hard-rejects an off-target AI verdict: when a target role is set and
// the post intent does not match it, priority is forced to "rejected" (verbatim guard, spec §6).
func applyTargetRoleGuard(intent ai.ClassifyIntent, aiResult *ai.UniversalClassifyResult) {
	if intent.TargetRole != "" && aiResult.Intent != "not_relevant" && aiResult.Intent != "spam" {
		if intent.TargetRole == "potential_customer" && aiResult.Intent != "potential_customer" {
			aiResult.Priority = "rejected"
			aiResult.Reason = "Hard-rejected: target is customer but post is " + aiResult.Intent
		} else if intent.TargetRole == "candidate" && aiResult.Intent != "candidate" {
			aiResult.Priority = "rejected"
			aiResult.Reason = "Hard-rejected: target is candidate but post is " + aiResult.Intent
		} else if intent.TargetRole == "partner" && aiResult.Intent != "partner" {
			aiResult.Priority = "rejected"
			aiResult.Reason = "Hard-rejected: target is partner but post is " + aiResult.Intent
		}
	}
}

// applyAIVerdict maps a non-nil AI result onto the Outcome + classification_log (spec §6).
// Returns true when the verdict hard-rejects AND ForceLead does not override (skip).
func applyAIVerdict(ctx context.Context, deps Deps, aiResult *ai.UniversalClassifyResult, intent ai.ClassifyIntent, logEntry leads.ClassificationLogEntry, out *Outcome) bool {
	// Hard-guard against the LLM assigning hot/warm priority to off-target intents.
	applyTargetRoleGuard(intent, aiResult)

	logEntry.AIIntent = aiResult.Intent
	logEntry.AIPriority = aiResult.Priority
	logEntry.AIReason = aiResult.Reason
	logEntry.AIScore = aiResult.Score

	if aiResult.Priority == "rejected" || aiResult.Intent == "not_relevant" || aiResult.Intent == "spam" || aiResult.Intent == "provider_ad" {
		out.Skipped = "rejected"
		out.Category = "rejected"
		out.Signals = append(out.Signals, "ai_intent:"+aiResult.Intent, "ai_reason:"+aiResult.Reason)
		logEntry.Decision = leads.ClassificationRejected
		recordClassification(ctx, deps, logEntry)
		// Explicit direct-post intake records the verdict but still creates the lead.
		if !deps.overrideVeto(out, "ai_rejected:"+aiResult.Intent) {
			return true
		}
		out.AIIntent = aiResult.Intent
		out.AIReason = aiResult.Reason
		return false
	}

	out.Score = aiResult.Score * 100
	out.Category = aiResult.Priority
	out.AIIntent = aiResult.Intent
	out.AIReason = aiResult.Reason
	out.Signals = append(out.Signals, "ai_intent:"+aiResult.Intent, "ai_reason:"+aiResult.Reason)
	if out.Category == "" {
		out.Category = "cold"
	}
	if out.Category == "cold" {
		logEntry.Decision = leads.ClassificationCold
	} else {
		logEntry.Decision = leads.ClassificationKept
	}
	recordClassification(ctx, deps, logEntry)
	return false
}

// recordClassification persists a classification_log row best-effort (only when a legacy DB
// is wired; the write error is intentionally ignored).
func recordClassification(ctx context.Context, deps Deps, logEntry leads.ClassificationLogEntry) {
	if deps.LegacyDB != nil {
		_ = deps.LegacyDB.Leads().RecordClassification(ctx, logEntry)
	}
}
