// Domain: connectors (see internal/store/DOMAINS.md)
package store

// ConnectorOwnsAccountStream reports whether the agent identified by agentID
// is the legitimate owner of the live browser stream for accountID inside
// orgID.
//
// Ownership is granted when either:
//
//  1. The most recent connector_screenshots row for (orgID, accountID) was
//     produced by this agent — i.e. the dashboard is currently rendering
//     this agent's screen for that account, or
//
//  2. The agent is online and either explicitly assigned to the account or
//     unassigned (kind="any") so it can serve any account in the org.
//
// Handlers that accept work attributed to an account (crawl results, input
// commands, identity updates) MUST call this before persisting state so a
// rogue connector that was paired against the org cannot impersonate another
// connector's stream.
func (s *Store) ConnectorOwnsAccountStream(orgID, agentID, accountID int64) (bool, error) {
	if orgID <= 0 || agentID <= 0 || accountID <= 0 {
		return false, nil
	}
	screen, err := s.GetLatestConnectorScreenshot(orgID, accountID)
	if err != nil {
		return false, err
	}
	if screen != nil && screen.AgentID == agentID {
		return true, nil
	}
	connectors, err := s.ListLocalConnectors(orgID)
	if err != nil {
		return false, err
	}
	for _, conn := range connectors {
		if conn.ID != agentID {
			continue
		}
		if !conn.Online {
			return false, nil
		}
		if conn.AssignedAccountID == accountID || conn.AssignedAccountID == 0 {
			return true, nil
		}
		return false, nil
	}
	return false, nil
}
