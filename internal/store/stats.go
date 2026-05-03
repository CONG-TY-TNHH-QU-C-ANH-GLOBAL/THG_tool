package store

import "github.com/thg/scraper/internal/models"

// ResetOrphanedOutbounds sets any 'approved' outbound messages to 'failed' on startup.
func (s *Store) ResetOrphanedOutbounds() error {
	_, err := s.db.Exec(`UPDATE outbound_messages SET status = 'failed' WHERE status = 'approved'`)
	return err
}

// GetStats returns dashboard statistics in a single read transaction for consistency.
func (s *Store) GetStats() (*models.Stats, error) {
	stats := &models.Stats{}

	tx, err := s.db.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback() //nolint:errcheck

	tx.QueryRow(`SELECT COUNT(*) FROM groups`).Scan(&stats.TotalGroups)
	tx.QueryRow(`SELECT COUNT(*) FROM groups WHERE active = 1`).Scan(&stats.ActiveGroups)
	tx.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&stats.TotalPosts)
	tx.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&stats.TotalComments)
	tx.QueryRow(`SELECT COUNT(*) FROM leads`).Scan(&stats.TotalLeads)
	tx.QueryRow(`SELECT COUNT(*) FROM leads WHERE score = 'hot'`).Scan(&stats.HotLeads)
	tx.QueryRow(`SELECT COUNT(*) FROM posts WHERE DATE(scraped_at) = DATE('now')`).Scan(&stats.TodayPosts)
	tx.QueryRow(`SELECT COUNT(*) FROM leads WHERE DATE(created_at) = DATE('now')`).Scan(&stats.TodayLeads)
	tx.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status = 'running'`).Scan(&stats.RunningJobs)
	tx.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&stats.TotalAccounts)
	tx.QueryRow(`SELECT COUNT(*) FROM accounts WHERE status = 'active'`).Scan(&stats.ActiveAccounts)
	tx.QueryRow(`SELECT COUNT(*) FROM prompt_logs`).Scan(&stats.TotalPrompts)

	return stats, tx.Commit()
}
