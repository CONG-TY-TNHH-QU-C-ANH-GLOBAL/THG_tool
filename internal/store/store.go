package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thg/scraper/internal/auth"
	"github.com/thg/scraper/internal/models"
	_ "modernc.org/sqlite"
)

// Store provides database access for the scraper system.
type Store struct {
	db     *sql.DB
	encKey string // AES-256-GCM key for sensitive fields; empty = no encryption
}

// New creates a new Store, initializing the SQLite database and running migrations.
func New(dbPath string) (*Store, error) {
	// Ensure data directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// SetEncryptionKey sets the AES-256-GCM key used to encrypt sensitive DB fields
// (cookies_json, proxy_url). Must be called before any account operations.
func (s *Store) SetEncryptionKey(key string) {
	s.encKey = key
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		name TEXT NOT NULL,
		url TEXT NOT NULL UNIQUE,
		active INTEGER NOT NULL DEFAULT 1,
		join_state TEXT NOT NULL DEFAULT 'none',
		last_scan DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		group_id INTEGER,
		group_name TEXT,
		url TEXT,
		author TEXT,
		author_url TEXT,
		author_avatar TEXT,
		content TEXT NOT NULL,
		images TEXT DEFAULT '[]',
		reactions INTEGER DEFAULT 0,
		comments INTEGER DEFAULT 0,
		posted_at DATETIME,
		scraped_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		dedup_hash TEXT NOT NULL UNIQUE,
		FOREIGN KEY (group_id) REFERENCES groups(id)
	);

	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER,
		platform TEXT NOT NULL,
		author TEXT,
		author_url TEXT,
		content TEXT NOT NULL,
		posted_at DATETIME,
		scraped_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		dedup_hash TEXT NOT NULL UNIQUE,
		FOREIGN KEY (post_id) REFERENCES posts(id)
	);

	CREATE TABLE IF NOT EXISTS inbox_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		sender TEXT,
		sender_url TEXT,
		content TEXT NOT NULL,
		is_read INTEGER NOT NULL DEFAULT 0,
		received_at DATETIME,
		scraped_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS leads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_type TEXT NOT NULL,
		source_id INTEGER NOT NULL,
		source_url TEXT DEFAULT '',
		platform TEXT NOT NULL,
		author TEXT,
		author_url TEXT,
		content TEXT NOT NULL,
		score TEXT NOT NULL DEFAULT 'cold',
		service_match TEXT DEFAULT 'None',
		author_role TEXT DEFAULT 'unknown',
		pain_point TEXT,
		ai_reasoning TEXT,
		niche TEXT NOT NULL DEFAULT 'logistics',
		classified_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS niches (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		emoji TEXT DEFAULT '🎯',
		active INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		platform TEXT NOT NULL,
		target TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		result TEXT,
		error TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME,
		done_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS scan_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		group_count INTEGER DEFAULT 0,
		post_count INTEGER DEFAULT 0,
		lead_count INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		errors TEXT DEFAULT '[]',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL DEFAULT 'facebook',
		name TEXT NOT NULL,
		email TEXT DEFAULT '',
		cookies_json TEXT DEFAULT '',
		proxy_url TEXT DEFAULT '',
		user_agent TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		notes TEXT DEFAULT '',
		last_used DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS prompt_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL DEFAULT 'telegram',
		user_prompt TEXT NOT NULL,
		ai_response TEXT DEFAULT '',
		action_taken TEXT DEFAULT '',
		action_args TEXT DEFAULT '',
		success INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS ai_memory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		prompt_hash TEXT NOT NULL UNIQUE,
		category TEXT DEFAULT 'other',
		user_prompt TEXT NOT NULL,
		best_action TEXT DEFAULT '',
		best_args TEXT DEFAULT '',
		use_count INTEGER DEFAULT 1,
		success_rate REAL DEFAULT 1.0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_posts_dedup ON posts(dedup_hash);
	CREATE INDEX IF NOT EXISTS idx_posts_platform ON posts(platform, scraped_at);
	CREATE INDEX IF NOT EXISTS idx_comments_dedup ON comments(dedup_hash);
	CREATE INDEX IF NOT EXISTS idx_leads_score ON leads(score);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_groups_active ON groups(active, platform);
	CREATE INDEX IF NOT EXISTS idx_accounts_platform ON accounts(platform, status);
	CREATE INDEX IF NOT EXISTS idx_memory_hash ON ai_memory(prompt_hash);

	CREATE TABLE IF NOT EXISTS outbound_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL DEFAULT 'comment',
		platform TEXT NOT NULL DEFAULT 'facebook',
		account_id INTEGER NOT NULL DEFAULT 0,
		target_url TEXT NOT NULL,
		target_name TEXT DEFAULT '',
		content TEXT NOT NULL,
		context TEXT DEFAULT '',
		image_path TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'draft',
		ai_model TEXT DEFAULT '',
		sent_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_outbound_status ON outbound_messages(status);

	CREATE TABLE IF NOT EXISTS company_images (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		telegram_file_id TEXT NOT NULL,
		local_path TEXT NOT NULL DEFAULT '',
		description TEXT DEFAULT '',
		category TEXT DEFAULT 'general',
		source_url TEXT DEFAULT '',
		use_count INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_company_images_category ON company_images(category);

	CREATE TABLE IF NOT EXISTS price_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_name TEXT NOT NULL,
		price TEXT NOT NULL,
		unit TEXT DEFAULT '',
		notes TEXT DEFAULT '',
		source TEXT DEFAULT 'text',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_context (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS career_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		location TEXT DEFAULT '',
		requirements TEXT DEFAULT '',
		benefits TEXT DEFAULT '',
		email TEXT DEFAULT '',
		url TEXT DEFAULT '',
		is_active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS group_quality (
		group_id INTEGER PRIMARY KEY,
		category TEXT DEFAULT '',
		relevance_score REAL DEFAULT 0,
		professionalism_score REAL DEFAULT 0,
		content_quality_score REAL DEFAULT 0,
		spam_penalty REAL DEFAULT 0,
		final_score REAL DEFAULT 0,
		decision TEXT DEFAULT 'monitor',
		reason TEXT DEFAULT '',
		whitelist INTEGER DEFAULT 0,
		blacklist INTEGER DEFAULT 0,
		scored_at DATETIME,
		last_post_at DATETIME,
		weekly_post_count INTEGER DEFAULT 0,
		candidate_yield INTEGER DEFAULT 0,
		spam_yield INTEGER DEFAULT 0,
		FOREIGN KEY(group_id) REFERENCES groups(id)
	);
	CREATE INDEX IF NOT EXISTS idx_group_quality_score ON group_quality(final_score DESC);
	CREATE INDEX IF NOT EXISTS idx_group_quality_decision ON group_quality(decision);

	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'sales',
		active INTEGER NOT NULL DEFAULT 1,
		failed_logins INTEGER NOT NULL DEFAULT 0,
		locked_until DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

	CREATE TABLE IF NOT EXISTS refresh_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
	CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		action TEXT NOT NULL,
		ip_address TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user_id, created_at DESC);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Multi-tenant: organizations table (each client = one org)
	s.db.Exec(`CREATE TABLE IF NOT EXISTS organizations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		domain TEXT DEFAULT '',
		plan_tier TEXT NOT NULL DEFAULT 'free',
		max_accounts INTEGER NOT NULL DEFAULT 1,
		active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	// Seed the default "platform" org for existing/bootstrap users
	s.db.Exec(`INSERT OR IGNORE INTO organizations (id, name, domain, plan_tier, max_accounts) VALUES (1, 'THG Platform', 'thgfulfill.com', 'enterprise', 0)`)
	// Add org_id to users (existing users → org 0 = superadmin)
	s.db.Exec(`ALTER TABLE users ADD COLUMN org_id INTEGER NOT NULL DEFAULT 0`)
	// Add org_id to accounts (existing accounts → org 1 = default org)
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN org_id INTEGER NOT NULL DEFAULT 1`)
	// Add org_id to groups
	s.db.Exec(`ALTER TABLE groups ADD COLUMN org_id INTEGER NOT NULL DEFAULT 1`)

	// Auto-migrate: career_jobs extended fields
	s.db.Exec(`ALTER TABLE career_jobs ADD COLUMN salary TEXT DEFAULT ''`)
	s.db.Exec(`ALTER TABLE career_jobs ADD COLUMN priority TEXT DEFAULT 'medium'`)
	s.db.Exec(`ALTER TABLE career_jobs ADD COLUMN urgency_score INTEGER DEFAULT 50`)
	// Auto-migrate: add source_url to leads if missing
	s.db.Exec(`ALTER TABLE leads ADD COLUMN source_url TEXT DEFAULT ''`)
	// Auto-migrate: add image_path to outbound_messages if missing
	s.db.Exec(`ALTER TABLE outbound_messages ADD COLUMN image_path TEXT DEFAULT ''`)
	// Auto-migrate: add niche to leads if missing
	s.db.Exec(`ALTER TABLE leads ADD COLUMN niche TEXT DEFAULT 'logistics'`)
	// Auto-migrate: add source_url to company_images if missing
	s.db.Exec(`ALTER TABLE company_images ADD COLUMN source_url TEXT DEFAULT ''`)
	// Auto-migrate: add assigned_user_id to accounts (which staff owns this FB account)
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN assigned_user_id INTEGER DEFAULT 0`)
	// Auto-migrate: execution_mode on jobs — "server" (VPS) or "local" (agent)
	s.db.Exec(`ALTER TABLE jobs ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'server'`)

	// Agent tokens: staff download the agent binary and authenticate with these tokens
	s.db.Exec(`CREATE TABLE IF NOT EXISTS agent_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		created_by INTEGER NOT NULL DEFAULT 0,
		hostname TEXT DEFAULT '',
		os TEXT DEFAULT '',
		version TEXT DEFAULT '',
		last_seen DATETIME,
		active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_tokens_hash ON agent_tokens(token_hash)`)

	// Auto-blacklist: pre-existing groups that are NOT from recruitment searches
	// These are logistics groups that must not be touched by the recruitment pipeline
	s.db.Exec(`INSERT INTO group_quality (group_id, category, decision, blacklist, reason, final_score)
		SELECT id, 'logistics', 'reject', 1, 'Pre-existing logistics group — auto-blacklisted from recruitment', 0.0
		FROM groups WHERE id NOT IN (SELECT group_id FROM group_quality)
		  AND created_at < '2026-04-21T10:00:00Z'
		ON CONFLICT(group_id) DO UPDATE SET blacklist=1, decision='reject'`)
	// Also blacklist ALL groups named "Auto-detected" — these are always logistics groups
	s.db.Exec(`UPDATE group_quality SET blacklist=1, decision='reject', category='logistics',
		reason='Auto-detected logistics group — blacklisted from recruitment'
		WHERE group_id IN (SELECT id FROM groups WHERE name = 'Auto-detected')`)
	s.db.Exec(`INSERT INTO group_quality (group_id, category, decision, blacklist, reason, final_score)
		SELECT id, 'logistics', 'reject', 1, 'Auto-detected logistics group — blacklisted from recruitment', 0.0
		FROM groups WHERE name = 'Auto-detected' AND id NOT IN (SELECT group_id FROM group_quality)
		ON CONFLICT(group_id) DO NOTHING`)
	// Seed default niches
	s.db.Exec(`INSERT OR IGNORE INTO niches (slug, name, emoji) VALUES ('logistics', 'Logistics & Vận chuyển', '🚛')`)
	s.db.Exec(`INSERT OR IGNORE INTO niches (slug, name, emoji) VALUES ('tuyen_dung', 'Tuyển dụng', '👔')`)
	// Backfill: assign leads with missing or unrecognised niche to logistics
	s.db.Exec(`UPDATE leads SET niche = 'logistics' WHERE niche IS NULL OR niche = '' OR niche NOT IN (SELECT slug FROM niches)`)

	// Backfill: match old leads (source_url empty) to posts by content
	s.db.Exec(`UPDATE leads SET source_url = (
		SELECT p.url FROM posts p WHERE p.content = leads.content AND p.url != '' LIMIT 1
	), source_id = (
		SELECT p.id FROM posts p WHERE p.content = leads.content LIMIT 1
	) WHERE (source_url IS NULL OR source_url = '') AND source_id = 0`)

	// Conversation threads: memory across sessions for each lead we're talking to
	s.db.Exec(`CREATE TABLE IF NOT EXISTS conversation_threads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		lead_id INTEGER DEFAULT 0,
		platform TEXT NOT NULL DEFAULT 'facebook',
		profile_url TEXT NOT NULL,
		profile_name TEXT DEFAULT '',
		niche TEXT DEFAULT 'logistics',
		status TEXT NOT NULL DEFAULT 'initiated',
		last_outbound_at DATETIME,
		last_inbound_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_thread_profile ON conversation_threads(profile_url)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_thread_status ON conversation_threads(status)`)

	s.db.Exec(`CREATE TABLE IF NOT EXISTS conversation_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		thread_id INTEGER NOT NULL,
		direction TEXT NOT NULL,
		content TEXT NOT NULL,
		ai_generated INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (thread_id) REFERENCES conversation_threads(id)
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_conv_msg_thread ON conversation_messages(thread_id, created_at)`)

	// Dedup index: prevent inserting same lead for same post twice across scans
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_dedup ON leads(source_type, source_id) WHERE source_id > 0`)

	// Composite indexes for hot-path queries (HasSentComment, HasSentInbox)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_outbound_type_url_status ON outbound_messages(type, target_url, status)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_leads_source_url ON leads(source_url) WHERE source_url != ''`)

	// Self-healing selector cache (LLM Vision updates this when FB changes UI)
	s.initSelectorCache()

	return nil
}

