package copilot

import (
	"github.com/thg/scraper/internal/drivers/copilot/promptprep"
	"github.com/thg/scraper/internal/drivers/copilot/textnorm"
)

// Copilot intent — text-normalization shims. The real helpers live in neutral leaves:
// generic Vietnamese folding/matching in internal/drivers/copilot/textnorm, and copilot
// prompt preprocessing in internal/drivers/copilot/promptprep (ARCHCP3). These thin
// package-local delegations keep the existing agent_* / routing_* call sites unchanged;
// the copilot/intent files already call the leaves directly. The shims migrate away
// when the remaining agent_* callers do.

// stripDashboardContext delegates to promptprep.StripDashboardContext (shim).
func stripDashboardContext(prompt string) string {
	return promptprep.StripDashboardContext(prompt)
}

// foldVietnameseForMatch delegates to textnorm.Fold (shim).
func foldVietnameseForMatch(value string) string {
	return textnorm.Fold(value)
}

// containsAnyFolded delegates to textnorm.ContainsAny (shim).
func containsAnyFolded(value string, needles []string) bool {
	return textnorm.ContainsAny(value, needles)
}
