package system

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

type crawlProgressState struct {
	lastNotify time.Time
	lastFetch  int
}

var (
	crawlProgressMu    sync.Mutex
	crawlProgressTrack = map[string]*crawlProgressState{}
)

const (
	crawlProgressMinInterval = 30 * time.Second
	crawlProgressMinDelta    = 25
)

// ShouldEmitCrawlProgress returns true when the heartbeat should produce a
// user-facing notification for the given task.
func ShouldEmitCrawlProgress(taskID string, fetched int, done bool) bool {
	crawlProgressMu.Lock()
	defer crawlProgressMu.Unlock()
	state, ok := crawlProgressTrack[taskID]
	if !ok {
		crawlProgressTrack[taskID] = &crawlProgressState{lastNotify: time.Now(), lastFetch: fetched}
		return true
	}
	if done {
		delete(crawlProgressTrack, taskID)
		return true
	}
	if time.Since(state.lastNotify) >= crawlProgressMinInterval || (fetched-state.lastFetch) >= crawlProgressMinDelta {
		state.lastNotify = time.Now()
		state.lastFetch = fetched
		return true
	}
	return false
}

func RecordDashboardAutomationEvent(db *store.Store, orgID, accountID int64, message, action, args string, success bool) {
	if db == nil || orgID <= 0 {
		return
	}
	if err := db.InsertSystemPromptLog(orgID, accountID, message, action, args, success); err != nil {
		log.Printf("[AutomationEvent] could not record dashboard event org=%d account=%d action=%s: %v", orgID, accountID, action, err)
	}
}

func NotifyOutboundQueued(db *store.Store, notifier func(string), orgID, accountID, id int64, typ string, status models.OutboundStatus) {
	state := "draft waiting for approval"
	if status == models.OutboundApproved {
		state = "approved for Chrome Extension execution"
	}
	label := "Facebook outbound"
	switch typ {
	case "comment":
		label = "Facebook comment"
	case "inbox":
		label = "Facebook inbox"
	case "group_post":
		label = "Facebook posting"
	}
	msg := fmt.Sprintf("[THG Agent] %s #%d queued as %s. Org #%d, account #%d.", label, id, state, orgID, accountID)
	log.Printf("[Outbound] %s", msg)
	RecordDashboardAutomationEvent(db, orgID, accountID, msg, "system_outbound_queued", fmt.Sprintf(`{"id":%d,"type":%q,"status":%q}`, id, typ, status), true)
	if notifier != nil {
		notifier(msg)
	}
}

func NotifyOutboundStatus(db *store.Store, notifier func(string), orgID, id int64, status models.OutboundStatus) {
	if db == nil {
		return
	}
	msg, err := db.GetOutboundForOrg(orgID, id)
	if err != nil || msg == nil {
		return
	}
	text := fmt.Sprintf("[THG Agent] Facebook %s #%d status: %s. Target: %s", msg.Type, msg.ID, status, msg.TargetName)
	log.Printf("[Outbound] %s", text)
	RecordDashboardAutomationEvent(db, orgID, msg.AccountID, text, "system_outbound_status", fmt.Sprintf(`{"id":%d,"type":%q,"status":%q}`, msg.ID, msg.Type, status), status != models.OutboundFailed)
	if notifier != nil {
		notifier(text)
	}
}

func NotifyCrawlSummary(db *store.Store, notifier func(string), orgID, accountID int64, taskID, intent string, totalItems, fetched, inserted int, sourceURL string) {
	label := strings.TrimSpace(intent)
	if label == "" {
		label = "facebook_crawl"
	}
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		sourceURL = "Facebook source selected by the workspace"
	}
	rejected := fetched - inserted
	if rejected < 0 {
		rejected = 0
	}
	skipped := totalItems - fetched
	if skipped < 0 {
		skipped = 0
	}
	outcome := fmt.Sprintf("%d raw items, %d analyzable posts, %d qualified leads saved, %d filtered by Market Signal Gate, %d skipped", totalItems, fetched, inserted, rejected, skipped)
	if inserted == 0 {
		outcome = fmt.Sprintf("%d raw items, %d analyzable posts, but 0 leads passed Market Signal Gate (%d filtered, %d skipped)", totalItems, fetched, rejected, skipped)
	}
	text := fmt.Sprintf("[THG Agent] Crawl %s completed. Task %s. Org #%d, account #%d. %s. Source: %s", label, taskID, orgID, accountID, outcome, sourceURL)
	log.Printf("[ConnectorCrawl] %s", text)
	RecordDashboardAutomationEvent(db, orgID, accountID, text, "system_crawl_summary", fmt.Sprintf(`{"task_id":%q,"intent":%q,"raw_items":%d,"fetched":%d,"qualified":%d,"filtered":%d,"skipped":%d,"source_url":%q}`, taskID, label, totalItems, fetched, inserted, rejected, skipped, sourceURL), true)
	if notifier != nil {
		notifier(text)
	}
}

func NotifyCrawlProgress(db *store.Store, notifier func(string), orgID, accountID int64, taskID, intent, stage string, fetched, max int, sourceURL string) {
	label := strings.TrimSpace(intent)
	if label == "" {
		label = "facebook_crawl"
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "scraping"
	}
	progress := fmt.Sprintf("%d", fetched)
	if max > 0 {
		progress = fmt.Sprintf("%d/%d", fetched, max)
	}
	source := strings.TrimSpace(sourceURL)
	if source == "" {
		source = "(source not reported)"
	}
	text := fmt.Sprintf("[THG Agent] Crawl %s in progress. Task %s. Org #%d, account #%d. Stage: %s. Progress: %s posts. Source: %s",
		label, taskID, orgID, accountID, stage, progress, source)
	log.Printf("[ConnectorCrawl] %s", text)
	RecordDashboardAutomationEvent(db, orgID, accountID, text, "system_crawl_progress",
		fmt.Sprintf(`{"task_id":%q,"intent":%q,"stage":%q,"fetched":%d,"max":%d,"source_url":%q}`, taskID, label, stage, fetched, max, source), true)
	if notifier != nil {
		notifier(text)
	}
}

func NotifyCrawlFailure(db *store.Store, notifier func(string), orgID, accountID int64, taskID, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Chrome Extension crawl failed without an explicit error"
	}
	text := fmt.Sprintf("[THG Agent] Crawl task %s failed. Org #%d, account #%d. Reason: %s", taskID, orgID, accountID, reason)
	log.Printf("[ConnectorCrawl] %s", text)
	RecordDashboardAutomationEvent(db, orgID, accountID, text, "system_crawl_failure", fmt.Sprintf(`{"task_id":%q,"reason":%q}`, taskID, reason), false)
	if notifier != nil {
		notifier(text)
	}
}
