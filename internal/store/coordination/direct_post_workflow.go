package coordination

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Direct-post intake → comment continuation workflow (spec:
// specs/domains/facebook-sales-intelligence/features/direct-post-intake/technical.md). PR-1 = DATA FOUNDATION ONLY: typed CRUD +
// CAS/lease transitions over direct_post_comment_workflows. NO runtime poller, NO
// Copilot/Telegram/comment behavior here — that is PR-2. Coordination owns this as
// process-manager runtime state; it imports NO leads/outbound (single-table CRUD).

// Workflow statuses (direct_post_comment_workflows.status). Persisted as TEXT — no
// DB CHECK constraint (repo style); this is the canonical valid set.
const (
	DPStatusRequested            = "requested"
	DPStatusImportQueued         = "import_queued"
	DPStatusImporting            = "importing"
	DPStatusLeadCreated          = "lead_created"
	DPStatusCommentQueued        = "comment_queued"
	DPStatusCompleted            = "completed"
	DPStatusRetryScheduled       = "retry_scheduled"
	DPStatusFailed               = "failed"
	DPStatusCancelled            = "cancelled"
	DPStatusConnectorUnavailable = "connector_unavailable"
	DPStatusLoginRequired        = "login_required"
	DPStatusChallengeRequired    = "challenge_required"
	DPStatusImportFailed         = "import_failed"
	DPStatusLeadUpsertFailed     = "lead_upsert_failed"
	DPStatusCommentFailed        = "comment_failed"
)

// Process-manager tuning (consumed by the PR-2 poller; defined here so PR-2 has no
// magic numbers). MaxRetryCount=5 with exponential backoff (BaseRetryDelay<<n) spans a
// ~31-minute window (1+2+4+8+16 min) before giving up — generous for normal connector
// import latency. DefaultLeaseDuration mirrors the reverify claim lease so a crashed
// poller's claim is re-offered within 5 min.
const (
	DPMaxRetryCount        = 5
	DPDefaultLeaseDuration = 5 * time.Minute
	DPBaseRetryDelay       = 1 * time.Minute
)

// DPErrLeadNotObserved is the terminal error_code when the post lead never appears
// within the retry window. We only observe the LEAD (no job-status oracle), so this is
// honest: it does NOT claim a connector/import failure that we cannot actually confirm.
const DPErrLeadNotObserved = "lead_not_observed_after_retries"

// DPErrIdentityMismatch is the terminal error_code when a post lead sharing the
// requested post id exists but its GROUP/source context conflicts with the requested
// post (a different group, or a generic permalink.php lead). We refuse to comment on a
// possibly-wrong post and surface it for operator review instead.
const DPErrIdentityMismatch = "imported_lead_identity_mismatch"

// DirectPostCommentWorkflow is one row of direct_post_comment_workflows.
type DirectPostCommentWorkflow struct {
	ID                int64
	OrgID             int64
	RequestedByUserID int64
	UserRole          string
	AccountID         int64
	CanonicalPostURL  string
	PostFBID          string
	GroupRef          string
	Prompt            string
	LeadID            sql.NullInt64
	ImportTaskID      string
	Status            string
	IntakeKey         string
	IdempotencyKey    string
	ErrorCode         string
	ErrorMessage      string
	RetryCount        int
	LeaseOwner        string
	LeaseUntil        sql.NullTime
	NextRunAt         sql.NullTime
	LastAttemptAt     sql.NullTime
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CompletedAt       sql.NullTime
	ExpiresAt         sql.NullTime
}

// DirectPostWorkflowInput is the create payload. Keys are DERIVED from these
// fields (DirectPostIntakeKey / DirectPostIdempotencyKey) — callers do not pass raw
// keys, so the two-key semantics stay centralized.
type DirectPostWorkflowInput struct {
	OrgID             int64
	RequestedByUserID int64
	UserRole          string
	AccountID         int64
	CanonicalPostURL  string
	PostFBID          string
	GroupRef          string
	Prompt            string
}

// DirectPostIntakeKey scopes a SINGLE-POST IMPORT: org + canonical post URL. One
// post is imported once; the imported lead is shared across requesters.
func DirectPostIntakeKey(orgID int64, canonicalPostURL string) string {
	return fmt.Sprintf("%d|%s", orgID, strings.TrimSpace(canonicalPostURL))
}

// DirectPostIdempotencyKey scopes a COMMENT-WORKFLOW REQUEST: org + canonical post
// URL + acting account + requesting user + action. Distinct actors/accounts may each
// request a comment on the same post without colliding (the UNIQUE boundary).
func DirectPostIdempotencyKey(orgID, accountID, userID int64, canonicalPostURL string) string {
	return fmt.Sprintf("%d|%s|a%d|u%d|comment", orgID, strings.TrimSpace(canonicalPostURL), accountID, userID)
}

const dpwColumns = `id, org_id, requested_by_user_id, user_role, account_id,
	canonical_post_url, post_fbid, group_ref, prompt, lead_id, import_task_id,
	status, intake_key, idempotency_key, error_code, error_message, retry_count,
	lease_owner, lease_until, next_run_at, last_attempt_at, created_at, updated_at,
	completed_at, expires_at`

func scanDPW(row interface{ Scan(...any) error }) (*DirectPostCommentWorkflow, error) {
	var w DirectPostCommentWorkflow
	err := row.Scan(&w.ID, &w.OrgID, &w.RequestedByUserID, &w.UserRole, &w.AccountID,
		&w.CanonicalPostURL, &w.PostFBID, &w.GroupRef, &w.Prompt, &w.LeadID, &w.ImportTaskID,
		&w.Status, &w.IntakeKey, &w.IdempotencyKey, &w.ErrorCode, &w.ErrorMessage, &w.RetryCount,
		&w.LeaseOwner, &w.LeaseUntil, &w.NextRunAt, &w.LastAttemptAt, &w.CreatedAt, &w.UpdatedAt,
		&w.CompletedAt, &w.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// CreateOrGetDirectPostCommentWorkflow upserts by idempotency_key (UNIQUE): a
// second request for the same actor/action/post returns the EXISTING row (no
// duplicate workflow). New rows start in DPStatusRequested, runnable now.
func (s *Store) CreateOrGetDirectPostCommentWorkflow(ctx context.Context, in DirectPostWorkflowInput) (*DirectPostCommentWorkflow, error) {
	canonical := strings.TrimSpace(in.CanonicalPostURL)
	if in.OrgID <= 0 || canonical == "" {
		return nil, fmt.Errorf("direct-post workflow requires org_id and canonical_post_url")
	}
	intakeKey := DirectPostIntakeKey(in.OrgID, canonical)
	idemKey := DirectPostIdempotencyKey(in.OrgID, in.AccountID, in.RequestedByUserID, canonical)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO direct_post_comment_workflows
			(org_id, requested_by_user_id, user_role, account_id, canonical_post_url,
			 post_fbid, group_ref, prompt, status, intake_key, idempotency_key,
			 next_run_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.OrgID, in.RequestedByUserID, in.UserRole, in.AccountID, canonical,
		strings.TrimSpace(in.PostFBID), strings.TrimSpace(in.GroupRef), in.Prompt,
		DPStatusRequested, intakeKey, idemKey, now, now, now); err != nil {
		return nil, err
	}
	return s.GetDirectPostCommentWorkflowByIdempotencyKey(ctx, in.OrgID, idemKey)
}

// GetDirectPostCommentWorkflowByID returns the org-scoped workflow or (nil, nil).
func (s *Store) GetDirectPostCommentWorkflowByID(ctx context.Context, orgID, id int64) (*DirectPostCommentWorkflow, error) {
	return scanDPW(s.db.QueryRowContext(ctx,
		`SELECT `+dpwColumns+` FROM direct_post_comment_workflows WHERE org_id = ? AND id = ?`, orgID, id))
}

// GetDirectPostCommentWorkflowByIdempotencyKey returns the org-scoped workflow for a
// request key, or (nil, nil).
func (s *Store) GetDirectPostCommentWorkflowByIdempotencyKey(ctx context.Context, orgID int64, key string) (*DirectPostCommentWorkflow, error) {
	return scanDPW(s.db.QueryRowContext(ctx,
		`SELECT `+dpwColumns+` FROM direct_post_comment_workflows WHERE org_id = ? AND idempotency_key = ?`, orgID, key))
}

// FindActiveDirectPostCommentWorkflowByIntakeKey returns the most recent NON-terminal
// workflow for a post (shared import), or (nil, nil). Used to reuse an in-flight import.
func (s *Store) FindActiveDirectPostCommentWorkflowByIntakeKey(ctx context.Context, orgID int64, intakeKey string) (*DirectPostCommentWorkflow, error) {
	return scanDPW(s.db.QueryRowContext(ctx,
		`SELECT `+dpwColumns+` FROM direct_post_comment_workflows
		 WHERE org_id = ? AND intake_key = ?
		   AND status NOT IN ('completed','failed','cancelled')
		 ORDER BY created_at DESC, id DESC LIMIT 1`, orgID, intakeKey))
}

// FindActiveImportTaskForIntakeKey returns the import_task_id of the FIRST (oldest)
// non-terminal workflow that already dispatched the single-post import for this
// intake_key, or "". This enforces ONE import per post: later workflows (other
// actors/accounts) reuse the shared task id instead of enqueuing a duplicate crawl.
func (s *Store) FindActiveImportTaskForIntakeKey(ctx context.Context, orgID int64, intakeKey string) (string, error) {
	var taskID string
	err := s.db.QueryRowContext(ctx,
		`SELECT import_task_id FROM direct_post_comment_workflows
		 WHERE org_id = ? AND intake_key = ? AND import_task_id <> ''
		   AND status NOT IN ('completed','failed','cancelled')
		 ORDER BY created_at ASC, id ASC LIMIT 1`, orgID, intakeKey).Scan(&taskID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return taskID, err
}
