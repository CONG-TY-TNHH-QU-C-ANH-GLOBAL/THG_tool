package crawlingest

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram/control"
)

// Connector crawl-result ingest PROCESSOR (Fiber-free domain layer). The HTTP handler
// (agentConnectorCrawlResult in crawl.go) is a thin edge adapter: it parses the request,
// runs the basic edge validations it can do from *fiber.Ctx, then calls this processor and
// maps the returned domain errors to HTTP status. Nothing below imports or returns Fiber —
// it takes standard Go values and returns a typed result + a standard error, so the ingest
// flow (the P1.3 hot path) is unit-testable without an HTTP server. Request/result types and
// the domain error sentinels live in crawl_ingest_types.go.

// processConnectorCrawlResult is the Fiber-free ingest pipeline: ownership validation, task
// lifecycle, dependency setup, direct-post workflow resolution, the per-item loop, the
// deterministic direct-post terminal-failure decision, the crawl summary, and forensics.
// Synchronous — it completes before returning (no goroutines spawned on ctx).
func (h *Handler) processConnectorCrawlResult(ctx context.Context, agentID, orgID int64, req connectorCrawlResultRequest) (connectorCrawlProcessResult, error) {
	if err := h.resolveCrawlOwnership(orgID, agentID, req); err != nil {
		return connectorCrawlProcessResult{}, err
	}

	appStore, err := store.NewAppStore(h.db)
	if err != nil {
		return connectorCrawlProcessResult{}, err
	}
	intent := strings.TrimSpace(req.Intent)
	if intent == "" {
		intent = "facebook_crawl"
	}
	_ = appStore.CreateTask(ctx, req.TaskID, orgID, intent)
	_ = appStore.StartTask(ctx, req.TaskID)

	if strings.EqualFold(req.Status, "failed") || strings.TrimSpace(req.Error) != "" {
		return h.handleFailedCrawl(ctx, orgID, req, appStore), nil
	}

	// Explicit direct-post intake? (req.TaskID == workflow.import_task_id). When so, the
	// chosen post must be force-created as a lead even if the market filter would reject it,
	// and must keep the requested group-context URL — see crawl_direct_post.go.
	p := &crawlResultProcessor{
		h:          h,
		appStore:   appStore,
		orgID:      orgID,
		taskID:     req.TaskID,
		deps:       h.buildConnectorCrawlIngestDeps(orgID, req, appStore),
		directPost: h.resolveDirectPostIntake(ctx, orgID, req.TaskID),
	}
	if p.directPost != nil && strings.TrimSpace(p.directPost.CanonicalPostURL) != "" {
		p.primarySourceURL = strings.TrimSpace(p.directPost.CanonicalPostURL) // summary prefers requested URL
	}

	p.ingestItems(ctx, req.Items)
	p.finalizeDirectPost(ctx, orgID, req)

	_ = appStore.CompleteTask(ctx, req.TaskID, p.fetched, p.inserted)
	scrollNote := logConnectorCrawlForensics(ctx, orgID, req)
	system.NotifyCrawlSummary(h.db, h.notifier, orgID, req.AccountID, req.TaskID, intent, len(req.Items), p.fetched, p.inserted, p.primarySourceURL, req.ExitReason, scrollNote)
	return connectorCrawlProcessResult{Status: "stored", TaskID: req.TaskID, Fetched: p.fetched, Inserted: p.inserted}, nil
}

// resolveCrawlOwnership enforces the two ownership gates before any task/lead
// mutation: the account must belong to the org, and the calling connector must
// own that account's stream. Returns the typed forbidden sentinel on rejection.
func (h *Handler) resolveCrawlOwnership(orgID, agentID int64, req connectorCrawlResultRequest) error {
	if acc, err := h.db.Identities().GetAccountForOrg(req.AccountID, orgID); err != nil || acc == nil {
		return errCrawlForbiddenAccount
	}
	ownsStream, err := h.db.Connectors().ConnectorOwnsAccountStream(orgID, agentID, req.AccountID)
	if err != nil {
		return err
	}
	if !ownsStream {
		return errCrawlForbiddenStream
	}
	return nil
}

