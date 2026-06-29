package promptprep

import "testing"

// StripDashboardContext removes the appended "Dashboard context:" block and trims.
// Pins the behavior the copilot classifier depends on (ARCHCP3 move).
func TestStripDashboardContext(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"strips marker block", "comment bài này\n\nDashboard context: org=5 acc=0", "comment bài này"},
		{"no marker just trims", "  hãy đăng bài  ", "hãy đăng bài"},
		{"marker at start yields empty", "\n\nDashboard context: x", ""},
		{"empty", "", ""},
		{"marker substring without the exact newlines is not stripped", "talk about Dashboard context: inline", "talk about Dashboard context: inline"},
	}
	for _, c := range cases {
		if got := StripDashboardContext(c.in); got != c.want {
			t.Errorf("%s: StripDashboardContext(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
