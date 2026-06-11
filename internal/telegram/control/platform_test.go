package control_test

import (
	"testing"

	"github.com/thg/scraper/internal/telegram/control"
)

// A stored bot token encrypted under the server's key, read by a runtime WITHOUT that key, decrypts
// to junk. The dispatcher must detect this as an INTERNAL platform config problem: report
// EncryptionHealthy=false and REFUSE to send a garbage token (no customer-facing delivery failure).
func TestPlatformConfigMissingOnKeyMismatch(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_platform.db", fs, control.Flags{NotifyEnabled: true})
	_, _ = st.UpsertDestination(dest(7, -100, `["lead_created"]`))

	// Save a real-shaped token encrypted under the server key, then drop the key (wrong runtime).
	st.SetEncryptionKey("server-encryption-key")
	if err := st.UpsertBotCredential(7, 1, "123456789:AAExampleRealLookingToken-abc_DEF", 123456789, "bot", "Bot"); err != nil {
		t.Fatal(err)
	}
	st.SetEncryptionKey("") // a runtime without the matching key → decrypt yields base64 junk

	if svc.EncryptionHealthy(7) {
		t.Fatal("expected EncryptionHealthy=false when the stored token cannot be decrypted")
	}
	svc.NotifyLead(control.LeadNotice{OrgID: 7, LeadID: 1, Excerpt: "đang tìm supplier", BaseURL: "https://x.com"})
	if len(fs.sent) != 0 {
		t.Fatalf("must NOT send a garbage token (got %d sends)", len(fs.sent))
	}

	// Healthy once the matching key is present again (round-trips to a valid-shaped token).
	st.SetEncryptionKey("server-encryption-key")
	if !svc.EncryptionHealthy(7) {
		t.Fatal("expected EncryptionHealthy=true with the correct key")
	}
}
