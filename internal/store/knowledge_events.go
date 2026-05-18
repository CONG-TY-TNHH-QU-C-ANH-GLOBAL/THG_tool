package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Workspace Knowledge OS — Layer 7 (Observability).
//
// This file implements the [observability.Metrics] interface against
// the knowledge_events table. The interface is in
// workspace_knowledge/observability; the wiring (dispatcher,
// retrieval engine → metrics) is everywhere those layers run.
//
// Two design decisions documented for future readers:
//
//  1. Three event types share one table. event_type is the
//     discriminator. Splitting into three tables (knowledge_syncs,
//     knowledge_retrievals, knowledge_outcomes) would scatter the
//     foreign-key joins the Operator Replay UI needs — that UI
//     wants ONE row per retrieval + outcome pair. Keep them together.
//
//  2. data_json is opaque to the store. The recorder serialises
//     event-type-specific fields into it; readers (the analytics
//     handlers, not the runtime hot path) parse on demand. This
//     keeps schema changes (new metric fields) from being migrations.

const (
	eventTypeSync      = "sync"
	eventTypeRetrieval = "retrieval"
	eventTypeOutcome   = "outcome"
	eventTypeEmbedding = "embedding_batch"
)

// RecordKnowledgeSync persists one ingestor-sync event. Satisfies
// [observability.Metrics.RecordSync] and the dispatcher's narrower
// [ingestion.SyncRecorder]; *store.Store satisfies both interfaces.
func (s *Store) RecordKnowledgeSync(
	ctx context.Context,
	orgID int64,
	sourceType sources.SourceType,
	assetsSeen, assetsCreated, assetsUpdated, assetsRejected int,
	durationMs int64,
	errs int,
) {
	if orgID <= 0 {
		return
	}
	payload := map[string]any{
		"assets_seen":     assetsSeen,
		"assets_created":  assetsCreated,
		"assets_updated":  assetsUpdated,
		"assets_rejected": assetsRejected,
		"errors":          errs,
	}
	data, _ := json.Marshal(payload)
	_, err := s.ExecContext(ctx, `
		INSERT INTO knowledge_events
			(org_id, event_type, source_type, data_json, duration_ms)
		VALUES (?, ?, ?, ?, ?)`,
		orgID, eventTypeSync, string(sourceType), string(data), durationMs,
	)
	if err != nil {
		// Telemetry is best-effort. A failed metric write must NOT
		// affect the sales action. We log to stderr via the package
		// logger so the surface is still observable.
		fmt.Printf("[knowledge_events] record sync org=%d: %v\n", orgID, err)
	}
}

// RecordKnowledgeRetrieval persists one retrieval event. retrievalID
// is the join key with the eventual outcome event.
func (s *Store) RecordKnowledgeRetrieval(
	ctx context.Context,
	orgID int64,
	retrievalID, query string,
	hits []retrieval.Hit,
	generatedAction string,
) {
	if orgID <= 0 {
		return
	}
	type hitSummary struct {
		AssetID int64   `json:"asset_id"`
		Title   string  `json:"title"`
		Score   float64 `json:"score"`
		Reason  string  `json:"reason"`
	}
	summaries := make([]hitSummary, 0, len(hits))
	for _, h := range hits {
		if h.Asset == nil {
			continue
		}
		summaries = append(summaries, hitSummary{
			AssetID: h.Asset.ID,
			Title:   h.Asset.Title,
			Score:   h.Score,
			Reason:  h.Reason,
		})
	}
	payload := map[string]any{
		"hits":             summaries,
		"generated_action": generatedAction,
	}
	data, _ := json.Marshal(payload)
	_, err := s.ExecContext(ctx, `
		INSERT INTO knowledge_events
			(org_id, event_type, retrieval_id, query, data_json)
		VALUES (?, ?, ?, ?, ?)`,
		orgID, eventTypeRetrieval, retrievalID, query, string(data),
	)
	if err != nil {
		fmt.Printf("[knowledge_events] record retrieval org=%d: %v\n", orgID, err)
	}
}

