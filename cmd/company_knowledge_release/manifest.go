// Command company_knowledge_release publishes one explicitly reviewed public
// release through the knowledge store owner. It never crawls or sends messages.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Frozen review bytes (LF normalized), approved by task owner in chat on this
// date. Changing content requires another review and a separate release.
const releaseHash = "ed882debb485e5e55eeb3829ad9aaf05c548d1cfb4fc318bd92ae966c2aa4410"
const approvalReference = "task-owner-chat-2026-10-21/eight-public-fact-projections/" + releaseHash
const releaseLabel = "Reviewed public facts 2026-10-21 " + releaseHash

type releaseRecord struct {
	ID      string           `json:"id"`
	Title   string           `json:"title"`
	Type    assets.AssetType `json:"asset_type"`
	Payload json.RawMessage  `json:"payload"`
	Sources json.RawMessage  `json:"sources"`
}

func loadRelease(path string) ([]releaseRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("release manifest unreadable")
	}
	raw = []byte(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != releaseHash {
		return nil, errors.New("release manifest differs from owner-approved review")
	}
	var doc struct {
		Schema  string          `json:"schema"`
		Records []releaseRecord `json:"records"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Schema != "company_reply_release_review_v1" || len(doc.Records) != 8 {
		return nil, errors.New("invalid eight-record release")
	}
	return doc.Records, nil
}

// Search metadata only; these existing pack aliases add no service claims.
var releaseSearchTerms = map[string][]string{
	"service-pod":            {"print on demand", "POD fulfillment", "custom printing"},
	"service-sourcing":       {"product sourcing", "Taobao sourcing", "1688 supplier", "buying agent"},
	"service-express":        {"international shipping", "international freight", "parcel shipping"},
	"service-warehouse":      {"US warehouse", "US fulfillment", "inventory storage", "pick and pack"},
	"shipping-international": {"international rates", "shipping origin destination"},
	"shipping-domestic":      {"US domestic shipping", "3PL fees", "postage"},
	"shipping-formal":        {"Vietnam US formal shipping", "formal shipment pricing"},
}

func materialize(records []releaseRecord, orgID, sourceID int64) []*assets.Asset {
	var items []*assets.Asset
	for _, r := range records {
		facts, _ := assets.ReplyKnowledgeFacts(r.Payload)
		items = append(items, &assets.Asset{OrgID: orgID, SourceID: sourceID,
			ExternalID: r.ID, Type: r.Type, Title: r.Title, Description: strings.Join(facts.Facts, "\n"),
			Tags: releaseSearchTerms[r.ID], Payload: r.Payload, State: assets.StateApproved})
	}
	return items
}
