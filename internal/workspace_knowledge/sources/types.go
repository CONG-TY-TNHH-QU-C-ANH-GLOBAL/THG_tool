// Package sources is the Layer-1 domain for the Workspace Knowledge
// OS. A Source is an external data connection an operator configured
// for the workspace — Shopify store, CSV upload, Google Sheet, Notion
// database, website, or another catalog.
//
// This package has no database/sql imports. It is the contract; the
// implementation lives in internal/store/knowledge_sources.go.
package sources

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// SourceType is the closed set of source kinds the system supports.
// Adding a value here is a real product decision — it requires (a) a
// concrete Ingestor implementation in workspace_knowledge/ingestion,
// (b) a connection_config JSON schema, and (c) a UI tile in the
// Sources panel. Do not add a value without those three.
type SourceType string

const (
	SourceShopify      SourceType = "shopify"
	SourceCSV          SourceType = "csv"
	SourceGoogleSheets SourceType = "google_sheets"
	SourceNotion       SourceType = "notion"
	SourceWebsite      SourceType = "website"
	SourceCatalog      SourceType = "catalog"
	// SourceRESTJSON is the generic configurable REST/JSON ingestor.
	// A tenant points it at any HTTP endpoint that returns JSON; the
	// connection_config carries the field_map, pagination scheme and
	// auth config. There is no per-tenant hardcoded vendor — the
	// platform's own catalog hub is just one config among many. See
	// internal/workspace_knowledge/ingestion/rest_json.
	SourceRESTJSON SourceType = "rest_json"
)

// IsKnown reports whether t is a SourceType the system currently
// supports. Used at deserialization boundaries to reject unknown
// values explicitly instead of letting them flow into downstream code
// that branches on string comparison. Per
// feedback_no_implicit_business_meaning.md, every business question
// goes through a resolver — IsKnown is the resolver for "do we know
// this source type?".
func (t SourceType) IsKnown() bool {
	switch t {
	case SourceShopify, SourceCSV, SourceGoogleSheets, SourceNotion, SourceWebsite, SourceCatalog, SourceRESTJSON:
		return true
	}
	return false
}

// SyncPolicy is how often the runtime should re-pull from the source.
// "manual" means the operator triggers each sync; the scheduler
// otherwise reads this column to decide what's due.
type SyncPolicy string

const (
	SyncRealtime SyncPolicy = "realtime"
	SyncHourly   SyncPolicy = "hourly"
	SyncDaily    SyncPolicy = "daily"
	SyncManual   SyncPolicy = "manual"
)

func (p SyncPolicy) IsKnown() bool {
	switch p {
	case SyncRealtime, SyncHourly, SyncDaily, SyncManual:
		return true
	}
	return false
}

// HealthStatus reports the source's current operational state. It is
// a fact about the LAST sync, not a derived "currently working"
// boolean — derive that from the value as needed.
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthSyncing   HealthStatus = "syncing"
	HealthStale     HealthStatus = "stale"
	HealthError     HealthStatus = "error"
	HealthNeedsAuth HealthStatus = "needs_auth"
)

func (h HealthStatus) IsKnown() bool {
	switch h {
	case HealthHealthy, HealthSyncing, HealthStale, HealthError, HealthNeedsAuth:
		return true
	}
	return false
}

// Health bundles the three columns the schema persists together
// (health_status, health_message, last_sync_at). They are read and
// written as a unit so callers do not get a half-updated row where
// status flipped to "error" but the message is still last week's.
//
// LastSyncAt is a pointer because nil ("never synced") is meaningfully
// different from the zero time. See specs/domains/knowledge-platform/features/knowledge-os/technical.md §5.
type Health struct {
	Status     HealthStatus
	Message    string
	LastSyncAt *time.Time
}

// Source is the cross-boundary representation of a knowledge_sources row.
//
// Operators construct Sources through the handlers; the runtime reads
// them via the repository; the ingestor consumes them when it pulls.
// Persistence is one direction below this type, NOT this type itself.
type Source struct {
	ID    int64
	OrgID int64

	Type             SourceType
	Label            string
	ConnectionConfig json.RawMessage // type-specific; opaque to this package
	SyncPolicy       SyncPolicy

	Health         Health
	LastAssetCount int

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MarshalJSON emits the wire shape the frontend (and any HTTP client)
// consumes: snake_case keys with the Health bundle FLATTENED into
// health_status / health_message / last_sync_at. Without this the
// default marshaller used the Go field names (PascalCase) and nested
// Health, so the frontend's KnowledgeSource read `id`/`last_asset_count`
// as undefined — which made the Connect-Catalog wizard POST
// /knowledge/sources/undefined/sync and the server reject it with
// "invalid source id". Source is only ever marshaled OUT (handler input
// uses createSourceBody; the store scans columns, never JSON), so a
// custom marshaller here cannot break an inbound parse.
//
// Value receiver so it applies to both Source and *Source (list
// responses use one, single-source responses the other).
func (s Source) MarshalJSON() ([]byte, error) {
	cc := s.ConnectionConfig
	if len(cc) == 0 {
		cc = json.RawMessage("null")
	}
	return json.Marshal(struct {
		ID               int64           `json:"id"`
		OrgID            int64           `json:"org_id"`
		Type             SourceType      `json:"type"`
		Label            string          `json:"label"`
		ConnectionConfig json.RawMessage `json:"connection_config"`
		SyncPolicy       SyncPolicy      `json:"sync_policy"`
		HealthStatus     HealthStatus    `json:"health_status"`
		HealthMessage    string          `json:"health_message"`
		LastSyncAt       *time.Time      `json:"last_sync_at"`
		LastAssetCount   int             `json:"last_asset_count"`
		CreatedAt        time.Time       `json:"created_at"`
		UpdatedAt        time.Time       `json:"updated_at"`
	}{
		ID:               s.ID,
		OrgID:            s.OrgID,
		Type:             s.Type,
		Label:            s.Label,
		ConnectionConfig: cc,
		SyncPolicy:       s.SyncPolicy,
		HealthStatus:     s.Health.Status,
		HealthMessage:    s.Health.Message,
		LastSyncAt:       s.Health.LastSyncAt,
		LastAssetCount:   s.LastAssetCount,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	})
}

// Validate enforces the boundary invariants that the schema cannot.
// Schemas catch type mismatches and NOT NULL; this catches "label is
// just whitespace," "type is empty," and "config is not parseable
// JSON." Called by the repository on every write so handlers cannot
// smuggle malformed rows into the store.
func (s *Source) Validate() error {
	if s == nil {
		return errors.New("sources: nil source")
	}
	if s.OrgID <= 0 {
		return errors.New("sources: org_id must be positive")
	}
	if !s.Type.IsKnown() {
		return errors.New("sources: unknown type: " + string(s.Type))
	}
	if strings.TrimSpace(s.Label) == "" {
		return errors.New("sources: label is required")
	}
	if !s.SyncPolicy.IsKnown() {
		// Default at the boundary, not silently downstream.
		s.SyncPolicy = SyncManual
	}
	if len(s.ConnectionConfig) == 0 {
		s.ConnectionConfig = json.RawMessage(`{}`)
	} else if !json.Valid(s.ConnectionConfig) {
		return errors.New("sources: connection_config is not valid JSON")
	}
	if !s.Health.Status.IsKnown() {
		s.Health.Status = HealthHealthy
	}
	return nil
}

// ListFilter narrows a list query. Zero-value means "all sources for
// the org" — the repository never returns sources for a different
// org, so the org_id filter is implicit on the caller.
type ListFilter struct {
	Types  []SourceType   // empty = any type
	Health []HealthStatus // empty = any health
}