// --- Career Jobs ---

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

// GetCareerJobsByPriority returns active jobs ordered high → medium → low, then urgency_score DESC.
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

// DeactivateAllCareerJobs deletes all career jobs (full wipe before re-scrape).
func (s *Store) DeactivateAllCareerJobs() error {
	_, err := s.db.Exec(`DELETE FROM career_jobs`)
	return err
}

// --- Group Quality ---

// UpsertGroupQuality inserts or replaces the quality record for a group.
func (s *Store) UpsertGroupQuality(q *models.GroupQuality) error {
	_, err := s.db.Exec(`
		INSERT INTO group_quality
			(group_id, category, relevance_score, professionalism_score, content_quality_score,
			 spam_penalty, final_score, decision, reason, whitelist, blacklist, scored_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(group_id) DO UPDATE SET
			category=excluded.category, relevance_score=excluded.relevance_score,
			professionalism_score=excluded.professionalism_score,
			content_quality_score=excluded.content_quality_score,
			spam_penalty=excluded.spam_penalty, final_score=excluded.final_score,
			decision=excluded.decision, reason=excluded.reason,
			whitelist=excluded.whitelist, blacklist=excluded.blacklist,
			scored_at=CURRENT_TIMESTAMP`,
		q.GroupID, q.Category, q.RelevanceScore, q.ProfessionalismScore, q.ContentQualityScore,
		q.SpamPenalty, q.FinalScore, q.Decision, q.Reason, q.Whitelist, q.Blacklist,
	)
	return err
}

