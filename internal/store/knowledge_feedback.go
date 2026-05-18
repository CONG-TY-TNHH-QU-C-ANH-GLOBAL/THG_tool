package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Workspace Knowledge OS — Goal G10 (Human Feedback) persistence.
//
// IRREVOCABLE INVARIANTS this file enforces:
//
//  1. Feedback events are APPEND-ONLY. There is no Update / Delete
//     method exposed. The schema has no `revised_at` column. Once
//     written, a feedback row stays as-is.
//
//  2. The retrieval engine MUST NOT read feedback. The package
//     diagram is one-way: agent runtime → retrieval, operator UI →
//     feedback. Feedback never flows back to retrieval. This is a
//     STRUCTURAL guarantee, not a policy — the only readers of
//     knowledge_feedback are the analytics handlers and the
//     gold-dataset enricher in soak/ . The runtime/ package does
//     NOT import this file's read methods.
//
//  3. Auto-training is FORBIDDEN. Any future feature that wants to
//     use feedback signals MUST go through an offline pipeline
//     (export → human review → manual rerank tune). This invariant
//     is enforced by the audit trail: changing rerank weights based
//     on feedback would leave a code review trail; we cannot
//     prevent it at the database layer, but we make it visible.
//
// The schema separates feedback from knowledge_events on purpose —
// feedback is a different lifecycle (immutable, human-sourced) and
// goes through a different review process (operators see raw
// retrievals daily; they see feedback rollups weekly).

// FeedbackKind is the closed enumeration of feedback types. Adding
// a new kind is a deliberate decision — operators need to know it
// exists and dashboards need to render it. We do NOT auto-accept
// freeform kinds.
type FeedbackKind string

const (
	FeedbackThumbsUp   FeedbackKind = "thumbs_up"
	FeedbackThumbsDown FeedbackKind = "thumbs_down"
	FeedbackApprove    FeedbackKind = "approve"
	FeedbackReject     FeedbackKind = "reject"
	FeedbackEdit       FeedbackKind = "edit"
	FeedbackRating     FeedbackKind = "rating"
)

func (k FeedbackKind) IsKnown() bool {
	switch k {
	case FeedbackThumbsUp, FeedbackThumbsDown,
		FeedbackApprove, FeedbackReject,
		FeedbackEdit, FeedbackRating:
		return true
	}
	return false
}

// FeedbackEvent is one immutable feedback row. Returned by reads;
// constructed by RecordFeedback callers.
type FeedbackEvent struct {
	ID          int64           `json:"id"`
	OrgID       int64           `json:"org_id"`
	UserID      int64           `json:"user_id"`
	RetrievalID string          `json:"retrieval_id"`
	AssetID     int64           `json:"asset_id,omitempty"`
	Kind        FeedbackKind    `json:"kind"`
	Data        json.RawMessage `json:"data"`
	OccurredAt  string          `json:"occurred_at"`
}

// RecordFeedback appends one immutable feedback event. ALWAYS
// successful in writing the row (subject to DB availability); never
// modifies existing rows.
//
// retrievalID + assetID are optional individually but ONE of them
// SHOULD be set. Operator-rating-the-whole-session feedback uses
// neither; operator-rating-one-asset-in-a-session uses both.
//
// data is the kind-specific payload (star rating, edit diff text,
// comment text). Stored as-is — NO sanitisation here because
// feedback is operator-authored, not adversary-controlled. (If a
// hostile operator inserts payloads into feedback, that's a
// privilege-escalation problem, not a knowledge-injection problem.)
func (s *Store) RecordFeedback(ctx context.Context, ev FeedbackEvent) error {
	if ev.OrgID <= 0 {
		return errors.New("knowledge_feedback: org_id required")
	}
	if !ev.Kind.IsKnown() {
		return fmt.Errorf("knowledge_feedback: unknown kind %q", ev.Kind)
	}
	dataJSON := string(ev.Data)
	if dataJSON == "" {
		dataJSON = "{}"
	}
	_, err := s.ExecContext(ctx, `
		INSERT INTO knowledge_feedback
			(org_id, user_id, retrieval_id, asset_id, kind, data_json)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ev.OrgID, ev.UserID, ev.RetrievalID, ev.AssetID, string(ev.Kind), dataJSON,
	)
	return err
}

// ListFeedbackForOrg returns recent feedback events. Pagination via
// limit (default 50, max 200). Read-only — analytics surfaces ONLY.
// The retrieval engine MUST NOT call this method; that would create
// the auto-training feedback loop G10 forbids.
func (s *Store) ListFeedbackForOrg(ctx context.Context, orgID int64, limit int) ([]FeedbackEvent, error) {
	if orgID <= 0 {
		return nil, errors.New("knowledge_feedback: org_id required")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.QueryContext(ctx, `
		SELECT id, org_id, user_id, retrieval_id, asset_id, kind, data_json, occurred_at
		  FROM knowledge_feedback
		 WHERE org_id = ?
		 ORDER BY occurred_at DESC, id DESC
		 LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FeedbackEvent, 0, limit)
	for rows.Next() {
		var ev FeedbackEvent
		var kind, dataJSON string
		if err := rows.Scan(&ev.ID, &ev.OrgID, &ev.UserID, &ev.RetrievalID, &ev.AssetID, &kind, &dataJSON, &ev.OccurredAt); err != nil {
			return nil, err
		}
		ev.Kind = FeedbackKind(kind)
		ev.Data = json.RawMessage(dataJSON)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// FeedbackRollup is the aggregate the Operator Replay dashboard
// shows: "how does this org rate AI-generated comments lately?"
type FeedbackRollup struct {
	OrgID       int64                  `json:"org_id"`
	Window      string                 `json:"window"`
	TotalEvents int                    `json:"total_events"`
	ByKind      map[FeedbackKind]int   `json:"by_kind"`
}

// GetFeedbackRollupForOrg aggregates feedback events over a window.
func (s *Store) GetFeedbackRollupForOrg(ctx context.Context, orgID int64, days int) (*FeedbackRollup, error) {
	if orgID <= 0 {
		return nil, errors.New("knowledge_feedback: org_id required")
	}
	if days <= 0 {
		days = 30
	}
	q := `SELECT kind, COUNT(*) FROM knowledge_feedback
	       WHERE org_id = ? AND occurred_at >= ` + s.dialect.IntervalDaysExpr(days) + `
	       GROUP BY kind`
	rows, err := s.QueryContext(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &FeedbackRollup{
		OrgID:  orgID,
		Window: fmt.Sprintf("%dd", days),
		ByKind: map[FeedbackKind]int{},
	}
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		out.ByKind[FeedbackKind(kind)] = count
		out.TotalEvents += count
	}
	return out, rows.Err()
}

// Package-level NO-AUTO-TRAIN assertion. Future contributors who
// search the codebase for "auto_train" will find this comment.
//
// THIS IS A POLICY MARKER, NOT A COMPILER GUARANTEE.
//
// The runtime/ package MUST NOT import ListFeedbackForOrg or
// GetFeedbackRollupForOrg. If a code review proposes such an import,
// it requires explicit override of the G10 invariant — that is a
// product decision, not an engineering one.
const _AutoTrainPolicyMarker = "FORBIDDEN: do not read feedback from retrieval runtime; see G10."
