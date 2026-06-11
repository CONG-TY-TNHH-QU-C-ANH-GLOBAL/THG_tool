package telegram_test

import (
	"testing"

	tgstore "github.com/thg/scraper/internal/store/telegram"
)

func TestDestinationsCRUD(t *testing.T) {
	s := newStore(t, "tg_dest.db")
	const org = int64(7)

	id, err := s.UpsertDestination(tgstore.Destination{
		OrgID: org, DestinationType: "channel", ChatID: -1001234, Title: "THG Ops", Username: "thgops",
		EventTypes: `["lead_created"]`, ChannelFilter: "all",
	})
	if err != nil || id == 0 {
		t.Fatalf("UpsertDestination: id=%d err=%v", id, err)
	}

	// Reconnect SAME (org, chat) → updates the existing row, not a duplicate.
	if _, err := s.UpsertDestination(tgstore.Destination{OrgID: org, DestinationType: "channel", ChatID: -1001234, Title: "THG Ops v2"}); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListDestinations(org)
	if len(all) != 1 || all[0].Title != "THG Ops v2" {
		t.Fatalf("reconnect should update in place: %+v", all)
	}

	// Another org with the same chat id is a SEPARATE row (tenant isolation).
	_, _ = s.UpsertDestination(tgstore.Destination{OrgID: 8, DestinationType: "channel", ChatID: -1001234})
	if other, _ := s.ListDestinations(org); len(other) != 1 {
		t.Fatalf("tenant leak: org7 sees %d destinations", len(other))
	}

	// Preferences round-trip.
	if err := s.UpdateDestinationPreferences(org, id, `["comment_submitted","comment_failed"]`, "facebook"); err != nil {
		t.Fatal(err)
	}
	d, _ := s.GetDestination(org, id)
	if d.ChannelFilter != "facebook" || d.EventTypes != `["comment_submitted","comment_failed"]` {
		t.Fatalf("prefs not persisted: %+v", d)
	}

	// Delivery result: ok stamps last_delivery_at; failure flips to needs_attention.
	_ = s.RecordDelivery(org, id, true, "")
	if d, _ = s.GetDestination(org, id); !d.LastDeliveryAt.Valid || d.Status != "active" {
		t.Fatalf("ok delivery: %+v", d)
	}
	_ = s.RecordDelivery(org, id, false, "chat not found")
	if d, _ = s.GetDestination(org, id); d.Status != "needs_attention" || d.LastError == "" {
		t.Fatalf("failed delivery should set needs_attention: %+v", d)
	}

	// Active list + count exclude disabled.
	if n, _ := s.CountDestinations(org); n != 1 {
		t.Fatalf("active count = %d", n)
	}
	_ = s.DisableDestination(org, id)
	if act, _ := s.ListActiveDestinations(org); len(act) != 0 {
		t.Fatal("disabled destination must not be active")
	}
	if n, _ := s.CountDestinations(org); n != 0 {
		t.Fatalf("count after disable = %d", n)
	}
}
