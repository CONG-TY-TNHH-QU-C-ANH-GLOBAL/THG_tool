-- 0001_legacy_baseline (SQLite). GENERATED from the frozen migrate() schema —
-- the single source of truth for the SQLite baseline. Do NOT edit by hand;
-- new schema changes go in a new NNNN_*.up.sql migration. Applied once by the
-- transactional, fail-fast runMigrations() runner (see migrator.go).

-- ===== Tables =====
CREATE TABLE account_behaviour_profiles (
		account_id       INTEGER PRIMARY KEY,
		org_id           INTEGER NOT NULL DEFAULT 0,
		trust_level      TEXT    NOT NULL DEFAULT 'warming',
		account_age_days INTEGER NOT NULL DEFAULT 0,
		persona_type     TEXT    NOT NULL DEFAULT '',
		workspace_role   TEXT    NOT NULL DEFAULT '',
		capabilities     TEXT    NOT NULL DEFAULT '{}',
		caps_override    TEXT    NOT NULL DEFAULT '{}',
		notes            TEXT    NOT NULL DEFAULT '',
		created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE account_runtime_state (
		account_id          INTEGER PRIMARY KEY,
		org_id              INTEGER NOT NULL DEFAULT 0,
		counters_day        TEXT    NOT NULL DEFAULT '',
		comments_today      INTEGER NOT NULL DEFAULT 0,
		inbox_today         INTEGER NOT NULL DEFAULT 0,
		group_posts_today   INTEGER NOT NULL DEFAULT 0,
		profile_posts_today INTEGER NOT NULL DEFAULT 0,
		risk_score          REAL    NOT NULL DEFAULT 0,
		recent_failures     INTEGER NOT NULL DEFAULT 0,
		cooldown_until      DATETIME,
		last_action_at      DATETIME,
		updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE accounts (
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
	, org_id INTEGER NOT NULL DEFAULT 1, assigned_user_id INTEGER DEFAULT 0, browser_logged_in INTEGER NOT NULL DEFAULT 0, fb_user_id TEXT NOT NULL DEFAULT '', fb_display_name TEXT NOT NULL DEFAULT '', fb_username TEXT NOT NULL DEFAULT '', fb_profile_url TEXT NOT NULL DEFAULT '');

CREATE TABLE action_ledger (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL,
		action_type TEXT NOT NULL,
		target_type TEXT NOT NULL DEFAULT '',
		target_url TEXT NOT NULL,
		account_id INTEGER NOT NULL DEFAULT 0,
		outbound_id INTEGER NOT NULL DEFAULT 0,
		performed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		cooldown_until DATETIME,
		outcome TEXT NOT NULL DEFAULT 'queued',
		reason TEXT NOT NULL DEFAULT ''
	, created_by INTEGER NOT NULL DEFAULT 0, channel TEXT NOT NULL DEFAULT 'facebook');

CREATE TABLE action_policies (
		id                   INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id               INTEGER NOT NULL,
		action_type          TEXT    NOT NULL,
		dedup_scope          TEXT    NOT NULL DEFAULT 'per_account',
		block_on_planned     INTEGER NOT NULL DEFAULT 0,
		block_on_executing   INTEGER NOT NULL DEFAULT 1,
		cooldown_seconds     INTEGER NOT NULL DEFAULT 86400,
		conversation_aware   INTEGER NOT NULL DEFAULT 0,
		created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at           DATETIME, coordination_scope TEXT NOT NULL DEFAULT '',
		UNIQUE(org_id, action_type)
	);

CREATE TABLE agent_tokens (
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
	, kind TEXT NOT NULL DEFAULT 'worker', transport TEXT NOT NULL DEFAULT 'poll', assigned_account_id INTEGER NOT NULL DEFAULT 0, capabilities_json TEXT NOT NULL DEFAULT '{}', current_url TEXT NOT NULL DEFAULT '', fb_user_id TEXT NOT NULL DEFAULT '', fb_display_name TEXT NOT NULL DEFAULT '', fb_username TEXT NOT NULL DEFAULT '', fb_profile_url TEXT NOT NULL DEFAULT '', stream_status TEXT NOT NULL DEFAULT 'idle', chrome_error TEXT NOT NULL DEFAULT '');

CREATE TABLE ai_memory (
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

CREATE TABLE audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		action TEXT NOT NULL,
		ip_address TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE career_jobs (
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
	, salary TEXT DEFAULT '', priority TEXT DEFAULT 'medium', urgency_score INTEGER DEFAULT 50);

CREATE TABLE classification_log (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id          INTEGER NOT NULL,
		task_id         TEXT    NOT NULL DEFAULT '',
		account_id      INTEGER NOT NULL DEFAULT 0,
		source_url      TEXT    NOT NULL DEFAULT '',
		author_name     TEXT    NOT NULL DEFAULT '',
		content_snippet TEXT    NOT NULL DEFAULT '',
		ai_intent       TEXT    NOT NULL DEFAULT '',
		ai_priority     TEXT    NOT NULL DEFAULT '',
		ai_reason       TEXT    NOT NULL DEFAULT '',
		ai_score        REAL    NOT NULL DEFAULT 0,
		target_role     TEXT    NOT NULL DEFAULT '',
		decision        TEXT    NOT NULL,
		user_prompt     TEXT    NOT NULL DEFAULT '',
		created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE comments (
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

CREATE TABLE company_images (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		telegram_file_id TEXT NOT NULL,
		local_path TEXT NOT NULL DEFAULT '',
		description TEXT DEFAULT '',
		category TEXT DEFAULT 'general',
		source_url TEXT DEFAULT '',
		use_count INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE connector_commands (
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
	);

CREATE TABLE connector_pairing_codes (
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
	);

CREATE TABLE connector_screenshots (
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
	);

CREATE TABLE conversation_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		thread_id INTEGER NOT NULL,
		direction TEXT NOT NULL,
		content TEXT NOT NULL,
		ai_generated INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (thread_id) REFERENCES conversation_threads(id)
	);

CREATE TABLE conversation_threads (
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
	, unread_count INTEGER NOT NULL DEFAULT 0, org_id INTEGER NOT NULL DEFAULT 1);

CREATE TABLE data_sources (
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
	);

CREATE TABLE execution_attempts (
		id                INTEGER PRIMARY KEY AUTOINCREMENT,
		action_ledger_id  INTEGER NOT NULL DEFAULT 0,
		outbound_id       INTEGER NOT NULL DEFAULT 0,
		org_id            INTEGER NOT NULL,
		account_id        INTEGER NOT NULL DEFAULT 0,
		target_url        TEXT    NOT NULL DEFAULT '',
		action_type       TEXT    NOT NULL DEFAULT '',
		attempt           INTEGER NOT NULL DEFAULT 1,
		status            TEXT    NOT NULL DEFAULT 'queued',
		outcome           TEXT    NOT NULL DEFAULT '',
		failure_reason    TEXT    NOT NULL DEFAULT '',
		evidence_json     TEXT    NOT NULL DEFAULT '{}',
		dom_verified      INTEGER NOT NULL DEFAULT 0,
		network_verified  INTEGER NOT NULL DEFAULT 0,
		started_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		finished_at       DATETIME
	, transition_type TEXT NOT NULL DEFAULT 'finalize', execution_id TEXT NOT NULL DEFAULT '', resulting_state TEXT NOT NULL DEFAULT '', resulting_outcome TEXT, lease_expiry DATETIME, created_by INTEGER NOT NULL DEFAULT 0);

CREATE TABLE group_quality (
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

CREATE TABLE groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		name TEXT NOT NULL,
		url TEXT NOT NULL UNIQUE,
		active INTEGER NOT NULL DEFAULT 1,
		join_state TEXT NOT NULL DEFAULT 'none',
		last_scan DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	, org_id INTEGER NOT NULL DEFAULT 1);

CREATE TABLE inbox_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		sender TEXT,
		sender_url TEXT,
		content TEXT NOT NULL,
		is_read INTEGER NOT NULL DEFAULT 0,
		received_at DATETIME,
		scraped_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE jobs (
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
	, claimed_by TEXT NOT NULL DEFAULT '', claimed_at DATETIME, execution_mode TEXT NOT NULL DEFAULT 'server');

CREATE TABLE knowledge_assets (
		id                    INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id                INTEGER  NOT NULL,
		source_id             INTEGER  NOT NULL,
		external_id           TEXT     NOT NULL DEFAULT '',
		type                  TEXT     NOT NULL,
		title                 TEXT     NOT NULL,
		description           TEXT     NOT NULL DEFAULT '',
		tags                  TEXT     NOT NULL DEFAULT '[]',
		payload               TEXT     NOT NULL DEFAULT '{}',
		state                 TEXT     NOT NULL DEFAULT 'pending',
		pinned                INTEGER  NOT NULL DEFAULT 0,
		boost                 INTEGER  NOT NULL DEFAULT 0,
		retrieval_count_30d   INTEGER  NOT NULL DEFAULT 0,
		conversion_count_30d  INTEGER  NOT NULL DEFAULT 0,
		last_retrieved_at     DATETIME,
		created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (source_id) REFERENCES knowledge_sources(id)
	);

CREATE TABLE knowledge_events (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id        INTEGER  NOT NULL,
		event_type    TEXT     NOT NULL,
		retrieval_id  TEXT     NOT NULL DEFAULT '',
		source_type   TEXT     NOT NULL DEFAULT '',
		query         TEXT     NOT NULL DEFAULT '',
		data_json     TEXT     NOT NULL DEFAULT '{}',
		duration_ms   INTEGER  NOT NULL DEFAULT 0,
		occurred_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE knowledge_feedback (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id        INTEGER  NOT NULL,
		user_id       INTEGER  NOT NULL DEFAULT 0,
		retrieval_id  TEXT     NOT NULL DEFAULT '',
		asset_id      INTEGER  NOT NULL DEFAULT 0,
		kind          TEXT     NOT NULL,
		data_json     TEXT     NOT NULL DEFAULT '{}',
		occurred_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE knowledge_sources (
		id                     INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id                 INTEGER  NOT NULL,
		type                   TEXT     NOT NULL,
		label                  TEXT     NOT NULL,
		connection_config      TEXT     NOT NULL DEFAULT '{}',
		sync_policy            TEXT     NOT NULL DEFAULT 'manual',
		health_status          TEXT     NOT NULL DEFAULT 'healthy',
		health_message         TEXT     NOT NULL DEFAULT '',
		last_sync_at           DATETIME,
		last_sync_asset_count  INTEGER  NOT NULL DEFAULT 0,
		created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE kpi_config (
		org_id     INTEGER PRIMARY KEY,
		conv_pts   INTEGER NOT NULL DEFAULT 10,
		conv2_pts  INTEGER NOT NULL DEFAULT 50,
		cmt_pts    INTEGER NOT NULL DEFAULT 2,
		bonus_pts  INTEGER NOT NULL DEFAULT 1000,
		bonus_amt  INTEGER NOT NULL DEFAULT 500000,
		pen_pts    INTEGER NOT NULL DEFAULT 300,
		pen_amt    INTEGER NOT NULL DEFAULT 100000,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE leads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL DEFAULT 0,
		source_type TEXT NOT NULL,
		source_id INTEGER NOT NULL,
		source_url TEXT DEFAULT '',
		secondary_url TEXT DEFAULT '',
		post_fbid TEXT DEFAULT '',
		comment_fbid TEXT DEFAULT '',
		group_fbid TEXT DEFAULT '',
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
	, thread_role TEXT NOT NULL DEFAULT 'intent_originator');

CREATE TABLE niches (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		emoji TEXT DEFAULT '🎯',
		active INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE org_crawl_intents (
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
		status TEXT NOT NULL DEFAULT 'active',
		dedup_hash TEXT NOT NULL,
		cursor_last_post_id TEXT NOT NULL DEFAULT '',
		cursor_last_post_at DATETIME,
		cursor_updated_at DATETIME,
		next_run_at DATETIME NOT NULL,
		last_run_at DATETIME,
		last_task_id TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, dedup_hash)
	);

CREATE TABLE org_invites (
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
	);

CREATE TABLE org_skills (
			org_id     INTEGER NOT NULL,
			skill_id   TEXT    NOT NULL,
			enabled    INTEGER NOT NULL DEFAULT 1,
			config     TEXT    NOT NULL DEFAULT '{}',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_by INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (org_id, skill_id)
		);

CREATE TABLE organizations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		domain TEXT DEFAULT '',
		plan_tier TEXT NOT NULL DEFAULT 'free',
		max_accounts INTEGER NOT NULL DEFAULT 1,
		active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	, abbr TEXT NOT NULL DEFAULT '', color TEXT NOT NULL DEFAULT '#4f46e5', logo_path TEXT NOT NULL DEFAULT '', avatar_path TEXT NOT NULL DEFAULT '');

CREATE TABLE outbound_messages (
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
	, claimed_by TEXT NOT NULL DEFAULT '', claimed_at DATETIME, execution_id TEXT NOT NULL DEFAULT '', lease_expiry DATETIME, execution_state TEXT NOT NULL DEFAULT 'planned', verification_outcome TEXT, created_by INTEGER NOT NULL DEFAULT 0);

CREATE TABLE posts (
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

CREATE TABLE price_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_name TEXT NOT NULL,
		price TEXT NOT NULL,
		unit TEXT DEFAULT '',
		notes TEXT DEFAULT '',
		source TEXT DEFAULT 'text',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE private_files (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id     INTEGER NOT NULL,
		name       TEXT NOT NULL,
		path       TEXT NOT NULL,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		mime_type  TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE prompt_logs (
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
	, routing_decision_json TEXT NOT NULL DEFAULT '{}');

CREATE TABLE refresh_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

CREATE TABLE runtime_events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id      INTEGER NOT NULL DEFAULT 0,
			account_id  INTEGER NOT NULL DEFAULT 0,
			event       TEXT NOT NULL,
			level       TEXT NOT NULL DEFAULT 'info',
			outbound_id INTEGER NOT NULL DEFAULT 0,
			attempt_id  INTEGER NOT NULL DEFAULT 0,
			target_url  TEXT NOT NULL DEFAULT '',
			attrs_json  TEXT NOT NULL DEFAULT '{}',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

CREATE TABLE scan_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT NOT NULL,
		group_count INTEGER DEFAULT 0,
		post_count INTEGER DEFAULT 0,
		lead_count INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		errors TEXT DEFAULT '[]',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE selector_cache (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		action      TEXT NOT NULL,
		platform    TEXT NOT NULL,
		selectors   TEXT NOT NULL DEFAULT '{}',
		hit_count   INTEGER NOT NULL DEFAULT 0,
		updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(action, platform)
	);

CREATE TABLE skill_executions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id      INTEGER NOT NULL,
			user_id     INTEGER NOT NULL DEFAULT 0,
			source      TEXT    NOT NULL,
			skill_id    TEXT    NOT NULL,
			args_json   TEXT    NOT NULL DEFAULT '{}',
			summary     TEXT    NOT NULL DEFAULT '',
			success     INTEGER NOT NULL DEFAULT 0,
			error       TEXT    NOT NULL DEFAULT '',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

CREATE TABLE staff_kpi (
		user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		org_id     INTEGER NOT NULL DEFAULT 1,
		convs      INTEGER NOT NULL DEFAULT 0,
		converted  INTEGER NOT NULL DEFAULT 0,
		cmts       INTEGER NOT NULL DEFAULT 0,
		pts        INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE user_context (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

CREATE TABLE user_execution_context (
		org_id             INTEGER NOT NULL,
		user_id            INTEGER NOT NULL,
		default_account_id INTEGER NOT NULL DEFAULT 0,
		updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (org_id, user_id)
	);

CREATE TABLE users (
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
	, org_id INTEGER NOT NULL DEFAULT 0);

-- ===== Indexes =====
CREATE INDEX idx_accounts_platform ON accounts(platform, status);

CREATE INDEX idx_action_ledger_account
		ON action_ledger(org_id, account_id, action_type, performed_at DESC);

CREATE INDEX idx_action_ledger_engagement
		ON action_ledger(org_id, target_url, performed_at DESC);

CREATE INDEX idx_action_ledger_member ON action_ledger(org_id, created_by, performed_at DESC);

CREATE INDEX idx_action_ledger_target
		ON action_ledger(org_id, action_type, target_url, performed_at DESC);

CREATE INDEX idx_agent_tokens_hash ON agent_tokens(token_hash);

CREATE INDEX idx_agent_tokens_kind ON agent_tokens(org_id, kind, active);

CREATE INDEX idx_agent_tokens_org ON agent_tokens(org_id, active);

CREATE INDEX idx_audit_logs_created ON audit_logs(created_at DESC);

CREATE INDEX idx_audit_logs_user ON audit_logs(user_id, created_at DESC);

CREATE INDEX idx_behaviour_profile_org
		ON account_behaviour_profiles(org_id, trust_level);

CREATE INDEX idx_classification_log_org_decision ON classification_log(org_id, decision, created_at DESC);

CREATE INDEX idx_classification_log_org_task ON classification_log(org_id, task_id, created_at DESC);

CREATE INDEX idx_comments_dedup ON comments(dedup_hash);

CREATE INDEX idx_company_images_category ON company_images(category);

CREATE INDEX idx_connector_commands_account ON connector_commands(org_id, account_id, status, id);

CREATE INDEX idx_connector_commands_agent ON connector_commands(agent_id, status, id);

CREATE INDEX idx_connector_pairing_hash ON connector_pairing_codes(code_hash);

CREATE INDEX idx_connector_pairing_org ON connector_pairing_codes(org_id, expires_at);

CREATE INDEX idx_connector_screenshots_agent ON connector_screenshots(agent_id, updated_at);

CREATE INDEX idx_connector_screenshots_org ON connector_screenshots(org_id, updated_at);

CREATE INDEX idx_conv_msg_thread ON conversation_messages(thread_id, created_at);

CREATE INDEX idx_data_sources_org ON data_sources(org_id, type, status);

CREATE INDEX idx_execution_attempts_account
		ON execution_attempts(org_id, account_id, started_at DESC);

CREATE INDEX idx_execution_attempts_latest
		ON execution_attempts(outbound_id, started_at DESC);

CREATE INDEX idx_execution_attempts_ledger
		ON execution_attempts(action_ledger_id, started_at DESC);

CREATE INDEX idx_execution_attempts_org_outcome
		ON execution_attempts(org_id, outcome, started_at DESC);

CREATE INDEX idx_execution_attempts_outbound
		ON execution_attempts(outbound_id, attempt DESC);

CREATE INDEX idx_group_quality_decision ON group_quality(decision);

CREATE INDEX idx_group_quality_score ON group_quality(final_score DESC);

CREATE INDEX idx_groups_active ON groups(active, platform);

CREATE INDEX idx_jobs_status ON jobs(status);

CREATE INDEX idx_knowledge_assets_org_pin_boost
		ON knowledge_assets(org_id, pinned DESC, boost DESC, retrieval_count_30d DESC);

CREATE INDEX idx_knowledge_assets_org_source
		ON knowledge_assets(org_id, source_id);

CREATE INDEX idx_knowledge_assets_org_state
		ON knowledge_assets(org_id, state);

CREATE INDEX idx_knowledge_events_org_time
		ON knowledge_events(org_id, occurred_at DESC);

CREATE INDEX idx_knowledge_events_retrieval
		ON knowledge_events(org_id, retrieval_id);

CREATE INDEX idx_knowledge_feedback_org_time
		ON knowledge_feedback(org_id, occurred_at DESC);

CREATE INDEX idx_knowledge_feedback_retrieval
		ON knowledge_feedback(org_id, retrieval_id);

CREATE INDEX idx_knowledge_sources_org
		ON knowledge_sources(org_id, health_status);

CREATE INDEX idx_knowledge_sources_sync
		ON knowledge_sources(sync_policy, last_sync_at);

CREATE UNIQUE INDEX idx_leads_dedup ON leads(source_type, source_id) WHERE source_id > 0;

CREATE INDEX idx_leads_org_score ON leads(org_id, score);

CREATE INDEX idx_leads_org_thread_role ON leads(org_id, thread_role);

CREATE INDEX idx_leads_score ON leads(score);

CREATE INDEX idx_leads_source_url ON leads(source_url) WHERE source_url != '';

CREATE INDEX idx_memory_hash ON ai_memory(prompt_hash);

CREATE INDEX idx_org_crawl_intents_due ON org_crawl_intents(enabled, next_run_at);

CREATE INDEX idx_org_crawl_intents_org ON org_crawl_intents(org_id, enabled);

CREATE INDEX idx_org_crawl_intents_status_due ON org_crawl_intents(status, next_run_at);

CREATE INDEX idx_org_invites_email ON org_invites(email, used_at, expires_at);

CREATE INDEX idx_org_invites_org ON org_invites(org_id);

CREATE INDEX idx_org_invites_token ON org_invites(token);

CREATE UNIQUE INDEX idx_outbound_active_target
		ON outbound_messages(org_id, account_id, type, target_url)
		WHERE execution_state IN ('planned','executing');

CREATE INDEX idx_outbound_exec_lease ON outbound_messages(execution_state, lease_expiry) WHERE execution_state = 'executing';

CREATE INDEX idx_outbound_exec_state ON outbound_messages(org_id, execution_state);

CREATE INDEX idx_outbound_status ON outbound_messages(status);

CREATE INDEX idx_outbound_type_url_status ON outbound_messages(type, target_url, status);

CREATE INDEX idx_outbound_verify_outcome ON outbound_messages(org_id, verification_outcome);

CREATE INDEX idx_posts_dedup ON posts(dedup_hash);

CREATE INDEX idx_posts_platform ON posts(platform, scraped_at);

CREATE INDEX idx_private_files_org ON private_files(org_id);

CREATE INDEX idx_prompt_logs_org_created ON prompt_logs(org_id, created_at DESC);

CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE INDEX idx_runtime_events_event_time
			ON runtime_events(event, created_at DESC);

CREATE INDEX idx_runtime_events_org_time
			ON runtime_events(org_id, created_at DESC);

CREATE INDEX idx_runtime_events_outbound
			ON runtime_events(outbound_id, created_at DESC)
			WHERE outbound_id > 0;

CREATE INDEX idx_runtime_state_org
		ON account_runtime_state(org_id, cooldown_until);

CREATE INDEX idx_skill_executions_org
			ON skill_executions(org_id, created_at DESC);

CREATE INDEX idx_skill_executions_skill
			ON skill_executions(skill_id, created_at DESC);

CREATE INDEX idx_staff_kpi_org ON staff_kpi(org_id);

CREATE UNIQUE INDEX idx_thread_org_profile ON conversation_threads(org_id, profile_url);

CREATE INDEX idx_thread_status ON conversation_threads(status);

CREATE INDEX idx_users_email ON users(email);

CREATE UNIQUE INDEX uq_accounts_org_fb_identity
		ON accounts(org_id, fb_user_id) WHERE fb_user_id != '';

CREATE UNIQUE INDEX uq_knowledge_assets_idem
		ON knowledge_assets(org_id, source_id, external_id)
		WHERE external_id != '';

-- ===== Seed data (org / niches / action policies) =====
INSERT OR IGNORE INTO organizations (id, name, domain, plan_tier, max_accounts) VALUES (1, 'THG Platform', 'thgfulfill.com', 'enterprise', 0);
INSERT OR IGNORE INTO niches (slug, name, emoji) VALUES ('logistics', 'Logistics & Vận chuyển', '🚛');
INSERT OR IGNORE INTO niches (slug, name, emoji) VALUES ('tuyen_dung', 'Tuyển dụng', '👔');
INSERT OR IGNORE INTO action_policies (org_id, action_type, dedup_scope, block_on_planned, block_on_executing, cooldown_seconds, conversation_aware) VALUES
  (0, 'comment',      'per_account', 1, 1, 86400, 0),
  (0, 'inbox',        'workspace',   1, 1, 86400, 1),
  (0, 'group_post',   'per_account', 1, 1, 86400, 0),
  (0, 'profile_post', 'per_account', 1, 1, 86400, 0);
