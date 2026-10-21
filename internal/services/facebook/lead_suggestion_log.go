package facebook

import (
	"log"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Suggestion gates recorded per lead, so a notice without "Gợi ý trả lời"
// can be explained from the service journal.
const (
	SuggestionGateDisabled   = "disabled"
	SuggestionGateNotAllowed = "org_not_allowed"
	SuggestionGateBusy       = "runner_busy"
	SuggestionGateAttempted  = "attempted"
)

// LogLeadSuggestionOutcome records why a lead notice has or lacks a draft.
// It logs no post text, author or reply: only the org, the gate, whether a
// reply exists and the fixed operator status note.
func LogLeadSuggestionOutcome(path string, orgID, leadID int64, gate string, suggestion models.LeadSuggestion, eventImages int) {
	log.Printf("[LeadSuggestion] path=%s org_id=%d lead_id=%d gate=%s analysis=%s wording=%s reply=%t supplier=%t product=%t event_images=%d model_images=%d image_analysis=%s note=%q",
		path, orgID, leadID, gate, suggestion.AnalysisStatus, suggestion.ReplyStatus, strings.TrimSpace(suggestion.Reply) != "", suggestion.Supplier.HasOffer(),
		strings.TrimSpace(suggestion.ProductURL) != "", boundedImageCount(eventImages), boundedImageCount(suggestion.ImageInputs), imageAnalysisLogCode(suggestion.ImageAnalysis), suggestion.SourcingNote)
}

func boundedImageCount(count int) int {
	if count < 0 {
		return 0
	}
	if count > 3 {
		return 3
	}
	return count
}

func imageAnalysisLogCode(status string) string {
	switch status {
	case "none", "references_rejected", "analysis_failed", "observations", "no_observations", "conflict":
		return status
	default:
		return "not_recorded"
	}
}

// Image audit state describes transport/result shape, never visual accuracy.
func pendingImageStatus(received, submitted int) string {
	if submitted > 0 {
		return "analysis_failed"
	}
	if received > 0 {
		return "references_rejected"
	}
	return "none"
}

func completedImageStatus(pending string, submitted int, hasObservations, conflict bool) string {
	if submitted == 0 {
		return pending
	}
	if conflict {
		return "conflict"
	}
	if hasObservations {
		return "observations"
	}
	return "no_observations"
}
