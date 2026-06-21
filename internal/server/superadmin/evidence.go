package superadmin

import "strings"

// extractEvidenceField pulls one string field out of a proof JSON blob
// without importing encoding/json — handles the simple
// `"<field>":"<value>"` pattern that ClassifyExtensionReport emits.
// Returns empty when field absent.
func extractEvidenceField(blob, field string) string {
	needle := `"` + field + `":`
	i := strings.Index(blob, needle)
	if i < 0 {
		return ""
	}
	rest := blob[i+len(needle):]
	rest = strings.TrimLeft(rest, " ")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	for j := 0; j < len(rest); j++ {
		if rest[j] == '\\' && j+1 < len(rest) {
			j++
			continue
		}
		if rest[j] == '"' {
			return rest[:j]
		}
	}
	return ""
}
