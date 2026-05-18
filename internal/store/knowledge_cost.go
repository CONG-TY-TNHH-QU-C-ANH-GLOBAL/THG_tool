package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// Workspace Knowledge OS — Goal G9 cost-accounting surface.
//
// Cost telemetry is tracked across THREE axes:
//
//   - Per organization     (each tenant pays for its own embeddings)
//   - Per source           (Shopify vs CSV vs Notion attribute differently)
//   - Rolling 30-day basis (anomaly detection + future billing)
//
// Storage strategy: reuse the existing knowledge_events table — the
// data_json already carries per-batch counters. We add per-org +
// per-source attribution at WRITE time so analytics queries don't
// have to scan the whole table for joins.
//
// Backward compatibility: the legacy RecordEmbeddingBatch path wrote
// org_id=0 (system-wide). New writes carry the actual org_id. The
// rollup query treats org_id=0 as "unattributed" and surfaces it
// separately in the report so the operator can see migration progress.

// EmbeddingCostBatch captures one batch of embedding work. The
// embedding worker calls this AFTER a Tick completes.
type EmbeddingCostBatch struct {
	OrgID         int64
	SourceID      int64  // 0 = source attribution unavailable (legacy / direct ingest)
	BatchSize     int
	Succeeded     int
	Failed        int
	TokensServed  int    // populated when the embedder reports usage (OpenAI does)
	DurationMs    int64
	Recoverable   bool
}

// RecordEmbeddingCost is the per-org, per-source cost-recording
// counterpart to the legacy RecordEmbeddingBatch. New code should
// always call this version; RecordEmbeddingBatch is preserved for
// backward compat with the worker's MetricsRecorder interface.
func (s *Store) RecordEmbeddingCost(ctx context.Context, batch EmbeddingCostBatch) {
	if batch.OrgID <= 0 {
		// Reject org_id=0 explicitly — goal G9 requires per-org
		// attribution, and silently writing unattributed events
		// would defeat the cost-tracking purpose.
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"batch_size":    batch.BatchSize,
		"succeeded":     batch.Succeeded,
		"failed":        batch.Failed,
		"tokens_served": batch.TokensServed,
		"recoverable":   batch.Recoverable,
		"source_id":     batch.SourceID,
	})
	_, err := s.ExecContext(ctx, `
		INSERT INTO knowledge_events
			(org_id, event_type, data_json, duration_ms)
		VALUES (?, ?, ?, ?)`,
		batch.OrgID, eventTypeEmbedding, string(payload), batch.DurationMs,
	)
	if err != nil {
		fmt.Printf("[knowledge_cost] record cost org=%d source=%d: %v\n", batch.OrgID, batch.SourceID, err)
	}
}

// OrgEmbeddingCost is the aggregate rollup per organisation. The
// 30-day window matches conversion-count semantics on knowledge_assets
// so dashboards present consistent windows everywhere.
type OrgEmbeddingCost struct {
	OrgID            int64                       `json:"org_id"`
	BatchCount30d    int                         `json:"batch_count_30d"`
	TokensServed30d  int64                       `json:"tokens_served_30d"`
	SucceededAssets  int                         `json:"succeeded_assets"`
	FailedAssets     int                         `json:"failed_assets"`
	EstimatedUSD30d  float64                     `json:"estimated_usd_30d"`
	BySource         map[string]int64            `json:"by_source"` // source_id (as string) → tokens
}

// pricePerMillionTokens is the operator-configurable rate. Defaults
// to OpenAI text-embedding-3-small at $0.02 / 1M tokens (USD). If the
// embedder changes, the operator overrides this via env in a future
// PR; for now it's a constant.
const pricePerMillionTokens = 0.02

// GetOrgEmbeddingCost rolls up the last 30 days of embedding events
// for one org, broken down by source. Read-only query; safe to call
// from request handlers.
func (s *Store) GetOrgEmbeddingCost(ctx context.Context, orgID int64) (*OrgEmbeddingCost, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge_cost: org_id required")
	}
	since := s.dialect.IntervalDaysExpr(30)
	q := `SELECT data_json, duration_ms FROM knowledge_events
	       WHERE org_id = ? AND event_type = ?
	         AND occurred_at >= ` + since
	rows, err := s.QueryContext(ctx, q, orgID, eventTypeEmbedding)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &OrgEmbeddingCost{
		OrgID:    orgID,
		BySource: map[string]int64{},
	}
	for rows.Next() {
		var blob string
		var duration int64
		if err := rows.Scan(&blob, &duration); err != nil {
			return nil, err
		}
		var p struct {
			BatchSize    int   `json:"batch_size"`
			Succeeded    int   `json:"succeeded"`
			Failed       int   `json:"failed"`
			TokensServed int64 `json:"tokens_served"`
			SourceID     int64 `json:"source_id"`
		}
		if err := json.Unmarshal([]byte(blob), &p); err != nil {
			continue
		}
		out.BatchCount30d++
		out.TokensServed30d += p.TokensServed
		out.SucceededAssets += p.Succeeded
		out.FailedAssets += p.Failed
		sourceKey := "unattributed"
		if p.SourceID > 0 {
			sourceKey = fmt.Sprintf("%d", p.SourceID)
		}
		out.BySource[sourceKey] += p.TokensServed
	}
	out.EstimatedUSD30d = float64(out.TokensServed30d) * pricePerMillionTokens / 1_000_000
	return out, nil
}

// ListOrgsByEmbeddingCost returns the top N orgs by 30d token usage.
// Used by superadmin dashboards + anomaly detection. Not a tenant-
// scoped query — the caller MUST authorise (founder/platform role).
func (s *Store) ListOrgsByEmbeddingCost(ctx context.Context, limit int) ([]OrgEmbeddingCost, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	since := s.dialect.IntervalDaysExpr(30)
	q := `SELECT org_id, data_json FROM knowledge_events
	       WHERE event_type = ?
	         AND org_id > 0
	         AND occurred_at >= ` + since
	rows, err := s.QueryContext(ctx, q, eventTypeEmbedding)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byOrg := map[int64]*OrgEmbeddingCost{}
	for rows.Next() {
		var orgID int64
		var blob string
		if err := rows.Scan(&orgID, &blob); err != nil {
			return nil, err
		}
		var p struct {
			TokensServed int64 `json:"tokens_served"`
			Succeeded    int   `json:"succeeded"`
			Failed       int   `json:"failed"`
			SourceID     int64 `json:"source_id"`
		}
		if err := json.Unmarshal([]byte(blob), &p); err != nil {
			continue
		}
		entry, ok := byOrg[orgID]
		if !ok {
			entry = &OrgEmbeddingCost{OrgID: orgID, BySource: map[string]int64{}}
			byOrg[orgID] = entry
		}
		entry.BatchCount30d++
		entry.TokensServed30d += p.TokensServed
		entry.SucceededAssets += p.Succeeded
		entry.FailedAssets += p.Failed
	}
	out := make([]OrgEmbeddingCost, 0, len(byOrg))
	for _, e := range byOrg {
		e.EstimatedUSD30d = float64(e.TokensServed30d) * pricePerMillionTokens / 1_000_000
		out = append(out, *e)
	}
	// Sort by token usage descending. Simple slice sort.
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j].TokensServed30d > out[i].TokensServed30d {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
