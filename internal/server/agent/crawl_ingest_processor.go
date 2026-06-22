package agent

import (
	"context"
	"log"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// crawlResultProcessor is the short-lived, per-request accumulator for the crawl
// ingest item loop and its direct-post finalization. It holds only the values
// shared across loop iterations; it is created and used once per
// processConnectorCrawlResult call and never reused or stored. Mutating methods
// use a pointer receiver so the accumulators persist across iterations; context
// is passed explicitly to the methods, never stored on the struct.
type crawlResultProcessor struct {
	h          *Handler
	appStore   *store.AppStore
	deps       leadingest.Deps
	directPost *coordination.DirectPostCommentWorkflow
	orgID      int64
	taskID     string

	fetched          int
	inserted         int
	primarySourceURL string
	dpValidObserved  bool
	dpFailed         bool
	dpFailureCode    string
}

// ingestItems runs each observed item through the per-item ingest pipeline and
// folds the outcomes into the accumulators. P1.3C import-result bubbling:
// dpValidObserved / dpFailed track whether THIS finished import produced a valid
// requested-post lead or already failed the workflow on a poisoned item.
//
// Items are passed BY VALUE (connectorCrawlItem); the loop never takes the
// address of the range variable and no goroutine captures it — so each item is
// ingested in isolation exactly as the former inline loop did.
func (p *crawlResultProcessor) ingestItems(ctx context.Context, items []connectorCrawlItem) {
	for _, item := range items {
		o := p.h.processConnectorCrawlItem(ctx, p.orgID, p.taskID, item, p.deps, p.appStore, p.directPost)
		if o.fetched {
			p.fetched++
		}
		if o.inserted {
			p.inserted++
		}
		if p.primarySourceURL == "" && o.primaryURL != "" {
			p.primarySourceURL = o.primaryURL
		}
		if o.dpValidObserved {
			p.dpValidObserved = true
		}
		if o.dpFailed {
			p.dpFailed = true
			if o.dpFailureCode != "" {
				p.dpFailureCode = o.dpFailureCode
			}
		}
	}
}

// finalizeDirectPost makes the deterministic direct-post terminal-failure
// decision after the import task FINISHED: if this was an explicit direct-post
// intake that produced no valid requested-post lead (and did not already fail on
// a poisoned item), fail the workflow with a typed reason (CAS-guarded; also
// surfaces it in the requester's Copilot history) instead of leaving the poller
// to time out. It then emits the forensic log line. Behavior-identical to the
// former inline block.
func (p *crawlResultProcessor) finalizeDirectPost(ctx context.Context, orgID int64, req connectorCrawlResultRequest) {
	if code, fail := directPostImportFailureCode(p.dpValidObserved, p.dpFailed); fail && p.directPost != nil {
		log.Printf("[ConnectorCrawl] direct_post_intake=true wf=%d import_task_id=%q expected_post_fbid=%q expected_group_ref=%q raw_items=%d — no valid observed item for requested post; failing workflow code=%s",
			p.directPost.ID, req.TaskID, p.directPost.PostFBID, p.directPost.GroupRef, len(req.Items), code)
		p.dpFailureCode = code
		p.h.failDirectPostImport(ctx, orgID, p.directPost, code, "import finished but the requested post was not observed")
	}
	if p.directPost != nil {
		logDirectPostImportForensics(ctx, req, p.directPost, p.dpValidObserved, p.dpFailed || p.dpFailureCode != "", p.dpFailureCode)
	}
}
