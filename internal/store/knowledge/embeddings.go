package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
)

// Workspace Knowledge OS — Layer 2.5 persistence: embedding state.
//
// These methods are the narrow store surface the embedding worker
// (workspace_knowledge/embedding.Worker) consumes. The worker holds
// only a PendingStore interface (declared in worker.go); *knowledge.Store
// satisfies it by implementing the four methods below.
//
// SQLite vs Postgres:
//
//   - All four methods work on both dialects. The metadata columns
//     (status, model_version, generated_at, input_hash, attempts,
//     last_error) exist on both.
//   - SQLite does NOT have a VECTOR column, so the actual vector is
//     NEVER written on SQLite. UpdateEmbeddingVector silently skips
//     the vector portion on SQLite and updates only metadata —
//     useful for local-dev testing of the pipeline state machine
//     without standing up Postgres.
//   - On Postgres, the vector portion is written and the HNSW index
//     picks it up immediately (HNSW is incremental).
//
// Concurrency on Postgres: ListPendingEmbeddingsForUpdate uses
// "FOR UPDATE SKIP LOCKED" so multiple worker replicas can co-process
// the pending queue without racing on the same row. On SQLite the
// same SQL works but locks aggressively — single-worker only there
// (SQLite cannot host vectors anyway, see above).

