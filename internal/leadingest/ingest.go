// Package leadingest centralizes the post-crawl classify + persist pipeline so
// the worker handler (internal/jobhandlers/facebook_crawl) and the Chrome
// Extension crawl-result endpoint (internal/server/agent/crawl.go) share
// one implementation. Before this package, the connector path only ran
// deterministic scoring; the worker path also ran AI UniversalClassify. That
// drift caused leads from the extension to be silently undersorted.
package leadingest

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/fburl"
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
	// IntentID is the recurring crawl intent driving this run (0 for one-shot
	// runs). When > 0 AND LegacyDB is set, IngestPost advances the per-intent
	// cursor after each successful insert. See project_scheduled_intelligence.md.
	IntentID int64
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

// Input is one crawled lead candidate — a post, or a comment that MUST route
// to its parent post. Every lead carries a usable POST url as PrimaryURL;
// the routing contract is enforced by ValidateRouting before persist.
// See project_lead_routing_gap.md.
type Input struct {
	TaskID           string
	OrgID            int64
	SourceType       string // "post" | "comment" — empty defaults to "post"
	PrimaryURL       string // canonical POST url — ALWAYS the post, never a standalone comment
	SecondaryURL     string // optional COMMENT url, set only when SourceType == "comment"
	PostFBID         string // Facebook-side post id (traceability + fallback URL build)
	CommentFBID      string // Facebook-side comment id
	GroupFBID        string // Facebook-side group id
	AuthorName       string
	AuthorProfileURL string
	Content          string
	// PostedAt is the original post timestamp (zero when the crawler does not
	// emit one). Drives the conditional cursor advance — only newer posts move
	// the cursor forward. Zero falls back to last-call-wins.
	PostedAt  time.Time
	Reactions int
	Comments  int
	Shares    int
	// URLRepairPath is the crawler-side telemetry for HOW PrimaryURL was
	// built (anchor_clean | synth_from_fbid | dropped_transient). Surfaces
	// in Outcome.Signals as `url:<path>` and gets upgraded to
	// `repaired_in_pipeline` if repairPrimaryURL mutates the URL further.
	// Phase 1 observability — see project_crawler_trust_phase_plan.md.
	URLRepairPath string
}

// Outcome describes what happened to one input.
type Outcome struct {
	Inserted bool
	Skipped  string // "" | "filter" | "invalid_routing" | "cold" | "rejected" | "gate_negative" | "gate_low_confidence"
	Score    float64
	Category string
	Signals  []string
	AIReason string
	AIIntent string
}

// normalizeSourceType maps a free-form source type onto the two supported
// values. Empty / unknown defaults to "post" — the safe, common case.
func normalizeSourceType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "comment":
		return "comment"
	default:
		return "post"
	}
}

// URL helpers (LooksLikePostURL, looksLikeCommentOnlyURL,
// CanonicalPostPermalink, ExtractFacebookPostID) live in internal/fburl
// so the store-layer read path can use the same canonicalisation
// without an import cycle. The aliases below preserve the existing
// leadingest public surface.

// ExtractFacebookPostID is re-exported from internal/fburl for back-compat.
func ExtractFacebookPostID(u string) string { return fburl.ExtractFacebookPostID(u) }

// LooksLikePostURL is re-exported from internal/fburl for back-compat.
func LooksLikePostURL(u string) bool { return fburl.LooksLikePostURL(u) }

// CanonicalPostPermalink is re-exported from internal/fburl for back-compat.
func CanonicalPostPermalink(groupFBID, postFBID string) string {
	return fburl.CanonicalPostPermalink(groupFBID, postFBID)
}

// looksLikeCommentOnlyURL stays package-private — its callers are
// inside leadingest.ValidateRouting only. Thin delegate.
func looksLikeCommentOnlyURL(u string) bool { return fburl.LooksLikeCommentOnlyURL(u) }

// ValidateRouting enforces the lead routing contract before persist. The rule:
// every lead MUST carry a usable POST url as its primary link. A comment-sourced
// lead whose parent post was never resolved is invalid — dropped, not stored.
// This is enforcement, not inference: the crawler obeys the contract; the
// pipeline rejects what violates it. See project_lead_routing_gap.md.
func ValidateRouting(in Input) error {
	primary := strings.TrimSpace(in.PrimaryURL)
	if primary == "" {
		return errors.New("missing primary (post) URL")
	}
	if looksLikeCommentOnlyURL(primary) {
		return errors.New("primary URL is a comment link, not a post")
	}
	// A URL that points at a group/page/profile shell (no /posts/, no
	// /permalink/, no story_fbid) cannot serve as a post link. The dashboard
	// "Mở bài viết" button on such a URL would land the user on the feed,
	// not the specific post. This is the failure mode the user reported.
	if !LooksLikePostURL(primary) {
		return errors.New("primary URL has no post identifier (group/page shell)")
	}
	if normalizeSourceType(in.SourceType) == "comment" {
		// A comment lead must route to its parent post. If the primary link is
		// identical to the comment link, the parent post was never resolved.
		if primary == strings.TrimSpace(in.SecondaryURL) {
			return errors.New("comment did not resolve to a parent post")
		}
	}
	return nil
}

