package facebook

import "github.com/thg/scraper/internal/models"

// SyntheticLeadFromActionArgs builds the Facebook-specific synthetic lead that a
// prompt-targeted outbound action acts on when the target is a Facebook post/profile
// URL rather than an existing stored lead. It owns the FB conventions — Platform =
// Facebook, the "prompt_target" source type, and the comment-vs-other field mapping —
// so the neutral orchestrator (cmd/scraper) no longer hardcodes them.
//
// Behavior is identical to the inline shaping it replaces and is pinned by
// cmd/scraper/leads_from_action_args_test.go (and the focused test beside this file):
//   - msgType == "comment": first non-empty of postURL/targetURL → SourceURL, with the
//     action's author_url arg → AuthorURL. Empty target → no synthetic lead (ok=false).
//   - otherwise: targetURL → AuthorURL (author_url arg unused). Empty → ok=false.
//
// A false result means "no prompt target here" — the caller falls through to its
// normal lead resolution. This function is PURE (imports only models): no IO, no store.
func SyntheticLeadFromActionArgs(orgID int64, msgType, postURL, targetURL, targetName, authorURL, content string) (models.Lead, bool) {
	if msgType == "comment" {
		target := postURL
		if target == "" {
			target = targetURL
		}
		if target == "" {
			return models.Lead{}, false
		}
		return models.Lead{
			OrgID:      orgID,
			SourceURL:  target,
			Author:     targetName,
			AuthorURL:  authorURL,
			Content:    content,
			Score:      models.LeadHot,
			Platform:   models.PlatformFacebook,
			SourceType: "prompt_target",
		}, true
	}
	if targetURL == "" {
		return models.Lead{}, false
	}
	return models.Lead{
		OrgID:      orgID,
		AuthorURL:  targetURL,
		Author:     targetName,
		Content:    content,
		Score:      models.LeadHot,
		Platform:   models.PlatformFacebook,
		SourceType: "prompt_target",
	}, true
}
