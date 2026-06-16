package agent

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
	if acc, err := h.db.Identities().GetAccountForOrg(req.AccountID, orgID); err != nil || acc == nil {
		return connectorCrawlProcessResult{}, errCrawlForbiddenAccount
	}
	ownsStream, err := h.db.Connectors().ConnectorOwnsAccountStream(orgID, agentID, req.AccountID)
	if err != nil {
		return connectorCrawlProcessResult{}, err
	}
	if !ownsStream {
		return connectorCrawlProcessResult{}, errCrawlForbiddenStream
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
		errMsg := strings.TrimSpace(req.Error)
		if errMsg == "" {
			errMsg = "Chrome Extension crawl failed"
		}
		_ = appStore.FailTask(ctx, req.TaskID, errMsg)
		system.NotifyCrawlFailure(h.db, h.notifier, orgID, req.AccountID, req.TaskID, errMsg)
		return connectorCrawlProcessResult{Status: "failed", Error: errMsg}, nil
	}

	deps := h.buildConnectorCrawlIngestDeps(orgID, req, appStore)

	// Explicit direct-post intake? (req.TaskID == workflow.import_task_id). When so, the
	// chosen post must be force-created as a lead even if the market filter would reject it,
	// and must keep the requested group-context URL — see crawl_direct_post.go.
	directPost := h.resolveDirectPostIntake(ctx, orgID, req.TaskID)
	primarySourceURL := ""
	if directPost != nil && strings.TrimSpace(directPost.CanonicalPostURL) != "" {
		primarySourceURL = strings.TrimSpace(directPost.CanonicalPostURL) // summary prefers requested URL
	}

	// P1.3C import-result bubbling: track whether THIS finished import produced a valid
	// requested-post lead (dpValidObserved) or already failed the workflow (dpFailed). If
	// neither, the connector returned nothing usable for the requested post and the workflow
	// is failed deterministically below (no silent retry-forever loop).
	dpValidObserved, dpFailed := false, false
	dpFailureCode := ""
	inserted, fetched := 0, 0
	for _, item := range req.Items {
		o := h.processConnectorCrawlItem(ctx, orgID, req.TaskID, item, deps, appStore, directPost)
		if o.fetched {
			fetched++
		}
		if o.inserted {
			inserted++
		}
		if primarySourceURL == "" && o.primaryURL != "" {
			primarySourceURL = o.primaryURL
		}
		if o.dpValidObserved {
			dpValidObserved = true
		}
		if o.dpFailed {
			dpFailed = true
			if o.dpFailureCode != "" {
				dpFailureCode = o.dpFailureCode
			}
		}
	}

	// P1.3C: the import task FINISHED. If this was an explicit direct-post intake and it
	// produced no valid requested-post lead (and did not already fail on a poisoned item),
	// fail the workflow deterministically with a typed reason instead of leaving the poller
	// to retry until the generic lead_not_observed timeout. (CAS-guarded: stale/racing → no-op.)
	// failDirectPostImport also surfaces the typed reason in the requester's Copilot history.
	if code, fail := directPostImportFailureCode(dpValidObserved, dpFailed); fail && directPost != nil {
		log.Printf("[ConnectorCrawl] direct_post_intake=true wf=%d import_task_id=%q expected_post_fbid=%q expected_group_ref=%q raw_items=%d — no valid observed item for requested post; failing workflow code=%s",
			directPost.ID, req.TaskID, directPost.PostFBID, directPost.GroupRef, len(req.Items), code)
		dpFailureCode = code
		h.failDirectPostImport(ctx, orgID, directPost, code, "import finished but the requested post was not observed")
	}
	if directPost != nil {
		logDirectPostImportForensics(ctx, req, directPost, dpValidObserved, dpFailed || dpFailureCode != "", dpFailureCode)
	}

	_ = appStore.CompleteTask(ctx, req.TaskID, fetched, inserted)
	scrollNote := logConnectorCrawlForensics(ctx, orgID, req)
	system.NotifyCrawlSummary(h.db, h.notifier, orgID, req.AccountID, req.TaskID, intent, len(req.Items), fetched, inserted, primarySourceURL, req.ExitReason, scrollNote)
	return connectorCrawlProcessResult{Status: "stored", TaskID: req.TaskID, Fetched: fetched, Inserted: inserted}, nil
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
