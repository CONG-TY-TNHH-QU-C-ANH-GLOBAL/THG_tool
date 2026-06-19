package comment

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

// TestRepairCollapsesDuplicateGroundedURLs reproduces the dominant comment_all_leads
// skip (comment_multiple_urls): the generator emits the bare website AND a service
// deep link on the SAME domain. Both are "grounded", so the old strip pass kept
// both and the re-screen still failed. Repair must collapse them to ONE canonical
// company website so the lead is queued instead of skipped.
func TestRepairCollapsesDuplicateGroundedURLs(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com"}

	// website + service sub-page link on the same domain → 2 URLs → fails screen.
	bad := "Bên em hỗ trợ fulfill nhé. Web https://thgfulfill.com, dịch vụ https://thgfulfill.com/thg-fulfill ạ."
	if ok, r := ScreenCommentContacts(bad, id); ok {
		t.Fatalf("website + service deep link = 2 URLs should fail before repair, got %s", r)
	}

	repaired, changed := RepairCommentContacts(bad, id)
	if !changed {
		t.Fatalf("repair must collapse the duplicate company URLs, got no change: %q", repaired)
	}
	// Exactly one company URL remains, and the deep/service path is gone.
	if strings.Contains(repaired, "/thg-fulfill") {
		t.Errorf("service deep link must collapse to the bare website, got %q", repaired)
	}
	if n := strings.Count(repaired, "thgfulfill.com"); n != 1 {
		t.Errorf("exactly one company website URL expected, got %d in %q", n, repaired)
	}
	if ok, r := ScreenCommentContacts(repaired, id); !ok {
		t.Errorf("repaired single-URL comment must pass the gate, got %s", r)
	}

	// Two bare copies of the same website also collapse to one.
	dup := "Web https://thgfulfill.com nhé. Liên hệ https://thgfulfill.com ạ."
	rep2, ch2 := RepairCommentContacts(dup, id)
	if !ch2 || strings.Count(rep2, "thgfulfill.com") != 1 {
		t.Errorf("duplicate bare website must collapse to one, got %q", rep2)
	}
	if ok, _ := ScreenCommentContacts(rep2, id); !ok {
		t.Error("collapsed duplicate website must pass the gate")
	}
}

// TestURLHostAnchoring guards the brand-trust allowlist against lookalike hosts:
// only the EXACT configured host is grounded; a substring match used to let
// thgfulfill.com.evil.com and fake-thgfulfill.com survive repair.
func TestURLHostAnchoring(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com"}

	reject := []string{
		"Xem https://thgfulfill.com.evil.com/phish nhé.", // suffix lookalike
		"Web https://fake-thgfulfill.com nhé.",           // prefix lookalike
		"Truy cập https://thgfulfill.com.vn nhé.",        // different registrable host
	}
	for _, in := range reject {
		// The lookalike is the only URL → it must NOT be accepted as the grounded site.
		if ok, _ := ScreenCommentContacts(in, id); ok {
			t.Errorf("lookalike host must be rejected, passed: %q", in)
		}
		// Repair must strip it (non-grounded), leaving zero company URLs.
		rep, _ := RepairCommentContacts(in, id)
		if strings.Contains(rep, "evil.com") || strings.Contains(rep, "fake-thgfulfill") || strings.Contains(rep, "thgfulfill.com.vn") {
			t.Errorf("repair must strip the lookalike host, got %q", rep)
		}
	}

	// Exact host (and www / deep-link / trailing-punctuation variants) still pass.
	accept := []string{
		"Web https://thgfulfill.com nhé.",
		"Web https://www.thgfulfill.com nhé.",
		"Dịch vụ https://thgfulfill.com/thg-fulfill nhé.",
		"Web https://thgfulfill.com, inbox em nhé.", // trailing comma captured by the regex
	}
	for _, in := range accept {
		rep, _ := RepairCommentContacts(in, id)
		if !strings.Contains(rep, "thgfulfill.com") {
			t.Errorf("exact host must be kept, dropped in %q → %q", in, rep)
		}
		if ok, r := ScreenCommentContacts(rep, id); !ok {
			t.Errorf("repaired exact-host comment must pass, got %s for %q", r, in)
		}
	}
}

// TestRepairKeepsTelegramHandleNotCountedAsURL guards the rule that a Telegram/Zalo
// handle is NOT a URL: a comment with the website (1 URL) plus a bare @handle stays
// at one URL and passes — the @handle must never be miscounted as a second link.
func TestRepairKeepsTelegramHandleNotCountedAsURL(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com", OfficialContact: "@Moonzzz03"}
	in := "Web https://thgfulfill.com, Telegram @Moonzzz03 nhé."
	if ok, r := ScreenCommentContacts(in, id); !ok {
		t.Errorf("website + bare @handle should be a single URL and pass, got %s", r)
	}
}

// TestEnsureWebsite pins the deterministic website guarantee (Sprint-6 follow-up):
// a configured website appears EXACTLY ONCE, is never invented, and never duplicated.
func TestEnsureWebsite(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com/vi", OfficialContact: "Telegram @hairypotter98 · Zalo 0949716391"}

	// Model OMITTED the website (staff contact only) → appended exactly once.
	in := "Bên mình nhận fulfill US nhé. Inbox Telegram @hairypotter98 hoặc Zalo 0949716391."
	out, added := EnsureWebsite(in, id)
	if !added {
		t.Fatalf("expected website to be appended when omitted")
	}
	if got := strings.Count(out, "thgfulfill.com"); got != 1 {
		t.Fatalf("website must appear exactly once, got %d in %q", got, out)
	}
	if !strings.Contains(out, "https://thgfulfill.com/vi") {
		t.Fatalf("expected canonical website in %q", out)
	}
	// Staff contact channels are untouched; no company hotline/email invented.
	if !strings.Contains(out, "@hairypotter98") || !strings.Contains(out, "0949716391") {
		t.Fatalf("staff contact channels must be preserved, got %q", out)
	}

	// Website ALREADY present (canonical) → no-op, no duplicate.
	has := "Ghé https://thgfulfill.com/vi xem nhé, inbox @hairypotter98."
	if out2, added2 := EnsureWebsite(has, id); added2 || out2 != has {
		t.Fatalf("present website must be a no-op, got added=%v %q", added2, out2)
	}

	// Website present as a non-canonical variant (no scheme / www) → host matches → no-op.
	variant := "Xem www.thgfulfill.com/vi nha."
	if _, added3 := EnsureWebsite(variant, id); added3 {
		t.Fatalf("a grounded website variant already present must not be appended again")
	}

	// A grounded contact LINK (t.me/handle) is already present and there is no
	// website in the text → no-op, so we never append a SECOND URL.
	tme := "Liên hệ t.me/hairypotter98 nhé."
	if out5, added5 := EnsureWebsite(tme, id); added5 || out5 != tme {
		t.Fatalf("must not append a second URL when a grounded contact link is present, got added=%v %q", added5, out5)
	}

	// EMPTY Website → never invent a URL.
	empty := models.CompanyIdentity{CompanyName: "THG Fulfill", OfficialContact: "Zalo 0949716391"}
	noWeb := "Inbox Zalo 0949716391 nhé."
	if out4, added4 := EnsureWebsite(noWeb, empty); added4 || out4 != noWeb {
		t.Fatalf("no website configured must never invent a URL, got added=%v %q", added4, out4)
	}

	// The appended result stays within the ≤1-URL contact policy (screen-clean).
	if ok, r := ScreenCommentContacts(out, id); !ok {
		t.Fatalf("appended website must keep the comment screen-clean, got %s", r)
	}
}
