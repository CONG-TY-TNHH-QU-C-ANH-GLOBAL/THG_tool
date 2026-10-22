package leadcontext

import (
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/ai/visualcontext"
)

const scopePost = "Need thread in this style"

func scopeFixture() Context {
	return Context{Intent: Sourcing, Evidence: []string{"Need thread"}, Missing: []string{},
		Visual: visualcontext.Context{Product: "storage basket", Observations: []string{"A storage basket with ribbed sides is visible"}, Uncertainties: []string{}},
		Scope:  &ScopeClarification{Kind: "purchase_scope", Alternatives: []string{"thread", "storage basket"}}}
}

func TestScopeClarificationBindsAlternativesToAvailableInput(t *testing.T) {
	data, _ := json.Marshal(scopeFixture())
	got, err := ParseWithImages(data, scopePost, 1)
	if err != nil || got.Scope == nil || len(got.Scope.Alternatives) != 2 {
		t.Fatalf("grounded purchase question rejected: %v", err)
	}
	legacy := scopeFixture()
	legacy.Scope = nil
	data, _ = json.Marshal(legacy)
	if got, err := ParseWithImages(data, scopePost, 1); err != nil || got.Scope != nil {
		t.Fatalf("legacy response changed: %v", err)
	}
}

func TestScopeClarificationRejectsUnsupportedChoicesAndFields(t *testing.T) {
	for name, edit := range map[string]func(map[string]any){
		"unknown target": func(v map[string]any) {
			v["scope_clarification"] = ScopeClarification{"purchase_scope", []string{"thread", "cotton fabric"}}
		},
		"partial word": func(v map[string]any) {
			v["scope_clarification"] = ScopeClarification{"purchase_scope", []string{"read", "storage basket"}}
		},
		"duplicate": func(v map[string]any) {
			v["scope_clarification"] = ScopeClarification{"purchase_scope", []string{"thread", "THREAD"}}
		},
		"blank": func(v map[string]any) {
			v["scope_clarification"] = ScopeClarification{"purchase_scope", []string{"", "storage basket"}}
		},
		"three options": func(v map[string]any) {
			v["scope_clarification"] = ScopeClarification{"purchase_scope", []string{"thread", "storage basket", "fabric"}}
		},
		"pricing kind": func(v map[string]any) {
			v["scope_clarification"] = ScopeClarification{"free_shipping", []string{"thread", "storage basket"}}
		},
		"null": func(v map[string]any) { v["scope_clarification"] = nil },
		"unknown nested": func(v map[string]any) {
			v["scope_clarification"] = map[string]any{"kind": "purchase_scope", "alternatives": []string{"thread", "storage basket"}, "url": "https://unsafe.example"}
		},
		"no buyer need": func(v map[string]any) { v["intent"] = NonCommercial },
		"no evidence":   func(v map[string]any) { v["text_evidence"] = []string{} },
		"invented image": func(v map[string]any) {
			v["visual"] = visualcontext.Context{Observations: []string{}, Uncertainties: []string{}}
		},
		"unknown top": func(v map[string]any) { v["supplier_url"] = "https://unsafe.example" },
	} {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(scopeFixture())
			var v map[string]any
			_ = json.Unmarshal(data, &v)
			edit(v)
			data, _ = json.Marshal(v)
			if _, err := ParseWithImages(data, scopePost, 1); err == nil {
				t.Fatal("unsupported clarification accepted")
			}
		})
	}
}

func TestTextOnlyScopeQuestionCannotUsePhotoCategory(t *testing.T) {
	c := scopeFixture()
	c.Visual = visualcontext.Context{Observations: []string{}, Uncertainties: []string{}}
	data, _ := json.Marshal(c)
	if _, err := ParseWithImages(data, scopePost, 0); err == nil {
		t.Fatal("missing image supplied a category")
	}
	if _, err := ParseWithImages(data, "Need thread or a storage basket", 0); err != nil {
		t.Fatalf("two categories actually present in text rejected: %v", err)
	}
}
