// Domain: knowledge (see internal/store/DOMAINS.md)
//
// Note 2026-05-21 (Phase 3 of STORE_SUBPACKAGE_REFACTOR): this file
// was reassigned from `crawl` → `knowledge` after audit. The
// `data_sources` table is a LEGACY pre-Knowledge-OS connector
// registry; consumers are agent_brain / skills / autoflow handlers,
// not the crawl scheduler. Co-locating with the knowledge domain
// matches the architectural intent. Stays in top-level store/ for
// now; will be folded into the knowledge subpackage when that domain
// extracts (Phase 4) or deprecated in favour of `knowledge_sources`.
package store

import (
	"database/sql"
	"time"
)

// DataSource is an org-scoped external/private knowledge source.
type DataSource struct {
	ID           int64      `json:"id"`
	OrgID        int64      `json:"org_id"`
	Type         string     `json:"type"`
	Name         string     `json:"name"`
	SourceURL    string     `json:"source_url"`
	Status       string     `json:"status"`
	ItemCount    int        `json:"item_count"`
	Summary      string     `json:"summary"`
	MetadataJSON string     `json:"metadata_json"`
	LastError    string     `json:"last_error"`
	LastSyncAt   *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateDataSource registers a connector source without reading it yet.
func (s *Store) CreateDataSource(src *DataSource) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO data_sources (org_id, type, name, source_url, status, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?)`,
		src.OrgID, src.Type, src.Name, src.SourceURL, src.Status, src.MetadataJSON)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListDataSources returns all connector sources for an organization.
func (s *Store) ListDataSources(orgID int64) ([]DataSource, error) {
	rows, err := s.db.Query(`
		SELECT id, org_id, type, name, source_url, status, item_count, summary,
		       COALESCE(metadata_json,'{}'), COALESCE(last_error,''), last_sync_at, created_at, updated_at
		FROM data_sources
		WHERE org_id = ?
		ORDER BY updated_at DESC, created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DataSource
	for rows.Next() {
		src, err := scanDataSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *src)
	}
	return out, rows.Err()
}

// GetDataSourceForOrg returns one connector source scoped to the organization.
func (s *Store) GetDataSourceForOrg(orgID, id int64) (*DataSource, error) {
	row := s.db.QueryRow(`
		SELECT id, org_id, type, name, source_url, status, item_count, summary,
		       COALESCE(metadata_json,'{}'), COALESCE(last_error,''), last_sync_at, created_at, updated_at
		FROM data_sources
		WHERE org_id = ? AND id = ?`, orgID, id)
	return scanDataSource(row)
}

// UpdateDataSourceSyncResult stores the last sync status and source summary.
func (s *Store) UpdateDataSourceSyncResult(orgID, id int64, status string, itemCount int, summary, metadataJSON, lastError string, synced bool) error {
	if synced {
		_, err := s.db.Exec(`
			UPDATE data_sources
			SET status=?, item_count=?, summary=?, metadata_json=?, last_error=?,
			    last_sync_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP
			WHERE org_id=? AND id=?`,
			status, itemCount, summary, metadataJSON, lastError, orgID, id)
		return err
	}
	_, err := s.db.Exec(`
		UPDATE data_sources
		SET status=?, item_count=?, summary=?, metadata_json=?, last_error=?,
		    updated_at=CURRENT_TIMESTAMP
		WHERE org_id=? AND id=?`,
		status, itemCount, summary, metadataJSON, lastError, orgID, id)
	return err
}

// DeleteDataSourceForOrg removes one connector source from an organization.
func (s *Store) DeleteDataSourceForOrg(orgID, id int64) error {
	_, err := s.db.Exec(`DELETE FROM data_sources WHERE org_id=? AND id=?`, orgID, id)
	return err
}

func scanDataSource(row scanner) (*DataSource, error) {
	var src DataSource
	var lastSync sql.NullTime
	err := row.Scan(
		&src.ID, &src.OrgID, &src.Type, &src.Name, &src.SourceURL, &src.Status,
		&src.ItemCount, &src.Summary, &src.MetadataJSON, &src.LastError,
		&lastSync, &src.CreatedAt, &src.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if lastSync.Valid {
		src.LastSyncAt = &lastSync.Time
	}
	return &src, nil
}