// ListPendingEmbeddings returns up to `limit` rows in
// embedding_status='pending' state. Newest-first by id so the most
// recently ingested assets get vectors before older backlog. Workers
// call this every Tick().
//
// Returns ([], nil) when no rows are pending — that is the normal
// idle state, not an error.
//
// IMPORTANT: this method does NOT lock rows. Multiple concurrent
// workers calling it MAY return overlapping batches. PG-only
// deployments should switch to ListPendingEmbeddingsForUpdate (TODO).
// We accept this for now because the embedding API is idempotent for
// the same input text — overlap costs a redundant API call, not
// correctness.
//
// Returns embedding.Pending rows — the worker-facing shape declared
// in the embedding package so the store satisfies its [embedding.PendingStore]
// interface without an adapter.
func (s *Store) ListPendingEmbeddings(ctx context.Context, limit int) ([]embedding.Pending, error) {
	if limit <= 0 || limit > 256 {
		limit = 32
	}
	q := `SELECT id, org_id, title, description, tags, type, embedding_input_hash, embedding_attempts
	        FROM knowledge_assets
	       WHERE embedding_status = ?
	       ORDER BY id DESC
	       LIMIT ?`
	rows, err := s.queryContext(ctx, q, "pending", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]embedding.Pending, 0, limit)
	for rows.Next() {
		var r embedding.Pending
		if err := rows.Scan(&r.AssetID, &r.OrgID, &r.Title, &r.Description, &r.Tags, &r.AssetType, &r.InputHash, &r.Attempts); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateEmbeddingSuccess marks one asset as successfully embedded.
// vector is the raw float32 slice from the Embedder; it gets
// serialized to pgvector's text format on PG, dropped on SQLite.
//
// modelVersion is persisted verbatim — it is the stable string the
// Searcher uses to detect stale embeddings (PR-2). Caller MUST pass
// Embedder.ModelVersion() — not a hard-coded value — so model
// upgrades flow through correctly.
//
// inputHash is the hash of the text actually embedded (computed by
// the worker, NOT pulled from the asset row at this point).
// Decoupling protects against TOCTOU: if the asset was updated
// between the worker reading it and writing the result, the hash
// reflects what was embedded — which is what's currently in the
// vector column — not the asset's current state.
func (s *Store) UpdateEmbeddingSuccess(ctx context.Context, assetID, orgID int64, vector []float32, modelVersion, inputHash string) error {
	if assetID <= 0 || orgID <= 0 {
		return fmt.Errorf("knowledge: ids required")
	}
	if modelVersion == "" {
		return fmt.Errorf("knowledge: model_version required")
	}

	switch s.dialect.Name() {
	case "postgres":
		// PG path: vector + metadata in one statement.
		// pgvector accepts the array-literal form `'[1.0, 2.0, ...]'::vector`.
		_, err := s.execContext(ctx, `
			UPDATE knowledge_assets
			   SET embedding              = ?::vector,
			       embedding_status       = 'generated',
			       embedding_model_version = ?,
			       embedding_input_hash   = ?,
			       embedding_generated_at = `+s.dialect.NowExpr()+`,
			       embedding_attempts     = 0,
			       embedding_last_error   = ''
			 WHERE id = ? AND org_id = ?`,
			pgVectorLiteral(vector), modelVersion, inputHash, assetID, orgID,
		)
		return err
	default:
		// SQLite path: metadata only. The vector has no destination
		// here — workers running against SQLite are typically test or
		// local-dev runs that just want to exercise the state machine.
		_, err := s.execContext(ctx, `
			UPDATE knowledge_assets
			   SET embedding_status       = 'generated',
			       embedding_model_version = ?,
			       embedding_input_hash   = ?,
			       embedding_generated_at = `+s.dialect.NowExpr()+`,
			       embedding_attempts     = 0,
			       embedding_last_error   = ''
			 WHERE id = ? AND org_id = ?`,
			modelVersion, inputHash, assetID, orgID,
		)
		return err
	}
}

// RecordEmbeddingAttempt bumps the attempt counter after a failed
// embedding call. When attempts reaches maxAttempts, status flips to
// 'failed' so the worker stops retrying until operator action.
//
// errMsg is truncated to keep the column bounded.
func (s *Store) RecordEmbeddingAttempt(ctx context.Context, assetID, orgID int64, errMsg string, maxAttempts int) error {
	if assetID <= 0 || orgID <= 0 {
		return fmt.Errorf("knowledge: ids required")
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if len(errMsg) > 512 {
		errMsg = errMsg[:512]
	}
	// Two-step to keep the SQL portable: read current attempts, decide
	// the new status, write both. Wrapping in a transaction would be
	// safer against concurrent attempt-records on the same row, but
	// (a) one row corresponds to one worker batch at a time in
	// practice, and (b) over-counting attempts errs toward sooner
	// failure marking, which is the safe direction.
	var current int
	if err := s.queryRowContext(ctx,
		`SELECT embedding_attempts FROM knowledge_assets WHERE id = ? AND org_id = ?`,
		assetID, orgID,
	).Scan(&current); err != nil {
		if err == sql.ErrNoRows {
			return nil // row gone (deleted concurrently); not our problem
		}
		return err
	}
	newAttempts := current + 1
	newStatus := "pending"
	if newAttempts >= maxAttempts {
		newStatus = "failed"
	}
	_, err := s.execContext(ctx, `
		UPDATE knowledge_assets
		   SET embedding_attempts   = ?,
		       embedding_status     = ?,
		       embedding_last_error = ?
		 WHERE id = ? AND org_id = ?`,
		newAttempts, newStatus, errMsg, assetID, orgID,
	)
	return err
}

// ResetEmbeddingFailures clears the failed status on rows so the
// worker re-picks them up. Operator-facing — wired to a future
// /api/org/knowledge/embeddings/retry endpoint, or invoked by a CLI
// during incident recovery.
func (s *Store) ResetEmbeddingFailures(ctx context.Context, orgID int64) (int64, error) {
	if orgID <= 0 {
		return 0, fmt.Errorf("knowledge: org_id required")
	}
	res, err := s.execContext(ctx, `
		UPDATE knowledge_assets
		   SET embedding_status   = 'pending',
		       embedding_attempts = 0,
		       embedding_last_error = ''
		 WHERE org_id = ? AND embedding_status = 'failed'`,
		orgID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// EmbeddingStats is the aggregate snapshot the Replay/observability
// surfaces consume. Cheap aggregate query; no caching at MVP scale.
type EmbeddingStats struct {
	Pending   int `json:"pending"`
	Generated int `json:"generated"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// GetEmbeddingStatsForOrg returns the counts of each status.
// Drives the /api/org/knowledge/stats response in PR-2 and the
// operator-dashboard counters.
func (s *Store) GetEmbeddingStatsForOrg(ctx context.Context, orgID int64) (*EmbeddingStats, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("knowledge: org_id required")
	}
	rows, err := s.queryContext(ctx, `
		SELECT embedding_status, COUNT(*)
		  FROM knowledge_assets
		 WHERE org_id = ?
		 GROUP BY embedding_status`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &EmbeddingStats{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		switch status {
		case "pending":
			out.Pending = n
		case "generated":
			out.Generated = n
		case "failed":
			out.Failed = n
		case "skipped":
			out.Skipped = n
		}
	}
	return out, rows.Err()
}

// markEmbeddingPendingIfTextChanged is the post-Upsert hook that
// flips embedding_status to 'pending' when the asset's text content
// changed since the last embedding. It runs after UpsertAsset has
// persisted the new fields; the comparison is done in SQL via
// `WHERE embedding_input_hash <> ?`, so the no-change case is a
// zero-row UPDATE.
//
// Failures here are LOGGED, not returned. A failed hash-update means
// the next worker tick MIGHT re-embed an already-fresh asset (waste)
// or skip a changed one (stale). Both are recoverable on the next
// upsert; aborting the operator-facing INSERT to preserve hash
// hygiene is the worse trade.
//
// The function reads the canonical hash via [embedding.InputHash]
// over the asset struct the caller just persisted. tags is passed
// separately because the function caller already computed
// NormalizeTags — re-doing it here would be a redundant allocation.
func (s *Store) markEmbeddingPendingIfTextChanged(ctx context.Context, a *assets.Asset, normalizedTags []string) {
	if a == nil || a.OrgID <= 0 {
		return
	}
	// Build a hash-input asset with normalised tags. The embedding
	// worker will see exactly these same fields when it generates,
	// so the hashes match end-to-end.
	hashAsset := *a
	hashAsset.Tags = normalizedTags
	newHash := embedding.InputHash(&hashAsset)

	// Conditional UPDATE: only flips when content actually changed.
	// embedding_attempts reset to 0 so old failures don't haunt fresh
	// content. embedding_last_error cleared for the same reason.
	q := `UPDATE knowledge_assets
	         SET embedding_status     = 'pending',
	             embedding_input_hash = ?,
	             embedding_attempts   = 0,
	             embedding_last_error = ''
	       WHERE org_id = ?
	         AND (embedding_input_hash IS NULL OR embedding_input_hash <> ?)`
	args := []any{newHash, a.OrgID, newHash}
	// Scope by ID when known (no-ExternalID path), by (source_id,
	// external_id) otherwise. Both forms hit the row exactly once.
	if a.ID > 0 {
		q += ` AND id = ?`
		args = append(args, a.ID)
	} else {
		q += ` AND source_id = ? AND external_id = ?`
		args = append(args, a.SourceID, a.ExternalID)
	}
	if _, err := s.execContext(ctx, q, args...); err != nil {
		log.Printf("[knowledge] hash-update failed for org=%d asset=%d: %v", a.OrgID, a.ID, err)
	}
}

// pgVectorLiteral converts a Go float32 slice to pgvector's text
// representation: "[1.0,2.0,3.0]". The receiving column is typed
// VECTOR(N) so PG parses this back to a binary vector internally.
//
// Why not use a driver-native type? pgx supports a pgvector helper
// package but pulling it in is one more dependency. The text format
// is documented and stable; this 8-line function is the entire
// integration.
func pgVectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.Grow(len(v) * 8)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		// Go's default float formatting is fine for pgvector — it
		// accepts standard IEEE-754 decimal strings.
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}
