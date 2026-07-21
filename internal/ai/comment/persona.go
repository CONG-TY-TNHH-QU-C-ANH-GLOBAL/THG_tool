package comment

import (
	"strings"

	"github.com/thg/scraper/internal/models"
)

// BuildPersonaRule turns an ActorPersona (spec: specs/domains/facebook-sales-intelligence/features/multi-actor-coverage/technical.md) into a
// prompt rule so a SECOND account covering the same lead adds a DIFFERENT angle —
// no repeated website, no repeated hard CTA, no repeated phrasing. Empty persona
// (a fresh lead / first actor) adds nothing. Numbered 9 to follow the contact rule.
//
// Pure platform-neutral comment intelligence (Phase B): models + stdlib only.
// Exported on the package split so the ai message generator (msggen.go) can call it.
func BuildPersonaRule(p models.ActorPersona) string {
	if p.Role == "" && p.Tone == "" && p.LinkPolicy == "" && p.AllowedCTAStyle == "" && len(p.ForbiddenRepeatedPhrases) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("9. MULTI-ACTOR COVERAGE: a teammate may have already commented this lead — your comment MUST add a DIFFERENT angle, not repeat theirs.")
	if p.Role != "" {
		b.WriteString(" Write as a " + p.Role + ".")
	}
	if p.Tone != "" {
		b.WriteString(" Tone: " + p.Tone + ".")
	}
	if p.LinkPolicy == models.LinkNoLink {
		b.WriteString(" Do NOT include any website/URL — a teammate already shared it.")
	}
	if p.AllowedCTAStyle == models.CTAExperienceShare {
		b.WriteString(" Do NOT use a hard inbox CTA; instead share concrete sourcing/fulfillment EXPERIENCE or a useful tip that builds trust — a light sign-off is fine.")
	}
	if len(p.ForbiddenRepeatedPhrases) > 0 {
		b.WriteString(" Avoid repeating these angles already used: " + strings.Join(p.ForbiddenRepeatedPhrases, "; ") + ".")
	}
	return b.String()
}
