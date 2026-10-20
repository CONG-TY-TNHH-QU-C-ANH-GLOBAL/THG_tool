package media

import "testing"

func TestURLsBoundAndRejectNonPostMediaOrigins(t *testing.T) {
	const valid = "https://scontent.example.fbcdn.net/v/photo.jpg?token=private-reference"
	raw := []string{"http://scontent.fbcdn.net/p.jpg", "https://fbcdn.net.evil.example/p.jpg", "https://localhost/p.jpg",
		"https://user:pass@fbcdn.net/p.jpg", "https://fbcdn.net:444/p.jpg", "https://fbcdn.net/p.jpg#x", valid, valid,
		"https://external.fbsbx.com/another.png", "https://scontent.fbcdn.net/third.jpg", "https://scontent.fbcdn.net/fourth.jpg"}
	got := URLs(raw)
	if len(got) != 3 || got[0] != valid || got[2] != raw[9] {
		t.Fatalf("unexpected validated media references: %d entries", len(got))
	}
}
