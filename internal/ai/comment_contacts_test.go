package ai

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestScreenAndRepairContacts(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com", OfficialContact: "t.me/Moonzzz03"}

	// website (the one URL) + Telegram @handle from identity → passes.
	ok, _ := ScreenCommentContacts("Bên mình hỗ trợ. Web https://thgfulfill.com, Telegram @Moonzzz03 nhé.", id)
	if !ok {
		t.Error("website + matching @handle should pass")
	}

	// website + Zalo phone matching identity → passes.
	idZalo := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com", OfficialContact: "Zalo 0367689834"}
	if ok, r := ScreenCommentContacts("Web https://thgfulfill.com, Zalo 0367689834 nhé.", idZalo); !ok {
		t.Errorf("website + matching Zalo phone should pass, got %s", r)
	}

	// official contact AS a t.me URL (the only URL) → now allowed (in allowlist).
	if ok, r := ScreenCommentContacts("Liên hệ t.me/Moonzzz03 nhé.", id); !ok {
		t.Errorf("official t.me contact as the single URL should pass, got %s", r)
	}

	// website + t.me link → 2 URLs → fails, but repair converts t.me → @handle → passes.
	bad := "Web https://thgfulfill.com và t.me/Moonzzz03 nhé."
	if ok, _ := ScreenCommentContacts(bad, id); ok {
		t.Error("website + t.me link = 2 URLs should fail before repair")
	}
	repaired, changed := RepairCommentContacts(bad, id)
	if !changed || strings.Contains(repaired, "t.me/") || !strings.Contains(repaired, "@Moonzzz03") {
		t.Errorf("repair must convert t.me link to @handle, got %q", repaired)
	}
	if ok, r := ScreenCommentContacts(repaired, id); !ok {
		t.Errorf("repaired comment should pass, got %s", r)
	}

	// A non-grounded random link → repair strips it → passes; an invented phone is rejected.
	rep2, ch2 := RepairCommentContacts("Xem shop https://random-other.com nhé.", id)
	if !ch2 || strings.Contains(rep2, "random-other") {
		t.Errorf("repair must drop a non-grounded URL, got %q", rep2)
	}
	if ok, _ := ScreenCommentContacts("Gọi 0911222333 nhé.", id); ok {
		t.Error("a phone not in the identity must be rejected")
	}
}
