package store

import (
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/models"
)

// CreateThread creates a new conversation thread for a lead we're outreaching to.
func (s *Store) CreateThread(leadID int64, platform, profileURL, profileName, niche string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO conversation_threads (lead_id, platform, profile_url, profile_name, niche, status, last_outbound_at)
		 VALUES (?, ?, ?, ?, ?, 'initiated', CURRENT_TIMESTAMP)`,
		leadID, platform, profileURL, profileName, niche,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		// Already exists - return existing ID.
		s.db.QueryRow(`SELECT id FROM conversation_threads WHERE profile_url = ?`, profileURL).Scan(&id)
	}
	return id, nil
}

// CreateThreadForOrg creates or returns a conversation thread scoped to one org.
func (s *Store) CreateThreadForOrg(orgID, leadID int64, platform, profileURL, profileName, niche string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO conversation_threads (org_id, lead_id, platform, profile_url, profile_name, niche, status, last_outbound_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'initiated', CURRENT_TIMESTAMP)`,
		orgID, leadID, platform, profileURL, profileName, niche,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		s.db.QueryRow(`SELECT id FROM conversation_threads WHERE org_id = ? AND profile_url = ?`, orgID, profileURL).Scan(&id)
	}
	return id, nil
}

// GetThreadByProfile returns the thread for a profile URL, or nil if none.
func (s *Store) GetThreadByProfile(profileURL string) (*models.ConversationThread, error) {
	var t models.ConversationThread
	var lastOut, lastIn string
	err := s.db.QueryRow(
		`SELECT id, COALESCE(org_id,0), lead_id, platform, profile_url, profile_name, niche, status,
		 COALESCE(last_outbound_at,''), COALESCE(last_inbound_at,''), created_at
		 FROM conversation_threads WHERE profile_url = ?`, profileURL,
	).Scan(&t.ID, &t.OrgID, &t.LeadID, &t.Platform, &t.ProfileURL, &t.ProfileName, &t.Niche,
		&t.Status, &lastOut, &lastIn, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.LastOutboundAt = parseSQLiteTime(lastOut)
	t.LastInboundAt = parseSQLiteTime(lastIn)
	return &t, nil
}

// GetThreadByProfileForOrg returns the thread for a profile URL within one org.
func (s *Store) GetThreadByProfileForOrg(orgID int64, profileURL string) (*models.ConversationThread, error) {
	var t models.ConversationThread
	var lastOut, lastIn string
	err := s.db.QueryRow(
		`SELECT id, COALESCE(org_id,0), lead_id, platform, profile_url, profile_name, niche, status,
		 COALESCE(last_outbound_at,''), COALESCE(last_inbound_at,''), created_at
		 FROM conversation_threads WHERE org_id = ? AND profile_url = ?`, orgID, profileURL,
	).Scan(&t.ID, &t.OrgID, &t.LeadID, &t.Platform, &t.ProfileURL, &t.ProfileName, &t.Niche,
		&t.Status, &lastOut, &lastIn, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.LastOutboundAt = parseSQLiteTime(lastOut)
	t.LastInboundAt = parseSQLiteTime(lastIn)
	return &t, nil
}

// GetActiveThreads returns threads awaiting reply (we sent, they haven't replied yet).
func (s *Store) GetActiveThreads(limit int) ([]models.ConversationThread, error) {
	rows, err := s.db.Query(
		`SELECT id, lead_id, platform, profile_url, profile_name, niche, status,
		 COALESCE(last_outbound_at,''), COALESCE(last_inbound_at,''), created_at
		 FROM conversation_threads WHERE status IN ('initiated','follow_up_sent')
		 ORDER BY last_outbound_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreads(rows)
}

// GetThreadsWithNewReplies returns threads where last_inbound_at > last_outbound_at (unanswered reply).
func (s *Store) GetThreadsWithNewReplies(limit int) ([]models.ConversationThread, error) {
	rows, err := s.db.Query(
		`SELECT id, lead_id, platform, profile_url, profile_name, niche, status,
		 COALESCE(last_outbound_at,''), COALESCE(last_inbound_at,''), created_at
		 FROM conversation_threads
		 WHERE status = 'replied' AND last_inbound_at > COALESCE(last_outbound_at, created_at)
		 ORDER BY last_inbound_at ASC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanThreads(rows)
}

func scanThreads(rows *sql.Rows) ([]models.ConversationThread, error) {
	var threads []models.ConversationThread
	for rows.Next() {
		var t models.ConversationThread
		var lastOut, lastIn string
		if err := rows.Scan(&t.ID, &t.LeadID, &t.Platform, &t.ProfileURL, &t.ProfileName, &t.Niche,
			&t.Status, &lastOut, &lastIn, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.LastOutboundAt = parseSQLiteTime(lastOut)
		t.LastInboundAt = parseSQLiteTime(lastIn)
		threads = append(threads, t)
	}
	return threads, nil
}

// AddThreadMessage records a sent or received message in the thread.
func (s *Store) AddThreadMessage(threadID int64, direction, content string, aiGenerated bool) error {
	_, err := s.db.Exec(
		`INSERT INTO conversation_messages (thread_id, direction, content, ai_generated) VALUES (?, ?, ?, ?)`,
		threadID, direction, content, aiGenerated,
	)
	if err != nil {
		return err
	}
	if direction == "outbound" {
		_, err = s.db.Exec(`UPDATE conversation_threads
			SET last_outbound_at = CURRENT_TIMESTAMP,
			    status = CASE WHEN status = 'replied' THEN 'follow_up_sent' ELSE status END
			WHERE id = ?`, threadID)
	} else {
		_, err = s.db.Exec(
			`UPDATE conversation_threads SET last_inbound_at = CURRENT_TIMESTAMP, status = 'replied' WHERE id = ?`, threadID,
		)
	}
	return err
}

// GetThreadMessages returns the full conversation history ordered oldest-first.
func (s *Store) GetThreadMessages(threadID int64) ([]models.ConversationMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, thread_id, direction, content, ai_generated, created_at
		 FROM conversation_messages WHERE thread_id = ? ORDER BY created_at ASC`, threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []models.ConversationMessage
	for rows.Next() {
		var m models.ConversationMessage
		var aiGen int
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Direction, &m.Content, &aiGen, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.AIGenerated = aiGen == 1
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// UpdateThreadStatus sets the status of a thread.
func (s *Store) UpdateThreadStatus(threadID int64, status string) error {
	_, err := s.db.Exec(`UPDATE conversation_threads SET status = ? WHERE id = ?`, status, threadID)
	return err
}

// ThreadExistsForProfile returns true if we've already initiated a conversation with this profile.
func (s *Store) ThreadExistsForProfile(profileURL string) bool {
	var id int64
	s.db.QueryRow(`SELECT id FROM conversation_threads WHERE profile_url = ?`, profileURL).Scan(&id)
	return id > 0
}

type ThreadSummary struct {
	ID          int64
	ProfileName string
	ProfileURL  string
	Status      string
	UnreadCount int
	LastMessage string
	LastAt      time.Time
}

func (s *Store) GetThreadsByOrg(orgID int64, limit int) ([]ThreadSummary, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.profile_name, t.profile_url, t.status, t.unread_count,
		       COALESCE((SELECT content FROM conversation_messages WHERE thread_id=t.id ORDER BY created_at DESC LIMIT 1),''),
		       COALESCE(t.last_inbound_at, t.last_outbound_at, t.created_at)
		FROM conversation_threads t
		WHERE t.org_id = ?
		ORDER BY COALESCE(t.last_inbound_at, t.last_outbound_at, t.created_at) DESC
		LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ThreadSummary
	for rows.Next() {
		var r ThreadSummary
		if err := rows.Scan(&r.ID, &r.ProfileName, &r.ProfileURL, &r.Status, &r.UnreadCount, &r.LastMessage, &r.LastAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Store) CountThreadUnreadByOrg(orgID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(unread_count), 0) FROM conversation_threads WHERE org_id = ?`, orgID).Scan(&count)
	return count, err
}

func (s *Store) ThreadBelongsToOrg(threadID, orgID int64) (bool, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM conversation_threads WHERE id = ? AND org_id = ?`, threadID, orgID).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return id > 0, nil
}

func (s *Store) ClearThreadUnread(threadID int64) error {
	_, err := s.db.Exec(`UPDATE conversation_threads SET unread_count = 0 WHERE id = ?`, threadID)
	return err
}

func (s *Store) IncrementThreadUnread(threadID int64) error {
	_, err := s.db.Exec(`UPDATE conversation_threads SET unread_count = unread_count + 1 WHERE id = ?`, threadID)
	return err
}
