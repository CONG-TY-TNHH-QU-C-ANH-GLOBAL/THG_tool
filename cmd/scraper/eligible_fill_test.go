package main

import "testing"

func TestRequestedOutreachCount(t *testing.T) {
	cases := []struct {
		args map[string]any
		want int
	}{
		{map[string]any{"limit": int64(5)}, 5},
		{map[string]any{"max_items": int64(3)}, 3},
		{map[string]any{"limit": int64(0), "max_items": int64(7)}, 7},
		{map[string]any{}, 25},
	}
	for _, c := range cases {
		if got := requestedOutreachCount(c.args); got != c.want {
			t.Errorf("requestedOutreachCount(%v) = %d, want %d", c.args, got, c.want)
		}
	}
	// Scan pool is max(50, requested*10): a request of 5 scans up to 50; a request of
	// 8 scans 80. (The per-lead stop at `requested` queued is enforced in the loop.)
	if got := scanPoolFor(5); got != 50 {
		t.Errorf("scanPoolFor(5) = %d, want 50", got)
	}
	if got := scanPoolFor(8); got != 80 {
		t.Errorf("scanPoolFor(8) = %d, want 80", got)
	}
}
