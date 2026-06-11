package control

import "regexp"

// A Telegram bot token is "<bot_id>:<url-safe chars>". We use the SHAPE only — never the value —
// to tell a correctly-decrypted token from decryption GARBAGE. Decryption junk is the base64 of the
// stored ciphertext (no ':' — not in the base64 alphabet), so "digits ':' suffix" reliably means a
// real token while base64 junk does not. This lets the platform distinguish an INTERNAL
// misconfiguration (a runtime whose ENCRYPTION_KEY does not match the one that encrypted the
// credential → decrypt yields junk) from a real "no bot connected" state, WITHOUT blocking
// keyless-dev (tokens stored as plaintext round-trip intact and keep their real shape).
var botTokenShape = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{3,}$`)

func looksLikeBotToken(tok string) bool { return botTokenShape.MatchString(tok) }

// reasonPlatformConfig marks failures that are the PLATFORM's responsibility (deployment/secret
// config), never the customer's Telegram setup. The UI must surface these as an admin-config
// message and must NOT mark the customer's channel as wrong.
const reasonPlatformConfig = "platform_config_missing"

// EncryptionHealthy reports whether THIS runtime can correctly decrypt the org's stored bot token.
// true when there is no credential (nothing to decrypt) OR the decrypted token has a valid shape.
// false means a credential exists but decrypts to junk → internal ENCRYPTION_KEY misconfiguration.
func (s *Service) EncryptionHealthy(orgID int64) bool {
	tok, ok := s.store.GetDecryptedBotToken(orgID)
	if !ok || tok == "" {
		return true // no stored credential → nothing for the platform to decrypt
	}
	return looksLikeBotToken(tok)
}
