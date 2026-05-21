package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Workspace Knowledge OS — Operator Replay query surface.
//
// These methods power the /api/org/knowledge/* HTTP handlers and the
// Operator Replay UI panel. They are read-only — every write to the
// knowledge_events table goes through Record* in events.go, not here.
//
// The Replay UI's hot path is "list recent retrievals with outcomes."
// That query joins knowledge_events to itself on retrieval_id, so the
// (org_id, retrieval_id) index added in schema.go is load-bearing.
// If a future contributor drops that index, this file's queries
// degrade to full table scans on growing event volumes.

// ReplayEvent is one row of the replay timeline. Carries enough
// context for the UI to render a card without follow-up queries —
// the assembled trace + budget + outcome are all on one row.
//
// Stable wire shape; the HTTP handler serialises it directly. Adding
// optional fields is safe; removing or renaming requires both a
// backend deploy AND a frontend update.
type ReplayEvent struct {
	RetrievalID     string          `json:"retrieval_id"`
	OccurredAt      string          `json:"occurred_at"`
	Query           string          `json:"query"`
	GeneratedAction string          `json:"generated_action"`
	Outcome         string          `json:"outcome"` // "queued" | "approved" | "sent" | "rejected" | "failed" | "" (no outcome yet)
	OutcomeAt       string          `json:"outcome_at,omitempty"`
	Trace           json.RawMessage `json:"trace"`  // matches retrieval.Trace shape
	Budget          json.RawMessage `json:"budget"` // matches retrieval.AssemblyBudget shape
}

// ListReplayEventsForOrg returns recent retrieval events joined with
// their outcome events. Newest-first. `before` is a pagination cursor
// — pass the smallest OccurredAt from the previous page (empty string
// fetches page 1). `limit` is capped at 100.
//
// The query strategy: pull the most-recent retrievals first, then for
// each one look up the matching outcome by retrieval_id. Two passes
// keeps the SQL simple and the result deterministic; for very large
// event volumes the team would replace this with a materialized view.
func (s *Store) ListReplayEventsForOrg(ctx context.Context, orgID int64, before string, limit int) ([]ReplayEvent, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge: org_id required")
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	args := []any{orgID, eventTypeRetrieval}
	q := `SELECT retrieval_id, query, data_json, occurred_at
	        FROM knowledge_events
	       WHERE org_id = ? AND event_type = ?`
	if before != "" {
		q += ` AND occurred_at < ?`
		args = append(args, before)
	}
	q += ` ORDER BY occurred_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ReplayEvent, 0, limit)
	for rows.Next() {
		var ev ReplayEvent
		var blob string
		if err := rows.Scan(&ev.RetrievalID, &ev.Query, &blob, &ev.OccurredAt); err != nil {
			return nil, err
		}
		ev.Trace, ev.Budget, ev.GeneratedAction = splitRetrievalPayload(blob)
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Second pass: pull outcome rows for the retrieval IDs we just
	// loaded. Single round trip via an IN clause.
	if len(out) > 0 {
		ids := make([]any, 0, len(out)+2)
		ids = append(ids, orgID, eventTypeOutcome)
		for _, ev := range out {
			ids = append(ids, ev.RetrievalID)
		}
		oq := `SELECT retrieval_id, data_json, occurred_at
		         FROM knowledge_events
		        WHERE org_id = ? AND event_type = ?
		          AND retrieval_id IN (` + placeholders(len(out)) + `)
		        ORDER BY occurred_at DESC, id DESC`
		orows, err := s.queryContext(ctx, oq, ids...)
		if err == nil {
			// Track the latest (first encountered) outcome per
			// retrieval_id. If two outcomes exist (queued then sent),
			// we surface the most-recent one.
			seen := map[string]struct{}{}
			outcomes := map[string]struct {
				outcome string
				at      string
			}{}
			for orows.Next() {
				var (
					rid, blob, occurredAt string
				)
				if err := orows.Scan(&rid, &blob, &occurredAt); err != nil {
					continue
				}
				if _, dup := seen[rid]; dup {
					continue
				}
				seen[rid] = struct{}{}
				var parsed struct {
					Outcome string `json:"outcome"`
				}
				_ = json.Unmarshal([]byte(blob), &parsed)
				outcomes[rid] = struct {
					outcome string
					at      string
				}{parsed.Outcome, occurredAt}
			}
			orows.Close()
			for i := range out {
				if oc, ok := outcomes[out[i].RetrievalID]; ok {
					out[i].Outcome = oc.outcome
					out[i].OutcomeAt = oc.at
				}
			}
		}
	}

	return out, nil
}

// GetReplayEvent returns one retrieval event by ID, including its
// outcome (if any). Returns sql.ErrNoRows if the retrieval does not
// exist OR belongs to another org.
func (s *Store) GetReplayEvent(ctx context.Context, orgID int64, retrievalID string) (*ReplayEvent, error) {
	if orgID <= 0 || retrievalID == "" {
		return nil, sql.ErrNoRows
	}
	row := s.queryRowContext(ctx, `
		SELECT retrieval_id, query, data_json, occurred_at
		  FROM knowledge_events
		 WHERE org_id = ? AND event_type = ? AND retrieval_id = ?
		 ORDER BY id DESC LIMIT 1`,
		orgID, eventTypeRetrieval, retrievalID,
	)
	var ev ReplayEvent
	var blob string
	if err := row.Scan(&ev.RetrievalID, &ev.Query, &blob, &ev.OccurredAt); err != nil {
		return nil, err
	}
	ev.Trace, ev.Budget, ev.GeneratedAction = splitRetrievalPayload(blob)

	// Latest outcome, if any.
	orow := s.queryRowContext(ctx, `
		SELECT data_json, occurred_at
		  FROM knowledge_events
		 WHERE org_id = ? AND event_type = ? AND retrieval_id = ?
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		orgID, eventTypeOutcome, retrievalID,
	)
	var ob, oat string
	if err := orow.Scan(&ob, &oat); err == nil {
		var parsed struct {
			Outcome string `json:"outcome"`
		}
		_ = json.Unmarshal([]byte(ob), &parsed)
		ev.Outcome = parsed.Outcome
		ev.OutcomeAt = oat
	}
	return &ev, nil
}

