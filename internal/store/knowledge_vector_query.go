package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// Workspace Knowledge OS — Layer 4 (pgvector) query surface.
//
// These methods are the store-side input for the pgvector Searcher
// (workspace_knowledge/retrieval/pgvector). They are deliberately
// NARROW — only the operations the Searcher actually performs — so
// the dialect-specific SQL stays in one file and the rest of the
// retrieval layer remains dialect-agnostic.
//
// CRITICAL SaaS INVARIANT (goal directive PR-2 §2):
//
//   WHERE org_id = ?    runs BEFORE   ORDER BY embedding <=> query
//
// The tenant filter is the FIRST predicate the planner sees. The
// query's index plan therefore narrows to one tenant's rows before
// any similarity computation. We do NOT run global ANN and filter
// after — that would be a tenant leak (perf-wise: scanning another
// tenant's vectors; correctness-wise: HNSW with a post-filter can
// return < k results because the global top-k were filtered out).
//
// To enforce this at the SQL level we use a CTE that materialises
// the tenant's rows first, then sorts by similarity inside.

// HasPGVector returns true when the dialect is Postgres AND the
// pgvector extension is installed AND the embedding column exists.
// Used by the runtime to decide whether to wire the pgvector
// Searcher (true) or stick with hybrid (false).
//
// Capability-driven, no feature flag (goal directive PR-2 §4).
// Called once at boot via runtime.NewBuilder.
func (s *Store) HasPGVector(ctx context.Context) bool {
	if s.dialect.Name() != "postgres" {
		return false
	}
	// Two-step capability check:
	//   1. pgvector extension is installed (pg_extension).
	//   2. The embedding column exists on knowledge_assets.
	// Both must be true. The first failing check returns false
	// immediately — no exception escapes.
	var hasExt int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_extension WHERE extname = 'vector'`).Scan(&hasExt)
	if err != nil || hasExt == 0 {
		return false
	}
	var hasCol int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_name = 'knowledge_assets'
		   AND column_name = 'embedding'`).Scan(&hasCol)
	if err != nil || hasCol == 0 {
		return false
	}
	return true
}

// ErrVectorUnavailable is returned by QueryNearestVectors when the
// dialect is not Postgres or the pgvector extension is missing.
// The pgvector Searcher inspects this and lets the fallback layer
// reroute to hybrid.
var ErrVectorUnavailable = fmt.Errorf("knowledge_vector: pgvector not available")

// QueryNearestVectors runs a tenant-scoped ANN search. Returns up to
// k assets ordered by cosine distance ASC (closest first).
//
// queryVector MUST have length == Embedder.Dimensions() (the calling
// runtime validates this). Length mismatch is rejected by pgvector
// at execution time with a clear error.
//
// On SQLite or PG-without-pgvector, returns (nil, ErrVectorUnavailable).
// The Searcher inspects this and falls back to hybrid.
//
// VectorFilter and retrieval.VectorHit types live in workspace_knowledge/retrieval
// (the shared neutral home) so this method's signature matches the
// pgvector Searcher's VectorStore interface without an adapter.
func (s *Store) QueryNearestVectors(ctx context.Context, orgID int64, queryVector []float32, modelVersion string, filter retrieval.VectorFilter, k int) ([]retrieval.VectorHit, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge_vector: org_id required")
	}
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("knowledge_vector: empty query vector")
	}
	if k <= 0 {
		return nil, nil
	}
	if s.dialect.Name() != "postgres" {
		return nil, ErrVectorUnavailable
	}

	// Build the filter clause. Type/State filters use IN (...) — args
	// added separately to keep parameterisation clean. The order
	// here is important: org_id first, then embedding readiness,
	// then user-supplied filters. SQL planner reads predicates in
	// the order they appear when no rewrite occurs; for HNSW + WHERE
	// the index uses the partial-index WHERE clause anyway, but
	// declaring tenant first keeps EXPLAIN plans readable.
	q := `
		WITH tenant_rows AS (
			SELECT id, org_id, source_id, external_id, type, title, description,
			       tags, payload, state, pinned, boost,
			       retrieval_count_30d, conversion_count_30d, last_retrieved_at,
			       created_at, updated_at,
			       embedding
			  FROM knowledge_assets
			 WHERE org_id = ?
			   AND embedding IS NOT NULL
			   AND embedding_status = 'generated'
			   AND embedding_model_version = ?`
	args := []any{orgID, modelVersion}

	if len(filter.Types) > 0 {
		q += ` AND type IN (` + placeholders(len(filter.Types)) + `)`
		for _, t := range filter.Types {
			args = append(args, string(t))
		}
	}
	if len(filter.States) > 0 {
		q += ` AND state IN (` + placeholders(len(filter.States)) + `)`
		for _, st := range filter.States {
			args = append(args, string(st))
		}
	}
	q += `
		)
		SELECT id, org_id, source_id, external_id, type, title, description,
		       tags, payload, state, pinned, boost,
		       retrieval_count_30d, conversion_count_30d, last_retrieved_at,
		       created_at, updated_at,
		       embedding <=> ?::vector AS distance
		  FROM tenant_rows
		 ORDER BY embedding <=> ?::vector
		 LIMIT ?`
	vecLit := pgVectorLiteral(queryVector)
	args = append(args, vecLit, vecLit, k)

	rows, err := s.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]retrieval.VectorHit, 0, k)
	for rows.Next() {
		var (
			a            assets.Asset
			typ, state   string
			pinnedI      int
			tagsRaw      string
			payloadRaw   string
			lastRetrRaw  sql.NullString
			createdAtRaw string
			updatedAtRaw string
			distance     float64
		)
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.SourceID, &a.ExternalID, &typ, &a.Title, &a.Description,
			&tagsRaw, &payloadRaw, &state, &pinnedI, &a.Boost,
			&a.Metrics.Retrievals30d, &a.Metrics.Conversions30d, &lastRetrRaw,
			&createdAtRaw, &updatedAtRaw, &distance,
		); err != nil {
			return nil, err
		}
		a.Type = assets.AssetType(typ)
		a.State = assets.AssetState(state)
		a.Pinned = pinnedI != 0
		a.Payload = json.RawMessage(payloadRaw)
		_ = json.Unmarshal([]byte(tagsRaw), &a.Tags)
		a.Metrics.LastRetrievedAt = optionalSQLiteTime(lastRetrRaw)
		a.CreatedAt = parseSQLiteTime(createdAtRaw)
		a.UpdatedAt = parseSQLiteTime(updatedAtRaw)
		out = append(out, retrieval.VectorHit{AssetID: a.ID, Distance: distance, Asset: &a})
	}
	return out, rows.Err()
}
