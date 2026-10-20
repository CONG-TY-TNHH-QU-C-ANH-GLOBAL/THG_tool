package assets

import (
	"encoding/json"
	"strings"
)

// ReplyKnowledgeV1 is an explicit customer-safe projection, not a raw SOP.
// Approval still belongs to Asset.State; provenance is not approval.
type ReplyKnowledgeV1 struct {
	Schema       string   `json:"schema"`
	Audience     string   `json:"audience"`
	Verification string   `json:"verification"`
	Facts        []string `json:"facts"`
	SourceIDs    []string `json:"source_ids"`
}

// ReplyKnowledgeFacts rejects legacy/untyped payloads and internal guidance.
// Callers must additionally enforce org, state, source health and asset type.
func ReplyKnowledgeFacts(payload json.RawMessage) (ReplyKnowledgeV1, bool) {
	var knowledge ReplyKnowledgeV1
	if json.Unmarshal(payload, &knowledge) != nil || knowledge.Schema != "company_reply_v1" ||
		knowledge.Audience != "public_reference" || knowledge.Verification != "public_source_checked" {
		return ReplyKnowledgeV1{}, false
	}
	if len(knowledge.Facts) == 0 || len(knowledge.Facts) > 8 || len(knowledge.SourceIDs) == 0 {
		return ReplyKnowledgeV1{}, false
	}
	for _, text := range append(append([]string{}, knowledge.Facts...), knowledge.SourceIDs...) {
		if strings.TrimSpace(text) == "" || len([]rune(text)) > 1000 {
			return ReplyKnowledgeV1{}, false
		}
	}
	return knowledge, true
}
