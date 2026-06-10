package ai

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestBuildPersonaRule(t *testing.T) {
	// First actor (empty persona) → no rule.
	if r := buildPersonaRule(models.ActorPersona{}); r != "" {
		t.Errorf("empty persona should add no rule, got %q", r)
	}
	// Later actor: no link + experience-share + avoid used angles.
	p := models.ActorPersona{
		LinkPolicy:               models.LinkNoLink,
		AllowedCTAStyle:          models.CTAExperienceShare,
		ForbiddenRepeatedPhrases: []string{"price_focus"},
		Role:                     "fulfillment advisor",
	}
	r := buildPersonaRule(p)
	for _, want := range []string{"DIFFERENT angle", "Do NOT include any website", "Do NOT use a hard inbox CTA", "price_focus", "fulfillment advisor"} {
		if !strings.Contains(r, want) {
			t.Errorf("persona rule missing %q, got: %q", want, r)
		}
	}
}
