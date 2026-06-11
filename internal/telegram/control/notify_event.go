package control

// NotifyEvent is the PRIMARY notification path: it routes an automation event to the org's active
// notification DESTINATIONS (Telegram channels). Emitters (crawler / comment / posting / inbox /
// connector) call this — they NEVER touch Telegram directly. Each destination is filtered by its
// subscribed event types + channel filter; the delivery result is recorded + audited. Returns the
// number of destinations delivered to. `message` is pre-rendered by the caller (see render.*).
func (s *Service) NotifyEvent(orgID int64, eventType, channel, message string) (int, error) {
	if !s.flags.NotifyEnabled || !IsValidEventType(eventType) {
		return 0, nil
	}
	dests, err := s.store.ListActiveDestinations(orgID)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, d := range dests {
		if !alertTypeEnabled(d.EventTypes, eventType) {
			continue
		}
		if d.ChannelFilter != "all" && d.ChannelFilter != channel {
			continue
		}
		sendErr := s.tg.Send(d.ChatID, message)
		_ = s.store.RecordDelivery(orgID, d.ID, sendErr == nil, errText(sendErr))
		if sendErr == nil {
			sent++
			s.audit(orgID, 0, 0, AuditNotificationSent, eventType, channel)
		} else {
			s.audit(orgID, 0, 0, AuditNotificationFailed, eventType, channel)
		}
	}
	return sent, nil
}
