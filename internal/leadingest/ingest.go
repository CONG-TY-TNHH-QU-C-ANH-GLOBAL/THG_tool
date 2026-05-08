// Package leadingest centralizes the post-crawl classify + persist pipeline so
// the worker handler (internal/jobhandlers/facebook_crawl) and the Chrome
// Extension crawl-result endpoint (internal/server/agent/crawl.go) share
// one implementation. Before this package, the connector path only ran
// deterministic scoring; the worker path also ran AI UniversalClassify. That
// drift caused leads from the extension to be silently undersorted.
package leadingest

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
)

// Deps captures the per-run dependencies that the ingest function needs.
// Zero-valued fields are tolerated where indicated; callers that don't have
// AI classification configured may pass AIClass=nil and BusinessProfile=nil.
type Deps struct {
	AppStore        *store.AppStore
	LegacyDB        *store.Store
	Scorer          *scoring.Scorer
	Guidance        scoring.Guidance
	BusinessProfile *ai.BusinessProfile
	AIClass         *ai.MessageGenerator
	SignalGate      SignalGate
	Keywords        []string
	UserPrompt      string   // free-form user prompt that triggered this crawl, used to anchor classifier intent
	ExtraSignals    []string // appended to every Outcome.Signals (e.g. "chrome_extension_crawl")
	ClassifyTimeout time.Duration
}

// SignalGate mirrors brain.MarketSignalGate but lives in this package to avoid
// pulling the full agent package. Empty fields disable each rule.
type SignalGate struct {
	TargetRole      string
	PositiveSignals []string
	NegativeSignals []string
	RejectRules     []string
	MinConfidence   float64
}

// Input is one crawled post.
type Input struct {
	TaskID           string
	OrgID            int64
	SourceURL        string
	AuthorName       string
	AuthorProfileURL string
	Content          string
	Reactions        int
	Comments         int
	Shares           int
}

// Outcome describes what happened to one input.
type Outcome struct {
	Inserted bool
	Skipped  string // "" | "filter" | "cold" | "rejected" | "gate_negative" | "gate_low_confidence"
	Score    float64
	Category string
	Signals  []string
	AIReason string
	AIIntent string
}