// RecordKnowledgeRetrievalWithTrace is the explainability-aware
// variant. It persists the full [retrieval.Trace] and
// [retrieval.AssemblyBudget] in the data_json column. The Operator
// Replay UI's expand-row view reads this back through
// ListKnowledgeRetrievalEvents.
//
// Schema-wise this is the SAME table as RecordKnowledgeRetrieval —
// data_json is opaque to the store, so growing the trace shape is
// schemaless. The retrieval_id field is critical: it is the join key
// with the eventual outcome event recorded by RecordKnowledgeOutcome.
func (s *Store) RecordKnowledgeRetrievalWithTrace(
	ctx context.Context,
	orgID int64,
	retrievalID, query, generatedAction string,
	trace retrieval.Trace,
	budget retrieval.AssemblyBudget,
) {
	if orgID <= 0 {
		return
	}
	payload := map[string]any{
		"trace":            trace,
		"budget":           budget,
		"generated_action": generatedAction,
	}
	data, _ := json.Marshal(payload)
	_, err := s.ExecContext(ctx, `
		INSERT INTO knowledge_events
			(org_id, event_type, retrieval_id, query, data_json)
		VALUES (?, ?, ?, ?, ?)`,
		orgID, eventTypeRetrieval, retrievalID, truncateQueryForStore(query), string(data),
	)
	if err != nil {
		fmt.Printf("[knowledge_events] record retrieval with trace org=%d: %v\n", orgID, err)
	}
}

// truncateQueryForStore caps the query column at 1KB so a giant lead
// body does not bloat the events table. The trace inside data_json
// can carry more if needed (already truncated by the searcher).
func truncateQueryForStore(q string) string {
	const maxQueryColLen = 1024
	if len(q) <= maxQueryColLen {
		return q
	}
	return q[:maxQueryColLen] + "…"
}

// RecordKnowledgeOutcome closes the loop on a previously-recorded
// retrieval event. outcome is one of "approved" | "sent" | "rejected"
// | "failed" | "converted" (free-form; the UI groups them).
func (s *Store) RecordKnowledgeOutcome(
	ctx context.Context,
	orgID int64,
	retrievalID, outcome string,
) {
	if orgID <= 0 || retrievalID == "" {
		return
	}
	payload, _ := json.Marshal(map[string]any{"outcome": outcome})
	_, err := s.ExecContext(ctx, `
		INSERT INTO knowledge_events
			(org_id, event_type, retrieval_id, data_json)
		VALUES (?, ?, ?, ?)`,
		orgID, eventTypeOutcome, retrievalID, string(payload),
	)
	if err != nil {
		fmt.Printf("[knowledge_events] record outcome org=%d retrieval=%q: %v\n", orgID, retrievalID, err)
		return
	}

	// Side-effect: increment conversion counters on every asset in
	// the matching retrieval event. We do this only for the "sent"
	// and "converted" outcomes — "rejected" and "failed" do NOT
	// count toward conversion. This is the only place
	// conversion_count_30d is written.
	if outcome != "sent" && outcome != "converted" {
		return
	}
	rows, err := s.QueryContext(ctx, `
		SELECT data_json FROM knowledge_events
		 WHERE org_id = ? AND retrieval_id = ? AND event_type = ?
		 LIMIT 1`,
		orgID, retrievalID, eventTypeRetrieval,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	if !rows.Next() {
		return
	}
	var blob string
	if err := rows.Scan(&blob); err != nil {
		return
	}
	var parsed struct {
		Hits []struct {
			AssetID int64 `json:"asset_id"`
		} `json:"hits"`
	}
	if err := json.Unmarshal([]byte(blob), &parsed); err != nil {
		return
	}
	for _, h := range parsed.Hits {
		if h.AssetID == 0 {
			continue
		}
		_, _ = s.ExecContext(ctx, `
			UPDATE knowledge_assets
			   SET conversion_count_30d = conversion_count_30d + 1
			 WHERE id = ? AND org_id = ?`,
			h.AssetID, orgID,
		)
	}
}

// --- Analytics helpers (for the Operator Replay UI + dashboards) ---

// KnowledgeSyncSummary is one row in the "recent syncs" panel.
type KnowledgeSyncSummary struct {
	SourceType     string `json:"source_type"`
	AssetsSeen     int    `json:"assets_seen"`
	AssetsCreated  int    `json:"assets_created"`
	AssetsUpdated  int    `json:"assets_updated"`
	AssetsRejected int    `json:"assets_rejected"`
	Errors         int    `json:"errors"`
	DurationMs     int64  `json:"duration_ms"`
	OccurredAt     string `json:"occurred_at"`
}

// ListRecentSyncsForOrg returns the last `limit` sync events for the
// org, newest first. Used by the Sources panel "sync history" drawer.
func (s *Store) ListRecentSyncsForOrg(ctx context.Context, orgID int64, limit int) ([]KnowledgeSyncSummary, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge_events: org_id required")
	}
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.QueryContext(ctx, `
		SELECT source_type, data_json, duration_ms, occurred_at
		  FROM knowledge_events
		 WHERE org_id = ? AND event_type = ?
		 ORDER BY occurred_at DESC, id DESC
		 LIMIT ?`, orgID, eventTypeSync, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]KnowledgeSyncSummary, 0, limit)
	for rows.Next() {
		var (
			sourceType, blob, occurredAt string
			duration                     int64
		)
		if err := rows.Scan(&sourceType, &blob, &duration, &occurredAt); err != nil {
			return nil, err
		}
		var parsed struct {
			Seen, Created, Updated, Rejected, Errors int
			AssetsSeen                               int `json:"assets_seen"`
			AssetsCreated                            int `json:"assets_created"`
			AssetsUpdated                            int `json:"assets_updated"`
			AssetsRejected                           int `json:"assets_rejected"`
			Err                                      int `json:"errors"`
		}
		_ = json.Unmarshal([]byte(blob), &parsed)
		out = append(out, KnowledgeSyncSummary{
			SourceType:     sourceType,
			AssetsSeen:     parsed.AssetsSeen,
			AssetsCreated:  parsed.AssetsCreated,
			AssetsUpdated:  parsed.AssetsUpdated,
			AssetsRejected: parsed.AssetsRejected,
			Errors:         parsed.Err,
			DurationMs:     duration,
			OccurredAt:     occurredAt,
		})
	}
	return out, rows.Err()
}

