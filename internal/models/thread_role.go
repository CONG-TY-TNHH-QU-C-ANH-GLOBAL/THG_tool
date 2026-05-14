package models

import "strings"

// Coordination Plane — Lead Thread Role (project_thread_role_architecture.md
// Phase B). A thread participant is NOT automatically a buyer lead. The
// thread_role axis is orthogonal to lead_score: a vendor responding "ib em
// hỗ trợ" on a buyer's post can score high on buyer-keyword classification
// yet is still a supplier, not a lead.
//
// This file holds the DETERMINISTIC role inference — derived from the
// crawler's source_type, the classifier's intent output, and negative
// vendor-speak signals in the content. A later enhancement may feed the
// speaker-vs-post relation into the LLM directly; until then this rule
// engine is the role source of truth (unit-testable, no LLM dependency).
type LeadThreadRole string

const (
	// ThreadRoleIntentOriginator — the post author declaring a need / asking
	// the question. The real lead. Routes to the source post.
	ThreadRoleIntentOriginator LeadThreadRole = "intent_originator"
	// ThreadRoleSupplierResponder — a vendor offering a service in a comment.
	// NOT a lead. Routes to the comment so we can see the offer.
	ThreadRoleSupplierResponder LeadThreadRole = "supplier_responder"
	// ThreadRoleBuyerResponder — another buyer chiming in on the same post.
	// A weaker lead than the originator but still a lead.
	ThreadRoleBuyerResponder LeadThreadRole = "buyer_responder"
	// ThreadRoleCompetitor — a competing brand pushing their own offer as a
	// post. NOT a lead; should be filtered out of the main list.
	ThreadRoleCompetitor LeadThreadRole = "competitor"
	// ThreadRoleNoise — generic engagement, spam, or off-topic. NOT a lead.
	ThreadRoleNoise LeadThreadRole = "noise"
)

// IsLeadRole reports whether a thread role represents an actual sales lead
// (someone with buying intent) vs a vendor / competitor / noise participant.
// The dashboard uses this to keep the "Khách quan tâm" surface clean.
func (r LeadThreadRole) IsLeadRole() bool {
	return r == ThreadRoleIntentOriginator || r == ThreadRoleBuyerResponder
}

// vendorSpeakSignals are phrases a vendor / supplier uses when responding to
// a seeking-post. Vietnamese-first since the workspace operates on VN groups;
// a few English equivalents for mixed-language groups. Kept high-signal —
// false positives here misclassify a real buyer as a vendor.
var vendorSpeakSignals = []string{
	"ib em", "ib mình", "ib shop", "inbox em", "inbox mình", "inbox shop",
	"bên mình", "bên em", "shop mình", "shop em", "bên shop",
	"mình nhận", "em nhận", "nhận làm", "nhận order", "nhận fulfill", "nhận sỉ",
	"mình có mẫu", "em có mẫu", "mình có hàng", "có sẵn mẫu",
	"báo giá cho", "liên hệ em", "liên hệ mình", "tư vấn cho",
	"ib em hỗ trợ", "ib mình hỗ trợ", "em hỗ trợ nhé", "mình hỗ trợ nhé",
	"we offer", "we provide", "we supply", "dm me", "contact us", "our service",
}

// hasVendorSpeak reports whether the content reads like a vendor pitch
// rather than a buyer's need. Case-insensitive substring match.
func hasVendorSpeak(content string) bool {
	lower := strings.ToLower(content)
	for _, sig := range vendorSpeakSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// InferThreadRole derives the thread role from the three signals available
// at ingest time:
//   - sourceType: "post" | "comment" (the crawler's structural classification)
//   - intent:     the classifier's intent label (potential_customer, candidate,
//                 partner, provider_ad, not_relevant, spam — or "" / legacy
//                 buyer/seller values)
//   - content:    the participant's utterance, scanned for vendor-speak
//
// The vendor-speak signal OVERRIDES a buyer-leaning intent for comments —
// that is the exact failure the user reported (a vendor comment tagged
// "khách quan tâm"). The classifier sees only the words; the role engine
// sees the words AND the speaker's structural position.
func InferThreadRole(sourceType, intent, content string) LeadThreadRole {
	st := strings.ToLower(strings.TrimSpace(sourceType))
	it := strings.ToLower(strings.TrimSpace(intent))
	vendor := hasVendorSpeak(content)

	// Spam is noise regardless of structural position.
	if it == "spam" {
		return ThreadRoleNoise
	}

	isComment := st == "comment"

	if isComment {
		// A vendor responding in a comment is a supplier — even if the
		// classifier mistook the buyer-keyword-heavy pitch for buyer intent.
		if vendor || it == "provider_ad" || it == "partner" {
			return ThreadRoleSupplierResponder
		}
		if it == "not_relevant" {
			return ThreadRoleNoise
		}
		// Buyer-leaning or unknown comment → a secondary buyer on the thread.
		return ThreadRoleBuyerResponder
	}

	// Post path.
	if it == "provider_ad" || it == "partner" {
		return ThreadRoleCompetitor
	}
	// A post written in vendor-speak that the classifier did NOT tag as a
	// buyer is a competing offer, not a lead.
	if vendor && it != "potential_customer" && it != "candidate" {
		return ThreadRoleCompetitor
	}
	if it == "not_relevant" {
		return ThreadRoleNoise
	}
	// Default: a post is the originator of the thread's intent.
	return ThreadRoleIntentOriginator
}

// NormalizeThreadRole maps an arbitrary stored string onto a known role.
// Empty / unknown values fall back to intent_originator — the safe default
// for legacy rows crawled before the role axis existed (all of which were
// post-sourced leads).
func NormalizeThreadRole(s string) LeadThreadRole {
	switch LeadThreadRole(strings.ToLower(strings.TrimSpace(s))) {
	case ThreadRoleIntentOriginator:
		return ThreadRoleIntentOriginator
	case ThreadRoleSupplierResponder:
		return ThreadRoleSupplierResponder
	case ThreadRoleBuyerResponder:
		return ThreadRoleBuyerResponder
	case ThreadRoleCompetitor:
		return ThreadRoleCompetitor
	case ThreadRoleNoise:
		return ThreadRoleNoise
	default:
		return ThreadRoleIntentOriginator
	}
}
