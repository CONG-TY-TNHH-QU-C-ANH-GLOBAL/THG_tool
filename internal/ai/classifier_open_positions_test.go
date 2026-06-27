package ai

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// formatOpenPositions renders the recruitment OPEN POSITIONS prompt block. It
// must include the header/footer, omit empty location/requirements, clip long
// requirements to 150 chars, and return "" for no jobs.
func TestFormatOpenPositions(t *testing.T) {
	if got := formatOpenPositions(nil); got != "" {
		t.Fatalf("no jobs must yield empty string, got %q", got)
	}

	long := strings.Repeat("x", 200)
	out := formatOpenPositions([]models.CareerJob{
		{Title: "Backend Engineer", Location: "HCM", Requirements: long},
		{Title: "Recruiter"}, // no location / requirements
	})

	if !strings.Contains(out, "OPEN POSITIONS:") || !strings.Contains(out, "Score candidates based on fit") {
		t.Fatalf("missing header/footer: %q", out)
	}
	if !strings.Contains(out, "- Backend Engineer (HCM): ") {
		t.Fatalf("first job line malformed: %q", out)
	}
	if strings.Contains(out, strings.Repeat("x", 151)) {
		t.Fatalf("requirements must be clipped to 150 chars: %q", out)
	}
	// The bare-title job must not gain an empty "()" or trailing ": ".
	if !strings.Contains(out, "- Recruiter\n") {
		t.Fatalf("bare-title job line malformed: %q", out)
	}
}
