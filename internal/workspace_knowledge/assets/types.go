// Package assets is the Layer-3 domain for the Workspace Knowledge OS.
// An Asset is a discrete piece of retrievable knowledge — one POD SKU,
// one FAQ entry, one shipping-policy clause, one banned claim, one CTA
// snippet. The catalog is just "all assets where org_id = ? AND state = 'approved'".
//
// This package has no database/sql imports. Persistence lives in
// internal/store/knowledge_assets.go.
package assets

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// AssetType is the closed set of asset kinds. The retrieval engine
// uses Type as a filter and to pick the right prompt template — e.g.
// a POD_product gets injected differently from a CTA snippet.
//
// Adding a value requires (a) a normalization rule in normalize.go,
// (b) a UI badge in the Product Explorer i18n catalog, and (c) a
// decision about how the retrieval engine should weight it. Do not
// add a value without those three.
type AssetType string

const (
	AssetPODProduct     AssetType = "POD_product"
	AssetFAQ            AssetType = "faq"
	AssetShippingPolicy AssetType = "shipping_policy"
	AssetSalesPlaybook  AssetType = "sales_playbook"
	AssetPricingRule    AssetType = "pricing_rule"
	AssetBannedClaim    AssetType = "banned_claim"
	AssetCTA            AssetType = "cta"
)

func (t AssetType) IsKnown() bool {
	switch t {
	case AssetPODProduct, AssetFAQ, AssetShippingPolicy,
		AssetSalesPlaybook, AssetPricingRule, AssetBannedClaim, AssetCTA:
		return true
	}
	return false
}

// AssetState is the operator-controlled lifecycle of an asset.
//
//   - pending  — ingested but not yet reviewed. NOT retrieved by the
//     runtime. Visible in the Product Explorer "Pending" filter.
//   - approved — operator approved; the runtime is allowed to retrieve.
//   - hidden   — operator suppressed; runtime never retrieves but the
//     asset stays in the table so retrieval logs continue to resolve.
//
// Re-ingest of an asset that is already approved does NOT regress it
// to pending. See [Asset.MergeFromIngest] for the merge rule.
type AssetState string

const (
	StatePending  AssetState = "pending"
	StateApproved AssetState = "approved"
	StateHidden   AssetState = "hidden"
)

func (s AssetState) IsKnown() bool {
	switch s {
	case StatePending, StateApproved, StateHidden:
		return true
	}
	return false
}

// Metrics holds the system-derived counters for an asset. These never
// flow back to ingestion: a sync that finds the same SKU again must
// not reset Retrievals30d to zero. The repository writes operator
// fields and metrics through different SQL statements; see the design
// doc §6 ("Operator vs system writes").
type Metrics struct {
	Retrievals30d   int
	Conversions30d  int
	LastRetrievedAt *time.Time // nil = never retrieved
}

// Asset is the cross-boundary representation of a knowledge_assets row.
type Asset struct {
	ID         int64
	OrgID      int64
	SourceID   int64
	ExternalID string // stable ID from the source; "" means CSV-row-hash will be computed by the ingestor

	Type        AssetType
	Title       string
	Description string
	Tags        []string        // already normalized (lower, deduped, trimmed)
	Payload     json.RawMessage // type-specific blob; opaque here

	State  AssetState
	Pinned bool
	Boost  int // 0..100

	Metrics Metrics

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate enforces the boundary invariants the schema cannot. Called
// by the repository on every write.
func (a *Asset) Validate() error {
	if a == nil {
		return errors.New("assets: nil asset")
	}
	if a.OrgID <= 0 {
		return errors.New("assets: org_id must be positive")
	}
	if a.SourceID <= 0 {
		return errors.New("assets: source_id must be positive")
	}
	if !a.Type.IsKnown() {
		return errors.New("assets: unknown type: " + string(a.Type))
	}
	if strings.TrimSpace(a.Title) == "" {
		return errors.New("assets: title is required")
	}
	if !a.State.IsKnown() {
		a.State = StatePending
	}
	if a.Boost < 0 {
		a.Boost = 0
	}
	if a.Boost > 100 {
		a.Boost = 100
	}
	if len(a.Payload) == 0 {
		a.Payload = json.RawMessage(`{}`)
	} else if !json.Valid(a.Payload) {
		return errors.New("assets: payload is not valid JSON")
	}
	return nil
}

// MergeFromIngest merges fresh ingestor data into an existing asset.
// This is the rule that protects operator state during re-sync:
//
//   - Ingestor controls: Title, Description, Tags, Payload, ExternalID.
//   - Operator controls: State, Pinned, Boost.
//   - System controls:   Metrics, CreatedAt, UpdatedAt.
//
// If a previously-approved asset is re-ingested, it stays approved.
// If a hidden asset is re-ingested, it stays hidden (operator chose
// to suppress; ingestor data is irrelevant to that choice). This is
// load-bearing — see invariant 3 in the design doc.
//
// The caller MUST start from the existing asset (loaded from the
// store) and call MergeFromIngest with the fresh data. Repositories
// expose this through UpsertKnowledgeAsset.
func (a *Asset) MergeFromIngest(fresh *Asset) {
	if fresh == nil {
		return
	}
	// Ingestor-controlled fields — always overwrite.
	a.Type = fresh.Type
	a.Title = fresh.Title
	a.Description = fresh.Description
	a.Tags = fresh.Tags
	a.Payload = fresh.Payload
	if fresh.ExternalID != "" {
		a.ExternalID = fresh.ExternalID
	}
	// State, Pinned, Boost, Metrics, CreatedAt — NOT touched. Operator
	// and system fields survive the re-sync. UpdatedAt is managed by the
	// SQL CURRENT_TIMESTAMP on the row.
}

// ListFilter narrows a list query in the repository layer.
// All filters are AND-combined. Org isolation is enforced by the
// repository receiver — there is no OrgID field here.
type ListFilter struct {
	Types     []AssetType   // empty = any type
	States    []AssetState  // empty = any state. Note: the retrieval engine
	                        // overrides this to {approved} when reading on
	                        // the runtime hot path; the Product Explorer
	                        // panel reads with empty (to show pending+hidden).
	SourceID  int64         // 0 = any source
	SearchQ   string        // case-insensitive substring across title + tags
	Limit     int           // 0 = no limit
	Offset    int
	OrderBy   ListOrder
}

type ListOrder string

const (
	// OrderDefault: pinned DESC, boost DESC, retrieval_count_30d DESC.
	// Matches idx_knowledge_assets_org_pin_boost so the hot path is index-only.
	OrderDefault ListOrder = ""
	// OrderRecent: updated_at DESC. Used for the "what changed today" view.
	OrderRecent ListOrder = "recent"
)
