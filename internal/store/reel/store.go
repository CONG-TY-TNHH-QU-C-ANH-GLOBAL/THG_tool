// Package reel owns the reel-generation tables: reels, reel_scripts, reel_shots.
// One reel = one short video produced by an AI script + an external render provider,
// then posted through the existing outbound spine as a `post_reel` action.
//
// Every query is org-scoped (tenant isolation, per check_tenant_isolation.sh). The
// domain encodes the money invariant: render spend is committed once started and never
// cancelled — see render_idempotency_key (double-charge guard), render_lease_expiry
// (orphan detection → render_stuck), and the per-shot CAS in shots.go. The store holds
// NO cross-domain handles; queueing the finished video into outbound is the workflow
// layer's job (internal/services/reel), not this package's.
package reel

import (
	"database/sql"

	"github.com/thg/scraper/internal/store/dbutil"
)

// Reel lifecycle states (reels.status). There is deliberately no transition from
// rendering → cancelled: once render is called, spend is committed (money invariant).
const (
	StatusDraft        = "draft"
	StatusScripting    = "scripting"
	StatusScriptReady  = "script_ready"
	StatusApproved     = "approved"
	StatusRendering    = "rendering"
	StatusRenderDone   = "render_done"
	StatusAssembled    = "assembled"
	StatusPosting      = "posting"
	StatusPublished    = "published"
	StatusRenderStuck  = "render_stuck"  // lease orphan; a human resolves it
	StatusHumanReq     = "human_required" // connector login/challenge during posting
	StatusFailed       = "failed"
)

// Per-shot render states (reel_shots.render_state), mirroring the outbound CAS pattern.
const (
	ShotPlanned        = "planned"
	ShotRendering      = "rendering"
	ShotDone           = "done"
	ShotFailed         = "failed"
	ShotRetryScheduled = "retry_scheduled"
)

// Store provides reel-domain data access. No cross-domain Hooks — reel has zero
// cross-domain writes (it reads/writes only its own three tables).
type Store struct {
	db      *sql.DB
	dialect dbutil.Dialect
}

// NewStore constructs a reel Store.
func NewStore(db *sql.DB, dialect dbutil.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Dialect returns the SQL flavour.
func (s *Store) Dialect() dbutil.Dialect { return s.dialect }

// Reel is the aggregate row. JSON fields (keywords/product_refs) are stored as
// raw JSON strings; the workflow layer owns (de)serialisation.
type Reel struct {
	OrgID                int64   `json:"org_id"`
	ID                   int64   `json:"id"`
	MissionID            string  `json:"mission_id"`
	CreatedBy            int64   `json:"created_by"`
	Source               string  `json:"source"`
	Status               string  `json:"status"`
	BriefStyle           string  `json:"brief_style"`
	Keywords             string  `json:"keywords"`
	ProductRefs          string  `json:"product_refs"`
	TargetDurationSec    int     `json:"target_duration_sec"`
	RenderIdempotencyKey string  `json:"render_idempotency_key"`
	FinalOutputKey       string  `json:"final_output_key"`
	TotalCostUSD         float64 `json:"total_cost_usd"`
}

// Script is a versioned script + shot-list for a reel.
type Script struct {
	ID          int64  `json:"id"`
	ReelID      int64  `json:"reel_id"`
	OrgID       int64  `json:"org_id"`
	Version     int    `json:"version"`
	Dialogue    string `json:"dialogue"`
	ShotList    string `json:"shot_list"`
	Caption     string `json:"caption"`
	VerifyFlags string `json:"verify_flags"`
	Approved    bool   `json:"approved"`
}

// Shot is one render unit. provider_job_id is the webhook match key for idempotency.
type Shot struct {
	ID            int64   `json:"id"`
	ReelID        int64   `json:"reel_id"`
	OrgID         int64   `json:"org_id"`
	Scene         int     `json:"scene"`
	Kind          string  `json:"kind"`
	RenderState   string  `json:"render_state"`
	Provider      string  `json:"provider"`
	ProviderJobID string  `json:"provider_job_id"`
	OutputKey     string  `json:"output_key"`
	CostUSD       float64 `json:"cost_usd"`
	Attempts      int     `json:"attempts"`
}