// RecordEmbeddingBatch persists one batch outcome from the embedding
// worker. Satisfies [embedding.MetricsRecorder]. Drives the
// observability surface so operators can see embedding pipeline
// health at a glance: total throughput, failure rate, recoverable vs
// permanent failures, batch latency.
//
// orgID is intentionally 0 here — embedding batches span the whole
// catalog, not one tenant. Per-tenant breakdown comes from
// EmbeddingStats which aggregates the knowledge_assets table.
func (s *Store) RecordEmbeddingBatch(ctx context.Context, batchSize, succeeded, failed int, durationMs int64, recoverable bool) {
	payload, _ := json.Marshal(map[string]any{
		"batch_size":  batchSize,
		"succeeded":   succeeded,
		"failed":      failed,
		"recoverable": recoverable,
	})
	_, err := s.ExecContext(ctx, `
		INSERT INTO knowledge_events
			(org_id, event_type, data_json, duration_ms)
		VALUES (?, ?, ?, ?)`,
		0, eventTypeEmbedding, string(payload), durationMs,
	)
	if err != nil {
		fmt.Printf("[knowledge_events] record embedding_batch: %v\n", err)
	}
}

// CountStaleKnowledgeAssetsForOrg returns the number of assets that
// have not been retrieved in `daysIdle` days. The Sources panel uses
// this to highlight catalogs that may need cleanup. An asset never
// retrieved (last_retrieved_at IS NULL) is NOT stale by this
// definition — it might be brand new. Only assets with a prior
// retrieval that has gone quiet count.
//
// Cross-dialect: the interval expression differs between SQLite and
// Postgres (see POSTGRES_COMPAT_PLAN.md risk R8). Routed through the
// dialect helper so the same Go code compiles to either SQL form.
func (s *Store) CountStaleKnowledgeAssetsForOrg(ctx context.Context, orgID int64, daysIdle int) (int, error) {
	if orgID <= 0 {
		return 0, fmt.Errorf("knowledge_events: org_id required")
	}
	if daysIdle <= 0 {
		daysIdle = 30
	}
	q := fmt.Sprintf(`
		SELECT COUNT(*) FROM knowledge_assets
		 WHERE org_id = ?
		   AND last_retrieved_at IS NOT NULL
		   AND last_retrieved_at < %s`, s.dialect.IntervalDaysExpr(daysIdle))
	// s.QueryRowContext already rebinds — don't double-Rebind.
	row := s.QueryRowContext(ctx, q, orgID)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
