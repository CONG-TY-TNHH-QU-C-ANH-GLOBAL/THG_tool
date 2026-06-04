package sources

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSourceMarshalJSONContract locks the wire shape the frontend
// (KnowledgeSource in knowledgeService.ts) binds to. The struct has no
// field tags and a nested Health bundle, so without the custom
// MarshalJSON the keys were PascalCase + nested — which made the
// Connect-Catalog wizard read `id`/`last_asset_count` as undefined and
// POST /knowledge/sources/undefined/sync (server: "invalid source id").
func TestSourceMarshalJSONContract(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	s := Source{
		ID:               7,
		OrgID:            5,
		Type:             SourceRESTJSON,
		Label:            "Catalog THG",
		ConnectionConfig: json.RawMessage(`{"base_url":"https://hub.thgfulfill.com"}`),
		SyncPolicy:       SyncManual,
		Health:           Health{Status: HealthHealthy, Message: "ok", LastSyncAt: &now},
		LastAssetCount:   42,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Every key the frontend reads MUST be present, snake_case, flat.
	for _, k := range []string{
		"id", "org_id", "type", "label", "connection_config", "sync_policy",
		"health_status", "health_message", "last_sync_at", "last_asset_count",
		"created_at", "updated_at",
	} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing wire key %q in %s", k, string(b))
		}
	}
	// The bug signatures: PascalCase / nested Health must NOT appear.
	for _, bad := range []string{"ID", "OrgID", "Health", "LastAssetCount"} {
		if _, ok := m[bad]; ok {
			t.Errorf("leaked Go-field key %q (frontend cannot read it)", bad)
		}
	}
	if string(m["id"]) != "7" {
		t.Errorf("id = %s, want 7", m["id"])
	}
	if string(m["last_asset_count"]) != "42" {
		t.Errorf("last_asset_count = %s, want 42", m["last_asset_count"])
	}
	if string(m["health_status"]) != `"healthy"` {
		t.Errorf("health_status = %s, want \"healthy\"", m["health_status"])
	}
}
