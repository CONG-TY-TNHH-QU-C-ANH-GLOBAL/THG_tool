package server

import (
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/models"
)

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
	if s.cfg.Notifier == nil {
		return
	}
	s.cfg.Notifier(text)
}

func (s *Server) notifyCrawlSummary(orgID, accountID int64, taskID, intent string, fetched, inserted int, sourceURL string) {
	label := strings.TrimSpace(intent)
	if label == "" {
		label = "facebook_crawl"
	}
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		sourceURL = "Facebook source selected by the workspace"
	}
	outcome := fmt.Sprintf("%d posts fetched, %d qualified leads saved", fetched, inserted)
	if inserted == 0 {
		outcome = fmt.Sprintf("%d posts fetched, but 0 leads passed Market Signal Gate", fetched)
	}
	text := fmt.Sprintf("[THG Agent] Crawl %s completed. Task %s. Org #%d, account #%d. %s. Source: %s", label, taskID, orgID, accountID, outcome, sourceURL)
	log.Printf("[ConnectorCrawl] %s", text)
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
	if s == nil || s.cfg.Notifier == nil {
		return
	}
	s.cfg.Notifier(text)
}
