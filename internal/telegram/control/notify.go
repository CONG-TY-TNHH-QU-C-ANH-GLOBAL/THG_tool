package control

import (
	"encoding/json"

	"github.com/thg/scraper/internal/telegram/render"
)

// TestNotify sends a test message to the requesting user's OWN active binding(s) in an org. Backs
// the REST test-notification endpoint. Returns a reason the UI can show. Audited.
func (s *Service) TestNotify(orgID, userID int64) (bool, string) {
	if !s.flags.NotifyEnabled {
		return false, "notify_disabled"
	}
	gb := s.globalBot() // personal DM bindings live on the platform/webhook bot
	if gb == nil {
		return false, "bot_token_missing"
	}
	bindings, err := s.store.ListBindingsByUser(orgID, userID)
	if err != nil {
		return false, "error"
	}
	sent := 0
	for _, b := range bindings {
		if b.Status != "active" || b.ChatID == 0 {
			continue
		}
		if e := gb.Send(b.ChatID, render.TestMessage()); e == nil {
			sent++
		}
	}
	if sent == 0 {
		s.audit(orgID, userID, 0, AuditTestNotification, "no_active_binding", "")
		return false, "no_active_binding"
	}
	s.audit(orgID, userID, 0, AuditTestNotification, "sent", "")
	return true, ""
}

// NotifyBoundUsers pushes an alert to an org's active alert-recipient bindings, respecting the
// notify flag + the org's alert preferences (enabled, channel filter, per-type opt-in). Audits the
// delivery result. The message is pre-rendered by the caller. Channel-neutral.
func (s *Service) NotifyBoundUsers(orgID int64, alertType, channel, message string) (int, error) {
	if !s.flags.NotifyEnabled || !IsValidAlertType(alertType) {
		return 0, nil
	}
	prefs, err := s.store.GetAlertPrefs(orgID)
	if err != nil {
		return 0, err
	}
	if !prefs.AlertsEnabled || !alertTypeEnabled(prefs.AlertTypes, alertType) {
		return 0, nil
	}
	if prefs.ChannelFilter != "all" && prefs.ChannelFilter != channel {
		return 0, nil
	}
	gb := s.globalBot()
	if gb == nil {
		return 0, nil
	}
	recipients, err := s.store.ListAlertRecipients(orgID)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, b := range recipients {
		if b.ChatID == 0 {
			continue
		}
		if e := gb.Send(b.ChatID, message); e == nil {
			sent++
		}
	}
	result := AuditNotificationSent
	if sent == 0 {
		result = AuditNotificationFailed
	}
	s.audit(orgID, 0, 0, result, alertType, channel)
	return sent, nil
}

// alertTypeEnabled reports whether an org opted into an alert type. An empty/invalid list means no
// types are selected → no alert is sent (explicit opt-in).
func alertTypeEnabled(alertTypesJSON, alertType string) bool {
	var list []string
	if json.Unmarshal([]byte(alertTypesJSON), &list) != nil {
		return false
	}
	for _, t := range list {
		if t == alertType {
			return true
		}
	}
	return false
}
