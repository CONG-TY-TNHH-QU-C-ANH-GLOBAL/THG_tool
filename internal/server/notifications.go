package server

import (
	"fmt"

	"github.com/thg/scraper/internal/models"
)

func (s *Server) notifyOutboundQueued(orgID, accountID, id int64, typ string, status models.OutboundStatus) {
	if s == nil || s.cfg.Notifier == nil {
		return
	}
	state := "draft waiting for approval"
	if status == models.OutboundApproved {
		state = "approved for local runtime execution"
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
	s.cfg.Notifier(fmt.Sprintf("[THG Agent] %s #%d queued as %s. Org #%d, account #%d.", label, id, state, orgID, accountID))
}

func (s *Server) notifyOutboundStatus(orgID, id int64, status models.OutboundStatus) {
	if s == nil || s.cfg.Notifier == nil {
		return
	}
	msg, err := s.db.GetOutboundForOrg(orgID, id)
	if err != nil || msg == nil {
		return
	}
	s.cfg.Notifier(fmt.Sprintf("[THG Agent] Facebook %s #%d status: %s. Target: %s", msg.Type, msg.ID, status, msg.TargetName))
}
