package telegram_test

import (
	"encoding/json"
	"strings"
	"testing"

	tgstore "github.com/thg/scraper/internal/store/telegram"
)

func TestBotCredentialEncryptionAndScope(t *testing.T) {
	s := newStore(t, "tg_botcred.db")
	s.SetEncryptionKey("test-encryption-key") // exercise real AES-GCM (empty key = no-op)
	const token = "987654321:AA_PLAINTEXT_TOKEN_VALUE"

	if err := s.UpsertBotCredential(7, 99, token, 987654321, "thg_sale_bot", "THG Sale"); err != nil {
		t.Fatalf("UpsertBotCredential: %v", err)
	}

	// The raw column must be CIPHERTEXT, not the plaintext token.
	var raw string
	if err := s.DB().QueryRow(`SELECT token_encrypted FROM telegram_bot_credentials WHERE org_id = 7`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == token || strings.Contains(raw, "PLAINTEXT") {
		t.Fatalf("token stored in plaintext: %q", raw)
	}

	// Internal decryption returns the original token.
	if tok, ok := s.GetDecryptedBotToken(7); !ok || tok != token {
		t.Fatalf("decrypt round-trip failed: ok=%v tok=%q", ok, tok)
	}

	// The metadata DTO must NOT contain the token (only safe fields + last4).
	cred, _ := s.GetBotCredential(7)
	if cred == nil || cred.BotUsername != "thg_sale_bot" || cred.TokenLast4 != "ALUE" {
		t.Fatalf("credential metadata wrong: %+v", cred)
	}
	if b, _ := json.Marshal(cred); strings.Contains(string(b), "PLAINTEXT") || strings.Contains(string(b), token) {
		t.Fatalf("token leaked in DTO JSON: %s", b)
	}

	// Org-scoped: org 8 has no credential.
	if _, ok := s.GetDecryptedBotToken(8); ok {
		t.Fatal("org 8 must not see org 7's bot token")
	}
	if c8, _ := s.GetBotCredential(8); c8 != nil {
		t.Fatal("org 8 must have no credential")
	}

	// Revoke wipes the token + flips status.
	if err := s.RevokeBotCredential(7); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetDecryptedBotToken(7); ok {
		t.Fatal("revoked credential must not return a token")
	}
	if c, _ := s.GetBotCredential(7); c == nil || c.Status != "revoked" {
		t.Fatalf("status after revoke = %+v", c)
	}
}

// Reconnecting a different token replaces in place (one bot per org).
func TestBotCredentialReplace(t *testing.T) {
	s := newStore(t, "tg_botcred2.db")
	_ = s.UpsertBotCredential(7, 99, "111:AAA", 111, "old_bot", "Old")
	_ = s.UpsertBotCredential(7, 99, "222:BBB", 222, "new_bot", "New")
	cred, _ := s.GetBotCredential(7)
	if cred.BotUsername != "new_bot" || cred.Status != "active" {
		t.Fatalf("replace failed: %+v", cred)
	}
	if tok, _ := s.GetDecryptedBotToken(7); tok != "222:BBB" {
		t.Fatalf("token not replaced: %q", tok)
	}
	_ = tgstore.BotCredential{} // keep import
}
