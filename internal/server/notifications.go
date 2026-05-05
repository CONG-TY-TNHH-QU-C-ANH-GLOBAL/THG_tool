package server

import (
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/models"
)

func (s *Server) recordDashboardAutomationEvent(orgID, accountID int64, message, action, args string, success bool) {
	if s == nil || s.db == nil || orgID <= 0 {
		return
	}
	if err := s.db.InsertSystemPromptLog(orgID, accountID, message, action, args, success); err != nil {
		log.Printf("[AutomationEvent] could not record dashboard event org=%d account=%d action=%s: %v", orgID, accountID, action, err)
	}
}

func (s *Server) notifyOutboundQueued(orgID, accountID, id int64, typ string, status models.OutboundStatus) {
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
	s.recordDashboardAutomationEvent(orgID, accountID, msg, "system_outbound_queued", fmt.Sprintf(`{"id":%d,"type":%q,"status":%q}`, id, typ, status), true)
	if s == nil || s.cfg.Notifier == nil {
		return
	}
	s.cfg.Notifier(msg)
}

func (s *Server) notifyOutboundStatus(orgID, id int64, status models.OutboundStatus) {
	if s == nil {
		return
	}
	msg, err := s.db.GetOutboundForOrg(orgID, id)
	if err != nil || msg == nil {
		return
	}
	text := fmt.Sprintf("[THG Agent] Facebook %s #%d status: %s. Target: %s", msg.Type, msg.ID, status, msg.TargetName)
	log.Printf("[Outbound] %s", text)
	s.recordDashboardAutomationEvent(orgID, msg.AccountID, text, "system_outbound_status", fmt.Sprintf(`{"id":%d,"type":%q,"status":%q}`, msg.ID, msg.Type, status), status != models.OutboundFailed)
	if s.cfg.Notifier == nil {
		return
	}
	s.cfg.Notifier(text)
}

func (s *Server) notifyCrawlSummary(orgID, accountID int64, taskID, intent string, totalItems, fetched, inserted int, sourceURL string) {
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
	s.recordDashboardAutomationEvent(orgID, accountID, text, "system_crawl_summary", fmt.Sprintf(`{"task_id":%q,"intent":%q,"raw_items":%d,"fetched":%d,"qualified":%d,"filtered":%d,"skipped":%d,"source_url":%q}`, taskID, label, totalItems, fetched, inserted, rejected, skipped, sourceURL), true)
	if s == nil || s.cfg.Notifier == nil {
		return
	}
	s.cfg.Notifier(text)
}

func (s *Server) notifyCrawlFailure(orgID, accountID int64, taskID, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Chrome Extension crawl failed without an explicit error"
	}
	text := fmt.Sprintf("[THG Agent] Crawl task %s failed. Org #%d, account #%d. Reason: %s", taskID, orgID, accountID, reason)
	log.Printf("[ConnectorCrawl] %s", text)
	s.recordDashboardAutomationEvent(orgID, accountID, text, "system_crawl_failure", fmt.Sprintf(`{"task_id":%q,"reason":%q}`, taskID, reason), false)
	if s == nil || s.cfg.Notifier == nil {
		return
	}
	s.cfg.Notifier(text)
}
