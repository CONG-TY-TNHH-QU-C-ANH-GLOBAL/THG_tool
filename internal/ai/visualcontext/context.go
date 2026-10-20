// Package visualcontext owns platform-neutral, tentative observations from
// text plus images. Observations are not verified supplier or company facts.
package visualcontext

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

type Context struct {
	Product       string   `json:"product"`
	Observations  []string `json:"observations"`
	Uncertainties []string `json:"uncertainties"`
	Conflict      bool     `json:"text_image_conflict"`
}

// Parse validates the whole response, including required keys. Missing evidence
// must stay missing; absent conflict metadata must not be assumed to be false.
func Parse(data []byte) (Context, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 4 {
		return Context{}, errors.New("invalid visual context")
	}
	for _, key := range []string{"product", "observations", "uncertainties", "text_image_conflict"} {
		if len(fields[key]) == 0 || string(fields[key]) == "null" {
			return Context{}, errors.New("incomplete visual context")
		}
	}
	var out Context
	if json.Unmarshal(data, &out) != nil || !ValidProductHint(out.Product) || !validNotes(out.Observations) || !validNotes(out.Uncertainties) {
		return Context{}, errors.New("invalid visual observations")
	}
	if (strings.TrimSpace(out.Product) != "" || out.Conflict) && len(out.Observations) == 0 {
		return Context{}, errors.New("visual claim without observations")
	}
	out.Product = strings.TrimSpace(out.Product)
	return out, nil
}

// ValidProductHint bounds a generic search category, never a model or price.
func ValidProductHint(value string) bool {
	if len([]rune(value)) > 80 || len(strings.Fields(value)) > 8 {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && r != ' ' && r != '-' {
			return false
		}
	}
	return true
}

func validNotes(notes []string) bool {
	if len(notes) > 5 {
		return false
	}
	for _, note := range notes {
		if strings.TrimSpace(note) == "" || len([]rune(note)) > 240 || strings.Contains(note, "://") {
			return false
		}
		for _, r := range note {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}

func (c Context) PromptBlock() string {
	data, _ := json.Marshal(c)
	return string(data)
}
