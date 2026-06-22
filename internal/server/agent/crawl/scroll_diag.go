// Package crawl owns read-only crawl diagnostics extracted from the agent
// package — currently the typed, panic-safe view of the Chrome extension's
// scroll_diag payload. It does NOT perform crawl ingestion, execution, or any
// DB/connector mutation; those stay in the agent package. The package contains
// no runtime/wiring and imports no agent internals.
package crawl

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ConnectorScrollDiag is the typed, panic-safe view of the extension's scroll_diag payload.
// The wire form is map[string]any decoded from JSON, so a numeric field can arrive as float64
// (the default), json.Number, int (in tests), or even a string from an old/buggy extension —
// and any field can be missing or null. NormalizeConnectorScrollDiag coerces all of that to
// stable Go types instead of blind type assertions that would panic in production.
type ConnectorScrollDiag struct {
	Passes            int
	MaxArticlesSeen   int
	MaxScrollY        int
	MaxDocHeight      int
	ScrollMovedEver   bool
	FinalScrollTarget string
	LandedURL         string
}

// NormalizeConnectorScrollDiag converts the raw scroll_diag map into a ConnectorScrollDiag.
// A nil/empty map yields the zero value (all zero/false/""), never a panic.
func NormalizeConnectorScrollDiag(diag map[string]any) ConnectorScrollDiag {
	return ConnectorScrollDiag{
		Passes:            scrollDiagInt(diag, "passes"),
		MaxArticlesSeen:   scrollDiagInt(diag, "max_articles_seen"),
		MaxScrollY:        scrollDiagInt(diag, "max_scroll_y"),
		MaxDocHeight:      scrollDiagInt(diag, "max_doc_height"),
		ScrollMovedEver:   scrollDiagBool(diag, "scroll_moved_ever"),
		FinalScrollTarget: scrollDiagString(diag, "final_scroll_target"),
		LandedURL:         scrollDiagString(diag, "landed_url"),
	}
}

// scrollDiagInt reads key as an int, supporting every JSON/Go numeric form the extension might
// send. Missing / null / unparseable → 0 (never panics).
func scrollDiagInt(diag map[string]any, key string) int {
	if diag == nil {
		return 0
	}
	switch v := diag[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
		if f, err := v.Float64(); err == nil {
			return int(f)
		}
		return 0
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}

// scrollDiagBool reads key as a bool. Accepts a real bool or a "true"/"1" string. Missing /
// null / unparseable → false (never panics).
func scrollDiagBool(diag map[string]any, key string) bool {
	if diag == nil {
		return false
	}
	switch v := diag[key].(type) {
	case bool:
		return v
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		return err == nil && b
	default:
		return false
	}
}

// scrollDiagString reads key as a string. Missing / null / non-string → "" (never panics).
func scrollDiagString(diag map[string]any, key string) string {
	if diag == nil {
		return ""
	}
	if s, ok := diag[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
