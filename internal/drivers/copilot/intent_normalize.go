package copilot

import (
	"strings"

	"github.com/thg/scraper/internal/drivers/copilot/textnorm"
)

// Copilot intent — text normalization layer. Pure string functions shared by the
// router, the self-sufficiency gates, and business-context inference. No DB /
// outbound / session access.
//
// The generic Vietnamese folding / folded-match helpers moved to the neutral leaf
// internal/drivers/copilot/textnorm (ARCHCP3); foldVietnameseForMatch / containsAnyFolded
// are kept here as thin package-local delegations so the ~42 existing copilot call sites
// stay unchanged. They migrate to textnorm.* directly in the follow-up that creates the
// copilot/intent subpackage. stripDashboardContext stays here — it strips the copilot
// "Dashboard context:" prompt marker, which is copilot-specific, not generic text-norm.

// stripDashboardContext drops the appended "Dashboard context:" block so the
// classifier sees only the user's own words, and trims surrounding whitespace.
func stripDashboardContext(prompt string) string {
	marker := "\n\nDashboard context:"
	if idx := strings.Index(prompt, marker); idx >= 0 {
		return strings.TrimSpace(prompt[:idx])
	}
	return strings.TrimSpace(prompt)
}

// foldVietnameseForMatch delegates to textnorm.Fold (temporary shim — see file header).
func foldVietnameseForMatch(value string) string {
	return textnorm.Fold(value)
}

// containsAnyFolded delegates to textnorm.ContainsAny (temporary shim — see file header).
func containsAnyFolded(value string, needles []string) bool {
	return textnorm.ContainsAny(value, needles)
}