// GetGroupQuality returns the quality record for a group, if it exists.
func (s *Store) GetGroupQuality(groupID int64) (*models.GroupQuality, error) {
	var q models.GroupQuality
	var scoredAt, lastPostAt string
	err := s.db.QueryRow(`
		SELECT gq.group_id, g.name, g.url, gq.category, gq.relevance_score,
		       gq.professionalism_score, gq.content_quality_score, gq.spam_penalty,
		       gq.final_score, gq.decision, gq.reason, gq.whitelist, gq.blacklist,
		       COALESCE(gq.scored_at,''), COALESCE(gq.last_post_at,''),
		       gq.weekly_post_count, gq.candidate_yield, gq.spam_yield
		FROM group_quality gq JOIN groups g ON g.id = gq.group_id
		WHERE gq.group_id = ?`, groupID,
	).Scan(&q.GroupID, &q.GroupName, &q.GroupURL, &q.Category,
		&q.RelevanceScore, &q.ProfessionalismScore, &q.ContentQualityScore, &q.SpamPenalty,
		&q.FinalScore, &q.Decision, &q.Reason, &q.Whitelist, &q.Blacklist,
		&scoredAt, &lastPostAt, &q.WeeklyPostCount, &q.CandidateYield, &q.SpamYield)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// GetQualityGroupsForDomain returns usable groups for a job domain category.
// Only returns groups with decision='use', final_score≥0.7, not blacklisted.
func (s *Store) GetQualityGroupsForDomain(category string) ([]models.Group, error) {
	rows, err := s.db.Query(`
		SELECT g.id, g.platform, g.name, g.url, g.active, g.join_state,
		       COALESCE(g.last_scan,''), g.created_at
		FROM groups g
		JOIN group_quality gq ON gq.group_id = g.id
		WHERE g.active = 1
		  AND gq.blacklist = 0
		  AND gq.decision = 'use'
		  AND gq.category = ?
		ORDER BY gq.final_score DESC, gq.candidate_yield DESC`,
		category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupRows(rows)
}

// GetAllScoredGroups returns all groups with quality scores for reporting, sorted by final_score DESC.
func (s *Store) GetAllScoredGroups() ([]models.GroupQuality, error) {
	rows, err := s.db.Query(`
		SELECT gq.group_id, g.name, g.url, gq.category, gq.relevance_score,
		       gq.professionalism_score, gq.content_quality_score, gq.spam_penalty,
		       gq.final_score, gq.decision, gq.reason, gq.whitelist, gq.blacklist,
		       COALESCE(gq.scored_at,''), COALESCE(gq.last_post_at,''),
		       gq.weekly_post_count, gq.candidate_yield, gq.spam_yield
		FROM group_quality gq JOIN groups g ON g.id = gq.group_id
		ORDER BY gq.final_score DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.GroupQuality
	for rows.Next() {
		var q models.GroupQuality
		var scoredAt, lastPostAt string
		if err := rows.Scan(&q.GroupID, &q.GroupName, &q.GroupURL, &q.Category,
			&q.RelevanceScore, &q.ProfessionalismScore, &q.ContentQualityScore, &q.SpamPenalty,
			&q.FinalScore, &q.Decision, &q.Reason, &q.Whitelist, &q.Blacklist,
			&scoredAt, &lastPostAt, &q.WeeklyPostCount, &q.CandidateYield, &q.SpamYield); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

// MarkGroupWhitelist sets or clears the whitelist flag for a group.
func (s *Store) MarkGroupWhitelist(groupID int64, v bool) error {
	_, err := s.db.Exec(`INSERT INTO group_quality(group_id, whitelist) VALUES(?,?)
		ON CONFLICT(group_id) DO UPDATE SET whitelist=excluded.whitelist`, groupID, v)
	return err
}

// MarkGroupBlacklist sets or clears the blacklist flag and resets decision to 'reject'.
func (s *Store) MarkGroupBlacklist(groupID int64, v bool) error {
	decision := "monitor"
	if v {
		decision = "reject"
	}
	_, err := s.db.Exec(`INSERT INTO group_quality(group_id, blacklist, decision) VALUES(?,?,?)
		ON CONFLICT(group_id) DO UPDATE SET blacklist=excluded.blacklist, decision=excluded.decision`,
		groupID, v, decision)
	return err
}

// UpdateGroupYield increments candidate/spam yield counters and updates decision based on accumulated data.
func (s *Store) UpdateGroupYield(groupID int64, qualityDelta, spamDelta int) error {
	_, err := s.db.Exec(`
		INSERT INTO group_quality(group_id, candidate_yield, spam_yield) VALUES(?,?,?)
		ON CONFLICT(group_id) DO UPDATE SET
			candidate_yield = candidate_yield + excluded.candidate_yield,
			spam_yield = spam_yield + excluded.spam_yield`,
		groupID, qualityDelta, spamDelta)
	if err != nil {
		return err
	}
	// Recalculate decision based on yield ratio
	_, err = s.db.Exec(`
		UPDATE group_quality SET decision = CASE
			WHEN blacklist = 1 THEN 'reject'
			WHEN candidate_yield >= 3 AND (spam_yield * 1.0 / MAX(candidate_yield+spam_yield,1)) < 0.3 THEN 'use'
			WHEN spam_yield >= 5 AND (spam_yield * 1.0 / MAX(candidate_yield+spam_yield,1)) > 0.7 THEN 'reject'
			ELSE decision
		END WHERE group_id = ?`, groupID)
	return err
}

// UpdateGroupLastPost records that we posted to this group and increments weekly counter.
func (s *Store) UpdateGroupLastPost(groupID int64) error {
	_, err := s.db.Exec(`
		INSERT INTO group_quality(group_id, last_post_at, weekly_post_count) VALUES(?, CURRENT_TIMESTAMP, 1)
		ON CONFLICT(group_id) DO UPDATE SET
			last_post_at=CURRENT_TIMESTAMP,
			weekly_post_count=weekly_post_count+1`, groupID)
	return err
}

// GetUnscoredGroups returns active groups that have no quality record yet.
func (s *Store) GetUnscoredGroups() ([]models.Group, error) {
	rows, err := s.db.Query(`
		SELECT g.id, g.platform, g.name, g.url, g.active, g.join_state,
		       COALESCE(g.last_scan,''), g.created_at
		FROM groups g
		LEFT JOIN group_quality gq ON gq.group_id = g.id
		WHERE g.active = 1 AND gq.group_id IS NULL
		ORDER BY g.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupRows(rows)
}

func scanGroupRows(rows *sql.Rows) ([]models.Group, error) {
	var groups []models.Group
	for rows.Next() {
		var g models.Group
		var lastScan string
		if err := rows.Scan(&g.ID, &g.Platform, &g.Name, &g.URL, &g.Active, &g.JoinState, &lastScan, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// --- Groups ---

// AddGroup inserts a new group to monitor.
func (s *Store) AddGroup(g *models.Group) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO groups (org_id, platform, name, url, active, join_state) VALUES (?, ?, ?, ?, ?, ?)`,
		g.OrgID, g.Platform, g.Name, g.URL, g.Active, g.JoinState,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GroupExistsByURL checks if a group with the given URL already exists.
func (s *Store) GroupExistsByURL(url string) bool {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM groups WHERE url = ? LIMIT 1`, url).Scan(&id)
	return err == nil && id > 0
}

// GetActiveGroups returns all active groups for a platform.
func (s *Store) GetActiveGroups(platform models.Platform) ([]models.Group, error) {
	rows, err := s.db.Query(
		`SELECT id, platform, name, url, active, join_state, COALESCE(last_scan, ''), created_at FROM groups WHERE active = 1 AND platform = ? ORDER BY last_scan ASC`,
		platform,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Group
	for rows.Next() {
		var g models.Group
		var lastScan string
		if err := rows.Scan(&g.ID, &g.Platform, &g.Name, &g.URL, &g.Active, &g.JoinState, &lastScan, &g.CreatedAt); err != nil {
			return nil, err
		}
		if lastScan != "" {
			g.LastScan, _ = time.Parse(time.RFC3339, lastScan)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GetAllGroups returns groups scoped to an org. orgID=0 returns all (superadmin).
func (s *Store) GetAllGroups(orgID int64) ([]models.Group, error) {
	q := `SELECT id, COALESCE(org_id,1), platform, name, url, active, join_state, COALESCE(last_scan, ''), created_at FROM groups`
	var args []any
	if orgID > 0 {
		q += ` WHERE org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Group
	for rows.Next() {
		var g models.Group
		var lastScan string
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Platform, &g.Name, &g.URL, &g.Active, &g.JoinState, &lastScan, &g.CreatedAt); err != nil {
			return nil, err
		}
		if lastScan != "" {
			g.LastScan, _ = time.Parse(time.RFC3339, lastScan)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// UpdateGroupLastScan updates the last scan timestamp for a group.
func (s *Store) UpdateGroupLastScan(groupID int64) error {
	_, err := s.db.Exec(`UPDATE groups SET last_scan = CURRENT_TIMESTAMP WHERE id = ?`, groupID)
	return err
}

// ToggleGroup activates or deactivates a group.
func (s *Store) ToggleGroup(groupID int64, active bool) error {
	_, err := s.db.Exec(`UPDATE groups SET active = ? WHERE id = ?`, active, groupID)
	return err
}

// DeleteGroup removes a group.
func (s *Store) DeleteGroup(groupID int64) error {
	_, err := s.db.Exec(`DELETE FROM groups WHERE id = ?`, groupID)
	return err
}

// --- Posts ---

// InsertPost inserts a post if it doesn't already exist (by dedup_hash).
func (s *Store) InsertPost(p *models.Post) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO posts (platform, group_id, group_name, url, author, author_url, author_avatar, content, images, reactions, comments, posted_at, dedup_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Platform, p.GroupID, p.GroupName, p.URL, p.Author, p.AuthorURL, p.AuthorAvatar,
		p.Content, p.Images, p.Reactions, p.Comments, p.PostedAt, p.DedupHash,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetRecentPosts returns recent posts with pagination. orgID=0 returns all.
func (s *Store) GetRecentPosts(limit, offset int, orgID int64) ([]models.Post, error) {
	q := `SELECT p.id, p.platform, p.group_id, p.group_name, p.url, p.author, p.author_url, p.author_avatar, p.content, p.images, p.reactions, p.comments, p.posted_at, p.scraped_at, p.dedup_hash
		 FROM posts p`
	var args []any
	if orgID > 0 {
		q += ` JOIN groups g ON p.group_id = g.id WHERE g.org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY p.scraped_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.Platform, &p.GroupID, &p.GroupName, &p.URL, &p.Author, &p.AuthorURL, &p.AuthorAvatar, &p.Content, &p.Images, &p.Reactions, &p.Comments, &p.PostedAt, &p.ScrapedAt, &p.DedupHash); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// DeletePost removes a post by ID.
func (s *Store) DeletePost(postID int64) error {
	_, err := s.db.Exec(`DELETE FROM posts WHERE id = ?`, postID)
	return err
}

// DeleteAllPosts removes all posts (keeps groups).
func (s *Store) DeleteAllPosts() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM posts`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Comments ---

// InsertComment inserts a comment if it doesn't already exist.
func (s *Store) InsertComment(c *models.Comment) (int64, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO comments (post_id, platform, author, author_url, content, posted_at, dedup_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.PostID, c.Platform, c.Author, c.AuthorURL, c.Content, c.PostedAt, c.DedupHash,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// --- Leads ---

// InsertLead inserts a classified lead. Returns 0 if the lead already exists (dedup via unique index).
func (s *Store) InsertLead(l *models.Lead) (int64, error) {
	if l.Niche == "" {
		l.Niche = "logistics"
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO leads (source_type, source_id, source_url, platform, author, author_url, content, score, service_match, author_role, pain_point, ai_reasoning, niche, classified_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.SourceType, l.SourceID, l.SourceURL, l.Platform, l.Author, l.AuthorURL, l.Content,
		l.Score, l.ServiceMatch, l.AuthorRole, l.PainPoint, l.AIReasoning, l.Niche, l.ClassifiedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLeads returns leads with optional filtering by score and niche (cross-org, for internal use).
func (s *Store) GetLeads(score string, limit, offset int) ([]models.Lead, error) {
	return s.GetLeadsFiltered(score, "", limit, offset, 0)
}

// GetLeadsFiltered returns leads filtered by score, niche, and org. orgID=0 returns all.
func (s *Store) GetLeadsFiltered(score, niche string, limit, offset int, orgID int64) ([]models.Lead, error) {
	query := `SELECT l.id, l.source_type, l.source_id,
	           COALESCE(NULLIF(l.source_url, ''), p.url, '') as source_url,
	           l.platform, l.author, l.author_url, l.content, l.score, l.service_match,
	           l.author_role, l.pain_point, l.ai_reasoning, COALESCE(NULLIF(l.niche,''),'logistics'),
	           l.classified_at, l.created_at,
	           EXISTS(SELECT 1 FROM outbound_messages om WHERE om.target_url = COALESCE(NULLIF(l.source_url,''),p.url,'') AND om.type='comment' AND om.status = 'sent') as commented
	          FROM leads l LEFT JOIN posts p ON l.source_id = p.id`
	if orgID > 0 {
		query += ` LEFT JOIN groups g ON p.group_id = g.id`
	}
	var args []any
	var where []string
	if orgID > 0 {
		where = append(where, "(g.org_id = ? OR p.group_id IS NULL)")
		args = append(args, orgID)
	}
	if score != "" {
		where = append(where, "l.score = ?")
		args = append(args, score)
	}
	if niche != "" {
		where = append(where, "COALESCE(NULLIF(l.niche,''),'logistics') = ?")
		args = append(args, niche)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY l.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []models.Lead
	for rows.Next() {
		var l models.Lead
		if err := rows.Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourceURL, &l.Platform,
			&l.Author, &l.AuthorURL, &l.Content, &l.Score, &l.ServiceMatch,
			&l.AuthorRole, &l.PainPoint, &l.AIReasoning, &l.Niche,
			&l.ClassifiedAt, &l.CreatedAt, &l.Commented); err != nil {
			return nil, err
		}
		leads = append(leads, l)
	}
	return leads, nil
}

// DeleteLead removes a lead by ID.
func (s *Store) DeleteLead(leadID int64) error {
	_, err := s.db.Exec(`DELETE FROM leads WHERE id = ?`, leadID)
	return err
}

// DeleteLeads removes leads scoped by niche.
// If niche is empty, all leads are deleted (global wipe).
// If niche is set (e.g. "logistics", "tuyen_dung"), only that domain is deleted.
func (s *Store) DeleteLeads(niche string) (int64, error) {
	var result sql.Result
	var err error
	if niche == "" {
		result, err = s.db.Exec(`DELETE FROM leads`)
	} else {
		result, err = s.db.Exec(`DELETE FROM leads WHERE niche = ?`, niche)
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Jobs ---

// CreateJob creates a new job entry.
func (s *Store) CreateJob(j *models.Job) (int64, error) {
	if j.ExecutionMode == "" {
		j.ExecutionMode = models.ExecutionServer
	}
	res, err := s.db.Exec(
		`INSERT INTO jobs (type, platform, target, status, execution_mode) VALUES (?, ?, ?, ?, ?)`,
		j.Type, j.Platform, j.Target, models.JobPending, j.ExecutionMode,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateJobStatus updates a job's status and optional result/error.
func (s *Store) UpdateJobStatus(jobID int64, status models.JobStatus, result, errMsg string) error {
	switch status {
	case models.JobRunning:
		_, err := s.db.Exec(`UPDATE jobs SET status = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`, status, jobID)
		return err
	case models.JobDone, models.JobFailed:
		_, err := s.db.Exec(`UPDATE jobs SET status = ?, result = ?, error = ?, done_at = CURRENT_TIMESTAMP WHERE id = ?`, status, result, errMsg, jobID)
		return err
	default:
		_, err := s.db.Exec(`UPDATE jobs SET status = ? WHERE id = ?`, status, jobID)
		return err
	}
}

// GetNextLocalJob returns the oldest pending job with execution_mode='local'.
func (s *Store) GetNextLocalJob() (*models.Job, error) {
	row := s.db.QueryRow(`SELECT id, type, platform, target, status, COALESCE(execution_mode,'local'), COALESCE(result,''), COALESCE(error,''), created_at, COALESCE(started_at,created_at), COALESCE(done_at,created_at) FROM jobs WHERE status = 'pending' AND execution_mode = 'local' ORDER BY created_at ASC LIMIT 1`)
	var j models.Job
	err := row.Scan(&j.ID, &j.Type, &j.Platform, &j.Target, &j.Status, &j.ExecutionMode, &j.Result, &j.Error, &j.CreatedAt, &j.StartedAt, &j.DoneAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
}

// GetJobs returns jobs filtered by status.
func (s *Store) GetJobs(status string, limit int) ([]models.Job, error) {
	query := `SELECT id, type, platform, target, status, COALESCE(execution_mode,'server'), COALESCE(result,''), COALESCE(error,''), created_at, COALESCE(started_at, created_at), COALESCE(done_at, created_at) FROM jobs`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var j models.Job
		if err := rows.Scan(&j.ID, &j.Type, &j.Platform, &j.Target, &j.Status, &j.ExecutionMode, &j.Result, &j.Error, &j.CreatedAt, &j.StartedAt, &j.DoneAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// --- Stats ---

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

// --- Scan Logs ---

// InsertScanLog records a scan cycle.
func (s *Store) InsertScanLog(log *models.ScanLog) error {
	_, err := s.db.Exec(
		`INSERT INTO scan_logs (platform, group_count, post_count, lead_count, duration, errors) VALUES (?, ?, ?, ?, ?, ?)`,
		log.Platform, log.GroupCount, log.PostCount, log.LeadCount, log.Duration, log.Errors,
	)
	return err
}

// --- Accounts ---

// AddAccount inserts a new social account, encrypting cookies_json at rest.
func (s *Store) AddAccount(a *models.Account) (int64, error) {
	encCookies, err := auth.Encrypt(a.CookiesJSON, s.encKey)
	if err != nil {
		return 0, fmt.Errorf("encrypt cookies: %w", err)
	}
	res, err := s.db.Exec(
		`INSERT INTO accounts (org_id, platform, name, email, cookies_json, proxy_url, user_agent, status, notes, assigned_user_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.OrgID, a.Platform, a.Name, a.Email, encCookies, a.ProxyURL, a.UserAgent, a.Status, a.Notes, a.AssignedUserID,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetAccount returns an account by ID, decrypting cookies_json.
func (s *Store) GetAccount(id int64) (*models.Account, error) {
	var a models.Account
	var lastUsed string
	err := s.db.QueryRow(
		`SELECT a.id, COALESCE(a.org_id,0), a.platform, a.name, a.email, a.cookies_json, a.proxy_url, a.user_agent,
		        a.status, a.notes, COALESCE(a.last_used,''), a.created_at,
		        COALESCE(a.assigned_user_id,0), COALESCE(u.name,'')
		 FROM accounts a LEFT JOIN users u ON u.id = a.assigned_user_id
		 WHERE a.id = ?`, id,
	).Scan(&a.ID, &a.OrgID, &a.Platform, &a.Name, &a.Email, &a.CookiesJSON, &a.ProxyURL, &a.UserAgent,
		&a.Status, &a.Notes, &lastUsed, &a.CreatedAt, &a.AssignedUserID, &a.AssignedUserName)
	if err != nil {
		return nil, err
	}
	if lastUsed != "" {
		a.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
	}
	a.CookiesJSON, _ = auth.Decrypt(a.CookiesJSON, s.encKey)
	return &a, nil
}

// GetAllAccounts returns accounts scoped to an org. orgID=0 returns all (superadmin).
func (s *Store) GetAllAccounts(orgID int64) ([]models.Account, error) {
	q := `SELECT a.id, COALESCE(a.org_id,0), a.platform, a.name, a.email, a.cookies_json, a.proxy_url, a.user_agent,
		        a.status, a.notes, COALESCE(a.last_used,''), a.created_at,
		        COALESCE(a.assigned_user_id,0), COALESCE(u.name,'')
		 FROM accounts a LEFT JOIN users u ON u.id = a.assigned_user_id`
	var args []any
	if orgID > 0 {
		q += ` WHERE a.org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY a.created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		var lastUsed string
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Platform, &a.Name, &a.Email, &a.CookiesJSON, &a.ProxyURL,
			&a.UserAgent, &a.Status, &a.Notes, &lastUsed, &a.CreatedAt,
			&a.AssignedUserID, &a.AssignedUserName); err != nil {
			return nil, err
		}
		if lastUsed != "" {
			a.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
		}
		a.CookiesJSON, _ = auth.Decrypt(a.CookiesJSON, s.encKey)
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// GetActiveAccounts returns active accounts for a platform with decrypted cookies.
func (s *Store) GetActiveAccounts(platform models.Platform) ([]models.Account, error) {
	rows, err := s.db.Query(
		`SELECT a.id, a.platform, a.name, a.email, a.cookies_json, a.proxy_url, a.user_agent,
		        a.status, a.notes, COALESCE(a.last_used,''), a.created_at,
		        COALESCE(a.assigned_user_id,0), COALESCE(u.name,'')
		 FROM accounts a LEFT JOIN users u ON u.id = a.assigned_user_id
		 WHERE a.status = 'active' AND a.platform = ? ORDER BY a.last_used ASC`,
		platform,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		var lastUsed string
		if err := rows.Scan(&a.ID, &a.Platform, &a.Name, &a.Email, &a.CookiesJSON, &a.ProxyURL,
			&a.UserAgent, &a.Status, &a.Notes, &lastUsed, &a.CreatedAt,
			&a.AssignedUserID, &a.AssignedUserName); err != nil {
			return nil, err
		}
		if lastUsed != "" {
			a.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
		}
		a.CookiesJSON, _ = auth.Decrypt(a.CookiesJSON, s.encKey)
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// UpdateAccountStatus updates an account's status.
func (s *Store) UpdateAccountStatus(id int64, status models.AccountStatus) error {
	_, err := s.db.Exec(`UPDATE accounts SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdateAccountLastUsed updates the last used timestamp.
func (s *Store) UpdateAccountLastUsed(id int64) error {
	_, err := s.db.Exec(`UPDATE accounts SET last_used = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// UpdateAccountCookies encrypts and stores new cookies for an account.
func (s *Store) UpdateAccountCookies(id int64, cookiesJSON string) error {
	enc, err := auth.Encrypt(cookiesJSON, s.encKey)
	if err != nil {
		return fmt.Errorf("encrypt cookies: %w", err)
	}
	_, err = s.db.Exec(`UPDATE accounts SET cookies_json = ? WHERE id = ?`, enc, id)
	return err
}

// DeleteAccount removes an account.
func (s *Store) DeleteAccount(id int64) error {
	_, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// --- Inbox Messages ---

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

// --- Prompt Logs ---

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

// --- AI Memory ---

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

// --- Helpers ---

// DedupHash generates a compound deduplication hash.
func DedupHash(platform, contentType, url, author, date, content string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s", platform, contentType, url, author, date, content)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:16]) // 32-char hex
}

// --- Outbound Messages ---

// InsertOutboundMessage creates a new outbound message in the queue.
func (s *Store) InsertOutboundMessage(msg *models.OutboundMessage) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO outbound_messages (type, platform, account_id, target_url, target_name, content, context, image_path, status, ai_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.Type, msg.Platform, msg.AccountID, msg.TargetURL, msg.TargetName, msg.Content, msg.Context, msg.ImagePath, msg.Status, msg.AIModel,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetOutboundByStatus returns outbound messages filtered by status.
func (s *Store) GetOutboundByStatus(status string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT id, type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		var m models.OutboundMessage
		var sentAt string
		err := rows.Scan(&m.ID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
			&m.Content, &m.Context, &m.ImagePath, &m.Status, &m.AIModel, &sentAt, &m.CreatedAt)
		if err != nil {
			continue
		}
		if sentAt != "" {
			m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// GetSentGroupPosts returns group_post messages that were successfully sent (within last N days).
func (s *Store) GetSentGroupPosts(withinDays int) ([]models.OutboundMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, type, platform, account_id, target_url, target_name, content, context,
			COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages
		WHERE type = 'group_post' AND status IN ('sent', 'approved')
		  AND created_at >= datetime('now', ?)
		ORDER BY created_at DESC`,
		fmt.Sprintf("-%d days", withinDays),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		var m models.OutboundMessage
		var sentAt string
		err := rows.Scan(&m.ID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
			&m.Content, &m.Context, &m.ImagePath, &m.Status, &m.AIModel, &sentAt, &m.CreatedAt)
		if err != nil {
			continue
		}
		if sentAt != "" {
			m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// GetOutbound returns a single outbound message by ID.
func (s *Store) GetOutbound(id int64) (*models.OutboundMessage, error) {
	var m models.OutboundMessage
	var sentAt string
	err := s.db.QueryRow(
		`SELECT id, type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at
		FROM outbound_messages WHERE id = ?`, id,
	).Scan(&m.ID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &m.Status, &m.AIModel, &sentAt, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	if sentAt != "" {
		m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
	}
	return &m, nil
}

// UpdateOutboundStatus updates the status of an outbound message.
func (s *Store) UpdateOutboundStatus(id int64, status models.OutboundStatus) error {
	query := `UPDATE outbound_messages SET status = ? WHERE id = ?`
	if status == models.OutboundSent {
		query = `UPDATE outbound_messages SET status = ?, sent_at = CURRENT_TIMESTAMP WHERE id = ?`
	}
	_, err := s.db.Exec(query, status, id)
	return err
}

// UpdateOutboundContent updates the content of a draft message.
func (s *Store) UpdateOutboundContent(id int64, content string) error {
	_, err := s.db.Exec(`UPDATE outbound_messages SET content = ? WHERE id = ? AND status = 'draft'`, content, id)
	return err
}

// DeleteOutbound deletes an outbound message.
func (s *Store) DeleteOutbound(id int64) error {
	_, err := s.db.Exec(`DELETE FROM outbound_messages WHERE id = ?`, id)
	return err
}

// CountOutboundByStatus returns counts for each status.
func (s *Store) CountOutboundByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM outbound_messages GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err == nil {
			counts[status] = count
		}
	}
	return counts, nil
}

// --- User Context (dynamic business rules) ---

// SetContext stores a key-value configuration.
func (s *Store) SetContext(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_context (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

// --- Niches ---

// GetNiches returns all niches.
func (s *Store) GetNiches() ([]models.Niche, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, emoji, active, created_at FROM niches ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var niches []models.Niche
	for rows.Next() {
		var n models.Niche
		var active int
		if err := rows.Scan(&n.ID, &n.Slug, &n.Name, &n.Emoji, &active, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.Active = active == 1
		niches = append(niches, n)
	}
	return niches, nil
}

// InsertNiche adds a new niche.
func (s *Store) InsertNiche(n *models.Niche) (int64, error) {
	if n.Emoji == "" {
		n.Emoji = "🎯"
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO niches (slug, name, emoji) VALUES (?, ?, ?)`,
		n.Slug, n.Name, n.Emoji,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteNiche removes a niche by slug.
func (s *Store) DeleteNiche(slug string) error {
	_, err := s.db.Exec(`DELETE FROM niches WHERE slug = ?`, slug)
	return err
}

// HasSentComment returns true if a comment has been successfully sent to the given post URL.
func (s *Store) HasSentComment(postURL string) bool {
	var count int
	s.db.QueryRow(
		`SELECT COUNT(*) FROM outbound_messages WHERE type = 'comment' AND target_url = ? AND status = 'sent'`,
		postURL,
	).Scan(&count)
	return count > 0
}

// HasContactedCandidate returns true if we already sent a comment_reply or inbox
// to this candidate (identified by their profile URL) in a previous run.
// This is the cross-run dedup check for the recruitment pipeline.
func (s *Store) HasContactedCandidate(authorURL string) bool {
	var count int
	s.db.QueryRow(
		`SELECT COUNT(*) FROM outbound_messages
		 WHERE type IN ('comment_reply', 'inbox')
		   AND (target_url = ? OR context LIKE '%author_url=' || ? || '%')
		   AND status NOT IN ('failed', 'rejected')`,
		authorURL, authorURL,
	).Scan(&count)
	return count > 0
}

// DeleteAllOutboundComments deletes all outbound comment messages (to allow re-commenting).
func (s *Store) DeleteAllOutboundComments() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM outbound_messages WHERE type = 'comment'`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// --- Company Images ---

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

// GetImageForService trả về ảnh phù hợp với serviceMatch hoặc các keyword bổ sung.
// CHỈ trả về ảnh thực sự match — KHÔNG fallback random để tránh gửi ảnh không liên quan.
// extraKeywords chứa các từ khóa trích xuất từ bài buyer (ví dụ: "mũ", "cup", "jersey")
func (s *Store) GetImageForService(serviceMatch string, extraKeywords ...string) (*models.CompanyImage, error) {
	var img models.CompanyImage

	// 1. Tìm theo serviceMatch trong description/category
	kw := "%" + strings.ToLower(serviceMatch) + "%"
	err := s.db.QueryRow(
		`SELECT id, telegram_file_id, local_path, description, category, COALESCE(source_url,''), use_count, created_at
		 FROM company_images WHERE LOWER(description) LIKE ? OR LOWER(category) LIKE ?
		 ORDER BY use_count ASC, RANDOM() LIMIT 1`, kw, kw,
	).Scan(&img.ID, &img.TelegramFileID, &img.LocalPath, &img.Description, &img.Category, &img.SourceURL, &img.UseCount, &img.CreatedAt)
	if err == nil {
		return &img, nil
	}

	// 2. Tìm theo từng extra keyword (từ nội dung buyer post)
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

	// 3. KHÔNG fallback random — trả về error để caller dùng catalog link
	return nil, fmt.Errorf("no matching image for service: %s", serviceMatch)
}

// GetImageForCareerJob returns a career_job image whose description best matches the given job title.
// Falls back to any career_job image if no title match found.
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
	// Fallback: any career_job image
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

// HasSentInbox kiểm tra đã gửi inbox tới authorURL chưa.
func (s *Store) HasSentInbox(authorURL string) bool {
	var count int
	s.db.QueryRow(
		`SELECT COUNT(*) FROM outbound_messages WHERE type='inbox' AND target_url=? AND status='sent'`,
		authorURL,
	).Scan(&count)
	return count > 0
}

// GetContext retrieves a context value by key.
func (s *Store) GetContext(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM user_context WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// GetAllContext returns all stored context.
func (s *Store) GetAllContext() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM user_context ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ctx := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			ctx[k] = v
		}
	}
	return ctx, nil
}

// --- Price Items ---

// InsertPriceItems saves multiple price items, optionally clearing old ones first.
func (s *Store) InsertPriceItems(items []models.PriceItem, source string) (int, error) {
	saved := 0
	for _, item := range items {
		if item.ServiceName == "" || item.Price == "" {
			continue
		}
		_, err := s.db.Exec(
			`INSERT INTO price_items (service_name, price, unit, notes, source) VALUES (?, ?, ?, ?, ?)`,
			item.ServiceName, item.Price, item.Unit, item.Notes, source,
		)
		if err == nil {
			saved++
		}
	}
	return saved, nil
}

// GetAllPriceItems returns all stored price items.
func (s *Store) GetAllPriceItems() ([]models.PriceItem, error) {
	rows, err := s.db.Query(
		`SELECT id, service_name, price, unit, notes, source, created_at FROM price_items ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []models.PriceItem
	for rows.Next() {
		var p models.PriceItem
		if err := rows.Scan(&p.ID, &p.ServiceName, &p.Price, &p.Unit, &p.Notes, &p.Source, &p.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, nil
}

// ClearPriceItems deletes all stored price items.
func (s *Store) ClearPriceItems() error {
	_, err := s.db.Exec(`DELETE FROM price_items`)
	return err
}

// --- Conversation Threads ---

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
		// Already exists — return existing ID
		s.db.QueryRow(`SELECT id FROM conversation_threads WHERE profile_url = ?`, profileURL).Scan(&id)
	}
	return id, nil
}

// GetThreadByProfile returns the thread for a profile URL, or nil if none.
func (s *Store) GetThreadByProfile(profileURL string) (*models.ConversationThread, error) {
	var t models.ConversationThread
	var lastOut, lastIn string
	err := s.db.QueryRow(
		`SELECT id, lead_id, platform, profile_url, profile_name, niche, status,
		 COALESCE(last_outbound_at,''), COALESCE(last_inbound_at,''), created_at
		 FROM conversation_threads WHERE profile_url = ?`, profileURL,
	).Scan(&t.ID, &t.LeadID, &t.Platform, &t.ProfileURL, &t.ProfileName, &t.Niche,
		&t.Status, &lastOut, &lastIn, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	if lastOut != "" {
		t.LastOutboundAt, _ = time.Parse("2006-01-02 15:04:05", lastOut)
	}
	if lastIn != "" {
		t.LastInboundAt, _ = time.Parse("2006-01-02 15:04:05", lastIn)
	}
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
		if lastOut != "" {
			t.LastOutboundAt, _ = time.Parse("2006-01-02 15:04:05", lastOut)
		}
		if lastIn != "" {
			t.LastInboundAt, _ = time.Parse("2006-01-02 15:04:05", lastIn)
		}
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
	// Update thread timestamps and status
	if direction == "outbound" {
		_, err = s.db.Exec(`UPDATE conversation_threads SET last_outbound_at = CURRENT_TIMESTAMP WHERE id = ?`, threadID)
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

// GetPriceListText returns a formatted price list string for injection into AI prompts.
func (s *Store) GetPriceListText() string {
	items, err := s.GetAllPriceItems()
	if err != nil || len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("BẢNG GIÁ DỊCH VỤ:\n")
	for _, item := range items {
		line := fmt.Sprintf("- %s: %s", item.ServiceName, item.Price)
		if item.Unit != "" {
			line += "/" + item.Unit
		}
		if item.Notes != "" {
			line += " (" + item.Notes + ")"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
