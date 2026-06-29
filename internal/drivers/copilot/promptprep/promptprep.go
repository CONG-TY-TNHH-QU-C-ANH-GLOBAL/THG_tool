// Package promptprep holds copilot prompt-preprocessing helpers — normalization of the
// raw Copilot prompt before classification/inference. Pure (stdlib `strings` only) and a
// neutral leaf, so the copilot intent classifier and the agent/routing layers can all
// depend on it without depending on each other. Distinct from textnorm (generic
// Vietnamese folding/matching): these helpers know about copilot prompt conventions
// (e.g. the appended "Dashboard context:" block). Extracted under ARCHCP3 so the eventual
// copilot/intent subpackage reaches prompt-prep via a leaf, not via package copilot.
package promptprep

import "strings"

// StripDashboardContext drops the appended "Dashboard context:" block so the classifier
// sees only the user's own words, and trims surrounding whitespace.
func StripDashboardContext(prompt string) string {
	marker := "\n\nDashboard context:"
	if idx := strings.Index(prompt, marker); idx >= 0 {
		return strings.TrimSpace(prompt[:idx])
	}
	return strings.TrimSpace(prompt)
}
