package store

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
		org_id INTEGER NOT NULL DEFAULT 0,
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
		org_id INTEGER NOT NULL DEFAULT 0,
		account_id INTEGER NOT NULL DEFAULT 0,
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
		org_id INTEGER NOT NULL DEFAULT 0,
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
	// Rename the legacy platform role; old JWTs with superadmin remain accepted by RBAC.
	s.db.Exec(`UPDATE users SET role = 'founder' WHERE role = 'superadmin' AND COALESCE(org_id,0) = 0`)
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
	s.db.Exec(`ALTER TABLE outbound_messages ADD COLUMN org_id INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE outbound_messages ADD COLUMN claimed_by TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE outbound_messages ADD COLUMN claimed_at DATETIME`)
	s.db.Exec(`UPDATE outbound_messages
		SET org_id = COALESCE((SELECT org_id FROM accounts WHERE accounts.id = outbound_messages.account_id), org_id)
		WHERE COALESCE(org_id,0) = 0 AND account_id > 0`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_outbound_org_status ON outbound_messages(org_id, status, type)`)
	s.db.Exec(`ALTER TABLE prompt_logs ADD COLUMN org_id INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE prompt_logs ADD COLUMN account_id INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_prompt_logs_org_created ON prompt_logs(org_id, created_at DESC)`)
	// Legacy jobs table remains for existing SQLite databases and
	// historical dashboard data. New connector execution uses
	// connector_commands, scheduler_jobs, app_tasks and outbound_messages.
	s.db.Exec(`ALTER TABLE jobs ADD COLUMN claimed_by TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE jobs ADD COLUMN claimed_at DATETIME`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_jobs_local_pending ON jobs(execution_mode, status, created_at)`)
	// Phase 2.1: partial UNIQUE index is the last-line guard against two
	// workers concurrently passing CanQueueOutboundForOrg and double-queuing
	// the same comment / inbox / group_post. Sent / failed / rejected rows
	// are excluded so historical sends don't block legitimate retries.
	s.db.Exec(`DROP INDEX IF EXISTS idx_outbound_active_target`)
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_outbound_active_target
		ON outbound_messages(org_id, type, target_url)
		WHERE status IN ('draft','approved','sending')`)
	s.db.Exec(`ALTER TABLE leads ADD COLUMN org_id INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_leads_org_score ON leads(org_id, score)`)
	// Auto-migrate: add niche to leads if missing
	s.db.Exec(`ALTER TABLE leads ADD COLUMN niche TEXT DEFAULT 'logistics'`)
	// Auto-migrate: add source_url to company_images if missing
	s.db.Exec(`ALTER TABLE company_images ADD COLUMN source_url TEXT DEFAULT ''`)
	// Auto-migrate: add assigned_user_id to accounts (which staff owns this FB account)
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN assigned_user_id INTEGER DEFAULT 0`)
	// Auto-migrate: execution_mode on jobs — "server" (VPS) or "local" (agent)
	s.db.Exec(`ALTER TABLE jobs ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'server'`)
	// Auto-migrate: browser_logged_in tracks whether account has logged into Facebook via dashboard browser
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN browser_logged_in INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN fb_user_id TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN fb_display_name TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN fb_username TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE accounts ADD COLUMN fb_profile_url TEXT NOT NULL DEFAULT ''`)
	// Org invites: token-based invite links for joining an org
	s.db.Exec(`CREATE TABLE IF NOT EXISTS org_invites (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id     INTEGER NOT NULL,
		email      TEXT NOT NULL DEFAULT '',
		role       TEXT NOT NULL DEFAULT 'sales',
		token      TEXT NOT NULL UNIQUE,
		created_by INTEGER NOT NULL,
		expires_at DATETIME NOT NULL,
		used_at    DATETIME,
		accepted_by INTEGER NOT NULL DEFAULT 0,
		email_status TEXT NOT NULL DEFAULT 'pending',
		email_sent_at DATETIME,
		email_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	s.db.Exec(`ALTER TABLE org_invites ADD COLUMN role TEXT NOT NULL DEFAULT 'sales'`)
	s.db.Exec(`ALTER TABLE org_invites ADD COLUMN accepted_by INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE org_invites ADD COLUMN email_status TEXT NOT NULL DEFAULT 'pending'`)
	s.db.Exec(`ALTER TABLE org_invites ADD COLUMN email_sent_at DATETIME`)
	s.db.Exec(`ALTER TABLE org_invites ADD COLUMN email_error TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_org_invites_token ON org_invites(token)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_org_invites_org ON org_invites(org_id)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_org_invites_email ON org_invites(email, used_at, expires_at)`)

	// Connector tokens: Chrome Extension instances authenticate with these tokens.
	s.db.Exec(`CREATE TABLE IF NOT EXISTS agent_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL DEFAULT 0,
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
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN org_id INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN kind TEXT NOT NULL DEFAULT 'worker'`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN transport TEXT NOT NULL DEFAULT 'poll'`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN assigned_account_id INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN capabilities_json TEXT NOT NULL DEFAULT '{}'`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN current_url TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN fb_user_id TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN fb_display_name TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN fb_username TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN fb_profile_url TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN stream_status TEXT NOT NULL DEFAULT 'idle'`)
	s.db.Exec(`ALTER TABLE agent_tokens ADD COLUMN chrome_error TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`UPDATE agent_tokens
		SET org_id = COALESCE((SELECT org_id FROM users WHERE users.id = agent_tokens.created_by), org_id)
		WHERE COALESCE(org_id,0) = 0 AND created_by > 0`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_tokens_hash ON agent_tokens(token_hash)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_tokens_org ON agent_tokens(org_id, active)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_tokens_kind ON agent_tokens(org_id, kind, active)`)

	s.db.Exec(`CREATE TABLE IF NOT EXISTS connector_pairing_codes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL,
		code_hash TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		created_by INTEGER NOT NULL DEFAULT 0,
		assigned_account_id INTEGER NOT NULL DEFAULT 0,
		expires_at DATETIME NOT NULL,
		used_at DATETIME,
		device_token_id INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_pairing_hash ON connector_pairing_codes(code_hash)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_pairing_org ON connector_pairing_codes(org_id, expires_at)`)

	s.db.Exec(`CREATE TABLE IF NOT EXISTS connector_screenshots (
		account_id INTEGER NOT NULL,
		org_id INTEGER NOT NULL,
		agent_id INTEGER NOT NULL DEFAULT 0,
		image_data TEXT NOT NULL,
		current_url TEXT NOT NULL DEFAULT '',
		fb_user_id TEXT NOT NULL DEFAULT '',
		fb_display_name TEXT NOT NULL DEFAULT '',
		fb_username TEXT NOT NULL DEFAULT '',
		fb_profile_url TEXT NOT NULL DEFAULT '',
		stream_status TEXT NOT NULL DEFAULT '',
		chrome_error TEXT NOT NULL DEFAULT '',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (org_id, account_id)
	)`)
	s.db.Exec(`ALTER TABLE connector_screenshots ADD COLUMN fb_display_name TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE connector_screenshots ADD COLUMN fb_username TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE connector_screenshots ADD COLUMN fb_profile_url TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE connector_screenshots ADD COLUMN chrome_error TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_screenshots_org ON connector_screenshots(org_id, updated_at)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_screenshots_agent ON connector_screenshots(agent_id, updated_at)`)

	s.db.Exec(`CREATE TABLE IF NOT EXISTS connector_commands (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL,
		account_id INTEGER NOT NULL,
		agent_id INTEGER NOT NULL DEFAULT 0,
		type TEXT NOT NULL,
		payload_json TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'pending',
		error_msg TEXT NOT NULL DEFAULT '',
		created_by INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		claimed_at DATETIME,
		completed_at DATETIME
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_commands_agent ON connector_commands(agent_id, status, id)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_commands_account ON connector_commands(org_id, account_id, status, id)`)

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

	// AutoFlow: per-user KPI metrics
	s.db.Exec(`CREATE TABLE IF NOT EXISTS staff_kpi (
		user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		org_id     INTEGER NOT NULL DEFAULT 1,
		convs      INTEGER NOT NULL DEFAULT 0,
		converted  INTEGER NOT NULL DEFAULT 0,
		cmts       INTEGER NOT NULL DEFAULT 0,
		pts        INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_staff_kpi_org ON staff_kpi(org_id)`)

	// AutoFlow: per-org KPI point weights
	s.db.Exec(`CREATE TABLE IF NOT EXISTS kpi_config (
		org_id     INTEGER PRIMARY KEY,
		conv_pts   INTEGER NOT NULL DEFAULT 10,
		conv2_pts  INTEGER NOT NULL DEFAULT 50,
		cmt_pts    INTEGER NOT NULL DEFAULT 2,
		bonus_pts  INTEGER NOT NULL DEFAULT 1000,
		bonus_amt  INTEGER NOT NULL DEFAULT 500000,
		pen_pts    INTEGER NOT NULL DEFAULT 300,
		pen_amt    INTEGER NOT NULL DEFAULT 100000,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	// AutoFlow: org-uploaded private files for AI context
	s.db.Exec(`CREATE TABLE IF NOT EXISTS private_files (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id     INTEGER NOT NULL,
		name       TEXT NOT NULL,
		path       TEXT NOT NULL,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		mime_type  TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_private_files_org ON private_files(org_id)`)

	// AutoFlow: org-scoped external data sources (Sheets/Drive/other connectors)
	s.db.Exec(`CREATE TABLE IF NOT EXISTS data_sources (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id        INTEGER NOT NULL,
		type          TEXT NOT NULL,
		name          TEXT NOT NULL,
		source_url    TEXT NOT NULL DEFAULT '',
		status        TEXT NOT NULL DEFAULT 'pending',
		item_count    INTEGER NOT NULL DEFAULT 0,
		summary       TEXT NOT NULL DEFAULT '',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		last_error    TEXT NOT NULL DEFAULT '',
		last_sync_at  DATETIME,
		created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_data_sources_org ON data_sources(org_id, type, status)`)
	s.db.Exec(`ALTER TABLE data_sources ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}'`)
	s.db.Exec(`ALTER TABLE data_sources ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`)

	// AutoFlow: org-scoped recurring crawl intents. The first prompt teaches the
	// segment/source; scheduled runs reuse this deterministic plan without
	// calling the AI again.
	s.db.Exec(`CREATE TABLE IF NOT EXISTS org_crawl_intents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL,
		account_id INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		intent TEXT NOT NULL DEFAULT 'facebook_crawl',
		source_type TEXT NOT NULL,
		source_url TEXT NOT NULL,
		source_label TEXT NOT NULL DEFAULT '',
		keywords_json TEXT NOT NULL DEFAULT '[]',
		interval_minutes INTEGER NOT NULL DEFAULT 30,
		max_items INTEGER NOT NULL DEFAULT 50,
		enabled INTEGER NOT NULL DEFAULT 1,
		dedup_hash TEXT NOT NULL,
		next_run_at DATETIME NOT NULL,
		last_run_at DATETIME,
		last_task_id TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, dedup_hash)
	)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_org_crawl_intents_due ON org_crawl_intents(enabled, next_run_at)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_org_crawl_intents_org ON org_crawl_intents(org_id, enabled)`)

	// AutoFlow: extend conversation_threads with org scoping and unread tracking
	s.db.Exec(`ALTER TABLE conversation_threads ADD COLUMN unread_count INTEGER NOT NULL DEFAULT 0`)
	s.db.Exec(`ALTER TABLE conversation_threads ADD COLUMN org_id INTEGER NOT NULL DEFAULT 1`)
	s.db.Exec(`DROP INDEX IF EXISTS idx_thread_profile`)
	s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_thread_org_profile ON conversation_threads(org_id, profile_url)`)

	// AutoFlow: extend organizations with branding fields
	s.db.Exec(`ALTER TABLE organizations ADD COLUMN abbr TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE organizations ADD COLUMN color TEXT NOT NULL DEFAULT '#4f46e5'`)
	s.db.Exec(`ALTER TABLE organizations ADD COLUMN logo_path TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE organizations ADD COLUMN avatar_path TEXT NOT NULL DEFAULT ''`)

	// Phase 6: open-prompt agent — org_skills (per-org enablement) and
	// skill_executions (audit trail). Idempotent.
	if err := s.migrateSkills(); err != nil {
		return err
	}

	return nil
}
