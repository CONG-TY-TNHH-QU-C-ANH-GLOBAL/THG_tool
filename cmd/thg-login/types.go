package main

import (
	"context"
	"sync"
	"time"
)

var version = "dev"

const capabilitiesJSON = `{"native_companion":true,"browser_control":"user_device","screen_capture":true,"multi_profile":true,"dashboard_stream":true,"input_relay":true,"local_login_first":true,"hide_local_after_login":true}`

const (
	streamStatusConnectorOnline       = "connector_online"
	streamStatusChromeNotConnected    = "chrome_not_connected"
	streamStatusFacebookLoginRequired = "facebook_login_required"
	streamStatusFacebookHumanRequired = "facebook_human_required"
	streamStatusFacebookLoggedIn      = "facebook_logged_in"
)

type connectorConfig struct {
	ServerURL     string    `json:"server_url"`
	DeviceToken   string    `json:"device_token"`
	ConnectorID   int64     `json:"connector_id"`
	ConnectorName string    `json:"connector_name"`
	WSPath        string    `json:"ws_path"`
	APIBase       string    `json:"api_base"`
	PairedAt      time.Time `json:"paired_at"`
}

type pairResponse struct {
	DeviceToken string `json:"device_token"`
	Connector   struct {
		ID    int64  `json:"id"`
		OrgID int64  `json:"org_id"`
		Name  string `json:"name"`
	} `json:"connector"`
	WSPath  string `json:"ws_path"`
	APIBase string `json:"api_base"`
}

type chromeBridge struct {
	accountID         int64
	accountName       string
	port              int
	pid               int
	ctx               context.Context
	cancel            context.CancelFunc
	err               error
	loginMu           sync.Mutex
	loginIdentifier   string
	loginCaptureLog   string
	snapMu            sync.Mutex
	lastSnap          chromeSnapshot
	lastSnapAt        time.Time
	windowHidden      bool
	windowWarned      bool
	lastWindowPosture time.Time
	lastLoginRecovery time.Time

	// Phase: connector resilience.
	// targetID is the CDP target ID this bridge is attached to. We pin
	// the chromedp context to a real Facebook page target instead of
	// letting chromedp auto-create a fresh "about:blank" tab, so the
	// probe sees the SAME tab the user is logging into. When the user
	// closes that tab, lastReattemptAt rate-limits the recovery loop
	// from spinning up a new bridge every heartbeat.
	targetID         string
	lastReattemptAt  time.Time
	reattemptWarned  bool
}

type chromeSnapshot struct {
	AccountID      int64
	AccountName    string
	CurrentURL     string
	FBUserID       string
	FBDisplayName  string
	FBUsername     string
	FBProfileURL   string
	LoginEmail     string
	Status         string
	ChromeError    string
	ScreenshotData string
}

type facebookIdentity struct {
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	ProfileURL  string `json:"profile_url"`
}

type browserTarget struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	FBUserID    string `json:"fb_user_id"`
	Status      string `json:"status"`
}

type browserTargetsResponse struct {
	Targets           []browserTarget `json:"targets"`
	Count             int             `json:"count"`
	HintCode          string          `json:"hint_code"`
	Hint              string          `json:"hint"`
	AssignedAccountID int64           `json:"assigned_account_id"`
}

type connectorCommand struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"account_id"`
	Type        string `json:"type"`
	PayloadJSON string `json:"payload_json"`
}

type connectorCommandsResponse struct {
	Commands []connectorCommand `json:"commands"`
}

type outboundMessage struct {
	ID         int64  `json:"id"`
	OrgID      int64  `json:"org_id"`
	Type       string `json:"type"`
	AccountID  int64  `json:"account_id"`
	TargetURL  string `json:"target_url"`
	TargetName string `json:"target_name"`
	Content    string `json:"content"`
	Context    string `json:"context"`
	Status     string `json:"status"`
}

type outboxResponse struct {
	Messages []outboundMessage `json:"messages"`
	Count    int               `json:"count"`
}

type localCrawlTask struct {
	TaskID    string            `json:"task_id"`
	OrgID     int64             `json:"org_id"`
	AccountID int64             `json:"account_id"`
	Intent    string            `json:"intent"`
	Keywords  []string          `json:"keywords"`
	CrawlPlan localCrawlPlan    `json:"crawl_plan"`
	Filters   localCrawlFilters `json:"filters"`
}

type localCrawlPlan struct {
	Sources   []localCrawlSource `json:"sources"`
	MaxItems  int                `json:"max_items"`
	BatchSize int                `json:"batch_size"`
}

type localCrawlSource struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Label string `json:"label"`
}

type localCrawlFilters struct {
	Keywords []string `json:"keywords"`
}

type localCrawlItem struct {
	ID               string `json:"id"`
	SourceURL        string `json:"source_url"`
	AuthorProfileURL string `json:"author_profile_url"`
	AuthorName       string `json:"author_name"`
	Content          string `json:"content"`
	Reactions        int    `json:"reactions"`
	Comments         int    `json:"comments"`
	Shares           int    `json:"shares"`
}

type localCrawlResult struct {
	TaskID    string           `json:"task_id"`
	Intent    string           `json:"intent"`
	AccountID int64            `json:"account_id"`
	Status    string           `json:"status"`
	Error     string           `json:"error,omitempty"`
	Keywords  []string         `json:"keywords"`
	Items     []localCrawlItem `json:"items"`
}
