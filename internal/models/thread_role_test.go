package models

import "testing"

// TestInferThreadRole locks the deterministic role engine. The headline
// case is the user-reported bug: a vendor comment ("ib em hỗ trợ") the
// classifier tagged as buyer intent must still resolve to supplier_responder.
func TestInferThreadRole(t *testing.T) {
	cases := []struct {
		name       string
		sourceType string
		intent     string
		content    string
		want       LeadThreadRole
	}{
		{
			name:       "post asking for a supplier is the intent originator",
			sourceType: "post",
			intent:     "potential_customer",
			content:    "Em tìm đơn vị fulfill sản phẩm này ạ",
			want:       ThreadRoleIntentOriginator,
		},
		{
			name:       "vendor comment with buyer-tagged intent still resolves supplier",
			sourceType: "comment",
			intent:     "potential_customer", // classifier mistake — the user's exact bug
			content:    "em có mẫu này y hệt mình quan tâm ib em hỗ trợ nhé ạ",
			want:       ThreadRoleSupplierResponder,
		},
		{
			name:       "comment from provider_ad intent is a supplier",
			sourceType: "comment",
			intent:     "provider_ad",
			content:    "shop mình nhận order số lượng lớn",
			want:       ThreadRoleSupplierResponder,
		},
		{
			name:       "buyer comment with no vendor speak is a buyer responder",
			sourceType: "comment",
			intent:     "potential_customer",
			content:    "Mình cũng đang cần cái này, ai có chỉ với",
			want:       ThreadRoleBuyerResponder,
		},
		{
			name:       "post advertising a service is a competitor",
			sourceType: "post",
			intent:     "provider_ad",
			content:    "Bên mình chuyên fulfill POD, báo giá cho ạ",
			want:       ThreadRoleCompetitor,
		},
		{
			name:       "post in vendor-speak the classifier did not tag as buyer is a competitor",
			sourceType: "post",
			intent:     "not_relevant",
			content:    "shop mình nhận sỉ toàn quốc, ib mình",
			want:       ThreadRoleCompetitor,
		},
		{
			name:       "spam is noise regardless of structure",
			sourceType: "post",
			intent:     "spam",
			content:    "KIẾM TIỀN ONLINE 500K/NGÀY",
			want:       ThreadRoleNoise,
		},
		{
			name:       "not_relevant comment is noise",
			sourceType: "comment",
			intent:     "not_relevant",
			content:    "đẹp quá",
			want:       ThreadRoleNoise,
		},
		{
			name:       "empty source type defaults to post path → intent originator",
			sourceType: "",
			intent:     "potential_customer",
			content:    "cần tìm xưởng may",
			want:       ThreadRoleIntentOriginator,
		},
		{
			name:       "unknown-intent comment with no vendor speak → buyer responder",
			sourceType: "comment",
			intent:     "",
			content:    "cho mình xin thông tin với",
			want:       ThreadRoleBuyerResponder,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InferThreadRole(tc.sourceType, tc.intent, tc.content)
			if got != tc.want {
				t.Errorf("InferThreadRole(%q, %q, %q) = %q, want %q",
					tc.sourceType, tc.intent, tc.content, got, tc.want)
			}
		})
	}
}

func TestLeadThreadRole_IsLeadRole(t *testing.T) {
	leadRoles := []LeadThreadRole{ThreadRoleIntentOriginator, ThreadRoleBuyerResponder}
	for _, r := range leadRoles {
		if !r.IsLeadRole() {
			t.Errorf("%s should be a lead role", r)
		}
	}
	nonLead := []LeadThreadRole{ThreadRoleSupplierResponder, ThreadRoleCompetitor, ThreadRoleNoise}
	for _, r := range nonLead {
		if r.IsLeadRole() {
			t.Errorf("%s should NOT be a lead role", r)
		}
	}
}

func TestNormalizeThreadRole(t *testing.T) {
	if got := NormalizeThreadRole("supplier_responder"); got != ThreadRoleSupplierResponder {
		t.Errorf("got %q", got)
	}
	if got := NormalizeThreadRole("  COMPETITOR "); got != ThreadRoleCompetitor {
		t.Errorf("case/space normalisation failed: got %q", got)
	}
	// Unknown / legacy empty → intent_originator (every pre-Phase-B lead
	// was a post-sourced lead).
	if got := NormalizeThreadRole(""); got != ThreadRoleIntentOriginator {
		t.Errorf("empty should default to intent_originator, got %q", got)
	}
	if got := NormalizeThreadRole("garbage"); got != ThreadRoleIntentOriginator {
		t.Errorf("unknown should default to intent_originator, got %q", got)
	}
}
