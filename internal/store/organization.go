package store

import (
	"database/sql"
	"errors"

	"github.com/thg/scraper/internal/models"
)

// CreateOrganization inserts a new organization and returns its ID.
func (s *Store) CreateOrganization(org *models.Organization) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO organizations (name, domain, plan_tier, max_accounts, active)
		VALUES (?, ?, ?, ?, 1)`,
		org.Name, org.Domain, string(org.PlanTier), org.MaxAccounts)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetOrganization returns an organization by ID.
func (s *Store) GetOrganization(id int64) (*models.Organization, error) {
	row := s.db.QueryRow(`
		SELECT id, name, domain, plan_tier, max_accounts, active, created_at
		FROM organizations WHERE id = ?`, id)
	return scanOrg(row)
}

// GetOrganizationByDomain returns an organization by domain (for registration).
func (s *Store) GetOrganizationByDomain(domain string) (*models.Organization, error) {
	row := s.db.QueryRow(`
		SELECT id, name, domain, plan_tier, max_accounts, active, created_at
		FROM organizations WHERE domain = ?`, domain)
	org, err := scanOrg(row)
	if err != nil || org == nil {
		return nil, err
	}
	return org, nil
}

// ListOrganizations returns all organizations (superadmin only).
func (s *Store) ListOrganizations() ([]models.Organization, error) {
	rows, err := s.db.Query(`
		SELECT id, name, domain, plan_tier, max_accounts, active, created_at
		FROM organizations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orgs []models.Organization
	for rows.Next() {
		org, err := scanOrg(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, *org)
	}
	return orgs, rows.Err()
}

// UpdateOrganization updates org name, plan_tier and max_accounts.
func (s *Store) UpdateOrganization(id int64, name, domain string, plan models.PlanTier, maxAccounts int, active bool) error {
	_, err := s.db.Exec(`
		UPDATE organizations SET name=?, domain=?, plan_tier=?, max_accounts=?, active=?
		WHERE id = ?`,
		name, domain, string(plan), maxAccounts, active, id)
	return err
}

// CountAccountsByOrg returns how many FB accounts an org has.
func (s *Store) CountAccountsByOrg(orgID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE org_id = ?`, orgID).Scan(&n)
	return n, err
}

func scanOrg(row scanner) (*models.Organization, error) {
	var o models.Organization
	var planTier string
	err := row.Scan(&o.ID, &o.Name, &o.Domain, &planTier, &o.MaxAccounts, &o.Active, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.PlanTier = models.PlanTier(planTier)
	return &o, nil
}
