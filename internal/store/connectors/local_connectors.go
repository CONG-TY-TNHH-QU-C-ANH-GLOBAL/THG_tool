// Domain: connectors (see internal/store/DOMAINS.md)
package connectors

import "database/sql"

// localConnectorColumns is the shared SELECT list for extension-connector
// reads; scanLocalConnector is its row-scan twin. List and point lookup must
// stay column-aligned — change them together.
const localConnectorColumns = `id, COALESCE(org_id,0), name, created_by,
	        COALESCE(hostname,''), COALESCE(os,''), COALESCE(version,''),
	        COALESCE(kind,'worker'), COALESCE(transport,'poll'), COALESCE(assigned_account_id,0),
	        COALESCE(capabilities_json,'{}'), COALESCE(current_url,''), COALESCE(fb_user_id,''),
	        COALESCE(fb_display_name,''), COALESCE(fb_username,''), COALESCE(fb_profile_url,''),
	        COALESCE(stream_status,'idle'), COALESCE(chrome_error,''),
	        COALESCE(identity_confidence,''), COALESCE(identity_extraction_method,''),
	        COALESCE(identity_last_verified_at,''),
	        COALESCE(browser_profile_id,''), COALESCE(machine_label,''),
	        COALESCE(build_number,''), COALESCE(release_channel,''),
	        last_seen, active, created_at`

func scanLocalConnector(scan func(...any) error) (AgentToken, error) {
	var t AgentToken
	var lastSeen sql.NullTime
	if err := scan(&t.ID, &t.OrgID, &t.Name, &t.CreatedBy, &t.Hostname, &t.OS, &t.Version,
		&t.Kind, &t.Transport, &t.AssignedAccountID, &t.CapabilitiesJSON, &t.CurrentURL, &t.FBUserID,
		&t.FBDisplayName, &t.FBUsername, &t.FBProfileURL, &t.StreamStatus, &t.ChromeError,
		&t.IdentityConfidence, &t.IdentityExtractionMethod, &t.IdentityLastVerifiedAt,
		&t.BrowserProfileID, &t.MachineLabel, &t.BuildNumber, &t.ReleaseChannel,
		&lastSeen, &t.Active, &t.CreatedAt); err != nil {
		return AgentToken{}, err
	}
	if lastSeen.Valid {
		t.LastSeen = &lastSeen.Time
	}
	t.Online = agentOnline(t.LastSeen, t.Active)
	return t, nil
}

func (s *Store) ListLocalConnectors(orgID int64) ([]AgentToken, error) {
	rows, err := s.db.Query(
		`SELECT `+localConnectorColumns+`
		 FROM agent_tokens
		 WHERE org_id = ? AND kind = 'extension_connector'
		   AND active = 1
		 ORDER BY last_seen DESC, created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentToken
	for rows.Next() {
		t, err := scanLocalConnector(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetLocalConnectorByID returns the ACTIVE extension connector with the exact
// id in the org, or (nil, nil) when absent or revoked — a binding released via
// Forget Device resolves to nil, never to another connector.
func (s *Store) GetLocalConnectorByID(id, orgID int64) (*AgentToken, error) {
	row := s.db.QueryRow(
		`SELECT `+localConnectorColumns+`
		 FROM agent_tokens
		 WHERE id = ? AND org_id = ? AND kind = 'extension_connector' AND active = 1`,
		id, orgID,
	)
	t, err := scanLocalConnector(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
