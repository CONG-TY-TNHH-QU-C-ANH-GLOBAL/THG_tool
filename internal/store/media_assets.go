package store

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// InsertCompanyImage saves a new company image to the database.
func (s *Store) InsertCompanyImage(img *models.CompanyImage) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO company_images (telegram_file_id, local_path, description, category, source_url) VALUES (?, ?, ?, ?, ?)`,
		img.TelegramFileID, img.LocalPath, img.Description, img.Category, img.SourceURL,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetCompanyImages returns saved company images.
func (s *Store) GetCompanyImages(limit int) ([]models.CompanyImage, error) {
	rows, err := s.db.Query(
		`SELECT id, telegram_file_id, local_path, description, category, COALESCE(source_url,''), use_count, created_at FROM company_images ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []models.CompanyImage
	for rows.Next() {
		var img models.CompanyImage
		if err := rows.Scan(&img.ID, &img.TelegramFileID, &img.LocalPath, &img.Description, &img.Category, &img.SourceURL, &img.UseCount, &img.CreatedAt); err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, nil
}

// GetRandomCompanyImage returns a random company image for use in comments.
func (s *Store) GetRandomCompanyImage() (*models.CompanyImage, error) {
	var img models.CompanyImage
	err := s.db.QueryRow(
		`SELECT id, telegram_file_id, local_path, description, category, COALESCE(source_url,''), use_count, created_at FROM company_images ORDER BY RANDOM() LIMIT 1`,
	).Scan(&img.ID, &img.TelegramFileID, &img.LocalPath, &img.Description, &img.Category, &img.SourceURL, &img.UseCount, &img.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &img, nil
}

// GetImageForService returns a real matching company image for a service or extracted keywords.
func (s *Store) GetImageForService(serviceMatch string, extraKeywords ...string) (*models.CompanyImage, error) {
	var img models.CompanyImage

	kw := "%" + strings.ToLower(serviceMatch) + "%"
	err := s.db.QueryRow(
		`SELECT id, telegram_file_id, local_path, description, category, COALESCE(source_url,''), use_count, created_at
		 FROM company_images WHERE LOWER(description) LIKE ? OR LOWER(category) LIKE ?
		 ORDER BY use_count ASC, RANDOM() LIMIT 1`, kw, kw,
	).Scan(&img.ID, &img.TelegramFileID, &img.LocalPath, &img.Description, &img.Category, &img.SourceURL, &img.UseCount, &img.CreatedAt)
	if err == nil {
		return &img, nil
	}

	for _, k := range extraKeywords {
		k = strings.TrimSpace(k)
		if k == "" || len(k) < 2 {
			continue
		}
		kw = "%" + strings.ToLower(k) + "%"
		err = s.db.QueryRow(
			`SELECT id, telegram_file_id, local_path, description, category, COALESCE(source_url,''), use_count, created_at
			 FROM company_images WHERE LOWER(description) LIKE ?
			 ORDER BY use_count ASC, RANDOM() LIMIT 1`, kw,
		).Scan(&img.ID, &img.TelegramFileID, &img.LocalPath, &img.Description, &img.Category, &img.SourceURL, &img.UseCount, &img.CreatedAt)
		if err == nil {
			return &img, nil
		}
	}

	return nil, fmt.Errorf("no matching image for service: %s", serviceMatch)
}

// GetImageForCareerJob returns a career_job image that best matches the job title.
func (s *Store) GetImageForCareerJob(jobTitle string) (*models.CompanyImage, error) {
	var img models.CompanyImage
	kw := "%" + strings.ToLower(jobTitle) + "%"
	err := s.db.QueryRow(
		`SELECT id, telegram_file_id, local_path, description, category, COALESCE(source_url,''), use_count, created_at
		 FROM company_images WHERE category = 'career_job' AND LOWER(description) LIKE ?
		 ORDER BY use_count ASC, RANDOM() LIMIT 1`, kw,
	).Scan(&img.ID, &img.TelegramFileID, &img.LocalPath, &img.Description, &img.Category, &img.SourceURL, &img.UseCount, &img.CreatedAt)
	if err == nil {
		return &img, nil
	}
	err = s.db.QueryRow(
		`SELECT id, telegram_file_id, local_path, description, category, COALESCE(source_url,''), use_count, created_at
		 FROM company_images WHERE category = 'career_job'
		 ORDER BY use_count ASC, RANDOM() LIMIT 1`,
	).Scan(&img.ID, &img.TelegramFileID, &img.LocalPath, &img.Description, &img.Category, &img.SourceURL, &img.UseCount, &img.CreatedAt)
	if err == nil {
		return &img, nil
	}
	return nil, fmt.Errorf("no career job images found")
}

// IncrementImageUseCount increments the use count of an image.
func (s *Store) IncrementImageUseCount(id int64) error {
	_, err := s.db.Exec(`UPDATE company_images SET use_count = use_count + 1 WHERE id = ?`, id)
	return err
}

// CountCompanyImages returns the total number of stored company images.
func (s *Store) CountCompanyImages() int {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM company_images`).Scan(&count)
	return count
}
