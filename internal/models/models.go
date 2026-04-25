package models

import "time"

// Platform represents a social media platform.
type Platform string

const (
	PlatformFacebook Platform = "facebook"
	PlatformTikTok   Platform = "tiktok"
	PlatformZalo     Platform = "zalo"
)

// JobType represents the type of scraping job.
type JobType string

const (
	JobScrapePost    JobType = "SCRAPE_POSTS"
	JobScrapeComment JobType = "SCRAPE_COMMENTS"
	JobScrapeInbox   JobType = "SCRAPE_INBOX"
	JobAutoComment   JobType = "AUTO_COMMENT"
	JobAutoInbox     JobType = "AUTO_INBOX"
)

// JobStatus represents the current status of a job.
type JobStatus string

const (
	JobPending  JobStatus = "pending"
	JobRunning  JobStatus = "running"
	JobDone     JobStatus = "done"
	JobFailed   JobStatus = "failed"
	JobCanceled JobStatus = "canceled"
)

// LeadScore represents the AI-classified quality of a lead.
type LeadScore string

const (
	LeadHot      LeadScore = "hot"
	LeadWarm     LeadScore = "warm"
	LeadCold     LeadScore = "cold"
	LeadRejected LeadScore = "rejected"
)

// Group represents a social media group/page to monitor.
type Group struct {
	ID        int64     `json:"id" db:"id"`
	OrgID     int64     `json:"org_id" db:"org_id"`
	Platform  Platform  `json:"platform" db:"platform"`
	Name      string    `json:"name" db:"name"`
	URL       string    `json:"url" db:"url"`
	Active    bool      `json:"active" db:"active"`
	JoinState string    `json:"join_state" db:"join_state"` // joined, pending, none
	LastScan  time.Time `json:"last_scan" db:"last_scan"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Post represents a scraped social media post.
type Post struct {
	ID           int64     `json:"id" db:"id"`
	Platform     Platform  `json:"platform" db:"platform"`
	GroupID      int64     `json:"group_id" db:"group_id"`
	GroupName    string    `json:"group_name" db:"group_name"`
	URL          string    `json:"url" db:"url"`
	Author       string    `json:"author" db:"author"`
	AuthorURL    string    `json:"author_url" db:"author_url"`
	AuthorAvatar string    `json:"author_avatar" db:"author_avatar"`
	Content      string    `json:"content" db:"content"`
	Images       string    `json:"images" db:"images"` // JSON array
	Reactions    int       `json:"reactions" db:"reactions"`
	Comments     int       `json:"comments" db:"comments"`
	PostedAt     time.Time `json:"posted_at" db:"posted_at"`
	ScrapedAt    time.Time `json:"scraped_at" db:"scraped_at"`
	DedupHash    string    `json:"dedup_hash" db:"dedup_hash"`
}

// Comment represents a scraped comment on a post.
type Comment struct {
	ID        int64     `json:"id" db:"id"`
	PostID    int64     `json:"post_id" db:"post_id"`
	Platform  Platform  `json:"platform" db:"platform"`
	Author    string    `json:"author" db:"author"`
	AuthorURL string    `json:"author_url" db:"author_url"`
	Content   string    `json:"content" db:"content"`
	PostedAt  time.Time `json:"posted_at" db:"posted_at"`
	ScrapedAt time.Time `json:"scraped_at" db:"scraped_at"`
	DedupHash string    `json:"dedup_hash" db:"dedup_hash"`
}

// InboxMessage represents a message from inbox/messenger.
type InboxMessage struct {
	ID         int64     `json:"id" db:"id"`
	Platform   Platform  `json:"platform" db:"platform"`
	Sender     string    `json:"sender" db:"sender"`
	SenderURL  string    `json:"sender_url" db:"sender_url"`
	Content    string    `json:"content" db:"content"`
	IsRead     bool      `json:"is_read" db:"is_read"`
	ReceivedAt time.Time `json:"received_at" db:"received_at"`
	ScrapedAt  time.Time `json:"scraped_at" db:"scraped_at"`
}

// Lead represents an AI-classified lead derived from a post or comment.
type Lead struct {
	ID           int64     `json:"id" db:"id"`
	SourceType   string    `json:"source_type" db:"source_type"` // post, comment, inbox
	SourceID     int64     `json:"source_id" db:"source_id"`
	SourceURL    string    `json:"source_url" db:"source_url"` // URL of the original post
	Platform     Platform  `json:"platform" db:"platform"`
	Author       string    `json:"author" db:"author"`
	AuthorURL    string    `json:"author_url" db:"author_url"`
	Content      string    `json:"content" db:"content"`
	Score        LeadScore `json:"score" db:"score"`
	ServiceMatch string    `json:"service_match" db:"service_match"`
	AuthorRole   string    `json:"author_role" db:"author_role"`
	PainPoint    string    `json:"pain_point" db:"pain_point"`
	AIReasoning  string    `json:"ai_reasoning" db:"ai_reasoning"`
	Niche        string    `json:"niche" db:"niche"`         // e.g. "logistics", "tuyen_dung"
	Commented    bool      `json:"commented" db:"commented"` // true if already commented
	ClassifiedAt time.Time `json:"classified_at" db:"classified_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Niche represents a business niche/industry for multi-domain lead management.
type Niche struct {
	ID        int64     `json:"id"`
	Slug      string    `json:"slug"`  // e.g. "logistics", "tuyen_dung"
	Name      string    `json:"name"`  // e.g. "Logistics & Vận chuyển"
	Emoji     string    `json:"emoji"` // e.g. "🚛"
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// ExecutionMode determines where a job runs.
type ExecutionMode string

const (
	ExecutionServer ExecutionMode = "server" // default: run on VPS
	ExecutionLocal  ExecutionMode = "local"  // run by local agent on staff's machine
)

// Job represents a scraping job in the queue.
type Job struct {
	ID            int64         `json:"id" db:"id"`
	Type          JobType       `json:"type" db:"type"`
	Platform      Platform      `json:"platform" db:"platform"`
	Target        string        `json:"target" db:"target"`
	Status        JobStatus     `json:"status" db:"status"`
	ExecutionMode ExecutionMode `json:"execution_mode" db:"execution_mode"`
	Result        string        `json:"result" db:"result"`
	Error         string        `json:"error" db:"error"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
	StartedAt     time.Time     `json:"started_at" db:"started_at"`
	DoneAt        time.Time     `json:"done_at" db:"done_at"`
}

// ScanLog represents a log entry for a scraping cycle.
type ScanLog struct {
	ID         int64     `json:"id" db:"id"`
	Platform   Platform  `json:"platform" db:"platform"`
	GroupCount int       `json:"group_count" db:"group_count"`
	PostCount  int       `json:"post_count" db:"post_count"`
	LeadCount  int       `json:"lead_count" db:"lead_count"`
	Duration   int       `json:"duration" db:"duration"` // seconds
	Errors     string    `json:"errors" db:"errors"`     // JSON array
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// AccountStatus represents the health state of a social account.
type AccountStatus string

const (
	AccountActive   AccountStatus = "active"
	AccountCooldown AccountStatus = "cooldown"
	AccountBanned   AccountStatus = "banned"
	AccountInactive AccountStatus = "inactive"
)

// Account represents a social media account used for scraping.
type Account struct {
	ID               int64         `json:"id" db:"id"`
	OrgID            int64         `json:"org_id" db:"org_id"`
	Platform         Platform      `json:"platform" db:"platform"`
	Name             string        `json:"name" db:"name"`
	Email            string        `json:"email" db:"email"`
	CookiesJSON      string        `json:"cookies_json" db:"cookies_json"` // encrypted JSON cookies
	ProxyURL         string        `json:"proxy_url" db:"proxy_url"`
	UserAgent        string        `json:"user_agent" db:"user_agent"`
	Status           AccountStatus `json:"status" db:"status"`
	Notes            string        `json:"notes" db:"notes"`
	LastUsed         time.Time     `json:"last_used" db:"last_used"`
	CreatedAt        time.Time     `json:"created_at" db:"created_at"`
	AssignedUserID   int64         `json:"assigned_user_id"`
	AssignedUserName string        `json:"assigned_user_name"` // resolved from users JOIN
	BrowserLoggedIn  bool          `json:"browser_logged_in" db:"browser_logged_in"`
}

// PromptLog records every AI prompt interaction for learning.
type PromptLog struct {
	ID          int64     `json:"id" db:"id"`
	Source      string    `json:"source" db:"source"` // telegram, dashboard
	UserPrompt  string    `json:"user_prompt" db:"user_prompt"`
	AIResponse  string    `json:"ai_response" db:"ai_response"`
	ActionTaken string    `json:"action_taken" db:"action_taken"` // function name called
	ActionArgs  string    `json:"action_args" db:"action_args"`   // JSON args
	Success     bool      `json:"success" db:"success"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// AIMemory stores learned prompt patterns for few-shot injection.
type AIMemory struct {
	ID          int64     `json:"id" db:"id"`
	PromptHash  string    `json:"prompt_hash" db:"prompt_hash"` // hash of normalized prompt
	Category    string    `json:"category" db:"category"`       // classify, scrape, manage, etc.
	UserPrompt  string    `json:"user_prompt" db:"user_prompt"`
	BestAction  string    `json:"best_action" db:"best_action"` // best function to call
	BestArgs    string    `json:"best_args" db:"best_args"`     // best args JSON
	UseCount    int       `json:"use_count" db:"use_count"`
	SuccessRate float64   `json:"success_rate" db:"success_rate"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Stats represents dashboard statistics.
type Stats struct {
	TotalGroups     int       `json:"total_groups"`
	ActiveGroups    int       `json:"active_groups"`
	TotalPosts      int       `json:"total_posts"`
	TotalComments   int       `json:"total_comments"`
	TotalLeads      int       `json:"total_leads"`
	HotLeads        int       `json:"hot_leads"`
	TodayPosts      int       `json:"today_posts"`
	TodayLeads      int       `json:"today_leads"`
	RunningJobs     int       `json:"running_jobs"`
	TotalAccounts   int       `json:"total_accounts"`
	ActiveAccounts  int       `json:"active_accounts"`
	TotalPrompts    int       `json:"total_prompts"`
	LastScanAt      time.Time `json:"last_scan_at"`
	AvgScanDuration int       `json:"avg_scan_duration"`
}

// OutboundStatus represents the approval state of an outbound message.
type OutboundStatus string

const (
	OutboundDraft    OutboundStatus = "draft"    // AI drafted, awaiting approval
	OutboundApproved OutboundStatus = "approved" // Approved, ready to send
	OutboundSent     OutboundStatus = "sent"     // Successfully sent
	OutboundRejected OutboundStatus = "rejected" // Rejected by user
	OutboundFailed   OutboundStatus = "failed"   // Send failed
)

// OutboundMessage represents an auto-comment or auto-inbox message.
type OutboundMessage struct {
	ID         int64          `json:"id" db:"id"`
	Type       string         `json:"type" db:"type"` // comment, inbox
	Platform   Platform       `json:"platform" db:"platform"`
	AccountID  int64          `json:"account_id" db:"account_id"`
	TargetURL  string         `json:"target_url" db:"target_url"`   // post URL or messenger URL
	TargetName string         `json:"target_name" db:"target_name"` // post author or inbox recipient
	Content    string         `json:"content" db:"content"`         // message content
	Context    string         `json:"context" db:"context"`         // original post/lead content for reference
	ImagePath  string         `json:"image_path" db:"image_path"`   // local path of company image to attach
	Status     OutboundStatus `json:"status" db:"status"`
	AIModel    string         `json:"ai_model" db:"ai_model"` // model used to generate
	SentAt     time.Time      `json:"sent_at" db:"sent_at"`
	CreatedAt  time.Time      `json:"created_at" db:"created_at"`
}

// PriceItem stores a learned pricing entry for a service or product.
type PriceItem struct {
	ID          int64     `json:"id"`
	ServiceName string    `json:"service_name"`
	Price       string    `json:"price"`
	Unit        string    `json:"unit"`
	Notes       string    `json:"notes"`
	Source      string    `json:"source"` // "image", "text"
	CreatedAt   time.Time `json:"created_at"`
}

// CompanyImage stores real company service images uploaded via Telegram or crawled from catalog URLs.
type CompanyImage struct {
	ID             int64     `json:"id"`
	TelegramFileID string    `json:"telegram_file_id"`
	LocalPath      string    `json:"local_path"`
	Description    string    `json:"description"`
	Category       string    `json:"category"`   // "general", "service", "catalog"
	SourceURL      string    `json:"source_url"` // catalog URL this image was crawled from
	UseCount       int       `json:"use_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConversationThread tracks an ongoing Messenger dialogue with a lead.
// One thread per profile URL — persists across sessions so AI always has context.
type ConversationThread struct {
	ID             int64     `json:"id"`
	LeadID         int64     `json:"lead_id"`
	Platform       Platform  `json:"platform"`
	ProfileURL     string    `json:"profile_url"`  // lead's FB profile URL (unique key)
	ProfileName    string    `json:"profile_name"` // display name
	Niche          string    `json:"niche"`
	Status         string    `json:"status"` // initiated, replied, follow_up_sent, converted, closed
	LastOutboundAt time.Time `json:"last_outbound_at"`
	LastInboundAt  time.Time `json:"last_inbound_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConversationMessage stores each individual message in a thread.
type ConversationMessage struct {
	ID          int64     `json:"id"`
	ThreadID    int64     `json:"thread_id"`
	Direction   string    `json:"direction"`    // outbound (we sent) | inbound (they replied)
	Content     string    `json:"content"`
	AIGenerated bool      `json:"ai_generated"` // true = our AI wrote it
	CreatedAt   time.Time `json:"created_at"`
}

// CareerJob represents an open job position scraped from the company careers page.
type CareerJob struct {
	ID           int64     `json:"id" db:"id"`
	Title        string    `json:"title" db:"title"`
	Description  string    `json:"description" db:"description"`
	Location     string    `json:"location" db:"location"`
	Requirements string    `json:"requirements" db:"requirements"`
	Benefits     string    `json:"benefits" db:"benefits"`
	Salary       string    `json:"salary" db:"salary"`
	Email        string    `json:"email" db:"email"`
	URL          string    `json:"url" db:"url"`
	Priority     string    `json:"priority" db:"priority"`           // "high", "medium", "low"
	UrgencyScore int       `json:"urgency_score" db:"urgency_score"` // 0-100
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// GroupQuality stores the NLP-based quality evaluation of a Facebook group.
// Kept in a separate table so the core Group struct stays clean.
type GroupQuality struct {
	GroupID              int64     `json:"group_id"`
	GroupName            string    `json:"group_name"` // joined from groups table
	GroupURL             string    `json:"group_url"`  // joined from groups table
	Category             string    `json:"category"`   // tech|sales|ops|finance|low_quality
	RelevanceScore       float64   `json:"relevance_score"`
	ProfessionalismScore float64   `json:"professionalism_score"`
	ContentQualityScore  float64   `json:"content_quality_score"`
	SpamPenalty          float64   `json:"spam_penalty"`
	FinalScore           float64   `json:"final_score"`
	Decision             string    `json:"decision"`  // use|monitor|reject
	Reason               string    `json:"reason"`
	Whitelist            bool      `json:"whitelist"`
	Blacklist            bool      `json:"blacklist"`
	ScoredAt             time.Time `json:"scored_at"`
	LastPostAt           time.Time `json:"last_post_at"`
	WeeklyPostCount      int       `json:"weekly_post_count"`
	CandidateYield       int       `json:"candidate_yield"` // high-quality candidates produced
	SpamYield            int       `json:"spam_yield"`      // irrelevant leads produced
}

// UserRole defines the access level for a dashboard user.
type UserRole string

const (
	RoleSuperAdmin UserRole = "superadmin" // platform owner — sees all orgs
	RoleAdmin      UserRole = "admin"      // org admin — manages their org
	RoleSales      UserRole = "sales"      // org member — read + action
)

// User represents a dashboard user account with RBAC.
type User struct {
	ID           int64     `json:"id"`
	OrgID        int64     `json:"org_id"` // 0 = superadmin (cross-org)
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"` // never serialized in JSON responses
	Role         UserRole  `json:"role"`
	Active       bool      `json:"active"`
	FailedLogins int       `json:"-"`
	LockedUntil  time.Time `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AuditLog records security-relevant events for compliance and intrusion detection.
type AuditLog struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Action    string    `json:"action"`
	IPAddress string    `json:"ip"`
	Metadata  string    `json:"metadata"` // JSON extras (role, resource, etc.)
	CreatedAt time.Time `json:"timestamp"`
}

// CandidateMatch is a scored pairing of a job-seeking commenter with a specific JD.
type CandidateMatch struct {
	Author      string
	AuthorURL   string
	Content     string  // candidate's comment text
	PostURL     string  // URL of the post they commented on
	PostContent string  // original post topic (for domain-aware matching)
	Job         CareerJob
	Score       float64 // 0.0–1.0 AI match score
	Reason      string  // one-sentence explanation
}
