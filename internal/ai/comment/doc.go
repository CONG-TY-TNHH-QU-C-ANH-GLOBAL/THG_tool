// Package comment owns reusable, platform-neutral comment intelligence.
//
// It is the intelligence layer for comments across any automation surface
// (Facebook first, later Instagram / TikTok / Telegram / marketplaces): the
// rules here reason about TEXT and a grounded company identity, never about a
// specific platform's runtime.
//
// This package OWNS:
//   - quality checks (SanitizeComment: dedupe, length, placeholder subreasons)
//   - contact/contact-URL screening (ScreenCommentContacts)
//   - comment repair (RepairCommentContacts: collapse duplicate grounded URLs,
//     strip non-grounded links, convert an official t.me link to its @handle)
//   - brand URL validation (CanonicalWebsite, RepairWebsiteMentions, host
//     anchoring so a lookalike like thgfulfill.com.evil.com is rejected)
//   - duplicate text detection (DetectRepeatedText)
//
// This package does NOT own, and MUST NOT import:
//   - Facebook (or any platform) session / account / actor readiness
//   - Facebook composer execution or URL parsing
//   - outbound queue persistence / dashboard eligibility
//   - agent routing, connector / browser runtime, server transport
//
// Dependency rule (binding — see specs/COMPONENT_STRUCTURE_RULES.md): this
// package may import only platform-neutral types (internal/models) and the
// standard library. It must NOT import internal/store, internal/server,
// internal/platform*, connector/outbound, or any Facebook-specific package.
// Platform-specific and orchestration logic depend on THIS package, never the
// reverse.
//
// Facade: callers use the exported verbs (SanitizeComment, ScreenCommentContacts,
// RepairCommentContacts, DetectRepeatedText, CanonicalWebsite,
// RepairWebsiteMentions); the regex/normalization helpers stay unexported.
package comment
