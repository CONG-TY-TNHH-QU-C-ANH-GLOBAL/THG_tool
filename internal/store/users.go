package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
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
	row := s.db.QueryRow(`
		SELECT id, email, name, password_hash, role, active,
		       failed_logins, locked_until, created_at, updated_at
		FROM users WHERE email = ? AND active = 1`, email)
	return scanUser(row)
}

// GetUserByID returns the user with the given ID.
func (s *Store) GetUserByID(id int64) (*models.User, error) {
	row := s.db.QueryRow(`
		SELECT id, email, name, password_hash, role, active,
		       failed_logins, locked_until, created_at, updated_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// CreateUser inserts a new user and returns their assigned ID.
func (s *Store) CreateUser(u *models.User) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO users (email, name, password_hash, role, active)
		VALUES (?, ?, ?, ?, 1)`,
		u.Email, u.Name, u.PasswordHash, u.Role)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListUsers returns all users without password hashes.
func (s *Store) ListUsers() ([]models.User, error) {
	rows, err := s.db.Query(`
		SELECT id, email, name, password_hash, role, active,
		       failed_logins, locked_until, created_at, updated_at
		FROM users ORDER BY created_at DESC`)
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

// EnsureAdminUser creates an admin if the users table is empty (bootstrap only).
func (s *Store) EnsureAdminUser(email, passwordHash, name string) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // users already exist — skip
	}
	_, err := s.db.Exec(`
		INSERT INTO users (email, name, password_hash, role, active)
		VALUES (?, ?, ?, 'admin', 1)`, email, name, passwordHash)
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

// scanner is a common interface for *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*models.User, error) {
	var u models.User
	var lockedUntil sql.NullTime
	err := row.Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash,
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
