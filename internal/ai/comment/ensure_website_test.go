package comment

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// ewIdentity is the standard staff+website identity used across the EnsureWebsite
// contract tests (staff contact channels, company website configured).
func ewIdentity() models.CompanyIdentity {
	return models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com/vi", OfficialContact: "Telegram @hairypotter98 · Zalo 0949716391"}
}

// urlCount counts http(s)/bare-domain URLs the contact policy would see.
func urlCount(s string) int { return len(reCommentURL.FindAllString(s, -1)) }

func wantContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("expected %q in %q", sub, s)
	}
}

func wantURLCount(t *testing.T, s string, n int) {
	t.Helper()
	if got := urlCount(s); got != n {
		t.Fatalf("expected %d URL(s), got %d in %q", n, got, s)
	}
}

func wantScreenClean(t *testing.T, id models.CompanyIdentity, s string) {
	t.Helper()
	if ok, r := ScreenCommentContacts(s, id); !ok {
		t.Fatalf("result must be screen-clean, reason=%s for %q", r, s)
	}
}

// Cases 1–3: omitted website appended exactly once; canonical already present kept
// once; a www/non-canonical variant normalized + deduped to ONE canonical website.
func TestEnsureWebsitePresentOrAppended(t *testing.T) {
	id := ewIdentity()

	out, added := EnsureWebsite("Bên mình nhận fulfill US. Inbox Telegram @hairypotter98 hoặc Zalo 0949716391.", id)
	if !added {
		t.Fatal("omitted website must be appended")
	}
	wantContains(t, out, "https://thgfulfill.com/vi")
	wantURLCount(t, out, 1)
	if strings.Count(out, "thgfulfill.com") != 1 {
		t.Fatalf("website must appear exactly once: %q", out)
	}
	wantScreenClean(t, id, out)

	present, _ := EnsureWebsite("Ghé https://thgfulfill.com/vi xem nhé, inbox @hairypotter98.", id)
	wantURLCount(t, present, 1)

	variant, _ := EnsureWebsite("Xem www.thgfulfill.com/vi nha.", id)
	wantContains(t, variant, "https://thgfulfill.com/vi")
	wantURLCount(t, variant, 1)
	wantScreenClean(t, id, variant)
}

// Case 4: a competing URL (t.me) is replaced by the company website and never left
// as a second URL — the website is the single preferred URL.
func TestEnsureWebsiteReplacesCompetingURL(t *testing.T) {
	id := ewIdentity()
	out, added := EnsureWebsite("Liên hệ https://t.me/hairypotter98 nhé.", id)
	if !added {
		t.Fatal("competing-URL comment must gain the company website")
	}
	wantContains(t, out, "https://thgfulfill.com/vi")
	wantURLCount(t, out, 1)
	if strings.Contains(out, "t.me/") {
		t.Fatalf("t.me link must not remain as a competing URL: %q", out)
	}
	wantScreenClean(t, id, out)
}

// Case 5: staff Telegram/Zalo TEXT handles survive; website still the single URL.
func TestEnsureWebsitePreservesStaffHandles(t *testing.T) {
	id := ewIdentity()
	out, _ := EnsureWebsite("Inbox Telegram @hairypotter98 · Zalo 0949716391 nhé.", id)
	wantContains(t, out, "@hairypotter98")
	wantContains(t, out, "0949716391")
	wantContains(t, out, "https://thgfulfill.com/vi")
	wantURLCount(t, out, 1)
}

// Case 6: empty website → never invent a URL.
func TestEnsureWebsiteEmptyNeverInvents(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", OfficialContact: "Zalo 0949716391"}
	in := "Inbox Zalo 0949716391 nhé."
	out, added := EnsureWebsite(in, id)
	if added || out != in {
		t.Fatalf("empty website must never invent a URL, got added=%v %q", added, out)
	}
	wantURLCount(t, out, 0)
}

// reasoning=live parity: the guard is identity-driven, so both comment paths get
// the same single company website guarantee.
func TestEnsureWebsiteLivePathParity(t *testing.T) {
	id := ewIdentity()
	out, added := EnsureWebsite("Bên mình hỗ trợ fulfill US, inbox @hairypotter98 nhé.", id)
	if !added {
		t.Fatal("live path must also guarantee the website")
	}
	wantContains(t, out, "https://thgfulfill.com/vi")
	wantURLCount(t, out, 1)
}
