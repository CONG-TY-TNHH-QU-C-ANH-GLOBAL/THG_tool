package models

import "testing"

// TestDeriveChampion pins the Coordination observability projection: champion =
// most verified touches (analytics only), contributors = distinct names in
// first-appearance order; blank names ignored.
func TestDeriveChampion(t *testing.T) {
	entries := []LeadEngagement{
		{UserName: "Alice"}, {UserName: "Bob"}, {UserName: "Alice"},
		{UserName: ""}, {UserName: "Alice"}, {UserName: "Bob"},
	}
	champ, contribs := DeriveChampion(entries)
	if champ != "Alice" {
		t.Fatalf("champion = %q, want Alice (3 vs 2)", champ)
	}
	if len(contribs) != 2 || contribs[0] != "Alice" || contribs[1] != "Bob" {
		t.Fatalf("contributors = %v, want [Alice Bob]", contribs)
	}

	if c, cc := DeriveChampion(nil); c != "" || len(cc) != 0 {
		t.Fatalf("empty entries should yield no champion, got %q %v", c, cc)
	}
}
