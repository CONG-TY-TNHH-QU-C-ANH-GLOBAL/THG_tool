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
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/fburl"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/app"
)

// Deps captures the per-run dependencies that the ingest function needs.
// Zero-valued fields are tolerated where indicated; callers that don't have
// AI classification configured may pass AIClass=nil and BusinessProfile=nil.
type Deps struct {
	AppStore        *app.Store
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
	// OnLeadCreated is an OPTIONAL best-effort notification hook fired once per NEW lead inserted
	// (e.g. to push a Telegram channel notification). Wired by the caller; nil = no notification.
	// It must never block or fail the ingest path.
	OnLeadCreated func(LeadEvent)
	// ForceLead, when true, marks this ingest as an EXPLICIT direct-post intake: the user
	// already chose the post as a lead candidate, so a market-signal veto (deterministic
	// reject, signal gate, AI reject, cold) is downgraded to annotation and the lead is
	// still created/upserted. Set ONLY by the direct-post intake path — never by broad
	// crawls — so normal filtering is unchanged. See force_lead.go.
	ForceLead bool
}

// LeadEvent is the data a notification needs when a new lead is created. Excerpt is RAW post text
// (the consumer sanitizes it); Reason is the matched signal/AI reason.
type LeadEvent struct {
	OrgID, LeadID int64
	AuthorName    string // author / lead name
	PostURL       string // canonical Facebook permalink
	Excerpt       string // raw post content (sanitized downstream)
	Reason        string // matched signal / AI reason
	SourceType    string // post | comment
	GroupFBID     string // source group id, when known
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
// The flow is split into three behaviour-preserving phases (see
// specs/domains/facebook-sales-intelligence/features/lead-ingestion/technical.md §9): early validation here, classification in
// classifyLead (ingest_flow.go), and persistence in persistLead/advanceCrawlCursor
// (ingest_persistence.go). The state machine and every observable Outcome.Skipped
// value, signal string, and side effect are unchanged.
func IngestPost(ctx context.Context, deps Deps, in Input) (Outcome, error) {
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return Outcome{Skipped: "filter"}, nil
	}

	// Server-side rescue: when the crawler emits a group/page shell URL but ALSO
	// sends PostFBID, synthesise the canonical post permalink before validating.
	// See project_lead_routing_gap.md. urlSignal carries the `url:<path>` telemetry.
	pipelineRepaired := repairPrimaryURL(&in)
	urlSignal := buildURLRepairSignal(in.URLRepairPath, pipelineRepaired)

	// Routing contract — a lead with no usable post link is dropped, not stored.
	if err := ValidateRouting(in); err != nil {
		return invalidRoutingOutcome(err, urlSignal), nil
	}
	sourceType := normalizeSourceType(in.SourceType)

	// Classification phase: deterministic score → market gate → AI override → cold
	// gate. Any veto short-circuits with skip unless ForceLead downgrades it.
	out, skip := classifyLead(ctx, deps, in, content, urlSignal)
	if skip {
		return out, nil
	}

	// Phase B — thread role, derived deterministically from source_type, the
	// classifier intent, and vendor-speak signals. See project_thread_role_architecture.md.
	threadRole := string(models.InferThreadRole(sourceType, out.AIIntent, content))

	// Persistence phase: task_leads insert (fatal), then best-effort legacy mirror +
	// thread seed + notification, then best-effort cursor advance.
	if err := persistLead(ctx, deps, in, content, sourceType, threadRole, &out); err != nil {
		return out, err
	}
	out.Inserted = true
	advanceCrawlCursor(ctx, deps, in)
	return out, nil
}

// invalidRoutingOutcome builds the dropped-lead Outcome for a routing-contract
// violation, preserving the exact `invalid_routing:<msg>` signal text and the url
// telemetry signal when present.
func invalidRoutingOutcome(err error, urlSignal string) Outcome {
	signals := []string{"invalid_routing:" + err.Error()}
	if urlSignal != "" {
		signals = append(signals, urlSignal)
	}
	return Outcome{Skipped: "invalid_routing", Signals: signals}
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
