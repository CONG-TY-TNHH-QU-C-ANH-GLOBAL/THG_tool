package finalize

import (
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/runtime"
)

// Resolvers for Telegram CHANNEL notification fields. Best-effort — a lookup miss degrades to a
// safe fallback label, never blocks the response. No secrets are read.

func (h *Handler) orgName(orgID int64) string {
	if org, _ := h.db.GetOrganization(orgID); org != nil {
		return org.Name
	}
	return ""
}

func (h *Handler) agentName(accountID int64) string {
	if acc, _ := h.db.Identities().GetAccount(accountID); acc != nil {
		if n := strings.TrimSpace(acc.FBDisplayName); n != "" {
			return "Facebook " + n
		}
		if n := strings.TrimSpace(acc.Name); n != "" {
			return n
		}
	}
	return "#" + strconv.FormatInt(accountID, 10)
}

// failureReasonText prefers the extension's granular failure note, falling back to the outcome code.
func failureReasonText(report runtime.ExtensionExecutionReport, fallback string) string {
	if strings.TrimSpace(report.FailureReason) != "" {
		return report.FailureReason
	}
	return fallback
}
