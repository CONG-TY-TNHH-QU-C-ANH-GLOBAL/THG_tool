package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// Workspace Knowledge OS — Production Soak (PR-4) observability surface.
//
// This file extends knowledge_replay.go with the metrics the soak
// dashboard needs to validate retrieval quality BEFORE the team
// builds orchestration on top. The metrics are computed from
// knowledge_events + knowledge_assets aggregates — no new tables.
//
// What the soak dashboard answers (goal directive PR-4 §1):
//
//   - retrieval hit rate     (retrievals with len(selected) > 0 / total)
//   - fallback rate          (fallback_primary_* reasons / total retrievals)
//   - zero-hit rate          (retrievals with empty selected / total)
//   - avg semantic score     (mean of Selected.Breakdown.Semantic when > 0)
//   - avg budget drop        (AssemblyBudget.DroppedByCap mean)
//   - stale asset count      (already in KnowledgeStats)
//   - compliance blocks      (RejectGovernance count / period)
//   - embedding model drift  (distinct model versions / period)
//
// Cost telemetry (§6) lives separately in knowledge_events
// `event_type='embedding_batch'` rows — already there from PR-1.

// KnowledgeSoakMetrics is the soak-dashboard payload. Aggregates over
// a recent window (default 24h) so dashboards can show "what's the
// system doing right now" without scanning the entire history.
type KnowledgeSoakMetrics struct {
	WindowHours        int     `json:"window_hours"`
	TotalRetrievals    int     `json:"total_retrievals"`
	HitRate            float64 `json:"hit_rate"`               // 0..1
	ZeroHitRate        float64 `json:"zero_hit_rate"`          // 0..1
	FallbackRate       float64 `json:"fallback_rate"`          // 0..1
	AvgSemanticScore   float64 `json:"avg_semantic_score"`     // mean of non-zero
	AvgBudgetDropped   float64 `json:"avg_budget_dropped"`
	AvgEstimatedTokens float64 `json:"avg_estimated_tokens"`
	ComplianceBlocks   int     `json:"compliance_blocks"`
	// Embedding model drift detection (§4): distinct version count.
	// Healthy = 1 (all embeddings under one model). >1 means a model
	// upgrade is in flight or a backfill is incomplete.
	DistinctEmbeddingModels int      `json:"distinct_embedding_models"`
	EmbeddingModels         []string `json:"embedding_models"`
}

