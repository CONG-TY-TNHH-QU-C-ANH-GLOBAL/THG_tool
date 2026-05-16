package ai

import "testing"

// TestInferTargetRoleFromPrompt pins the prompt → target_role inference,
// including the seller-as-customer case the user reported: a fulfillment
// crawl prompt produced target_role="" → the classifier auto-rejected
// every seller post as provider_ad.
func TestInferTargetRoleFromPrompt(t *testing.T) {
	cases := []struct {
		name, prompt, want string
	}{
		{
			name:   "the user's exact failing prompt",
			prompt: "Cào cho tôi 50 bài liên quan đến tệp seller có nhu cầu fulfill POD,dropship",
			want:   "potential_customer",
		},
		{
			name:   "shop có nhu cầu in ấn",
			prompt: "tìm shop có nhu cầu in ấn áo POD",
			want:   "potential_customer",
		},
		{
			name:   "english seller looking for fulfillment",
			prompt: "find sellers looking for fulfillment for dropship",
			want:   "potential_customer",
		},
		{
			name:   "explicit khách phrase still wins potential_customer",
			prompt: "tìm khách hàng có nhu cầu mua",
			want:   "potential_customer",
		},
		{
			name:   "recruitment intent still resolves to candidate",
			prompt: "tìm ứng viên sales POD",
			want:   "candidate",
		},
		{
			name:   "explicit supplier intent resolves to partner",
			prompt: "tìm nhà cung cấp logistic",
			want:   "partner",
		},
		{
			name:   "bare seller word with no service or need anchor stays empty",
			prompt: "phân tích các bài bán shopify",
			want:   "",
		},
		{
			name:   "service anchor alone (no subject) without buyer word stays empty",
			prompt: "phân tích bài về fulfillment",
			want:   "",
		},
		{
			name:   "need + service anchors without explicit subject still resolves",
			prompt: "tìm bài cần fulfill",
			want:   "potential_customer",
		},
		{
			name:   "empty prompt → empty inference",
			prompt: "",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InferTargetRoleFromPrompt(tc.prompt)
			if got != tc.want {
				t.Errorf("InferTargetRoleFromPrompt(%q) = %q, want %q", tc.prompt, got, tc.want)
			}
		})
	}
}
