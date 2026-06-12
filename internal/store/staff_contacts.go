package store

import (
	"database/sql"

	"github.com/thg/scraper/internal/models"
)

// Staff contact profiles (PR-5). Root-store methods, users-adjacent.
// Every read/write is org-scoped — a profile is only reachable through
// its own org.

const staffContactColumns = `user_id, org_id, display_name, role_title, telegram, zalo,
	phone, email, preferred_cta, signature_text, visibility, active`

func scanStaffContact(row interface{ Scan(...any) error }) (*models.StaffContactProfile, error) {
	var p models.StaffContactProfile
	var active int
	if err := row.Scan(&p.UserID, &p.OrgID, &p.DisplayName, &p.RoleTitle, &p.Telegram, &p.Zalo,
		&p.Phone, &p.Email, &p.PreferredCTA, &p.SignatureText, &p.Visibility, &active); err != nil {
		return nil, err
	}
	p.Active = active == 1
	return &p, nil
}

// GetStaffContactProfile returns the profile, or nil when absent.
func (s *Store) GetStaffContactProfile(orgID, userID int64) (*models.StaffContactProfile, error) {
	p, err := scanStaffContact(s.db.QueryRow(
		`SELECT `+staffContactColumns+` FROM staff_contact_profiles WHERE user_id = ? AND org_id = ?`,
		userID, orgID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// UpsertStaffContactProfile writes the full profile (full-replace
// semantics like company identity: an emptied field stays empty so the
// AI stops citing it).
func (s *Store) UpsertStaffContactProfile(p *models.StaffContactProfile) error {
	active := 0
	if p.Active {
		active = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO staff_contact_profiles
			(user_id, org_id, display_name, role_title, telegram, zalo, phone, email,
			 preferred_cta, signature_text, visibility, active, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET
			org_id = excluded.org_id,
			display_name = excluded.display_name,
			role_title = excluded.role_title,
			telegram = excluded.telegram,
			zalo = excluded.zalo,
			phone = excluded.phone,
			email = excluded.email,
			preferred_cta = excluded.preferred_cta,
			signature_text = excluded.signature_text,
			visibility = excluded.visibility,
			active = excluded.active,
			updated_at = CURRENT_TIMESTAMP`,
		p.UserID, p.OrgID, p.DisplayName, p.RoleTitle, p.Telegram, p.Zalo, p.Phone, p.Email,
		p.PreferredCTA, p.SignatureText, p.Visibility, active,
	)
	return err
}

// ListStaffContactProfiles returns every profile in an org (admin view).
func (s *Store) ListStaffContactProfiles(orgID int64) ([]models.StaffContactProfile, error) {
	rows, err := s.db.Query(
		`SELECT `+staffContactColumns+` FROM staff_contact_profiles WHERE org_id = ? ORDER BY display_name, user_id`,
		orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.StaffContactProfile{}
	for rows.Next() {
		p, err := scanStaffContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}
