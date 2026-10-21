package leadcontext

import (
	"encoding/json"
	"strings"
)

// ImageEvidence identifies visible request text in a supplied original image.
// It authorizes tentative interpretation only, never pricing or route inputs.
type ImageEvidence struct {
	ImageIndex int    `json:"image_index"`
	Quote      string `json:"quote"`
}

func validImageEvidence(raw json.RawMessage, imageCount int, observations []string) bool {
	if raw == nil {
		return true // Existing text-evidence responses remain compatible.
	}
	var entries []json.RawMessage
	if string(raw) == "null" || json.Unmarshal(raw, &entries) != nil || len(entries) > 4 {
		return false
	}
	seen := map[ImageEvidence]bool{}
	for _, entry := range entries {
		item, ok := imageEvidenceEntry(entry, imageCount, strings.Join(observations, "\n"))
		if !ok || seen[item] {
			return false
		}
		seen[item] = true
	}
	return true
}

func imageEvidenceEntry(raw json.RawMessage, imageCount int, observedText string) (ImageEvidence, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 2 || fields["image_index"] == nil || fields["quote"] == nil {
		return ImageEvidence{}, false
	}
	var item ImageEvidence
	if string(fields["image_index"]) == "null" || json.Unmarshal(raw, &item) != nil {
		return ImageEvidence{}, false
	}
	valid := item.ImageIndex >= 0 && item.ImageIndex < imageCount && validEvidence([]string{item.Quote}, observedText)
	return item, valid
}
