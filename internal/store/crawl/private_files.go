package crawl

import "time"

type PrivateFile struct {
	ID        int64
	OrgID     int64
	Name      string
	Path      string
	SizeBytes int64
	MimeType  string
	CreatedAt time.Time
}

func (s *Store) InsertPrivateFile(f *PrivateFile) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO private_files(org_id, name, path, size_bytes, mime_type) VALUES (?,?,?,?,?)`,
		f.OrgID, f.Name, f.Path, f.SizeBytes, f.MimeType)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetPrivateFiles(orgID int64) ([]PrivateFile, error) {
	rows, err := s.db.Query(`SELECT id, org_id, name, path, size_bytes, mime_type, created_at FROM private_files WHERE org_id = ? ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PrivateFile
	for rows.Next() {
		var f PrivateFile
		if err := rows.Scan(&f.ID, &f.OrgID, &f.Name, &f.Path, &f.SizeBytes, &f.MimeType, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (s *Store) DeletePrivateFile(id, orgID int64) error {
	_, err := s.db.Exec(`DELETE FROM private_files WHERE id = ? AND org_id = ?`, id, orgID)
	return err
}
