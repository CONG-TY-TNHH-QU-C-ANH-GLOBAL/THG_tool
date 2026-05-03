package store

import "github.com/thg/scraper/internal/models"

// InsertScanLog records a scan cycle.
func (s *Store) InsertScanLog(log *models.ScanLog) error {
	_, err := s.db.Exec(
		`INSERT INTO scan_logs (platform, group_count, post_count, lead_count, duration, errors) VALUES (?, ?, ?, ?, ?, ?)`,
		log.Platform, log.GroupCount, log.PostCount, log.LeadCount, log.Duration, log.Errors,
	)
	return err
}

// InsertInboxMessage inserts a new inbox message.
func (s *Store) InsertInboxMessage(m *models.InboxMessage) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO inbox_messages (platform, sender, sender_url, content, is_read, received_at) VALUES (?, ?, ?, ?, ?, ?)`,
		m.Platform, m.Sender, m.SenderURL, m.Content, m.IsRead, m.ReceivedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertPromptLog records an AI prompt interaction.
func (s *Store) InsertPromptLog(p *models.PromptLog) error {
	_, err := s.db.Exec(
		`INSERT INTO prompt_logs (source, user_prompt, ai_response, action_taken, action_args, success) VALUES (?, ?, ?, ?, ?, ?)`,
		p.Source, p.UserPrompt, p.AIResponse, p.ActionTaken, p.ActionArgs, p.Success,
	)
	return err
}

// GetPromptHistory returns recent prompt logs.
func (s *Store) GetPromptHistory(limit int) ([]models.PromptLog, error) {
	rows, err := s.db.Query(
		`SELECT id, source, user_prompt, ai_response, action_taken, action_args, success, created_at FROM prompt_logs ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.PromptLog
	for rows.Next() {
		var p models.PromptLog
		if err := rows.Scan(&p.ID, &p.Source, &p.UserPrompt, &p.AIResponse, &p.ActionTaken, &p.ActionArgs, &p.Success, &p.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, p)
	}
	return logs, nil
}

// InsertMemory stores a new learned pattern.
func (s *Store) InsertMemory(m *models.AIMemory) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO ai_memory (prompt_hash, category, user_prompt, best_action, best_args, use_count, success_rate) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.PromptHash, m.Category, m.UserPrompt, m.BestAction, m.BestArgs, m.UseCount, m.SuccessRate,
	)
	return err
}

// GetMemoryByHash returns a memory entry by prompt hash.
func (s *Store) GetMemoryByHash(hash string) (*models.AIMemory, error) {
	var m models.AIMemory
	err := s.db.QueryRow(
		`SELECT id, prompt_hash, category, user_prompt, best_action, best_args, use_count, success_rate, created_at, updated_at FROM ai_memory WHERE prompt_hash = ?`, hash,
	).Scan(&m.ID, &m.PromptHash, &m.Category, &m.UserPrompt, &m.BestAction, &m.BestArgs, &m.UseCount, &m.SuccessRate, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetRelevantMemories returns top memories sorted by success rate and usage.
func (s *Store) GetRelevantMemories(limit int) ([]models.AIMemory, error) {
	rows, err := s.db.Query(
		`SELECT id, prompt_hash, category, user_prompt, best_action, best_args, use_count, success_rate, created_at, updated_at FROM ai_memory ORDER BY use_count DESC, success_rate DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []models.AIMemory
	for rows.Next() {
		var m models.AIMemory
		if err := rows.Scan(&m.ID, &m.PromptHash, &m.Category, &m.UserPrompt, &m.BestAction, &m.BestArgs, &m.UseCount, &m.SuccessRate, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// UpdateMemoryUsage increments usage count and updates success rate.
func (s *Store) UpdateMemoryUsage(id int64, success bool) error {
	if success {
		_, err := s.db.Exec(`UPDATE ai_memory SET use_count = use_count + 1, success_rate = (success_rate * use_count + 1.0) / (use_count + 1), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
		return err
	}
	_, err := s.db.Exec(`UPDATE ai_memory SET use_count = use_count + 1, success_rate = (success_rate * use_count) / (use_count + 1), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}
