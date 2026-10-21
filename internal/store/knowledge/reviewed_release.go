package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/security"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// PublishReviewedAssets is an operator maintenance operation, not ingestion.
// It atomically inserts public projections, approval audit rows and healthy
// source status. It never edits existing assets or approves an entire source.
// The caller supplies a new, manual, syncing source and serializes releases.
func (s *Store) PublishReviewedAssets(ctx context.Context, orgID, sourceID int64, approval string, items []*assets.Asset) error {
	if strings.TrimSpace(approval) == "" || len(items) == 0 || len(items) > 32 {
		return errors.New("knowledge: bounded explicit approval required")
	}
	for _, a := range items {
		if err := validateReviewedAsset(a, orgID, sourceID); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // Commit makes this a harmless no-op.
	if err := s.checkEmptyReleaseSource(ctx, tx, orgID, sourceID); err != nil {
		return err
	}
	for _, a := range items {
		if err := s.insertReviewedAsset(ctx, tx, a, approval); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, s.dialect.Rebind(`UPDATE knowledge_sources
		SET health_status = 'healthy', health_message = 'reviewed public release',
		last_sync_at = CURRENT_TIMESTAMP, last_sync_asset_count = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND org_id = ?`), len(items), sourceID, orgID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func validateReviewedAsset(a *assets.Asset, orgID, sourceID int64) error {
	if a == nil || orgID <= 0 || sourceID <= 0 || a.ID != 0 || a.OrgID != orgID || a.SourceID != sourceID {
		return errors.New("knowledge: reviewed asset scope mismatch")
	}
	if err := a.Validate(); err != nil {
		return err
	}
	switch a.Type {
	case assets.AssetFAQ, assets.AssetShippingPolicy, assets.AssetSalesPlaybook, assets.AssetPricingRule:
	default:
		return errors.New("knowledge: reviewed release accepts only public reply knowledge types")
	}
	projection, ok := assets.ReplyKnowledgeFacts(a.Payload)
	if !ok {
		return errors.New("knowledge: reviewed release requires public fact projection")
	}
	for _, fact := range projection.Facts {
		if security.Sanitize(fact).Cleaned != fact {
			return errors.New("knowledge: public fact requires renewed review")
		}
	}
	if a.ExternalID == "" || a.State != assets.StateApproved || a.Pinned || a.Boost != 0 {
		return errors.New("knowledge: explicit reviewed asset identity/state required")
	}
	if security.Sanitize(a.Title).Cleaned != a.Title || security.Sanitize(a.Description).Cleaned != a.Description {
		return errors.New("knowledge: reviewed content requires sanitization and renewed review")
	}
	return nil
}

func (s *Store) checkEmptyReleaseSource(ctx context.Context, tx *sql.Tx, orgID, sourceID int64) error {
	var health, policy string
	err := tx.QueryRowContext(ctx, s.dialect.Rebind(`SELECT health_status, sync_policy
		FROM knowledge_sources WHERE id = ? AND org_id = ?`), sourceID, orgID).Scan(&health, &policy)
	if err != nil {
		return err
	}
	if health != string(sources.HealthSyncing) || policy != string(sources.SyncManual) {
		return errors.New("knowledge: release source must be manual and syncing")
	}
	var count int
	err = tx.QueryRowContext(ctx, s.dialect.Rebind(`SELECT COUNT(*) FROM knowledge_assets
		WHERE source_id = ? AND org_id = ?`), sourceID, orgID).Scan(&count)
	if err == nil && count != 0 {
		return errors.New("knowledge: release refuses existing source assets")
	}
	return err
}

func (s *Store) insertReviewedAsset(ctx context.Context, tx *sql.Tx, a *assets.Asset, approval string) error {
	tags, _ := json.Marshal(assets.NormalizeTags(a.Tags))
	var id int64
	err := tx.QueryRowContext(ctx, s.dialect.Rebind(`INSERT INTO knowledge_assets
		(org_id, source_id, external_id, type, title, description, tags, payload, state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'approved') RETURNING id`),
		a.OrgID, a.SourceID, a.ExternalID, string(a.Type), a.Title, a.Description, string(tags), string(a.Payload)).Scan(&id)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]string{"approval_reference": approval, "operation": "reviewed_public_release"})
	// Actor 0 denotes this maintenance process, not an impersonated admin.
	_, err = tx.ExecContext(ctx, s.dialect.Rebind(`INSERT INTO knowledge_feedback
		(org_id, user_id, retrieval_id, asset_id, kind, data_json)
		VALUES (?, 0, '', ?, 'approve', ?)`), a.OrgID, id, string(data))
	return err
}
