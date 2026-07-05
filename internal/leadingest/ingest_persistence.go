package leadingest

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/app"
	"github.com/thg/scraper/internal/textutil"
)

// Persistence phase for IngestPost — behaviour-preserving extraction of the
// persist + side-effect flow documented in specs/lead_ingestion_behavior.md §4/§5/§7.
// The fatal/best-effort split, the legacy SourceID=0 no-dedup quirk (§5), and the
// per-call OnLeadCreated firing are all unchanged.

// persistLead writes the lead to task_leads (FATAL on error — task_leads is the
// source of truth) and then best-effort mirrors it to the legacy leads table with
// its thread seed + notification. A nil store skips the corresponding leg silently;
// with both stores nil this is a no-op and the caller still marks the lead Inserted.
func persistLead(ctx context.Context, deps Deps, in Input, content, sourceType, threadRole string, out *Outcome) error {
	if deps.AppStore != nil {
		taskLead := app.TaskLead{
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
			return err
		}
	}
	if deps.LegacyDB != nil {
		mirrorLegacyLead(ctx, deps, in, content, sourceType, threadRole, out)
	}
	return nil
}

// mirrorLegacyLead inserts the best-effort legacy `leads` mirror, then seeds the
// conversation thread and fires the OnLeadCreated hook. None of these are fatal.
//
// SourceID stays 0 (spec §5): the partial UNIQUE idx_leads_dedup(source_type,
// source_id) WHERE source_id > 0 therefore never applies, so duplicate ingests grow
// the legacy table even though task_leads is deduped by UNIQUE(task_id, source_url).
// The OnLeadCreated hook fires once per call that reaches here (incl. duplicates).
func mirrorLegacyLead(ctx context.Context, deps Deps, in Input, content, sourceType, threadRole string, out *Outcome) {
	// AuthorRole carries the AI classifier intent so the dashboard renders a meaningful
	// per-lead tag instead of a generic "AI classifier" string.
	authorRole := strings.TrimSpace(out.AIIntent)
	if authorRole == "" {
		authorRole = "unknown"
	}
	// PainPoint is the human-readable AI reason; fall back to signals when missing.
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
		Niche:        legacyNiche(deps),
		ThreadRole:   threadRole,
		ClassifiedAt: time.Now().UTC(),
	}
	leadID, err := deps.LegacyDB.Leads().InsertLead(legacy)
	if err != nil {
		// Non-fatal: task_leads is the source of truth; the legacy mirror is best-effort.
		slog.WarnContext(ctx, "legacy lead mirror failed",
			"task_id", in.TaskID, "org_id", in.OrgID, "error", err)
	}

	// Seed conversation_threads so the lead-engagement projection sees a row before
	// the first outbound action. Idempotent (INSERT OR IGNORE), best-effort.
	if profile := strings.TrimSpace(in.AuthorProfileURL); profile != "" {
		if _, sErr := deps.LegacyDB.Threads().SeedThreadForOrg(in.OrgID, leadID, string(models.PlatformFacebook), profile, strings.TrimSpace(in.AuthorName), ""); sErr != nil {
			slog.WarnContext(ctx, "thread seed failed",
				"task_id", in.TaskID, "org_id", in.OrgID, "profile_url", profile, "error", sErr)
		}
	}
	// Best-effort notification hook (Telegram channel etc.). Raw content is passed; the
	// consumer sanitizes + caps it. Never affects the ingest result.
	if deps.OnLeadCreated != nil {
		deps.OnLeadCreated(LeadEvent{
			OrgID: in.OrgID, LeadID: leadID,
			AuthorName: strings.TrimSpace(in.AuthorName),
			PostURL:    in.PrimaryURL,
			Excerpt:    content,
			Reason:     textutil.FirstNonEmpty(out.AIReason, strings.Join(out.Signals, " / ")),
			SourceType: sourceType,
			GroupFBID:  in.GroupFBID,
		})
	}
}

// legacyNiche prefers a clean domain label (profile industry, then profile name)
// over the raw crawl keywords for the legacy mirror's Niche field.
func legacyNiche(deps Deps) string {
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
	return niche
}

// advanceCrawlCursor moves the per-intent crawl cursor forward for recurring runs
// (spec §4). Best-effort: a cursor write failure must not fail the lead insert. No-op
// for one-shot runs (IntentID==0), without a legacy DB, or when no post id resolves.
func advanceCrawlCursor(ctx context.Context, deps Deps, in Input) {
	if deps.IntentID <= 0 || deps.LegacyDB == nil {
		return
	}
	postID := strings.TrimSpace(in.PostFBID)
	if postID == "" {
		postID = ExtractFacebookPostID(in.PrimaryURL)
	}
	if postID == "" {
		return
	}
	if cErr := deps.LegacyDB.Crawl().AdvanceIntentCursor(ctx, deps.IntentID, postID, in.PostedAt); cErr != nil {
		slog.WarnContext(ctx, "advance crawl intent cursor failed",
			"intent_id", deps.IntentID, "post_id", postID, "error", cErr)
	}
}
