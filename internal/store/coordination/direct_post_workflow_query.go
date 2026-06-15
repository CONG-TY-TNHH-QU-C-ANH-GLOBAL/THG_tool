package coordination

import (
	"context"
	"strings"
)

// FindDirectPostWorkflowByImportTaskID returns the (oldest) org-scoped direct-post
// workflow whose single-post import was dispatched under import_task_id == taskID, or
// (nil, nil). This is the DURABLE provenance link the crawl-result ingest uses to tell
// "this imported post was explicitly requested by a user" (force-lead + context-
// preserving identity) WITHOUT any in-memory callback, user_context KV, or schema
// change — body.TaskID echoed back by the connector equals workflow.import_task_id.
//
// Multiple actors may share one import (same intake_key → same import_task_id); any of
// their workflows carries the identical canonical_post_url / post_fbid / group_ref, so
// the oldest is sufficient to recover the requested context.
func (s *Store) FindDirectPostWorkflowByImportTaskID(ctx context.Context, orgID int64, taskID string) (*DirectPostCommentWorkflow, error) {
	taskID = strings.TrimSpace(taskID)
	if orgID <= 0 || taskID == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+dpwColumns+` FROM direct_post_comment_workflows
		 WHERE org_id = ? AND import_task_id = ?
		 ORDER BY created_at ASC, id ASC LIMIT 1`, orgID, taskID)
	return scanDPW(row) // scanDPW maps sql.ErrNoRows → (nil, nil)
}
