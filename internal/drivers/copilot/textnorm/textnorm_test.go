package textnorm

import "testing"

// Fold lowercases + strips Vietnamese diacritics so plain-ASCII needles match accented
// input. Pins the folding behavior the copilot matchers depend on (ARCHCP3 move).
func TestFold(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Bình Luận", "binh luan"},
		{"ĐĂNG", "dang"},
		{"mến chào bạn", "men chao ban"},
		{"ALREADY ascii", "already ascii"},
		{"", ""},
		{"số 1 — ô tô", "so 1 — o to"}, // non-Vietnamese runes pass through
	}
	for _, c := range cases {
		if got := Fold(c.in); got != c.want {
			t.Errorf("Fold(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ContainsAny folds each NEEDLE and checks it against the value as-is — the caller is
// expected to pass an already-folded value (the historical contract: "the folded value
// contains any of the needles"). Pinned so the move stays behavior-identical.
func TestContainsAny(t *testing.T) {
	needles := []string{"binh luan", "dang bai"}
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"folded value matches needle", Fold("Cho mình xin bình luận nhé"), true},
		{"second needle matches", Fold("muốn đăng bài"), true},
		{"raw accented value does NOT match (caller must fold first)", "bình luận", false},
		{"no match", "hello world", false},
		{"empty value", "", false},
	}
	for _, c := range cases {
		if got := ContainsAny(c.value, needles); got != c.want {
			t.Errorf("%s: ContainsAny(%q) = %v, want %v", c.name, c.value, got, c.want)
		}
	}
	if ContainsAny("anything", nil) {
		t.Error("ContainsAny with no needles must be false")
	}
}