// GetKnowledgeSoakMetricsForOrg aggregates the last `windowHours` of
// retrieval events into the soak dashboard shape. Default 24h.
//
// This is a read-only diagnostic call. Computes from knowledge_events
// (retrieval rows) plus knowledge_assets aggregates. No write side
// effects.
func (s *Store) GetKnowledgeSoakMetricsForOrg(ctx context.Context, orgID int64, windowHours int) (*KnowledgeSoakMetrics, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge_soak: org_id required")
	}
	if windowHours <= 0 {
		windowHours = 24
	}
	out := &KnowledgeSoakMetrics{WindowHours: windowHours}

	// Build the time window predicate per dialect. The pattern matches
	// CountStaleKnowledgeAssetsForOrg's approach.
	windowExpr := s.dialect.IntervalDaysExpr(0) // placeholder
	_ = windowExpr
	// Dialect-specific "now minus N hours" — both SQLite and PG accept
	// these syntaxes. We compose inline because IntervalDaysExpr only
	// supports day-granularity; the soak window is hours.
	var sinceClause string
	switch s.dialect.Name() {
	case "postgres":
		sinceClause = fmt.Sprintf("NOW() - INTERVAL '%d hours'", windowHours)
	default:
		sinceClause = fmt.Sprintf("DATETIME('now', '-%d hours')", windowHours)
	}

	// Pull retrieval event payloads in the window. Inline the time
	// expression (dialect helper); the `?` parameter is just org_id.
	q := `SELECT data_json FROM knowledge_events
	       WHERE org_id = ? AND event_type = ?
	         AND occurred_at >= ` + sinceClause
	rows, err := s.QueryContext(ctx, q, orgID, eventTypeRetrieval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		semanticCount   int
		semanticTotal   float64
		droppedCount    int
		droppedTotal    float64
		tokensCount     int
		tokensTotal     float64
	)

	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		// Parse a minimal projection. We tolerate missing fields
		// (additive-compatible across schema evolutions, per goal
		// directive PR-2 §3).
		var parsed struct {
			Trace struct {
				Selected []struct {
					Breakdown struct {
						Semantic float64 `json:"semantic"`
					} `json:"breakdown"`
				} `json:"selected"`
				TotalByReason map[string]int `json:"total_by_reason"`
			} `json:"trace"`
			Budget struct {
				DroppedByCap      int `json:"dropped_by_cap"`
				ComplianceDropped int `json:"compliance_dropped"`
				EstimatedTokens   int `json:"estimated_tokens"`
			} `json:"budget"`
		}
		if err := json.Unmarshal([]byte(blob), &parsed); err != nil {
			continue
		}
		out.TotalRetrievals++

		// Hit / zero-hit categorisation.
		if len(parsed.Trace.Selected) == 0 {
			// zero-hit case
		}
		// Fallback rate: any fallback_primary_* reason counts as fallback.
		for reason := range parsed.Trace.TotalByReason {
			if reason == "fallback_primary_error" || reason == "fallback_primary_timeout" || reason == "fallback_primary_empty" {
				out.FallbackRate++ // accumulator; divided below
				break              // count each retrieval at most once
			}
		}

		// Average semantic score over hits with non-zero semantic.
		for _, sh := range parsed.Trace.Selected {
			if sh.Breakdown.Semantic > 0 {
				semanticTotal += sh.Breakdown.Semantic
				semanticCount++
			}
		}

		// Budget drop & token mean.
		if parsed.Budget.DroppedByCap > 0 || parsed.Budget.EstimatedTokens > 0 {
			droppedTotal += float64(parsed.Budget.DroppedByCap)
			droppedCount++
			tokensTotal += float64(parsed.Budget.EstimatedTokens)
			tokensCount++
		}

		// Compliance blocks: sum of ComplianceDropped + governance rejections.
		out.ComplianceBlocks += parsed.Budget.ComplianceDropped
		out.ComplianceBlocks += parsed.Trace.TotalByReason["governance_drop"]

		// Hit-rate accumulation
		if len(parsed.Trace.Selected) > 0 {
			// counts as a hit; computed below as out.HitRate / total
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Second pass for hit/zero-hit counters since first loop already
	// consumed rows. Cheap separate query.
	if out.TotalRetrievals > 0 {
		// Hit rate query — count rows where data_json has Selected
		// with at least one entry. Cross-dialect JSON probing is
		// painful; we approximate via a separate scan of the same
		// rows. For MVP, run the same query again with an
		// explicit ZeroHit check.
		zeroQ := `SELECT data_json FROM knowledge_events
		           WHERE org_id = ? AND event_type = ?
		             AND occurred_at >= ` + sinceClause
		zrows, err := s.QueryContext(ctx, zeroQ, orgID, eventTypeRetrieval)
		if err == nil {
			var zeros int
			for zrows.Next() {
				var blob string
				if err := zrows.Scan(&blob); err != nil {
					continue
				}
				var p struct {
					Trace struct {
						Selected []any `json:"selected"`
					} `json:"trace"`
				}
				_ = json.Unmarshal([]byte(blob), &p)
				if len(p.Trace.Selected) == 0 {
					zeros++
				}
			}
			zrows.Close()
			out.ZeroHitRate = float64(zeros) / float64(out.TotalRetrievals)
			out.HitRate = 1.0 - out.ZeroHitRate
		}
		out.FallbackRate = out.FallbackRate / float64(out.TotalRetrievals)
	}
	if semanticCount > 0 {
		out.AvgSemanticScore = semanticTotal / float64(semanticCount)
	}
	if droppedCount > 0 {
		out.AvgBudgetDropped = droppedTotal / float64(droppedCount)
	}
	if tokensCount > 0 {
		out.AvgEstimatedTokens = tokensTotal / float64(tokensCount)
	}

	// Embedding model drift (§4).
	mrows, err := s.QueryContext(ctx, `
		SELECT DISTINCT embedding_model_version
		  FROM knowledge_assets
		 WHERE org_id = ? AND embedding_model_version <> ''`, orgID)
	if err == nil {
		defer mrows.Close()
		for mrows.Next() {
			var v string
			if err := mrows.Scan(&v); err == nil {
				out.EmbeddingModels = append(out.EmbeddingModels, v)
			}
		}
		out.DistinctEmbeddingModels = len(out.EmbeddingModels)
	}

	return out, nil
}
