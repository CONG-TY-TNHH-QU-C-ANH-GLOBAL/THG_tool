package leadcontext

import (
	"encoding/json"
	"strings"
	"unicode"
)

// ScopeClarification records purchase uncertainty for operator review. It is
// not an instruction to ask the customer a question or block source retrieval.
// It never supplies a price, route or company capability.
type ScopeClarification struct {
	Kind         string   `json:"kind"`
	Alternatives []string `json:"alternatives"`
}

func validScopeClarification(raw json.RawMessage, c Context, post string, imageCount int) bool {
	if len(raw) == 0 {
		return true // Older responses retain their existing behavior.
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil || len(keys) != 2 || keys["kind"] == nil || keys["alternatives"] == nil {
		return false
	}
	if c.Scope == nil || c.Scope.Kind != "purchase_scope" || len(c.Scope.Alternatives) != 2 {
		return false
	}
	if (c.Intent != Sourcing && c.Intent != POD) || (len(c.Evidence) == 0 && len(c.ImageEvidence) == 0) {
		return false
	}
	evidence := post
	if imageCount > 0 {
		evidence += "\n" + strings.Join(c.Visual.Observations, "\n")
	}
	a, b := c.Scope.Alternatives[0], c.Scope.Alternatives[1]
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return validProduct(a) && validProduct(b) && scopePhrase(a) != scopePhrase(b) &&
		hasScopePhrase(evidence, a) && hasScopePhrase(evidence, b)
}

func scopePhrase(s string) string {
	return strings.ToLower(strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}), " "))
}

func hasScopePhrase(text, phrase string) bool {
	return strings.Contains(" "+scopePhrase(text)+" ", " "+scopePhrase(phrase)+" ")
}