// Stats is the aggregate-stats payload for the Replay dashboard
// headline. Computed on demand — cheap enough at MVP scale.
type Stats struct {
	TotalAssets      int        `json:"total_assets"`
	ApprovedAssets   int        `json:"approved_assets"`
	PendingAssets    int        `json:"pending_assets"`
	HiddenAssets     int        `json:"hidden_assets"`
	StaleAssets30d   int        `json:"stale_assets_30d"`
	Retrievals30d    int        `json:"retrievals_30d"`
	Conversions30d   int        `json:"conversions_30d"`
	TopRetrieved     []StatsRow `json:"top_retrieved"`
	LowConversionTop []StatsRow `json:"low_conversion_top"`
}

// StatsRow is one entry of the top-retrieved / low-conversion lists.
type StatsRow struct {
	AssetID        int64  `json:"asset_id"`
	Title          string `json:"title"`
	Retrievals30d  int    `json:"retrievals_30d"`
	Conversions30d int    `json:"conversions_30d"`
}

// GetStatsForOrg computes the headline metrics for the Replay
// dashboard. Cheap aggregate queries; no caching layer needed at MVP
// volume.
func (s *Store) GetStatsForOrg(ctx context.Context, orgID int64) (*Stats, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge: org_id required")
	}
	out := &Stats{}

	// Asset state counts in a single aggregate.
	rows, err := s.queryContext(ctx, `
		SELECT state, COUNT(*) FROM knowledge_assets
		 WHERE org_id = ? GROUP BY state`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			rows.Close()
			return nil, err
		}
		out.TotalAssets += n
		switch state {
		case "approved":
			out.ApprovedAssets = n
		case "pending":
			out.PendingAssets = n
		case "hidden":
			out.HiddenAssets = n
		}
	}
	rows.Close()

	stale, err := s.CountStaleAssetsForOrg(ctx, orgID, 30)
	if err == nil {
		out.StaleAssets30d = stale
	}

	// 30d sums from the assets table — already maintained by the
	// retrieval/outcome write paths.
	_ = s.queryRowContext(ctx, `
		SELECT COALESCE(SUM(retrieval_count_30d),0), COALESCE(SUM(conversion_count_30d),0)
		  FROM knowledge_assets WHERE org_id = ?`, orgID).
		Scan(&out.Retrievals30d, &out.Conversions30d)

	// Top retrieved + low conversion ranked subqueries.
	out.TopRetrieved = s.queryStatsRows(ctx, orgID, `
		SELECT id, title, retrieval_count_30d, conversion_count_30d
		  FROM knowledge_assets
		 WHERE org_id = ? AND retrieval_count_30d > 0
		 ORDER BY retrieval_count_30d DESC, id DESC
		 LIMIT 5`)
	out.LowConversionTop = s.queryStatsRows(ctx, orgID, `
		SELECT id, title, retrieval_count_30d, conversion_count_30d
		  FROM knowledge_assets
		 WHERE org_id = ? AND retrieval_count_30d >= 5
		 ORDER BY (conversion_count_30d * 1.0 / retrieval_count_30d) ASC, retrieval_count_30d DESC, id DESC
		 LIMIT 5`)
	return out, nil
}

func (s *Store) queryStatsRows(ctx context.Context, orgID int64, query string) []StatsRow {
	rows, err := s.queryContext(ctx, query, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []StatsRow{}
	for rows.Next() {
		var r StatsRow
		if err := rows.Scan(&r.AssetID, &r.Title, &r.Retrievals30d, &r.Conversions30d); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

// splitRetrievalPayload extracts trace + budget + generated_action
// from the data_json blob a retrieval event stores. The shape is
// produced by RecordRetrievalWithTrace — keep the two in sync.
// Pre-trace events (legacy RecordRetrieval) have a different shape;
// we surface what we can and leave the trace empty rather than failing.
func splitRetrievalPayload(blob string) (trace, budget json.RawMessage, generatedAction string) {
	var parsed struct {
		Trace           json.RawMessage `json:"trace"`
		Budget          json.RawMessage `json:"budget"`
		GeneratedAction string          `json:"generated_action"`
	}
	if err := json.Unmarshal([]byte(blob), &parsed); err != nil {
		return nil, nil, ""
	}
	return parsed.Trace, parsed.Budget, parsed.GeneratedAction
}
