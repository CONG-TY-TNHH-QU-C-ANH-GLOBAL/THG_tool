// Domain: identities (see internal/store/DOMAINS.md)
package identities

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
)

// AddAccount inserts a new social account, encrypting cookies_json at rest.
func (s *Store) AddAccount(a *models.Account) (int64, error) {
	encCookies, err := auth.Encrypt(a.CookiesJSON, s.encKey)
	if err != nil {
		return 0, fmt.Errorf("encrypt cookies: %w", err)
	}
	res, err := s.db.Exec(
		`INSERT INTO accounts (org_id, platform, name, email, cookies_json, proxy_url, user_agent, status, notes, assigned_user_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.OrgID, a.Platform, a.Name, a.Email, encCookies, a.ProxyURL, a.UserAgent, a.Status, a.Notes, a.AssignedUserID,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetAccountForOrg returns an account by ID only when the row's org_id matches
// orgID. Pass orgID=0 to bypass the check (superadmin / internal worker code).
// Returns (nil, sql.ErrNoRows) when the account exists in another org so handlers
// cannot leak the existence of a foreign account.
//
// All tenant-facing handlers MUST use this helper instead of GetAccount(id) so
// the org_id boundary check happens at the data layer once, not at every call
// site.
func (s *Store) GetAccountForOrg(id, orgID int64) (*models.Account, error) {
	acc, err := s.GetAccount(id)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, nil
	}
	if orgID > 0 && acc.OrgID != orgID {
		return nil, sql.ErrNoRows
	}
	return acc, nil
}

// GetAccount returns an account by ID, decrypting cookies_json.
//
// Prefer GetAccountForOrg in tenant-facing code paths. GetAccount remains
// available for internal/worker contexts that already proved org ownership
// elsewhere (e.g. token-bound agent handlers) or that operate outside any
// tenant scope.
func (s *Store) GetAccount(id int64) (*models.Account, error) {
	var a models.Account
	var lastUsed string
	err := s.db.QueryRow(
		`SELECT a.id, COALESCE(a.org_id,0), a.platform, a.name, a.email, a.cookies_json, a.proxy_url, a.user_agent,
		        a.status, a.notes, COALESCE(a.last_used,''), a.created_at,
		        COALESCE(a.assigned_user_id,0), COALESCE(u.name,''), COALESCE(a.browser_logged_in,0), COALESCE(a.fb_user_id,''),
		        COALESCE(a.fb_display_name,''), COALESCE(a.fb_username,''), COALESCE(a.fb_profile_url,'')
		 FROM accounts a LEFT JOIN users u ON u.id = a.assigned_user_id
		 WHERE a.id = ?`, id,
	).Scan(&a.ID, &a.OrgID, &a.Platform, &a.Name, &a.Email, &a.CookiesJSON, &a.ProxyURL, &a.UserAgent,
		&a.Status, &a.Notes, &lastUsed, &a.CreatedAt, &a.AssignedUserID, &a.AssignedUserName, &a.BrowserLoggedIn, &a.FBUserID,
		&a.FBDisplayName, &a.FBUsername, &a.FBProfileURL)
	if err != nil {
		return nil, err
	}
	if lastUsed != "" {
		a.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
	}
	a.CookiesJSON, _ = auth.Decrypt(a.CookiesJSON, s.encKey)
	return &a, nil
}

// GetAccountByFacebookIdentity returns the account that owns a Facebook identity
// in an org, or (nil, nil) when none exists. Backed by the partial unique index
// uq_accounts_org_fb_identity (one FB identity = one account per org).
func (s *Store) GetAccountByFacebookIdentity(orgID int64, fbUserID string) (*models.Account, error) {
	fbUserID = strings.TrimSpace(fbUserID)
	if orgID <= 0 || fbUserID == "" {
		return nil, nil
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM accounts WHERE org_id = ? AND fb_user_id = ? LIMIT 1`, orgID, fbUserID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

// ResolveOrCreateAccountForFacebookIdentity is the Organic Sales Network entry
// point that maps a logged-in Facebook identity to its owning account, creating
// one owned by ownerUserID on first sight. This replaces the old "force the
// pre-assigned slot" behaviour: the account is keyed by FB identity, not by a
// pairing-time slot, so each distinct Facebook login becomes its own account.
//
// Returns (account, created, err). created=true when a new row was inserted.
// Empty fbUserID is a no-op (nil, false, nil) — preserves not-logged-in callers.
//
// Race-safe: a concurrent heartbeat reporting the same FB login loses the INSERT
// to the unique index (PR1) and we re-select the winner — never two accounts.
// Ownership is NOT changed here (no-steal): an existing row keeps its owner; the
// caller (heartbeat) decides whether a different-owner match is a conflict.
func (s *Store) ResolveOrCreateAccountForFacebookIdentity(orgID, ownerUserID int64, fbUserID string, meta FacebookIdentityMeta, email string) (*models.Account, bool, error) {
	fbUserID = strings.TrimSpace(fbUserID)
	if orgID <= 0 || fbUserID == "" {
		return nil, false, nil
	}
	if existing, err := s.GetAccountByFacebookIdentity(orgID, fbUserID); err != nil {
		return nil, false, err
	} else if existing != nil {
		return existing, false, nil
	}
	meta = normalizeFacebookIdentityMeta(meta)
	name := meta.DisplayName
	if name == "" {
		name = "Facebook " + fbUserID
	}
	encCookies, err := auth.Encrypt("", s.encKey)
	if err != nil {
		return nil, false, fmt.Errorf("encrypt cookies: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO accounts (org_id, platform, name, email, cookies_json, status, assigned_user_id, browser_logged_in, fb_user_id, fb_display_name, fb_username, fb_profile_url)
		 VALUES (?, 'facebook', ?, ?, ?, 'active', ?, 1, ?, ?, ?, ?)`,
		orgID, name, strings.ToLower(strings.TrimSpace(email)), encCookies, ownerUserID, fbUserID, meta.DisplayName, meta.Username, meta.ProfileURL,
	)
	if err != nil {
		// Lost the race to a concurrent create (unique index) → return the winner.
		if existing, gErr := s.GetAccountByFacebookIdentity(orgID, fbUserID); gErr == nil && existing != nil {
			return existing, false, nil
		}
		return nil, false, err
	}
	created, err := s.GetAccountByFacebookIdentity(orgID, fbUserID)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// SetBrowserLoggedIn marks whether an account has successfully logged into Facebook via the dashboard browser.
func (s *Store) SetBrowserLoggedIn(accountID int64, loggedIn bool, fbUserID ...string) error {
	v := 0
	if loggedIn {
		v = 1
	}
	if loggedIn && len(fbUserID) > 0 && fbUserID[0] != "" {
		_, err := s.db.Exec(`UPDATE accounts SET browser_logged_in = ?, fb_user_id = ? WHERE id = ?`, v, fbUserID[0], accountID)
		return err
	}
	if !loggedIn {
		_, err := s.db.Exec(`UPDATE accounts SET browser_logged_in = ?, fb_user_id = '' WHERE id = ?`, v, accountID)
		return err
	}
	_, err := s.db.Exec(`UPDATE accounts SET browser_logged_in = ? WHERE id = ?`, v, accountID)
	return err
}

// SetBrowserLoggedInState updates browser_logged_in without clearing the
// remembered Facebook identity. A Chrome Extension can temporarily lose a Chrome
// target while the account slot identity must remain auditable.
func (s *Store) SetBrowserLoggedInState(accountID int64, loggedIn bool) error {
	v := 0
	if loggedIn {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE accounts SET browser_logged_in = ? WHERE id = ?`, v, accountID)
	return err
}

// FacebookIdentityMeta is the human-readable identity observed from Facebook.
// FBUserID remains the hard identity; these fields are labels for operators.
type FacebookIdentityMeta struct {
	DisplayName string
	Username    string
	ProfileURL  string
}

// SetAccountFacebookIdentity stores the Facebook identity observed from the
// Chrome Extension. Email is updated only when the current session's Facebook ID is
// compatible with the account slot, so a different Facebook profile cannot
// silently overwrite another account's identity.
func (s *Store) SetAccountFacebookIdentity(accountID int64, fbUserID, email string, meta ...FacebookIdentityMeta) error {
	fbUserID = strings.TrimSpace(fbUserID)
	email = strings.ToLower(strings.TrimSpace(email))
	if accountID <= 0 || fbUserID == "" {
		return nil
	}
	var m FacebookIdentityMeta
	if len(meta) > 0 {
		m = normalizeFacebookIdentityMeta(meta[0])
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var existingFBUserID string
	err = tx.QueryRow(`SELECT COALESCE(fb_user_id,'') FROM accounts WHERE id = ?`, accountID).Scan(&existingFBUserID)
	if err != nil {
		return err
	}
	existingFBUserID = strings.TrimSpace(existingFBUserID)
	if existingFBUserID != "" && existingFBUserID != fbUserID {
		return fmt.Errorf("facebook profile mismatch for account slot")
	}

	if email != "" {
		_, err = tx.Exec(
			`UPDATE accounts
			 SET browser_logged_in = 1,
			     fb_user_id = ?,
			     email = ?,
			     fb_display_name = CASE WHEN ? != '' THEN ? ELSE fb_display_name END,
			     fb_username = CASE WHEN ? != '' THEN ? ELSE fb_username END,
			     fb_profile_url = CASE WHEN ? != '' THEN ? ELSE fb_profile_url END
			 WHERE id = ?`,
			fbUserID, email,
			m.DisplayName, m.DisplayName,
			m.Username, m.Username,
			m.ProfileURL, m.ProfileURL,
			accountID,
		)
	} else {
		_, err = tx.Exec(
			`UPDATE accounts
			 SET browser_logged_in = 1,
			     fb_user_id = ?,
			     fb_display_name = CASE WHEN ? != '' THEN ? ELSE fb_display_name END,
			     fb_username = CASE WHEN ? != '' THEN ? ELSE fb_username END,
			     fb_profile_url = CASE WHEN ? != '' THEN ? ELSE fb_profile_url END
			 WHERE id = ?`,
			fbUserID,
			m.DisplayName, m.DisplayName,
			m.Username, m.Username,
			m.ProfileURL, m.ProfileURL,
			accountID,
		)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeFacebookIdentityMeta(meta FacebookIdentityMeta) FacebookIdentityMeta {
	meta.DisplayName = strings.TrimSpace(meta.DisplayName)
	meta.Username = strings.Trim(strings.TrimSpace(meta.Username), "@/ ")
	meta.ProfileURL = strings.TrimSpace(meta.ProfileURL)
	if len(meta.DisplayName) > 120 {
		meta.DisplayName = meta.DisplayName[:120]
	}
	if len(meta.Username) > 80 {
		meta.Username = meta.Username[:80]
	}
	if len(meta.ProfileURL) > 512 {
		meta.ProfileURL = meta.ProfileURL[:512]
	}
	return meta
}

// SetAccountEmailIfBlank saves a verified login email without overwriting an
// email that an admin already assigned to the account slot.
func (s *Store) SetAccountEmailIfBlank(accountID int64, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE accounts SET email = ? WHERE id = ? AND COALESCE(NULLIF(email, ''), '') = ''`, email, accountID)
	return err
}

// GetAllAccounts returns accounts scoped to an org. orgID=0 returns all (superadmin).
func (s *Store) GetAllAccounts(orgID int64) ([]models.Account, error) {
	q := `SELECT a.id, COALESCE(a.org_id,0), a.platform, a.name, a.email, a.cookies_json, a.proxy_url, a.user_agent,
		        a.status, a.notes, COALESCE(a.last_used,''), a.created_at,
		        COALESCE(a.assigned_user_id,0), COALESCE(u.name,''), COALESCE(a.browser_logged_in,0), COALESCE(a.fb_user_id,''),
		        COALESCE(a.fb_display_name,''), COALESCE(a.fb_username,''), COALESCE(a.fb_profile_url,'')
		 FROM accounts a LEFT JOIN users u ON u.id = a.assigned_user_id`
	var args []any
	if orgID > 0 {
		q += ` WHERE a.org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY a.created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		var lastUsed string
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Platform, &a.Name, &a.Email, &a.CookiesJSON, &a.ProxyURL,
			&a.UserAgent, &a.Status, &a.Notes, &lastUsed, &a.CreatedAt,
			&a.AssignedUserID, &a.AssignedUserName, &a.BrowserLoggedIn, &a.FBUserID,
			&a.FBDisplayName, &a.FBUsername, &a.FBProfileURL); err != nil {
			return nil, err
		}
		if lastUsed != "" {
			a.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
		}
		a.CookiesJSON, _ = auth.Decrypt(a.CookiesJSON, s.encKey)
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// GetAccountsForUser returns accounts in an org that are assigned to a
// specific user. Used by sales-staff handlers to filter the account list to
// owned-only (execution-layer scoping per the battlefield model — see
// feedback_shared_battlefield_not_crm.md). Admin / platform handlers should
// call GetAllAccounts directly.
func (s *Store) GetAccountsForUser(orgID, userID int64) ([]models.Account, error) {
	if orgID <= 0 || userID <= 0 {
		return nil, nil
	}
	q := `SELECT a.id, COALESCE(a.org_id,0), a.platform, a.name, a.email, a.cookies_json, a.proxy_url, a.user_agent,
		        a.status, a.notes, COALESCE(a.last_used,''), a.created_at,
		        COALESCE(a.assigned_user_id,0), COALESCE(u.name,''), COALESCE(a.browser_logged_in,0), COALESCE(a.fb_user_id,''),
		        COALESCE(a.fb_display_name,''), COALESCE(a.fb_username,''), COALESCE(a.fb_profile_url,'')
		 FROM accounts a LEFT JOIN users u ON u.id = a.assigned_user_id
		 WHERE a.org_id = ? AND a.assigned_user_id = ?
		 ORDER BY a.created_at DESC`
	rows, err := s.db.Query(q, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		var lastUsed string
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Platform, &a.Name, &a.Email, &a.CookiesJSON, &a.ProxyURL,
			&a.UserAgent, &a.Status, &a.Notes, &lastUsed, &a.CreatedAt,
			&a.AssignedUserID, &a.AssignedUserName, &a.BrowserLoggedIn, &a.FBUserID,
			&a.FBDisplayName, &a.FBUsername, &a.FBProfileURL); err != nil {
			return nil, err
		}
		if lastUsed != "" {
			a.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
		}
		a.CookiesJSON, _ = auth.Decrypt(a.CookiesJSON, s.encKey)
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// GetActiveAccounts returns active accounts for a platform with decrypted cookies.
func (s *Store) GetActiveAccounts(platform models.Platform) ([]models.Account, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(org_id,0), platform, name, email, cookies_json, proxy_url, user_agent, status, notes, COALESCE(last_used,''), created_at FROM accounts WHERE platform = ? AND status = 'active' ORDER BY last_used ASC`,
		platform,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		var lastUsed string
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Platform, &a.Name, &a.Email, &a.CookiesJSON, &a.ProxyURL, &a.UserAgent, &a.Status, &a.Notes, &lastUsed, &a.CreatedAt); err != nil {
			return nil, err
		}
		if lastUsed != "" {
			a.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
		}
		a.CookiesJSON, _ = auth.Decrypt(a.CookiesJSON, s.encKey)
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// UpdateAccountStatus updates an account's status.
func (s *Store) UpdateAccountStatus(id int64, status models.AccountStatus) error {
	_, err := s.db.Exec(`UPDATE accounts SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdateAccountLastUsed updates the last used timestamp.
func (s *Store) UpdateAccountLastUsed(id int64) error {
	_, err := s.db.Exec(`UPDATE accounts SET last_used = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// UpdateAccountCookies encrypts and stores new cookies for an account.
func (s *Store) UpdateAccountCookies(id int64, cookiesJSON string) error {
	enc, err := auth.Encrypt(cookiesJSON, s.encKey)
	if err != nil {
		return fmt.Errorf("encrypt cookies: %w", err)
	}
	_, err = s.db.Exec(`UPDATE accounts SET cookies_json = ? WHERE id = ?`, enc, id)
	return err
}

// DeleteAccount removes an account.
func (s *Store) DeleteAccount(id int64) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	return err
}
