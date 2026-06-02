package ai

import "testing"

func TestIsReasoningModel(t *testing.T) {
	cases := map[string]bool{
		"gpt-5.4":      true,
		"gpt-5.4-mini": true,
		"gpt-5":        true,
		"GPT-5.4":      true, // case-insensitive
		"o1":           true,
		"o1-mini":      true,
		"o3-mini":      true,
		"o4-mini":      true,
		"gpt-4o":       false,
		"gpt-4o-mini":  false,
		"gpt-4.1":      false,
		"gpt-3.5-turbo": false,
		"":             false,
	}
	for model, want := range cases {
		if got := isReasoningModel(model); got != want {
			t.Errorf("isReasoningModel(%q) = %v, want %v", model, got, want)
		}
	}
}

// TestChatCompletionBody_OmitsTemperatureForReasoningModels is the regression
// guard for the auto-comment bug: gpt-5* / o* reject temperature != 1, so the
// request body must NOT carry a temperature field for those models. Classic
// models keep temperature for output variety.
func TestChatCompletionBody_OmitsTemperatureForReasoningModels(t *testing.T) {
	reasoning := chatCompletionBody("gpt-5.4", "hello")
	if _, ok := reasoning["temperature"]; ok {
		t.Fatalf("reasoning model body must omit temperature, got %v", reasoning["temperature"])
	}
	if reasoning["max_completion_tokens"] != 2000 {
		t.Fatalf("max_completion_tokens = %v, want 2000", reasoning["max_completion_tokens"])
	}

	classic := chatCompletionBody("gpt-4o", "hello")
	if classic["temperature"] != 0.7 {
		t.Fatalf("classic model body must keep temperature 0.7, got %v", classic["temperature"])
	}
}
