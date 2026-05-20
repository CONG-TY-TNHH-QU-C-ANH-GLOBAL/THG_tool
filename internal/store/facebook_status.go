// Domain: identities (see internal/store/DOMAINS.md)
package store

type FacebookStatusSummary struct {
	Connected  bool
	Account    string
	Groups     int
	LeadsToday int
}

func (s *Store) GetFacebookStatusForOrg(orgID int64) FacebookStatusSummary {
	var result FacebookStatusSummary
	var account string

	_ = s.db.QueryRow(`
		SELECT COALESCE(NULLIF(fb_display_name,''), NULLIF(fb_username,''), NULLIF(fb_user_id,''), name)
		FROM accounts
		WHERE org_id = ? AND browser_logged_in = 1 AND status = 'active'
		LIMIT 1`, orgID).Scan(&account)
	result.Connected = account != ""
	result.Account = account

	_ = s.db.QueryRow(`SELECT COUNT(*) FROM groups WHERE org_id = ? AND active = 1`, orgID).Scan(&result.Groups)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM leads WHERE org_id = ? AND DATE(created_at) = DATE('now')`, orgID).Scan(&result.LeadsToday)
	return result
}
