package agent

import (
	"context"
	"log"
	"strings"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// connectorCrawlItemOutcome is the per-item verdict the loop accumulates. fetched mirrors the
// former fetched++ (every item with usable content, regardless of later skip); inserted is a
// real lead insert; primaryURL is the candidate that may seed the summary's primary URL; the
// dp* flags carry the direct-post import-result bubbling state up to the processor.
type connectorCrawlItemOutcome struct {
	fetched         bool
	inserted        bool
	primaryURL      string
	dpValidObserved bool
	dpFailed        bool
}

// processConnectorCrawlItem runs the per-item ingest for one observed post. Behavior-identical
// to the former inline loop body: skip empty/short content (no fetched), count fetched, resolve
// the source URL, apply direct-post zero-trust validation when this is a direct-post intake,
// dedupe by source URL, then IngestPost (log-and-continue on error). Fiber-free.
func (h *Handler) processConnectorCrawlItem(ctx context.Context, orgID int64, taskID string, item connectorCrawlItem, baseDeps leadingest.Deps, appStore *store.AppStore, directPost *coordination.DirectPostCommentWorkflow) connectorCrawlItemOutcome {
	content := strings.TrimSpace(item.Content)
	if content == "" || len([]rune(content)) < 20 {
		return connectorCrawlItemOutcome{}
	}
	out := connectorCrawlItemOutcome{fetched: true}

	sourceURL := strings.TrimSpace(item.SourceURL)
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(item.ID)
	}

	// Per-item identity + filter policy. For an explicit direct-post intake the observed item
	// is ZERO-TRUST validated before any override (P1.3A): only a positively-identified,
	// non-conflicting, meaningful post is force-created with the context-preserving canonical
	// URL. A poisoned REQUESTED post fails the workflow (skip ingest); a different/neighbour
	// post falls through to normal filtering.
	itemDeps := baseDeps
	primaryURL := sourceURL
	postFBID := strings.TrimSpace(item.PostFBID)
	groupFBID := strings.TrimSpace(item.GroupFBID)
	if directPost != nil {
		eval := h.evaluateDirectPostCrawlItem(ctx, orgID, taskID, directPost, item, baseDeps, sourceURL, content)
		out.dpValidObserved = eval.validObserved
		out.dpFailed = eval.failedWorkflow
		if eval.failedWorkflow {
			return out // poisoned requested post — no lead/outbound, workflow already failed
		}
		itemDeps = eval.deps
		primaryURL = eval.primaryURL
		postFBID = eval.postFBID
		groupFBID = eval.groupFBID
	}
	out.primaryURL = primaryURL

	// Deduplicate: memory check before hitting AI — avoids duplicate leads + wasted LLM tokens.
	if primaryURL != "" && appStore != nil {
		if exists, _ := appStore.HasLeadWithSourceURL(ctx, orgID, primaryURL); exists {
			return out
		}
	}

	outcome, err := leadingest.IngestPost(ctx, itemDeps, leadingest.Input{
		TaskID:           taskID,
		OrgID:            orgID,
		SourceType:       "post",
		PrimaryURL:       primaryURL,
		PostFBID:         postFBID,
		GroupFBID:        groupFBID,
		PostedAt:         parsePostedAtRFC3339(item.PostedAt),
		AuthorName:       strings.TrimSpace(item.AuthorName),
		AuthorProfileURL: strings.TrimSpace(item.AuthorProfileURL),
		Content:          content,
		Reactions:        item.Reactions,
		Comments:         item.Comments,
		Shares:           item.Shares,
	})
	if err != nil {
		log.Printf("[ConnectorCrawl] ingest failed task=%s: %v", taskID, err)
		return out
	}
	if outcome.Inserted {
		out.inserted = true
	}
	return out
}