// handleFailedCrawl records a connector-reported failed crawl: it fails the task,
// then either fails an explicit direct-post import workflow terminally (with the
// typed extension-error code, surfaced to the requester) or emits the admin
// crawl-failure notice. No lead / no outbound / no comment. Behavior-identical to
// the former inline failed-status branch.
func (h *Handler) handleFailedCrawl(ctx context.Context, orgID int64, req connectorCrawlResultRequest, appStore *store.AppStore) connectorCrawlProcessResult {
	errMsg := strings.TrimSpace(req.Error)
	if errMsg == "" {
		errMsg = "Chrome Extension crawl failed"
	}
	_ = appStore.FailTask(ctx, req.TaskID, errMsg)
	// P1.3E: when a FAILED crawl is an explicit direct-post import (target not rendered /
	// boilerplate / typed group/post mismatch reported by the extension), fail the workflow
	// terminally with the typed code AND surface it to the requester — instead of only an
	// admin crawl-failure notice + a poller timeout. No lead / no outbound / no comment.
	if wf := h.resolveDirectPostIntake(ctx, orgID, req.TaskID); wf != nil {
		code := directPostFailureCodeFromExtensionError(errMsg)
		log.Printf("[ConnectorCrawl] direct_post_intake=true wf=%d import_task_id=%q extension_error=%q → terminal code=%s",
			wf.ID, req.TaskID, errMsg, code)
		h.failDirectPostImport(ctx, orgID, wf, code, "direct-post import failed in connector: "+errMsg)
	} else {
		system.NotifyCrawlFailure(h.db, h.notifier, orgID, req.AccountID, req.TaskID, errMsg)
	}
	return connectorCrawlProcessResult{Status: "failed", Error: errMsg}
}

// buildConnectorCrawlIngestDeps assembles the leadingest dependencies (scoring guidance,
// keywords, business profile, AI classifier, market signal gate, OnLeadCreated Telegram
// notification). Behavior-identical to the former inline block in the handler.
func (h *Handler) buildConnectorCrawlIngestDeps(orgID int64, req connectorCrawlResultRequest, appStore *store.AppStore) leadingest.Deps {
	var aiClass *ai.MessageGenerator
	if h.aiClass != nil {
		aiClass = h.aiClass()
	}
	return leadingest.Deps{
		AppStore:        appStore,
		LegacyDB:        h.db,
		Scorer:          scoring.New(scoring.DefaultConfig()),
		Guidance:        orgScoringGuidance(h.db, orgID),
		BusinessProfile: ai.LoadProfileForOrg(h.db, orgID),
		AIClass:         aiClass,
		SignalGate:      leadingest.SignalGateFromMap(req.MarketSignalGate),
		Keywords:        normalizeCrawlKeywords(append(req.Keywords, orgIntelligenceKeywords(h.db, orgID)...)),
		UserPrompt:      strings.TrimSpace(req.UserPrompt),
		ExtraSignals:    []string{"chrome_extension_crawl"},
		IntentID:        req.IntentID,
		OnLeadCreated: func(ev leadingest.LeadEvent) {
			workspace := ""
			if org, _ := h.db.GetOrganization(ev.OrgID); org != nil {
				workspace = org.Name
			}
			h.tgEvents.NotifyLead(control.LeadNotice{
				OrgID: ev.OrgID, LeadID: ev.LeadID, Channel: "facebook", Workspace: workspace,
				Author: ev.AuthorName, PostURL: ev.PostURL, Excerpt: ev.Excerpt, Reason: ev.Reason, BaseURL: h.baseURL,
			})
		},
	}
}

// logConnectorCrawlForensics preserves the PR-CRAWL1 low-yield scroll diagnostic: when a
// crawl yields suspiciously few raw posts, log WHY (scroll moved? how many passes? articles
// seen?). Returns the scrollNote passed to the crawl summary. Behavior-identical to the
// former inline block; ScrollDiag values are rendered with %v (panic-safe, no type assert).
func logConnectorCrawlForensics(ctx context.Context, orgID int64, req connectorCrawlResultRequest) string {
	if len(req.Items) > 2 || req.ScrollDiag == nil {
		return ""
	}
	sd := req.ScrollDiag
	scrollNote := fmt.Sprintf("moved=%v passes=%v max_articles=%v target=%v",
		sd["scroll_moved_ever"], sd["passes"], sd["max_articles_seen"], sd["final_scroll_target"])
	slog.WarnContext(ctx, "crawl-forensic: low yield",
		"org_id", orgID, "account_id", req.AccountID, "task_id", req.TaskID,
		"raw_items", len(req.Items), "exit_reason", req.ExitReason,
		"scroll_diag", req.ScrollDiag,
	)
	return scrollNote
}
