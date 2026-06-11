package control

import (
	"encoding/json"
	"strings"

	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/render"
)

// defaultEventTypesJSON: a freshly connected destination subscribes to ALL events so it receives
// notifications immediately; the admin trims them in the UI. channel_filter defaults to "all".
func defaultEventTypesJSON() string { b, _ := json.Marshal(EventTypes); return string(b) }

// normalizeChannelRef ensures a public-channel reference is addressable ("@handle"). A numeric id
// (private channel) is left as-is.
func normalizeChannelRef(ref string) string {
	r := strings.TrimSpace(ref)
	if r == "" || strings.HasPrefix(r, "@") || strings.HasPrefix(r, "-") || isDigits(r) {
		return r
	}
	return "@" + strings.TrimPrefix(r, "@")
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// ConnectPublicChannel connects a PUBLIC channel by @username: it sends one confirmation message
// (the bot must already be an admin/sender of the channel) and stores the chat id/title Telegram
// returns. Audited. Returns a typed reason on failure (resolve_failed = bot not admin / not found).
func (s *Service) ConnectPublicChannel(orgID, userID int64, ref string) (*tgstore.Destination, string) {
	chatID, title, username, err := s.tg.Resolve(normalizeChannelRef(ref), render.ChannelConnected())
	if err != nil || chatID == 0 {
		s.audit(orgID, userID, 0, AuditDestinationConnected, "resolve_failed", ref)
		return nil, "resolve_failed"
	}
	id, err := s.store.UpsertDestination(tgstore.Destination{
		OrgID: orgID, DestinationType: "channel", ChatID: chatID, Title: title, Username: username,
		Status: "active", EventTypes: defaultEventTypesJSON(), ChannelFilter: "all", ConnectedByUserID: userID,
	})
	if err != nil {
		return nil, "error"
	}
	s.audit(orgID, userID, 0, AuditDestinationConnected, "ok", title)
	d, _ := s.store.GetDestination(orgID, id)
	return d, ""
}

// HandleChannelPost is the PRIVATE-channel connect path: the admin posts a one-time connect code in
// the channel; the bot (an admin there) receives the channel_post and matches the code to an org,
// then stores the channel as a destination. Non-matching posts are ignored silently.
func (s *Service) HandleChannelPost(chatID int64, title, username, text string) {
	cmd, arg := ParseCommand(text)
	code := arg
	if cmd != "connect" {
		code = strings.TrimSpace(text) // allow posting the bare code too
	}
	orgID, userID, ok, err := s.store.ConsumeBindCode(strings.ToUpper(strings.TrimSpace(code)))
	if err != nil || !ok {
		return
	}
	_, err = s.store.UpsertDestination(tgstore.Destination{
		OrgID: orgID, DestinationType: "channel", ChatID: chatID, Title: title, Username: username,
		Status: "active", EventTypes: defaultEventTypesJSON(), ChannelFilter: "all", ConnectedByUserID: userID,
	})
	if err != nil {
		return
	}
	s.audit(orgID, userID, 0, AuditDestinationConnected, "ok_channel_post", title)
	_ = s.tg.Send(chatID, render.ChannelConnected())
}

// TestDestination sends a test message to a destination and records the delivery result. Audited.
func (s *Service) TestDestination(orgID, id int64) (bool, string) {
	d, err := s.store.GetDestination(orgID, id)
	if err != nil || d == nil {
		return false, "not_found"
	}
	sendErr := s.tg.Send(d.ChatID, render.TestMessage())
	_ = s.store.RecordDelivery(orgID, id, sendErr == nil, errText(sendErr))
	if sendErr != nil {
		s.audit(orgID, 0, 0, AuditDestinationTest, "failed", d.Title)
		return false, "delivery_failed"
	}
	s.audit(orgID, 0, 0, AuditDestinationTest, "sent", d.Title)
	return true, ""
}

// SetDestinationPreferences validates + stores a destination's event subscriptions + channel
// filter. Audited.
func (s *Service) SetDestinationPreferences(orgID, id int64, eventTypes []string, channelFilter string) (bool, string) {
	if d, _ := s.store.GetDestination(orgID, id); d == nil {
		return false, "not_found"
	}
	raw, _ := json.Marshal(SanitizeEventTypes(eventTypes))
	if err := s.store.UpdateDestinationPreferences(orgID, id, string(raw), NormalizeChannelFilter(channelFilter)); err != nil {
		return false, "error"
	}
	s.audit(orgID, 0, 0, AuditDestinationPrefs, "ok", "")
	return true, ""
}

// DisableDestination soft-disables a destination. Audited.
func (s *Service) DisableDestination(orgID, id int64) (bool, string) {
	if d, _ := s.store.GetDestination(orgID, id); d == nil {
		return false, "not_found"
	}
	if err := s.store.DisableDestination(orgID, id); err != nil {
		return false, "error"
	}
	s.audit(orgID, 0, 0, AuditDestinationDisabled, "ok", "")
	return true, ""
}

// ListDestinations returns an org's destinations.
func (s *Service) ListDestinations(orgID int64) ([]tgstore.Destination, error) {
	return s.store.ListDestinations(orgID)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
