package org

import "testing"

// extractEvidenceField is the JSON field parser used by
// superAdminAccountDiagnostic to surface proof.notes / page_url_after
// without importing encoding/json (the evidence_json blob may be
// malformed for legacy rows + we want a forgiving extractor). It is
// load-bearing for the (B) sequence Step 1 diagnostic loop — a bug
// here means the founder sees wrong/missing landed_url notes in the
// diagnostic response, which then misroutes the targeted fix.
//
// Edge cases verified:
//   - field present with normal value
//   - field absent (must return empty, not crash)
//   - field at start, middle, end of blob
//   - escaped quote inside value
//   - empty blob / non-JSON garbage
//   - field name as substring of another field (no false positive)
//   - whitespace between `:` and `"value"` (FB / proof serializers
//     sometimes emit with space)
func TestExtractEvidenceField(t *testing.T) {
	cases := []struct {
		name  string
		blob  string
		field string
		want  string
	}{
		{
			name:  "simple field present",
			blob:  `{"notes":"identity_gate_1 landed_at=https://www.facebook.com/"}`,
			field: "notes",
			want:  "identity_gate_1 landed_at=https://www.facebook.com/",
		},
		{
			name:  "field absent returns empty",
			blob:  `{"notes":"hello"}`,
			field: "page_url_after",
			want:  "",
		},
		{
			name:  "field at end",
			blob:  `{"success":false,"failure_reason":"redirected_feed","page_url_after":"https://www.facebook.com/"}`,
			field: "page_url_after",
			want:  "https://www.facebook.com/",
		},
		{
			name:  "whitespace after colon",
			blob:  `{"notes": "value with leading space ok"}`,
			field: "notes",
			want:  "value with leading space ok",
		},
		{
			name:  "empty blob",
			blob:  "",
			field: "notes",
			want:  "",
		},
		{
			name:  "non-JSON garbage",
			blob:  "not even close to json",
			field: "notes",
			want:  "",
		},
		{
			name:  "empty value",
			blob:  `{"notes":""}`,
			field: "notes",
			want:  "",
		},
		{
			name:  "escaped quote in value",
			blob:  `{"notes":"he said \"hello\" politely"}`,
			field: "notes",
			want:  `he said \"hello\" politely`,
		},
		{
			name:  "field name is substring of another field",
			blob:  `{"page_url_before":"x","page_url_after":"y"}`,
			field: "page_url_after",
			want:  "y",
		},
		{
			name:  "multiple fields, pick the right one",
			blob:  `{"success":false,"failure_reason":"redirected_feed","notes":"the real notes","page_url_after":"https://www.facebook.com/home.php"}`,
			field: "notes",
			want:  "the real notes",
		},
		{
			name:  "value with URL containing slashes and colons",
			blob:  `{"page_url_after":"https://www.facebook.com/groups/1312868109620530/posts/2023880801852587/"}`,
			field: "page_url_after",
			want:  "https://www.facebook.com/groups/1312868109620530/posts/2023880801852587/",
		},
		{
			name:  "value with URL query params",
			blob:  `{"page_url_after":"https://www.facebook.com/photo/?fbid=X&set=gm.Y"}`,
			field: "page_url_after",
			want:  "https://www.facebook.com/photo/?fbid=X&set=gm.Y",
		},
		{
			name:  "long compound notes string with separators",
			blob:  `{"notes":"page navigated to feed/home after submit · identity_gate_1_no_article_or_unstable: target id=2023880801... landed_at=https://www.facebook.com/ nav_at_entry=https://www.facebook.com/groups/X/posts/Y/ did not settle within 8s"}`,
			field: "notes",
			want:  "page navigated to feed/home after submit · identity_gate_1_no_article_or_unstable: target id=2023880801... landed_at=https://www.facebook.com/ nav_at_entry=https://www.facebook.com/groups/X/posts/Y/ did not settle within 8s",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractEvidenceField(c.blob, c.field)
			if got != c.want {
				t.Errorf("extractEvidenceField(%q, %q):\n  got  = %q\n  want = %q",
					c.blob, c.field, got, c.want)
			}
		})
	}
}