// IngestPost classifies a single crawled post and, when qualified, persists
// it to both task_leads and the legacy leads table. Mirroring is unconditional
// so dashboard stats (which read from `leads`) always reflect connector output
// — the legacy table is the canonical view.
func IngestPost(ctx context.Context, deps Deps, in Input) (Outcome, error) {
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return Outcome{Skipped: "filter"}, nil
	}

	scorer := deps.Scorer
	if scorer == nil {
		scorer = scoring.New(scoring.DefaultConfig())
	}
	sr := scorer.ScoreWithGuidance(content, deps.Keywords, in.Reactions, in.Comments, in.AuthorProfileURL, deps.Guidance)
	signals := append([]string{}, sr.Signals...)
	if len(deps.ExtraSignals) > 0 {
		signals = append(signals, deps.ExtraSignals...)
	}
	out := Outcome{
		Score:    sr.Score,
		Category: sr.Category,
		Signals:  signals,
	}
	if sr.Category == "rejected" {
		out.Skipped = "rejected"
		return out, nil
	}

	// Brain-derived market_signal_gate: hard reject when any negative phrase
	// matches; honor positive phrases as a confidence floor.
	if hit := matchAny(content, deps.SignalGate.RejectRules); hit != "" {
		out.Skipped = "gate_negative"
		out.Category = "rejected"
		out.Signals = append(out.Signals, "gate_reject:"+hit)
		return out, nil
	}
	if hit := matchAny(content, deps.SignalGate.NegativeSignals); hit != "" {
		out.Skipped = "gate_negative"
		out.Category = "rejected"
		out.Signals = append(out.Signals, "gate_negative:"+hit)
		return out, nil
	}

	// AI classifier overrides deterministic when configured. Failures fall
	// back to deterministic so a flaky LLM never blocks lead capture.
	if deps.BusinessProfile != nil && deps.BusinessProfile.IsConfigured() && deps.AIClass != nil && deps.AIClass.Available() {
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
		// Brain sidecar (Python) is responsible for filling SignalGate.TargetRole,
		// but it can be offline or produce no inference. Fall back to a Go-side
		// keyword inference so the classifier still gets a scope hint instead of
		// treating every adjacent post as a generic match.
		if intent.TargetRole == "" {
			intent.TargetRole = ai.InferTargetRoleFromPrompt(deps.UserPrompt)
		}
		aiResult, err := deps.AIClass.UniversalClassify(classifyCtx, content, in.AuthorName, deps.BusinessProfile, intent)
		cancel()
		if err != nil {
			slog.WarnContext(ctx, "universal classify failed; using deterministic score",
				"task_id", in.TaskID, "org_id", in.OrgID, "error", err)
		} else if aiResult != nil {
			// Hard-guard against LLM assigning hot/warm priority to off-target intents
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

			if aiResult.Priority == "rejected" || aiResult.Intent == "not_relevant" || aiResult.Intent == "spam" || aiResult.Intent == "provider_ad" {
				out.Skipped = "rejected"
				out.Category = "rejected"
				out.Signals = append(out.Signals, "ai_intent:"+aiResult.Intent, "ai_reason:"+aiResult.Reason)
				return out, nil
			}
			out.Score = aiResult.Score * 100
			out.Category = aiResult.Priority
			out.AIIntent = aiResult.Intent
			out.AIReason = aiResult.Reason
			out.Signals = append(out.Signals,
				"ai_intent:"+aiResult.Intent,
				"ai_reason:"+aiResult.Reason,
			)
			if out.Category == "" {
				out.Category = "cold"
			}
		}
	}

	if out.Category == "cold" {
		out.Skipped = "cold"
		return out, nil
	}

	if deps.AppStore != nil {
		taskLead := store.TaskLead{
			TaskID:           in.TaskID,
			OrgID:            in.OrgID,
			SourceURL:        in.SourceURL,
			AuthorProfileURL: in.AuthorProfileURL,
			AuthorName:       in.AuthorName,
			Content:          content,
			LeadScore:        out.Score,
			Category:         out.Category,
			Signals:          out.Signals,
		}
		if err := deps.AppStore.InsertLead(ctx, in.TaskID, in.OrgID, taskLead); err != nil {
			return out, err
		}
	}
	if deps.LegacyDB != nil {
		// AuthorRole carries the AI classifier intent (candidate / potential_customer
		// / partner / provider_ad / not_relevant / spam) so the dashboard can render
		// a meaningful tag per lead instead of a generic "AI classifier" string.
		authorRole := strings.TrimSpace(out.AIIntent)
		if authorRole == "" {
			authorRole = "unknown"
		}
		// Niche prefers a clean domain label (industry from profile) over the raw
		// crawl keywords. Keywords are kept as fallback when no industry is set.
		niche := ""
		if deps.BusinessProfile != nil {
			niche = strings.TrimSpace(deps.BusinessProfile.Industry)
			if niche == "" {
				niche = strings.TrimSpace(deps.BusinessProfile.Name)
			}
		}
		if niche == "" {
			niche = strings.Join(deps.Keywords, ", ")
		}
		// PainPoint is the human-readable AI reason ("Author is asking for a POD
		// supplier from VN to ship to US"); fall back to signals only if reason
		// is missing. The dashboard shows this as the agent note.
		painPoint := strings.TrimSpace(out.AIReason)
		if painPoint == "" {
			painPoint = strings.Join(out.Signals, "; ")
		}
		legacy := &models.Lead{
			OrgID:        in.OrgID,
			SourceType:   "post",
			SourceID:     0,
			SourceURL:    in.SourceURL,
			Platform:     models.PlatformFacebook,
			Author:       in.AuthorName,
			AuthorURL:    in.AuthorProfileURL,
			Content:      content,
			Score:        models.LeadScore(out.Category),
			ServiceMatch: out.Category,
			AuthorRole:   authorRole,
			PainPoint:    painPoint,
			AIReasoning:  textutil.FirstNonEmpty(out.AIReason, strings.Join(out.Signals, "; ")),
			Niche:        niche,
			ClassifiedAt: time.Now().UTC(),
		}
		if _, err := deps.LegacyDB.InsertLead(legacy); err != nil {
			// Non-fatal: task_leads is the source of truth; legacy mirror is
			// best-effort for the existing dashboard.
			slog.WarnContext(ctx, "legacy lead mirror failed",
				"task_id", in.TaskID, "org_id", in.OrgID, "error", err)
		}
	}
	out.Inserted = true
	return out, nil
}

func matchAny(content string, phrases []string) string {
	if len(phrases) == 0 {
		return ""
	}
	lower := strings.ToLower(content)
	for _, phrase := range phrases {
		p := strings.ToLower(strings.TrimSpace(phrase))
		if p == "" {
			continue
		}
		if strings.Contains(lower, p) {
			return p
		}
	}
	return ""
}

// SignalGateFromMap unpacks the JSON-decoded gate map (envelope/payload) into
// the typed SignalGate. Missing fields are tolerated.
func SignalGateFromMap(raw map[string]any) SignalGate {
	if raw == nil {
		return SignalGate{}
	}
	out := SignalGate{
		TargetRole:    str(raw["target_role"]),
		MinConfidence: f64(raw["min_confidence"]),
	}
	out.PositiveSignals = strSlice(raw["positive_signals"])
	out.NegativeSignals = strSlice(raw["negative_signals"])
	out.RejectRules = strSlice(raw["reject_rules"])
	return out
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func f64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}

func strSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := str(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
