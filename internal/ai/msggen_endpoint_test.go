package ai

import "testing"

// TestMessageGeneratorChatURL pins the provider-endpoint resolution so the
// OpenAI default and the Together-style override both stay stable.
func TestMessageGeneratorChatURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"blank keeps openai default", "", defaultChatCompletionsURL},
		{"together base", "https://api.together.xyz/v1", "https://api.together.xyz/v1/chat/completions"},
		{"trailing slash trimmed", "https://api.together.xyz/v1/", "https://api.together.xyz/v1/chat/completions"},
		{"whitespace trimmed", "  https://api.together.xyz/v1  ", "https://api.together.xyz/v1/chat/completions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mg := NewMessageGeneratorWithEndpoint("k", "m", tc.baseURL)
			if got := mg.chatURL(); got != tc.want {
				t.Fatalf("chatURL() = %q, want %q", got, tc.want)
			}
		})
	}

	// NewMessageGenerator must remain identical to the blank-endpoint path.
	if got := NewMessageGenerator("k", "m").chatURL(); got != defaultChatCompletionsURL {
		t.Fatalf("NewMessageGenerator default = %q, want %q", got, defaultChatCompletionsURL)
	}
}