// repairPrimaryURL is the server-side rescue for crawls where the
// extracted primary URL is a group/page shell but the crawler also
// supplied PostFBID. It synthesises a canonical post permalink and
// rewrites Input.PrimaryURL in place. No-op when:
//   - primary already looks like a post URL
//   - no PostFBID available to synthesise from
//
// Called BEFORE ValidateRouting so a synthesised URL passes the contract
// and the lead is persisted with a valid "Mở bài viết" target.
//
// Returns true when PrimaryURL was actually mutated — used to upgrade the
// URL repair telemetry signal to `repaired_in_pipeline` so the Chrome
// extension ingest path is distinguishable from the crawler-side synth.
func repairPrimaryURL(in *Input) bool {
	if in == nil {
		return false
	}
	primary := strings.TrimSpace(in.PrimaryURL)
	if primary != "" && LooksLikePostURL(primary) {
		return false
	}
	postID := strings.TrimSpace(in.PostFBID)
	if postID == "" {
		// Last-ditch: try to recover an ID from whatever URL we have.
		postID = ExtractFacebookPostID(primary)
		if postID == "" {
			return false
		}
		in.PostFBID = postID
	}
	if synth := CanonicalPostPermalink(in.GroupFBID, postID); synth != "" {
		in.PrimaryURL = synth
		return true
	}
	return false
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

	// Server-side rescue: when the crawler emits a group/page shell URL
	// but ALSO sends PostFBID, synthesise the canonical post permalink
	// before validating. Common case: Facebook lazy-renders the post
	// anchor, the JS falls back to expectedUrl (the group URL), but the
	// post id was still extracted off the page. See project_lead_routing_gap.md.
	pipelineRepaired := repairPrimaryURL(&in)

	// URL repair telemetry — `url:<path>` rides into every Outcome.Signals
	// so Phase 1 dashboards can count anchor_clean vs synth_from_fbid vs
	// dropped_transient. `repaired_in_pipeline` flags the Chrome-extension
	// path where the crawler emitted a shell but we synthesised here.
	urlSignal := buildURLRepairSignal(in.URLRepairPath, pipelineRepaired)

	// Routing contract — enforced before any scoring/classification work.
	// A lead with no usable post link is dropped, not stored. The Skipped
	// reason is surfaced so the crawler bug (not the data) gets fixed.
	if err := ValidateRouting(in); err != nil {
		signals := []string{"invalid_routing:" + err.Error()}
		if urlSignal != "" {
			signals = append(signals, urlSignal)
		}
		return Outcome{
			Skipped: "invalid_routing",
			Signals: signals,
		}, nil
	}
	sourceType := normalizeSourceType(in.SourceType)

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

	// AI classifier overrides deterministic when configured (or when an explicit prompt is provided).
	// Failures fall back to deterministic so a flaky LLM never blocks lead capture.
	hasAIContext := (deps.BusinessProfile != nil && deps.BusinessProfile.IsConfigured()) || strings.TrimSpace(deps.UserPrompt) != ""
	if hasAIContext && deps.AIClass != nil && deps.AIClass.Available() {
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

		// Observability: build a classification_log row for EVERY outcome
		// (kept, rejected, errored). Without this, rejected posts have no
		// DB footprint and "why did all 50 posts reject?" is unanswerable.
		logEntry := store.ClassificationLogEntry{
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
			logEntry.Decision = store.ClassificationError
			logEntry.AIReason = err.Error()
			if deps.LegacyDB != nil {
				_ = deps.LegacyDB.RecordClassification(ctx, logEntry)
			}
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

			logEntry.AIIntent = aiResult.Intent
			logEntry.AIPriority = aiResult.Priority
			logEntry.AIReason = aiResult.Reason
			logEntry.AIScore = aiResult.Score

			if aiResult.Priority == "rejected" || aiResult.Intent == "not_relevant" || aiResult.Intent == "spam" || aiResult.Intent == "provider_ad" {
				out.Skipped = "rejected"
				out.Category = "rejected"
				out.Signals = append(out.Signals, "ai_intent:"+aiResult.Intent, "ai_reason:"+aiResult.Reason)
				logEntry.Decision = store.ClassificationRejected
				if deps.LegacyDB != nil {
					_ = deps.LegacyDB.RecordClassification(ctx, logEntry)
				}
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
			if out.Category == "cold" {
				logEntry.Decision = store.ClassificationCold
			} else {
				logEntry.Decision = store.ClassificationKept
			}
			if deps.LegacyDB != nil {
				_ = deps.LegacyDB.RecordClassification(ctx, logEntry)
			}
		}
	}

	if out.Category == "cold" {
		out.Skipped = "cold"
		return out, nil
	}

	// Phase B — thread role. Derived deterministically from the crawler's
	// source_type, the classifier's intent, and vendor-speak signals in the
	// content. Orthogonal to score: a vendor comment that scored "warm" on
	// buyer keywords still resolves to supplier_responder. See
	// project_thread_role_architecture.md.
	threadRole := string(models.InferThreadRole(sourceType, out.AIIntent, content))

	if deps.AppStore != nil {
		taskLead := store.TaskLead{
			TaskID:           in.TaskID,
			OrgID:            in.OrgID,
			SourceURL:        in.PrimaryURL,
			AuthorProfileURL: in.AuthorProfileURL,
			AuthorName:       in.AuthorName,
			Content:          content,
			LeadScore:        out.Score,
			Category:         out.Category,
			ThreadRole:       threadRole,
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
			SourceType:   sourceType,
			SourceID:     0,
			SourceURL:    in.PrimaryURL,
			SecondaryURL: in.SecondaryURL,
			PostFBID:     in.PostFBID,
			CommentFBID:  in.CommentFBID,
			GroupFBID:    in.GroupFBID,
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
			ThreadRole:   threadRole,
			ClassifiedAt: time.Now().UTC(),
		}
		leadID, err := deps.LegacyDB.InsertLead(legacy)
		if err != nil {
			// Non-fatal: task_leads is the source of truth; legacy mirror is
			// best-effort for the existing dashboard.
			slog.WarnContext(ctx, "legacy lead mirror failed",
				"task_id", in.TaskID, "org_id", in.OrgID, "error", err)
		}

		// Seed conversation_threads at ingest time so lead-engagement
		// projection (badges, thread state) sees a row before the first
		// outbound action. Idempotent (INSERT OR IGNORE on
		// idx_thread_org_profile), best-effort — seed failure must not
		// fail the lead pipeline. See SeedThreadForOrg doc + PR-B in
		// stabilization plan for the cross-account concurrent-first-send
		// gate-rule follow-up that completes outbound audit #3.
		if profile := strings.TrimSpace(in.AuthorProfileURL); profile != "" {
			if _, sErr := deps.LegacyDB.SeedThreadForOrg(in.OrgID, leadID, string(models.PlatformFacebook), profile, strings.TrimSpace(in.AuthorName), ""); sErr != nil {
				slog.WarnContext(ctx, "thread seed failed",
					"task_id", in.TaskID, "org_id", in.OrgID, "profile_url", profile, "error", sErr)
			}
		}
	}
	out.Inserted = true

	// Advance the per-intent crawl cursor for recurring runs. Best-effort —
	// a cursor write failure must not fail the lead insert. The store-side
	// AdvanceCrawlIntentCursor only moves the cursor forward (or sets it
	// unconditionally when PostedAt is zero — last-call-wins fallback).
	if deps.IntentID > 0 && deps.LegacyDB != nil {
		postID := strings.TrimSpace(in.PostFBID)
		if postID == "" {
			postID = ExtractFacebookPostID(in.PrimaryURL)
		}
		if postID != "" {
			if cErr := deps.LegacyDB.Crawl().AdvanceIntentCursor(ctx, deps.IntentID, postID, in.PostedAt); cErr != nil {
				slog.WarnContext(ctx, "advance crawl intent cursor failed",
					"intent_id", deps.IntentID, "post_id", postID, "error", cErr)
			}
		}
	}

	return out, nil
}

// buildURLRepairSignal turns the crawler-emitted URLRepairPath into the
// `url:<path>` signal that rides through Outcome.Signals. When the in-
// pipeline repairPrimaryURL mutated the URL (Chrome-extension path where
// the crawler emitted a shell), the signal is upgraded so the two ingest
// surfaces are distinguishable in telemetry.
func buildURLRepairSignal(crawlerPath string, pipelineRepaired bool) string {
	path := strings.TrimSpace(crawlerPath)
	if pipelineRepaired {
		// In-pipeline synth supersedes whatever the crawler reported —
		// the URL the lead actually carries was built here.
		return "url:repaired_in_pipeline"
	}
	if path == "" {
		return ""
	}
	return "url:" + path
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
