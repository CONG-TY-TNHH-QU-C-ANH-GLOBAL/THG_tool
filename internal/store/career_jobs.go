package store

import "github.com/thg/scraper/internal/models"

// HR legacy playbook storage. Keep compiled for backward compatibility, but the
// core Agent Brain must use generic business/profile blueprints instead of
// hardcoding recruitment into the primary Facebook planning path.
const careerJobColumns = `id, title, description, location, requirements, benefits,
	COALESCE(salary,''), email, url,
	COALESCE(priority,'medium'), COALESCE(urgency_score,50), is_active, created_at`

func scanCareerJob(row interface{ Scan(...any) error }) (models.CareerJob, error) {
	var j models.CareerJob
	err := row.Scan(&j.ID, &j.Title, &j.Description, &j.Location, &j.Requirements, &j.Benefits,
		&j.Salary, &j.Email, &j.URL, &j.Priority, &j.UrgencyScore, &j.IsActive, &j.CreatedAt)
	return j, err
}

// InsertCareerJob inserts a new career job into the database.
func (s *Store) InsertCareerJob(job *models.CareerJob) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO career_jobs (title, description, location, requirements, benefits, salary, email, url, priority, urgency_score, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.Title, job.Description, job.Location, job.Requirements, job.Benefits,
		job.Salary, job.Email, job.URL, job.Priority, job.UrgencyScore, 1,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetActiveCareerJobs returns all active career jobs ordered by newest first.
func (s *Store) GetActiveCareerJobs() ([]models.CareerJob, error) {
	rows, err := s.db.Query(`SELECT ` + careerJobColumns + ` FROM career_jobs WHERE is_active = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.CareerJob
	for rows.Next() {
		j, err := scanCareerJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// GetCareerJobsByPriority returns active jobs ordered by priority, urgency, and recency.
func (s *Store) GetCareerJobsByPriority() ([]models.CareerJob, error) {
	rows, err := s.db.Query(`SELECT ` + careerJobColumns + `
		FROM career_jobs WHERE is_active = 1
		ORDER BY CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
		         urgency_score DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.CareerJob
	for rows.Next() {
		j, err := scanCareerJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// DeactivateAllCareerJobs deletes all career jobs.
func (s *Store) DeactivateAllCareerJobs() error {
	_, err := s.db.Exec(`DELETE FROM career_jobs`)
	return err
}
