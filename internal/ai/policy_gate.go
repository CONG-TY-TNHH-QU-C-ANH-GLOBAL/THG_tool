package ai

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Policy Gate (P2d — COMMENT_INTELLIGENCE_PIPELINE §6 / SaaS UX
// Hardening PR-7). Sits between the GROUNDED decision and generation:
// it never adds content (grounding already guarantees nothing invented)
// — it only SHAPES what the prompt may pitch, by confidence + org policy.

// CommentPolicies are the org-scoped knobs (org:{id}:* user_context keys).
type CommentPolicies struct {
	AllowPriceInComments       bool    // default true
	AllowProductLinkInComments bool    // default false (catalog product links; company website is PR-6's separate rule)
	MinConfidenceToQuotePrice  float64 // default 0.7
}

func DefaultCommentPolicies() CommentPolicies {
	return CommentPolicies{
		AllowPriceInComments:       true,
		AllowProductLinkInComments: false,
		MinConfidenceToQuotePrice:  0.7,
	}
}

// LoadOrgCommentPolicies reads the org's policy keys, falling back to
// defaults per key (absent/blank = default, never an error).
func LoadOrgCommentPolicies(db *store.Store, orgID int64) CommentPolicies {
	p := DefaultCommentPolicies()
	read := func(key string) string {
		v, err := db.Leads().GetContext(fmt.Sprintf("org:%d:%s", orgID, key))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(v)
	}
	if v := read("allow_price_in_comments"); v != "" {
		p.AllowPriceInComments = !strings.EqualFold(v, "false")
	}
	if v := read("allow_product_link_in_comments"); v != "" {
		p.AllowProductLinkInComments = strings.EqualFold(v, "true")
	}
	if v := read("min_confidence_to_quote_price"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			p.MinConfidenceToQuotePrice = f
		}
	}
	return p
}

// Gate modes (closed set).
const (
	GateProductPitch    = "product_pitch"    // high confidence: product name (+price if allowed)
	GateCategoryMention = "category_mention" // medium: service/category only, no exact price, inbox-for-quote
	GateGeneric         = "generic"          // low/no confidence: generic service comment
)

// GateVerdict is the pure gate output.
type GateVerdict struct {
	Mode       string
	AllowPrice bool
}

// EvaluateGate classifies one grounded decision against the policy. PURE.
func EvaluateGate(d *models.CommentDecision, p CommentPolicies) GateVerdict {
	if d == nil || d.KnowledgeGap || !d.Selected.HasOffer() {
		return GateVerdict{Mode: GateGeneric}
	}
	if len(d.Selected.Products) == 0 {
		// Service-only grounding: capabilities/proofs drive the pitch;
		// price never applies without a product.
		return GateVerdict{Mode: GateCategoryMention}
	}
	if d.Confidence >= p.MinConfidenceToQuotePrice {
		return GateVerdict{Mode: GateProductPitch, AllowPrice: p.AllowPriceInComments}
	}
	return GateVerdict{Mode: GateCategoryMention}
}

// ApplyGate SHAPES the decision per verdict — strictly subtractive
// (drop products / clear prices / flag gap); it can never reintroduce
// anything grounding removed, so hallucination guarantees are intact.
//
//   - generic          → KnowledgeGap=true (caller falls back to the
//     generic service comment — the honest degrade).
//   - category_mention → products dropped (service/category mention
//     only, "inbox để báo giá" comes from CTA), prices cleared.
//   - product_pitch    → prices cleared unless AllowPrice.
//
// Catalog product LINKS: GroundedItem carries no product URL today, so
// allow_product_link_in_comments=false is structurally enforced; the
// flag exists for when product URLs join the grounded shape.
func ApplyGate(d *models.CommentDecision, v GateVerdict) *models.CommentDecision {
	if d == nil {
		return nil
	}
	out := *d
	switch v.Mode {
	case GateGeneric:
		out.KnowledgeGap = true
		return &out
	case GateCategoryMention:
		out.Selected.Products = nil
		if !out.Selected.HasOffer() {
			out.KnowledgeGap = true // nothing left to say specifically → generic
			return &out
		}
	}
	if !v.AllowPrice {
		out.Selected.Products = clearPrices(out.Selected.Products)
		out.Selected.Capabilities = clearPrices(out.Selected.Capabilities)
		out.Selected.Proofs = clearPrices(out.Selected.Proofs)
	}
	return &out
}

func clearPrices(items []models.GroundedItem) []models.GroundedItem {
	if len(items) == 0 {
		return items
	}
	out := make([]models.GroundedItem, len(items))
	copy(out, items)
	for i := range out {
		out[i].PriceText = ""
	}
	return out
}
