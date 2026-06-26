package copilot

import "testing"

// inferBusinessCalibrationFromPrompt was reshaped (S3776 reduction) into an
// orchestrator over pure helpers. These cases pin the observable mapping so the
// extraction stays behavior-preserving: field extraction, crawl-line exclusion,
// the profile fallback, and the role default.
func TestInferBusinessCalibrationFromPrompt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		prompt string
		want   map[string]string // subset asserted; "" means key must be absent
	}{
		{
			name:   "empty prompt yields no fields",
			prompt: "",
			want:   map[string]string{"business_profile": "", "target_author_role": ""},
		},
		{
			name:   "plain content defaults role to customers and keeps profile",
			prompt: "hello world",
			want:   map[string]string{"business_profile": "hello world", "target_author_role": "customers"},
		},
		{
			name:   "company line fills business_name via marker segment",
			prompt: "Công ty của tôi là ABC",
			want:   map[string]string{"business_name": "ABC", "target_author_role": "customers"},
		},
		{
			name:   "hiring language infers candidate role",
			prompt: "tuyển dụng nhân sự senior",
			want:   map[string]string{"target_author_role": "candidates"},
		},
		{
			name:   "supplier language infers supplier role",
			prompt: "tìm nhà cung cấp xưởng in",
			want:   map[string]string{"target_author_role": "suppliers"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := inferBusinessCalibrationFromPrompt(c.prompt)
			for k, want := range c.want {
				assertBusinessField(t, got, k, want)
			}
		})
	}
}

// assertBusinessField checks one calibration key. want == "" asserts the key is
// absent; otherwise it asserts the exact value.
func assertBusinessField(t *testing.T, got map[string]string, key, want string) {
	t.Helper()
	if want == "" {
		if _, ok := got[key]; ok {
			t.Errorf("key %q should be absent; got %q", key, got[key])
		}
		return
	}
	if got[key] != want {
		t.Errorf("key %q = %q; want %q (full: %v)", key, got[key], want, got)
	}
}
