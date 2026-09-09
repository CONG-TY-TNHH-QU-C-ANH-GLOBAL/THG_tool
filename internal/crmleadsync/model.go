// Package crmleadsync owns the durable local outbox for CRM lead projection.
package crmleadsync

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/models"
)

const sourceSystem = "THG_TOOL"

type payload struct {
	SchemaVersion    int        `json:"schemaVersion"`
	EventID          string     `json:"eventId"`
	EventType        string     `json:"eventType"`
	EventKey         string     `json:"eventKey"`
	OccurredAt       string     `json:"occurredAt"`
	SourceSystem     string     `json:"sourceSystem"`
	OrgID            int64      `json:"orgId"`
	AuthorName       string     `json:"authorName"`
	AuthorProfileURL string     `json:"authorProfileUrl"`
	SourceURL        string     `json:"sourceUrl"`
	PostID           string     `json:"postId"`
	Excerpt          string     `json:"excerpt"`
	Score            float64    `json:"score"`
	Category         string     `json:"category"`
	AIReason         string     `json:"aiReason"`
	Enrichment       enrichment `json:"enrichment"`
}

// enrichment is the immutable operator-facing snapshot rendered by both
// Telegram and CRM. It is deliberately copied from the caller's already-built
// suggestion; this package never does retrieval or generation itself.
type enrichment struct {
	SuggestedReply  string    `json:"suggestedReply"`
	ProductName     string    `json:"productName"`
	ProductURL      string    `json:"productUrl"`
	ProductImageURL string    `json:"productImageUrl"`
	Supplier        *supplier `json:"supplier,omitempty"`
}

// supplier is the sourced 1688 / Taobao item behind the same snapshot. It is a
// separate block from the catalog product on purpose: the CRM must be able to
// tell "what we sell" apart from "what we can source", and the numbers here are
// supplier-side, not customer-facing pricing.
type supplier struct {
	Platform   string `json:"platform"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	ImageURL   string `json:"imageUrl,omitempty"`
	PriceText  string `json:"priceText,omitempty"`
	WeightText string `json:"weightText,omitempty"`
	MOQText    string `json:"moqText,omitempty"`
	ShipFrom   string `json:"shipFrom,omitempty"`
	ShopName   string `json:"shopName,omitempty"`
	CapturedAt string `json:"capturedAt,omitempty"`
}

// supplierFrom copies a matched supplier item into the wire block. Returns nil
// when the match lacks a name or link, so a partial match never reaches the CRM.
func supplierFrom(match *models.SupplierMatch) *supplier {
	if !match.HasOffer() {
		return nil
	}
	return &supplier{
		Platform: strings.TrimSpace(match.Platform), Name: strings.TrimSpace(match.Name),
		URL: strings.TrimSpace(match.URL), ImageURL: strings.TrimSpace(match.ImageURL),
		PriceText: strings.TrimSpace(match.PriceText), WeightText: strings.TrimSpace(match.WeightKG),
		MOQText: strings.TrimSpace(match.MOQText), ShipFrom: strings.TrimSpace(match.ShipFrom),
		ShopName: strings.TrimSpace(match.ShopName), CapturedAt: strings.TrimSpace(match.CapturedAt),
	}
}

func payloadFor(event leadingest.LeadEvent, suggestion models.LeadSuggestion) (payload, bool) {
	if event.OrgID <= 0 || strings.TrimSpace(event.PostURL) == "" || strings.TrimSpace(event.AuthorName) == "" {
		return payload{}, false
	}
	key := sourceSystem + ":" + itoa(event.OrgID) + ":" + digest(event.PostURL)
	occurredAt := event.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return payload{
		SchemaVersion: 1, EventID: key, EventType: "lead.qualified", EventKey: key,
		OccurredAt: occurredAt.Format(time.RFC3339Nano), SourceSystem: sourceSystem,
		OrgID: event.OrgID, AuthorName: strings.TrimSpace(event.AuthorName),
		AuthorProfileURL: strings.TrimSpace(event.AuthorProfileURL), SourceURL: strings.TrimSpace(event.PostURL),
		PostID: strings.TrimSpace(event.PostFBID), Excerpt: strings.TrimSpace(event.Excerpt),
		Score: event.Score, Category: strings.TrimSpace(event.Category), AIReason: strings.TrimSpace(event.Reason),
		Enrichment: enrichment{
			SuggestedReply: strings.TrimSpace(suggestion.Reply), ProductName: strings.TrimSpace(suggestion.ProductName),
			ProductURL: strings.TrimSpace(suggestion.ProductURL), ProductImageURL: strings.TrimSpace(suggestion.ProductImageURL),
			Supplier: supplierFrom(suggestion.Supplier),
		},
	}, true
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func itoa(value int64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf[i:])
}
