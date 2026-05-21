// Internal-package test: needs reach to unexported targetTypeFromAction
// helper. Per [[feedback_subpackage_contract]] rule 9 the bulk of
// coordination tests live in `package coordination_test`; this is the
// narrow exception for the unexported-helper round-trip.
package coordination

import "testing"

func TestTargetTypeFromAction(t *testing.T) {
	cases := map[string]string{
		"comment":      "post",
		"inbox":        "profile",
		"group_post":   "group",
		"profile_post": "profile",
		"COMMENT":      "post",
		" inbox ":      "profile",
		"unknown_type": "",
	}
	for in, want := range cases {
		if got := targetTypeFromAction(in); got != want {
			t.Errorf("targetTypeFromAction(%q) = %q, want %q", in, got, want)
		}
	}
}
