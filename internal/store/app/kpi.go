// Domain: app (see internal/store/DOMAINS.md)
package app

import (
	"database/sql"
	"time"
)

type StaffKPI struct {
	UserID    int64
	OrgID     int64
	Name      string
	Email     string
	Role      string
	Active    bool
	Joined    string
	Convs     int
	Converted int
	Cmts      int
	Pts       int
}

type KPIDelta struct {
	Convs     *int
	Converted *int
	Cmts      *int
}

func (s *Store) GetStaffWithKPI(orgID int64) ([]StaffKPI, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.org_id, u.name, u.email, u.role, u.active, u.created_at,
		       COALESCE(k.convs,0), COALESCE(k.converted,0), COALESCE(k.cmts,0), COALESCE(k.pts,0)
		FROM users u
		LEFT JOIN staff_kpi k ON k.user_id = u.id
		WHERE u.org_id = ?
		ORDER BY COALESCE(k.pts,0) DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StaffKPI
	for rows.Next() {
		var r StaffKPI
		var createdAt time.Time
		if err := rows.Scan(&r.UserID, &r.OrgID, &r.Name, &r.Email, &r.Role, &r.Active, &createdAt, &r.Convs, &r.Converted, &r.Cmts, &r.Pts); err != nil {
			return nil, err
		}
		r.Joined = createdAt.Format("02/01/2006")
		out = append(out, r)
	}
	return out, nil
}

func (s *Store) UpsertStaffKPI(userID, orgID int64, delta KPIDelta) error {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO staff_kpi(user_id, org_id) VALUES (?,?)`, userID, orgID); err != nil {
		return err
	}
	if delta.Convs != nil {
		if _, err := s.db.Exec(`UPDATE staff_kpi SET convs = convs + ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`, *delta.Convs, userID); err != nil {
			return err
		}
	}
	if delta.Converted != nil {
		if _, err := s.db.Exec(`UPDATE staff_kpi SET converted = converted + ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`, *delta.Converted, userID); err != nil {
			return err
		}
	}
	if delta.Cmts != nil {
		if _, err := s.db.Exec(`UPDATE staff_kpi SET cmts = cmts + ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`, *delta.Cmts, userID); err != nil {
			return err
		}
	}
	return nil
}

type KPIConfig struct {
	OrgID    int64
	ConvPts  int
	Conv2Pts int
	CmtPts   int
	BonusPts int
	BonusAmt int
	PenPts   int
	PenAmt   int
}

func (s *Store) GetKPIConfig(orgID int64) (*KPIConfig, error) {
	var c KPIConfig
	err := s.db.QueryRow(`SELECT org_id, conv_pts, conv2_pts, cmt_pts, bonus_pts, bonus_amt, pen_pts, pen_amt FROM kpi_config WHERE org_id = ?`, orgID).
		Scan(&c.OrgID, &c.ConvPts, &c.Conv2Pts, &c.CmtPts, &c.BonusPts, &c.BonusAmt, &c.PenPts, &c.PenAmt)
	if err == sql.ErrNoRows {
		return &KPIConfig{OrgID: orgID, ConvPts: 10, Conv2Pts: 50, CmtPts: 2, BonusPts: 1000, BonusAmt: 500000, PenPts: 300, PenAmt: 100000}, nil
	}
	return &c, err
}

func (s *Store) UpsertKPIConfig(orgID int64, c KPIConfig) error {
	_, err := s.db.Exec(`
		INSERT INTO kpi_config(org_id, conv_pts, conv2_pts, cmt_pts, bonus_pts, bonus_amt, pen_pts, pen_amt, updated_at)
		VALUES (?,?,?,?,?,?,?,?, CURRENT_TIMESTAMP)
		ON CONFLICT(org_id) DO UPDATE SET
		  conv_pts=excluded.conv_pts, conv2_pts=excluded.conv2_pts, cmt_pts=excluded.cmt_pts,
		  bonus_pts=excluded.bonus_pts, bonus_amt=excluded.bonus_amt, pen_pts=excluded.pen_pts,
		  pen_amt=excluded.pen_amt, updated_at=excluded.updated_at`,
		orgID, c.ConvPts, c.Conv2Pts, c.CmtPts, c.BonusPts, c.BonusAmt, c.PenPts, c.PenAmt)
	return err
}
