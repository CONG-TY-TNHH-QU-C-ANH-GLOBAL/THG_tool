// DB-backed regression for the telegram control-plane store. Uses the shared storetest schema
// template (one migrate per binary) per STORE_SUBPACKAGE_REFACTOR.
package telegram_test

import (
	"testing"
	"time"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
	"github.com/thg/scraper/internal/store/telegram"
)

// bootstrap opens + migrates a SQLite DB at path (store.New runs all migrations incl. 0013).
func bootstrap(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func newStore(t *testing.T, name string) *telegram.Store {
	dst := storetest.CopyTemplate(t, bootstrap, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.Telegram()
}

func TestSettingsAndAlerts(t *testing.T) {
	s := newStore(t, "tg_settings.db")
	const org = int64(7)

	got, err := s.GetSettings(org)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.Enabled {
		t.Fatal("fresh org must be disabled")
	}
	if err := s.SetEnabled(org, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got, _ = s.GetSettings(org); !got.Enabled {
		t.Fatal("expected enabled after SetEnabled(true)")
	}

	prefs, _ := s.GetAlertPrefs(org)
	if prefs.ChannelFilter != "all" || !prefs.AlertsEnabled {
		t.Fatalf("default alert prefs wrong: %+v", prefs)
	}
	if err := s.UpsertAlertPrefs(org, false, "facebook", `["connector_offline"]`); err != nil {
		t.Fatalf("UpsertAlertPrefs: %v", err)
	}
	prefs, _ = s.GetAlertPrefs(org)
	if prefs.AlertsEnabled || prefs.ChannelFilter != "facebook" || prefs.AlertTypes != `["connector_offline"]` {
		t.Fatalf("alert prefs not persisted: %+v", prefs)
	}
}

func TestBindCodeLifecycle(t *testing.T) {
	s := newStore(t, "tg_codes.db")
	const org, user = int64(7), int64(99)

	bc, err := s.CreateBindCode(org, user, telegram.GenerateCode(8), 10*time.Minute)
	if err != nil {
		t.Fatalf("CreateBindCode: %v", err)
	}
	gotOrg, gotUser, ok, err := s.ConsumeBindCode(bc.Code)
	if err != nil || !ok || gotOrg != org || gotUser != user {
		t.Fatalf("ConsumeBindCode: ok=%v org=%d user=%d err=%v", ok, gotOrg, gotUser, err)
	}
	if _, _, ok, _ := s.ConsumeBindCode(bc.Code); ok {
		t.Fatal("a used code must not be consumable again")
	}
	// Expired code never consumes.
	exp, _ := s.CreateBindCode(org, user, telegram.GenerateCode(8), -1*time.Minute)
	if _, _, ok, _ := s.ConsumeBindCode(exp.Code); ok {
		t.Fatal("an expired code must not be consumable")
	}
}

func TestBindingsAndCounts(t *testing.T) {
	s := newStore(t, "tg_bindings.db")
	const org, user = int64(7), int64(99)

	id, err := s.UpsertBinding(telegram.Binding{
		OrgID: org, UserID: user, TelegramUserID: 555, DisplayName: "Sales A",
		Role: "sales", AlertRecipient: true,
	})
	if err != nil || id == 0 {
		t.Fatalf("UpsertBinding: id=%d err=%v", id, err)
	}
	// Another org's binding must never leak into this org's views.
	_, _ = s.UpsertBinding(telegram.Binding{OrgID: 8, UserID: 1, TelegramUserID: 1})

	all, _ := s.ListBindings(org)
	if len(all) != 1 || all[0].OrgID != org {
		t.Fatalf("ListBindings tenant scope broken: %+v", all)
	}
	mine, _ := s.ListBindingsByUser(org, user)
	if len(mine) != 1 {
		t.Fatalf("ListBindingsByUser: got %d", len(mine))
	}
	counts, _ := s.CountBindings(org)
	if counts.Active != 1 || counts.AlertRecipients != 1 {
		t.Fatalf("counts wrong: %+v", counts)
	}

	if err := s.RevokeBinding(org, id); err != nil {
		t.Fatalf("RevokeBinding: %v", err)
	}
	counts, _ = s.CountBindings(org)
	if counts.Active != 0 {
		t.Fatalf("revoked binding still counted active: %+v", counts)
	}
	b, _ := s.GetBinding(org, id)
	if b == nil || b.Status != "revoked" || !b.RevokedAt.Valid {
		t.Fatalf("binding not revoked: %+v", b)
	}
}

func TestAudit(t *testing.T) {
	s := newStore(t, "tg_audit.db")
	const org = int64(7)
	if err := s.InsertAudit(org, 99, 555, "bind_code_generated", "ok", `{"k":1}`); err != nil {
		t.Fatalf("InsertAudit: %v", err)
	}
	rows, _ := s.ListAudit(org, 50)
	if len(rows) != 1 || rows[0].Action != "bind_code_generated" || rows[0].Result != "ok" {
		t.Fatalf("ListAudit: %+v", rows)
	}
	if other, _ := s.ListAudit(8, 50); len(other) != 0 {
		t.Fatal("audit leaked across orgs")
	}
}
