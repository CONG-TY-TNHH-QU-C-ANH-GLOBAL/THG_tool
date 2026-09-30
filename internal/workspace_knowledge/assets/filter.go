package assets

// ListFilter narrows a list query in the repository layer.
// All filters are AND-combined. Org isolation is enforced by the
// repository receiver — there is no OrgID field here.
type ListFilter struct {
	Types  []AssetType  // empty = any type
	States []AssetState // empty = any state. Note: the retrieval engine
	// overrides this to {approved} when reading on
	// the runtime hot path; the Product Explorer
	// panel reads with empty (to show pending+hidden).
	SourceID int64  // 0 = any source
	SearchQ  string // case-insensitive substring across title + tags
	Limit    int    // 0 = no limit
	Offset   int
	OrderBy  ListOrder
	// Hot path only: AI never quotes stale/error/needs_auth sources.
	ExcludeUnhealthySources bool
}

type ListOrder string

const (
	// OrderDefault: pinned DESC, boost DESC, retrieval_count_30d DESC.
	// Matches idx_knowledge_assets_org_pin_boost so the hot path is index-only.
	OrderDefault ListOrder = ""
	// OrderRecent: updated_at DESC. Used for the "what changed today" view.
	OrderRecent ListOrder = "recent"
)
