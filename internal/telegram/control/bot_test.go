package control_test

import (
	"testing"

	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
)

// noFallback flags: a global token is set but fallback is OFF, so without an org bot credential the
// org has NO usable bot (the enterprise default).
func noFallback() control.Flags {
	return control.Flags{NotifyEnabled: true, GlobalToken: "platform", AllowGlobalFallback: false}
}

func TestSaveBotTokenAndStatus(t *testing.T) {
	fs := &fakeSender{} // GetMe ok (fail=false)
	svc, _ := newSvc(t, "tg_savebot.db", fs, noFallback())

	if reason, ok := svc.SaveBotToken(7, 99, "123:VALIDTOKEN"); !ok || reason != "" {
		t.Fatalf("save valid token: ok=%v reason=%q", ok, reason)
	}
	cred, _ := svc.BotStatus(7)
	if cred == nil || cred.BotUsername != "fakebot" || cred.Status != "active" {
		t.Fatalf("bot status after save: %+v", cred)
	}
	// Invalid token (getMe fails) → typed reason.
	bad := &fakeSender{fail: true}
	svcBad, _ := newSvc(t, "tg_savebot_bad.db", bad, noFallback())
	if reason, ok := svcBad.SaveBotToken(7, 99, "123:BAD"); ok || reason != "bot_token_invalid" {
		t.Fatalf("invalid token: ok=%v reason=%q", ok, reason)
	}
}

// Channel connect uses the ORG's own bot; missing credential → bot_token_missing.
func TestConnectRequiresOrgBot(t *testing.T) {
	fs := &fakeSender{resChatID: -1009, resTitle: "Ch"}
	svc, _ := newSvc(t, "tg_connect_needsbot.db", fs, noFallback())

	// No bot connected yet → connect fails with bot_token_missing (no global fallback).
	if d, reason := svc.ConnectPublicChannel(7, 99, "@chan"); d != nil || reason != "bot_token_missing" {
		t.Fatalf("expected bot_token_missing, got d=%+v reason=%q", d, reason)
	}
	// After the org saves its bot, connect succeeds using THAT token.
	if _, ok := svc.SaveBotToken(7, 99, "123:ORGTOKEN"); !ok {
		t.Fatal("save bot failed")
	}
	d, reason := svc.ConnectPublicChannel(7, 99, "@chan")
	if d == nil || reason != "" {
		t.Fatalf("connect after bot saved: d=%+v reason=%q", d, reason)
	}
}

// Public-channel reference normalization: @handle / handle / t.me/handle / https://t.me/handle all
// resolve to "@handle".
func TestConnectNormalizesUsername(t *testing.T) {
	for _, in := range []string{"THG_Sale_Lead", "@THG_Sale_Lead", "t.me/THG_Sale_Lead", "https://t.me/THG_Sale_Lead"} {
		fs := &fakeSender{resChatID: -100, resTitle: "T"}
		svc, _ := newSvc(t, "tg_norm_"+in[:3]+string(rune('a'+len(in))), fs, control.Flags{NotifyEnabled: true})
		_, _ = svc.ConnectPublicChannel(7, 99, in)
		if fs.lastRef != "@THG_Sale_Lead" {
			t.Fatalf("normalize(%q) -> %q, want @THG_Sale_Lead", in, fs.lastRef)
		}
	}
}

// Revoking the bot disables channel delivery.
func TestRevokeBotDisablesDelivery(t *testing.T) {
	fs := &fakeSender{resChatID: -100}
	svc, st := newSvc(t, "tg_revokebot.db", fs, noFallback())
	_, _ = svc.SaveBotToken(7, 99, "123:TOK")
	_, _ = st.UpsertDestination(tgstore.Destination{OrgID: 7, DestinationType: "channel", ChatID: -100, EventTypes: `["lead_created"]`, ChannelFilter: "all"})
	if n, _ := svc.NotifyEvent(7, "lead_created", "facebook", "hi"); n != 1 {
		t.Fatalf("delivery before revoke = %d", n)
	}
	if err := svc.RevokeBot(7, 99); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.NotifyEvent(7, "lead_created", "facebook", "hi"); n != 0 {
		t.Fatal("after revoke, delivery must stop (bot_token_missing)")
	}
}
