// Domain: leads (see internal/store/DOMAINS.md)
package leads

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// leadDedupKey is the stable identity used to de-duplicate automation leads
// across the legacy + task-lead sources: author profile URL first, then the
// source URL, then a synthetic per-id key. Pure (no IO). Extracted from
// GetAutomationLeadsForOrg to keep that merge loop's cognitive load down.
func leadDedupKey(l models.Lead) string {
	if k := strings.TrimSpace(l.AuthorURL); k != "" {
		return k
	}
	if k := strings.TrimSpace(l.SourceURL); k != "" {
		return k
	}
	return fmt.Sprintf("lead:%d", l.ID)
}
