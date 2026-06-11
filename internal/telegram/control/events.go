package control

import (
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/telegram/render"
)

// High-level emit helpers used by the automation emitters (crawler, outbox agent). They render the
// message (control owns render) and route via NotifyEvent to the org's channel destinations. All
// are best-effort + nil-safe: a notification failure NEVER affects the crawl/comment path.

func leadLink(baseURL string, leadID int64) string {
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if b == "" {
		return ""
	}
	if leadID > 0 {
		return b + "/leads/" + strconv.FormatInt(leadID, 10)
	}
	return b + "/leads"
}

// NotifyLeadCreated emits a "new lead" notification (Facebook channel) for an org.
func (s *Service) NotifyLeadCreated(orgID, leadID int64, workspace, source, name, summary, baseURL string) {
	if s == nil || orgID == 0 {
		return
	}
	msg := render.LeadCreated(workspace, source, name, summary, leadLink(baseURL, leadID))
	_, _ = s.NotifyEvent(orgID, "lead_created", "facebook", msg)
}

// failureEvents are the agent-action events rendered as an attention/failure message.
var failureEvents = map[string]bool{
	"comment_failed": true, "comment_unverified": true, "post_failed": true, "inbox_failed": true,
}

// NotifyAgentAction emits a comment/post/inbox outcome notification for an org. eventType drives
// success-vs-failure rendering + the destination event-type filter.
func (s *Service) NotifyAgentAction(orgID int64, eventType, channel, account, lead, state, postURL, baseURL string) {
	if s == nil || orgID == 0 || !IsValidEventType(eventType) {
		return
	}
	dash := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	var msg string
	if failureEvents[eventType] {
		msg = render.Failure(channel, account, eventType, dash)
	} else {
		msg = render.AgentComment(channel, account, lead, state, postURL, dash)
	}
	_, _ = s.NotifyEvent(orgID, eventType, channel, msg)
}
