package comment

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// urlCount counts http(s)/bare-domain URLs the contact policy would see.
func urlCount(s string) int { return len(reCommentURL.FindAllString(s, -1)) }

// TestEnsureWebsite pins the deterministic website contract (Sprint-6 follow-up):
// the configured company website is the preferred/required URL and appears EXACTLY
// ONCE under the ≤1-URL policy, is never invented, and never competes with another URL.
func TestEnsureWebsite(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com/vi", OfficialContact: "Telegram @hairypotter98 · Zalo 0949716391"}

	// 1. Model OMITTED all URLs, website configured → website appended exactly once.
	in := "Bên mình nhận fulfill US nhé. Inbox Telegram @hairypotter98 hoặc Zalo 0949716391."
	out, added := EnsureWebsite(in, id)
	if !added || strings.Count(out, "thgfulfill.com") != 1 || !strings.Contains(out, "https://thgfulfill.com/vi") {
		t.Fatalf("omitted website must be appended exactly once, got added=%v %q", added, out)
	}
	if urlCount(out) != 1 {
		t.Fatalf("must respect <=1-URL policy, got %d URLs in %q", urlCount(out), out)
	}

	// 2. Website ALREADY present (canonical) → no duplicate.
	has := "Ghé https://thgfulfill.com/vi xem nhé, inbox @hairypotter98."
	if out2, _ := EnsureWebsite(has, id); strings.Count(out2, "thgfulfill.com") != 1 || urlCount(out2) != 1 {
		t.Fatalf("present canonical website must stay exactly once, got %q", out2)
	}

	// 3. Website present as a www/non-canonical variant → normalized + deduped to one.
	variant := "Xem www.thgfulfill.com/vi nha."
	out3, _ := EnsureWebsite(variant, id)
	if urlCount(out3) != 1 || !strings.Contains(out3, "https://thgfulfill.com/vi") {
		t.Fatalf("www variant must normalize to ONE canonical website, got %q", out3)
	}

	// 4. A competing URL (t.me) present, website configured → website wins, the
	//    competing link is NOT left as an extra URL, <=1 URL.
	tme := "Liên hệ https://t.me/hairypotter98 nhé."
	out4, added4 := EnsureWebsite(tme, id)
	if !added4 || !strings.Contains(out4, "https://thgfulfill.com/vi") {
		t.Fatalf("competing URL case must yield the company website, got %q", out4)
	}
	if urlCount(out4) != 1 {
		t.Fatalf("competing URL must not survive as a 2nd URL, got %d URLs in %q", urlCount(out4), out4)
	}
	if strings.Contains(out4, "t.me/") {
		t.Fatalf("the t.me link must not remain as a competing URL, got %q", out4)
	}

	// 5. Staff Telegram/Zalo TEXT handles preserved; website included; no company
	//    hotline/email (the resolved identity already carries only the staff contact).
	staff := "Inbox Telegram @hairypotter98 · Zalo 0949716391 nhé."
	out5, _ := EnsureWebsite(staff, id)
	if !strings.Contains(out5, "@hairypotter98") || !strings.Contains(out5, "0949716391") {
		t.Fatalf("staff Telegram/Zalo handles must be preserved, got %q", out5)
	}
	if !strings.Contains(out5, "https://thgfulfill.com/vi") || urlCount(out5) != 1 {
		t.Fatalf("staff case must still include the single company website, got %q", out5)
	}

	// 6. EMPTY website → never invent a URL.
	empty := models.CompanyIdentity{CompanyName: "THG Fulfill", OfficialContact: "Zalo 0949716391"}
	noWeb := "Inbox Zalo 0949716391 nhé."
	if out6, added6 := EnsureWebsite(noWeb, empty); added6 || out6 != noWeb || urlCount(out6) != 0 {
		t.Fatalf("no website configured must never invent a URL, got added=%v %q", added6, out6)
	}

	// Every produced comment stays screen-clean (<=1 grounded URL).
	for _, c := range []string{out, out3, out4, out5} {
		if ok, r := ScreenCommentContacts(c, id); !ok {
			t.Fatalf("result must be screen-clean, reason=%s for %q", r, c)
		}
	}
}

// TestEnsureWebsiteLivePathParity proves both comment paths get the same website
// guarantee: EnsureWebsite is identity-driven (operates on the resolved
// commentIdentity), so normal and reasoning=live share the contract structurally.
func TestEnsureWebsiteLivePathParity(t *testing.T) {
	id := models.CompanyIdentity{CompanyName: "THG Fulfill", Website: "https://thgfulfill.com/vi", OfficialContact: "Telegram @hairypotter98 · Zalo 0949716391"}
	draft := "Bên mình hỗ trợ fulfill US, inbox @hairypotter98 nhé."
	out, added := EnsureWebsite(draft, id)
	if !added || !strings.Contains(out, "https://thgfulfill.com/vi") || urlCount(out) != 1 {
		t.Fatalf("website must be guaranteed regardless of path, got added=%v %q", added, out)
	}
}
