// Domain: users (see internal/store/DOMAINS.md)
package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// hashToken returns a hex SHA-256 of the token for safe storage (so raw tokens never hit the DB).
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}

// GetUserByEmail returns the user with the given email, or nil if not found.
func (s *Store) GetUserByEmail(email string) (*models.User, error) {
	email = normalizeEmail(email)
	row := s.db.QueryRow(`
		SELECT id, COALESCE(org_id,0), email, name, password_hash, role, active,
		       failed_logins, locked_until, created_at, updated_at
		FROM users WHERE lower(trim(email)) = ? AND active = 1`, email)
	return scanUser(row)
}

// GetUserByID returns the user with the given ID.
func (s *Store) GetUserByID(id int64) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT id, COALESCE(org_id,0), email, name, password_hash, role, active,
		       failed_logins, locked_until, created_at, updated_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// CreateUser inserts a new user and returns their assigned ID.
func (s *Store) CreateUser(u *models.User) (int64, error) {
	u.Email = normalizeEmail(u.Email)
	res, err := s.db.Exec(`
		INSERT INTO users (org_id, email, name, password_hash, role, active)
		VALUES (?, ?, ?, ?, ?, 1)`,
		u.OrgID, u.Email, u.Name, u.PasswordHash, u.Role)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ProvisionedOrgClaim identifies an org that was pre-provisioned for an email
// before the dashboard user account existed.
type ProvisionedOrgClaim struct {
	OrgID    int64
	Role     models.UserRole
	Source   string
	InviteID int64
}

// FindProvisionedOrgByEmail resolves a pending workspace/org assignment for an
// email — founder-provisioned only: a Facebook account row with the same email.
//
// PENDING INVITES ARE DELIBERATELY NOT CLAIMS (membership-vulnerability fix):
// an invite must never grant membership at signup/login — joining requires the
// explicit «Đồng ý tham gia» accept (POST /auth/join/:token). The invite
// surfaces through /auth/me/invites + the notification bell instead.
func (s *Store) FindProvisionedOrgByEmail(email string) (*ProvisionedOrgClaim, error) {
	email = normalizeEmail(email)
	if email == "" {
		return nil, nil
	}

	var accountOrgID int64
	var memberCount int
	err := s.db.QueryRow(`
		SELECT a.org_id,
		       (SELECT COUNT(1)
		        FROM users u
		        WHERE u.org_id = a.org_id
		          AND u.active = 1
		          AND u.role NOT IN ('founder', 'superadmin')) AS member_count
		FROM accounts a
		JOIN organizations o ON o.id = a.org_id
		WHERE lower(trim(COALESCE(a.email, ''))) = ?
		  AND a.org_id > 0
		  AND o.active = 1
		ORDER BY a.created_at DESC
		LIMIT 1`, email).Scan(&accountOrgID, &memberCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	role := models.RoleSales
	if memberCount == 0 {
		role = models.RoleAdmin
	}
	return &ProvisionedOrgClaim{OrgID: accountOrgID, Role: role, Source: "account_email"}, nil
}

func (s *Store) MarkInviteUsed(inviteID, acceptedBy int64) error {
	if inviteID <= 0 || acceptedBy <= 0 {
		return nil
	}
	_, err := s.db.Exec(`
		UPDATE org_invites
		SET used_at = COALESCE(used_at, CURRENT_TIMESTAMP),
		    accepted_by = CASE WHEN accepted_by = 0 THEN ? ELSE accepted_by END
		WHERE id = ?`,
		acceptedBy, inviteID)
	return err
}

// ListUsers returns all users for an org (orgID=0 means all users — superadmin only).
func (s *Store) ListUsers(orgID int64) ([]models.User, error) {
	var rows *sql.Rows
	var err error
	if orgID == 0 {
		rows, err = s.db.Query(`
			SELECT id, COALESCE(org_id,0), email, name, password_hash, role, active,
			       failed_logins, locked_until, created_at, updated_at
			FROM users ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.Query(`
			SELECT id, COALESCE(org_id,0), email, name, password_hash, role, active,
			       failed_logins, locked_until, created_at, updated_at
			FROM users WHERE org_id = ? ORDER BY created_at DESC`, orgID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = "" // never return hashes in listings
		users = append(users, *u)
	}
	return users, rows.Err()
}

// SearchUsersForInvite returns up to `limit` users whose email or name matches
// the query prefix. Used by the workspace invite UI's autocomplete so admins
// can invite users that are already registered (whether in another workspace
// or with org_id=0). Platform/founder users are excluded — they manage all
// workspaces and should never be invited as members.
func (s *Store) SearchUsersForInvite(query string, limit int) ([]models.User, error) {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 25 {
		limit = 8
	}
	pattern := q + "%"
	rows, err := s.db.Query(`
		SELECT id, COALESCE(org_id,0), email, name, password_hash, role, active,
		       failed_logins, locked_until, created_at, updated_at
		FROM users
		WHERE active = 1
		  AND role NOT IN ('founder', 'super_admin')
		  AND (lower(email) LIKE ? OR lower(name) LIKE ?)
		ORDER BY email ASC
		LIMIT ?`, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = ""
		users = append(users, *u)
	}
	return users, rows.Err()
}

// EnsureAdminUser creates a superadmin if the users table is empty (bootstrap only).
func (s *Store) EnsureAdminUser(email, passwordHash, name string) error {
	email = normalizeEmail(email)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // users already exist — skip
	}
	_, err := s.db.Exec(`
		INSERT INTO users (org_id, email, name, password_hash, role, active)
		VALUES (0, ?, ?, ?, ?, 1)`, email, name, passwordHash, models.RoleFounder)
	return err
}

// EnsureFounder upserts the platform founder unconditionally (unlike EnsureAdminUser which
// only runs on an empty DB). Safe to call on every startup: if the email already exists, the
// account is promoted to founder with org_id=0 and the password hash is refreshed.
func (s *Store) EnsureFounder(email, passwordHash, name string) error {
	email = normalizeEmail(email)
	_, err := s.db.Exec(`
		INSERT INTO users (org_id, email, name, password_hash, role, active)
		VALUES (0, ?, ?, ?, ?, 1)
		ON CONFLICT(email) DO UPDATE SET
			password_hash = excluded.password_hash,
			role          = excluded.role,
			active        = 1,
			org_id        = 0,
			updated_at    = CURRENT_TIMESTAMP`,
		email, name, passwordHash, models.RoleFounder)
	return err
}

// EnsureSuperAdmin is kept for legacy callers; new code should use EnsureFounder.
func (s *Store) EnsureSuperAdmin(email, passwordHash, name string) error {
	return s.EnsureFounder(email, passwordHash, name)
}

// UpdateUserOrg assigns a user to an org and sets their role (used after onboarding).
func (s *Store) UpdateUserOrg(id, orgID int64, role models.UserRole) error {
	_, err := s.db.Exec(
		`UPDATE users SET org_id = ?, role = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		orgID, string(role), id)
	return err
}

// UpdateUserPassword sets a new bcrypt hash for the user.
func (s *Store) UpdateUserPassword(id int64, newHash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, newHash, id)
	return err
}

// UpdateUser updates name, role and active status for a user.
func (s *Store) UpdateUser(id int64, name string, role models.UserRole, active bool) error {
	_, err := s.db.Exec(`UPDATE users SET name = ?, role = ?, active = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, name, string(role), active, id)
	return err
}

// DeleteUser permanently removes a user and all their sessions.
func (s *Store) DeleteUser(id int64) error {
	s.db.Exec(`DELETE FROM refresh_tokens WHERE user_id = ?`, id)
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// IncrementFailedLogins bumps the failed login counter and optionally applies a lockout.
func (s *Store) IncrementFailedLogins(id int64, newCount int, lockedUntil time.Time) error {
	var lockVal any
	if !lockedUntil.IsZero() {
		lockVal = lockedUntil
	}
	_, err := s.db.Exec(`
		UPDATE users SET failed_logins = ?, locked_until = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, newCount, lockVal, id)
	return err
}

// ResetFailedLogins clears the failure counter and any lockout for a user.
func (s *Store) ResetFailedLogins(id int64) error {
	_, err := s.db.Exec(`
		UPDATE users SET failed_logins = 0, locked_until = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, id)
	return err
}

// SaveRefreshToken stores a hashed refresh token, enforcing max 5 concurrent sessions per user.
func (s *Store) SaveRefreshToken(userID int64, token string, expiresAt time.Time) error {
	// Evict oldest tokens beyond the 5-session cap
	s.db.Exec(`
		DELETE FROM refresh_tokens
		WHERE user_id = ? AND id NOT IN (
			SELECT id FROM refresh_tokens WHERE user_id = ?
			ORDER BY created_at DESC LIMIT 4
		)`, userID, userID)
	_, err := s.db.Exec(`
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES (?, ?, ?)`, userID, hashToken(token), expiresAt)
	return err
}

// ValidateRefreshToken checks if the token is valid and non-expired, returning its user ID.
func (s *Store) ValidateRefreshToken(token string) (int64, error) {
	var userID int64
	var expiresAt time.Time
	err := s.db.QueryRow(`
		SELECT user_id, expires_at FROM refresh_tokens WHERE token_hash = ?`,
		hashToken(token)).Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("invalid refresh token")
	}
	if err != nil {
		return 0, err
	}
	if time.Now().After(expiresAt) {
		s.db.Exec(`DELETE FROM refresh_tokens WHERE token_hash = ?`, hashToken(token))
		return 0, errors.New("refresh token expired")
	}
	return userID, nil
}

// DeleteRefreshToken removes one token (single-device logout).
func (s *Store) DeleteRefreshToken(token string) error {
	_, err := s.db.Exec(`DELETE FROM refresh_tokens WHERE token_hash = ?`, hashToken(token))
	return err
}

// DeleteUserRefreshTokens removes all tokens for a user (logout all devices).
func (s *Store) DeleteUserRefreshTokens(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM refresh_tokens WHERE user_id = ?`, userID)
	return err
}

// InsertAuditLog records a security event (login, delete, role change, etc.).
func (s *Store) InsertAuditLog(userID int64, action, ip, metadata string) error {
	_, err := s.db.Exec(`
		INSERT INTO audit_logs (user_id, action, ip_address, metadata)
		VALUES (?, ?, ?, ?)`, userID, action, ip, metadata)
	return err
}

// GetAuditLogs returns the most recent audit log entries.
func (s *Store) GetAuditLogs(limit int) ([]models.AuditLog, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, action, ip_address, metadata, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.IPAddress, &l.Metadata, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// GetAuditLogsByOrg returns audit events for users in one organization.
func (s *Store) GetAuditLogsByOrg(orgID int64, limit int) ([]models.AuditLog, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.user_id, a.action, a.ip_address, a.metadata, a.created_at
		FROM audit_logs a
		JOIN users u ON u.id = a.user_id
		WHERE u.org_id = ?
		ORDER BY a.created_at DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.IPAddress, &l.Metadata, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// scanner is a common interface for *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*models.User, error) {
	var u models.User
	var lockedUntil sql.NullTime
	err := row.Scan(
		&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash,
		&u.Role, &u.Active, &u.FailedLogins,
		&lockedUntil, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lockedUntil.Valid {
		u.LockedUntil = lockedUntil.Time
	}
	return &u, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
