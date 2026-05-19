package rest_json

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// lookupPath walks a dot-path into a parsed JSON value and returns
// the nested value or false. Examples:
//
//	lookupPath({"a":{"b":1}}, "a.b")   → 1, true
//	lookupPath({"a":1},        "a")    → 1, true
//	lookupPath({"a":1},        "")     → nil, false  (empty path = not configured)
//	lookupPath({"a":1},        "a.b")  → nil, false  (path stops at 1, can't descend)
//
// Path segments are pure JSON object keys. Array indexing and
// wildcards are NOT supported — out of scope for v1 per package doc.
func lookupPath(root any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	cur := root
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, exists := m[p]
		if !exists {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// asString coerces a JSON value into a Go string. JSON numbers, bools,
// and nulls are stringified so a field_map pointed at a numeric SKU
// column still works. Anything more complex (arrays, objects) returns
// the empty string.
func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers are float64; render integer floats without
		// trailing ".0" so a SKU like 12345 stays "12345".
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json_Number:
		return string(x)
	}
	return ""
}

// json_Number is a tiny alias to avoid importing encoding/json here
// just for the Number type — keeps the type switch above readable.
// The real json.Number satisfies this same shape.
type json_Number string

// asFloat coerces a JSON value into a *float64. Returns nil for
// missing, null, or non-numeric inputs — the canonical product
// treats nil price as "unknown", so silent coercion of garbage
// would lie to downstream code.
func asFloat(v any) *float64 {
	switch x := v.(type) {
	case nil:
		return nil
	case float64:
		f := x
		return &f
	case int:
		f := float64(x)
		return &f
	case int64:
		f := float64(x)
		return &f
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &f
	}
	return nil
}

// asStringSlice coerces a JSON value into []string. Accepts native
// arrays of strings/numbers as well as comma-separated string ("a,
// b, c") so a field_map pointed at a flat denormalised column still
// works. Returns nil-but-not-empty for completely missing values.
func asStringSlice(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s := asString(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return nil
}

// asTime coerces a JSON value into a time.Time. Returns zero time on
// failure — the canonical Validate then surfaces it as a clear error
// ("source_updated_at must be non-zero") so an unmappable timestamp
// becomes one rejected row, not silently-zero data.
//
// Supported formats, tried in order:
//   - RFC3339 / RFC3339Nano (ISO-8601 with timezone)
//   - "2006-01-02 15:04:05" (SQL DATETIME without timezone, treated UTC)
//   - "2006-01-02"           (date-only, treated as midnight UTC)
//   - Unix epoch seconds (numeric JSON value or numeric string)
//   - Unix epoch milliseconds (numeric JSON value > 10^12)
func asTime(v any) time.Time {
	switch x := v.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return x
	case float64:
		return epochToTime(int64(x))
	case int:
		return epochToTime(int64(x))
	case int64:
		return epochToTime(x)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return time.Time{}
		}
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02",
		} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC()
			}
		}
		// Last resort: maybe it's a numeric string.
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return epochToTime(n)
		}
	}
	return time.Time{}
}

func epochToTime(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	// Heuristic: >10^12 means milliseconds, otherwise seconds.
	if n > 1e12 {
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}

// resolveTemplate substitutes simple {var} placeholders against a
// map. Used for FieldMap.SourceURLTemplate: pattern "https://x/{id}"
// with {"id":"abc"} → "https://x/abc". Unknown placeholders are
// left in place so the operator sees the broken substitution rather
// than getting silently truncated.
func resolveTemplate(tmpl string, vars map[string]string) string {
	if tmpl == "" || len(vars) == 0 {
		return tmpl
	}
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

// pathExists is true iff a non-empty value is reachable at path.
// Used by validators that want a clearer message than "got zero".
func pathExists(root any, path string) bool {
	v, ok := lookupPath(root, path)
	if !ok {
		return false
	}
	return v != nil && asString(v) != ""
}

// fmtErr is a small helper that prefixes adapter errors with the
// item index for noisy logs. Adapters call it inside the per-item
// loop so SyncResult.Errors carries actionable context.
func fmtErr(index int, format string, args ...any) string {
	return fmt.Sprintf("item[%d] "+format, append([]any{index}, args...)...)
}
