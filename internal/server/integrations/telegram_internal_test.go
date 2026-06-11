package integrations

import (
	"testing"

	"github.com/thg/scraper/internal/telegram/control"
)

// computeStatus headline-state derivation (pure; no DB/HTTP).
func TestComputeStatus(t *testing.T) {
	cases := []struct {
		name           string
		enabled        bool
		botConfigured  bool
		activeBindings int
		want           string
	}{
		{"fresh org", false, false, 0, "not_connected"},
		{"enabled but no bot token", true, false, 1, "needs_attention"},
		{"enabled, configured, no bindings", true, true, 0, "needs_attention"},
		{"connected", true, true, 2, "connected"},
		{"bindings exist but disabled + no token", false, false, 1, "needs_attention"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := computeStatus(c.enabled, c.botConfigured, c.activeBindings); got != c.want {
				t.Errorf("computeStatus(%v,%v,%d) = %q, want %q", c.enabled, c.botConfigured, c.activeBindings, got, c.want)
			}
		})
	}
}

// canViewAllBindings — only admins/platform owners manage every binding; members are self-scoped.
func TestCanViewAllBindings(t *testing.T) {
	for role, want := range map[string]bool{
		"admin": true, "founder": true, "superadmin": true, "sales": false, "": false,
	} {
		if got := canViewAllBindings(role); got != want {
			t.Errorf("canViewAllBindings(%q) = %v, want %v", role, got, want)
		}
	}
}

// validation allow-lists (now owned by the shared control package) must reject unknown values.
func TestValidationAllowLists(t *testing.T) {
	if control.IsValidChannelFilter("myspace") {
		t.Error("unknown channel filter must be rejected")
	}
	if !control.IsValidChannelFilter("facebook") || !control.IsValidChannelFilter("1688") || !control.IsValidChannelFilter("all") {
		t.Error("known channel filters must be accepted")
	}
	if control.IsValidAlertType("delete_everything") {
		t.Error("unknown alert type must be rejected")
	}
	if !control.IsValidAlertType("circuit_breaker_triggered") {
		t.Error("known alert type must be accepted")
	}
}
