package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/cdpclient"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
	browserworkspace "github.com/thg/scraper/internal/workspace"
)

type cdpEndpoint = cdpclient.Endpoint
type cdpTargetInfo = cdpclient.TargetInfo

func (h *Handler) recordBrowserSession(accountID, orgID int64, inst *browserworkspace.Instance, status, errorMsg string) {
	if inst == nil {
		return
	}
	appStore, err := store.NewAppStore(h.db)
	if err != nil {
		log.Printf("[Workspace] session store unavailable for account %d: %v", accountID, err)
		return
	}
	if status != "checkpoint" && status != "idle" && status != "terminated" && status != "error" {
		if existing, err := appStore.GetSession(context.Background(), accountID); err == nil && existing != nil && existing.Status == "checkpoint" {
			return
		}
	}
	_ = appStore.UpsertSession(context.Background(), store.BrowserSession{
		AccountID:    accountID,
		OrgID:        orgID,
		Status:       status,
		CDPPort:      inst.CDPPort,
		VNCPort:      inst.VNCPort,
		StartedAt:    inst.StartedAt.UTC(),
		LastActiveAt: time.Now().UTC(),
		ErrorMsg:     errorMsg,
	})
}

func (h *Handler) workspaceInstanceForAccount(accountID int64, accountName string) *browserworkspace.Instance {
	if h.workspace == nil {
		return nil
	}
	if inst := h.workspace.Get(accountID); inst != nil && (inst.CDPPort > 0 || inst.VNCPort > 0) {
		return inst
	}

	appStore, err := store.NewAppStore(h.db)
	if err != nil {
		return nil
	}
	sess, err := appStore.GetSession(context.Background(), accountID)
	if err != nil || sess == nil || sess.Status == "terminated" {
		return nil
	}

	// The API can restart while Docker browser containers keep running. Reconcile
	// before returning "not running" so sync-session/VNC do not lose a live login.
	h.workspace.ReconcileRunning()
	if inst := h.workspace.Get(accountID); inst != nil && (inst.CDPPort > 0 || inst.VNCPort > 0) {
		return inst
	}

	if sess.CDPPort <= 0 && sess.VNCPort <= 0 {
		return nil
	}
	if !localPortReachable(sess.CDPPort, 250*time.Millisecond) && !localPortReachable(sess.VNCPort, 250*time.Millisecond) {
		return nil
	}
	startedAt := sess.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	log.Printf("[Workspace] Using recovered browser ports for account %d (vnc=%d cdp=%d)", accountID, sess.VNCPort, sess.CDPPort)
	return &browserworkspace.Instance{
		AccountID:   accountID,
		AccountName: accountName,
		ProfileDir:  "",
		ContainerID: "",
		CDPPort:     sess.CDPPort,
		VNCPort:     sess.VNCPort,
		StartedAt:   startedAt,
	}
}

func localPortReachable(port int, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (h *Handler) recordWorkspaceHumanRequired(accountID, orgID int64, inst *browserworkspace.Instance, snap *facebookSessionSnapshot) {
	if inst == nil {
		return
	}
	reason := "Meta requires manual verification"
	if snap != nil && snap.HumanReason != "" {
		reason = snap.HumanReason
	}
	if snap != nil && snap.CurrentURL != "" {
		reason += ": " + snap.CurrentURL
	}

	wasCheckpoint := false
	if appStore, err := store.NewAppStore(h.db); err == nil {
		if sess, err := appStore.GetSession(context.Background(), accountID); err == nil && sess != nil && sess.Status == "checkpoint" {
			wasCheckpoint = true
		}
	}
	h.recordBrowserSession(accountID, orgID, inst, "checkpoint", reason)
	checkpointURL := ""
	if snap != nil {
		checkpointURL = snap.CurrentURL
	}
	_, _ = h.db.DB().ExecContext(context.Background(),
		`UPDATE browser_sessions
		 SET checkpoint_url = ?, checkpoint_at = COALESCE(checkpoint_at, CURRENT_TIMESTAMP), last_active_at = CURRENT_TIMESTAMP
		 WHERE account_id = ?`,
		checkpointURL, accountID,
	)
	if !wasCheckpoint {
		_, _ = h.db.DB().ExecContext(context.Background(),
			`UPDATE accounts SET checkpoint_count = COALESCE(checkpoint_count, 0) + 1 WHERE id = ?`,
			accountID,
		)
	}
	log.Printf("[Workspace] Account %d requires human verification: %s", accountID, reason)
}

func (h *Handler) persistFacebookBrowserSession(accountID, orgID int64, inst *browserworkspace.Instance, fbUserID, cookiesJSON string) error {
	if fbUserID == "" {
		return fmt.Errorf("facebook user id is empty")
	}
	if err := h.db.SetBrowserLoggedIn(accountID, true, fbUserID); err != nil {
		return err
	}
	if cookiesJSON != "" {
		if err := h.db.UpdateAccountCookies(accountID, cookiesJSON); err != nil {
			return fmt.Errorf("save cookies failed: %w", err)
		}
	}
	_ = h.db.UpdateAccountStatus(accountID, models.AccountActive)
	if appStore, err := store.NewAppStore(h.db); err == nil {
		sess, err := appStore.GetSession(context.Background(), accountID)
		if err == nil && sess != nil && sess.Status != "terminated" {
			sess.Status = "idle"
			sess.LastActiveAt = time.Now().UTC()
			sess.ErrorMsg = ""
			if inst != nil {
				sess.CDPPort = inst.CDPPort
				sess.VNCPort = inst.VNCPort
			}
			_ = appStore.UpsertSession(context.Background(), *sess)
		} else if inst != nil {
			h.recordBrowserSession(accountID, orgID, inst, "idle", "")
		}
	}
	_, _ = h.db.DB().ExecContext(context.Background(),
		`UPDATE browser_sessions SET checkpoint_url = '', checkpoint_at = NULL WHERE account_id = ?`,
		accountID,
	)
	return nil
}

func (h *Handler) watchWorkspaceLogin(accountID, orgID int64, inst *browserworkspace.Instance) {
	if inst == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			return
		case <-ticker.C:
			if h.workspace == nil {
				return
			}
			current := h.workspace.Get(accountID)
			if current == nil || current.ContainerID != inst.ContainerID {
				return
			}
			acc, err := h.db.GetAccountForOrg(accountID, orgID)
			if err != nil || acc == nil {
				return
			}
			if acc.BrowserLoggedIn && acc.FBUserID != "" {
				return
			}
			snap, err := facebookSessionSnapshotFromInstance(inst)
			if err != nil {
				continue
			}
			if snap.FBUserID == "" {
				if snap.HumanRequired {
					h.recordWorkspaceHumanRequired(accountID, orgID, inst, snap)
				}
				continue
			}
			fbUserID, cookiesJSON := snap.FBUserID, snap.cookiesJSON
			if acc.FBUserID != "" && acc.FBUserID != fbUserID {
				msg := "Facebook profile mismatch; create a separate account slot for this Facebook user"
				h.recordBrowserSession(accountID, orgID, inst, "error", msg)
				log.Printf("[Workspace] Account %d login mismatch stored=%s current=%s", accountID, acc.FBUserID, fbUserID)
				return
			}
			if err := h.persistFacebookBrowserSession(accountID, orgID, inst, fbUserID, cookiesJSON); err != nil {
				log.Printf("[Workspace] Account %d auto session persist failed: %v", accountID, err)
				continue
			}
			if snap.HumanRequired {
				h.recordWorkspaceHumanRequired(accountID, orgID, inst, snap)
				continue
			}
			log.Printf("[Workspace] Account %d Facebook session auto-saved (fb_user_id=%s)", accountID, fbUserID)
			return
		}
	}
}

func (h *Handler) watchWorkspaceReadiness(accountID, orgID int64, inst *browserworkspace.Instance) {
	if inst == nil {
		return
	}

	vncCh := make(chan bool, 1)
	cdpCh := make(chan bool, 1)
	go func() { vncCh <- browserworkspace.WaitForVNC(inst.VNCPort, 60*time.Second) }()
	go func() { cdpCh <- browserworkspace.WaitForCDP(inst.CDPPort, 90*time.Second) }()

	vncReady := <-vncCh
	if vncReady {
		// VNC is the operator-facing live browser session. Mark it separately so the
		// dashboard can render the browser even while CDP is still warming up.
		h.recordBrowserSession(accountID, orgID, inst, "display_ready", "")
		log.Printf("[Workspace] Account %d browser display ready, vnc=%d cdp=%d", accountID, inst.VNCPort, inst.CDPPort)
	} else {
		cdpReady := <-cdpCh
		msg := "VNC did not become ready; check x11vnc/Xvfb in docker logs"
		if !cdpReady {
			msg = "VNC and Chrome CDP did not become ready; rebuild thg-browser and check docker logs"
		}
		h.recordBrowserSession(accountID, orgID, inst, "error", msg)
		log.Printf("[Workspace] Account %d browser startup warning: %s", accountID, msg)
		return
	}

	cdpReady := <-cdpCh
	if cdpReady {
		h.recordBrowserSession(accountID, orgID, inst, "ready", "")
		log.Printf("[Workspace] Account %d browser ready, vnc=%d cdp=%d", accountID, inst.VNCPort, inst.CDPPort)
		return
	}

	msg := "Browser display is visible, but Chrome CDP did not become ready; automation/login verification may wait. Check Chromium startup in docker logs"
	h.recordBrowserSession(accountID, orgID, inst, "display_ready", msg)
	log.Printf("[Workspace] Account %d browser startup warning: %s", accountID, msg)
}

type facebookSessionSnapshot struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	LoggedIn      bool   `json:"logged_in"`
	FBUserID      string `json:"fb_user_id,omitempty"`
	StoredFBID    string `json:"stored_fb_user_id,omitempty"`
	CurrentURL    string `json:"current_url,omitempty"`
	CurrentTitle  string `json:"current_title,omitempty"`
	Checkpoint    bool   `json:"checkpoint"`
	HumanRequired bool   `json:"human_required"`
	HumanReason   string `json:"human_reason,omitempty"`
	CookieError   string `json:"cookie_error,omitempty"`
	CookiesCount  int    `json:"cookies_count,omitempty"`
	cookiesJSON   string
}

func facebookSessionSnapshotFromInstance(inst *browserworkspace.Instance) (*facebookSessionSnapshot, error) {
	if inst == nil {
		return nil, fmt.Errorf("browser instance is not running")
	}
	var lastSnap *facebookSessionSnapshot
	var errors []string
	for _, ep := range cdpEndpointsForInstance(inst) {
		snap, err := facebookSessionSnapshotFromCDPEndpoint(ep)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", ep.Label, err))
			continue
		}
		if snap.HumanRequired || snap.CookieError == "" || snap.FBUserID != "" {
			return snap, nil
		}
		if !isCDPUnavailableMessage(snap.CookieError) {
			return snap, nil
		}
		lastSnap = snap
		errors = append(errors, fmt.Sprintf("%s: %s", ep.Label, snap.CookieError))
	}
	if lastSnap != nil {
		if len(errors) > 0 {
			lastSnap.CookieError = strings.Join(errors, "; ")
		}
		return lastSnap, nil
	}
	return nil, fmt.Errorf("no CDP endpoint succeeded: %s", strings.Join(errors, "; "))
}

func facebookSessionSnapshotFromCDPEndpoint(ep cdpEndpoint) (*facebookSessionSnapshot, error) {
	snap := &facebookSessionSnapshot{}
	var targets []cdpTargetInfo
	targets, err := cdpclient.FetchTargetsFromEndpoint(ep)
	if err != nil {
		snap.CookieError = "CDP target list unavailable: " + err.Error()
	} else {
		for _, t := range targets {
			if t.Type != "page" {
				continue
			}
			if strings.Contains(strings.ToLower(t.URL), "facebook.com") || snap.CurrentURL == "" {
				snap.CurrentURL = t.URL
				snap.CurrentTitle = t.Title
			}
		}
	}

	if err := enrichSnapshotFromCDP(ep, snap, targets); err != nil && snap.CookieError == "" {
		snap.CookieError = "CDP page probe unavailable: " + err.Error()
	}
	applyFacebookHumanChallengeDetection(snap, "")

	fbUserID, cookiesJSON, cookieCount, err := facebookCookiesFromCDPEndpointTargets(ep, targets)
	if err != nil {
		if snap.CookieError != "" {
			snap.CookieError += "; cookies: " + err.Error()
		} else {
			snap.CookieError = err.Error()
		}
		return snap, nil
	}
	snap.FBUserID = fbUserID
	snap.LoggedIn = fbUserID != ""
	snap.CookiesCount = cookieCount
	snap.cookiesJSON = cookiesJSON
	return snap, nil
}

func enrichSnapshotFromCDP(ep cdpEndpoint, snap *facebookSessionSnapshot, targets []cdpTargetInfo) error {
	if snap == nil {
		return nil
	}
	targetID := bestFacebookCDPTargetID(targets)
	ctx, cancel, err := cdpContextForEndpointTarget(ep, targetID, 5*time.Second)
	if err != nil {
		return err
	}
	defer cancel()

	var dom struct {
		URL   string `json:"url"`
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	script := `({
		url: location.href || "",
		title: document.title || "",
		text: document.body && document.body.innerText ? document.body.innerText.slice(0, 12000) : ""
	})`
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &dom)); err != nil {
		return err
	}
	if dom.URL != "" && dom.URL != "about:blank" && !strings.HasPrefix(dom.URL, "devtools://") {
		snap.CurrentURL = dom.URL
	}
	if dom.Title != "" {
		snap.CurrentTitle = dom.Title
	}
	applyFacebookHumanChallengeDetection(snap, dom.Text)
	return nil
}

func bestFacebookCDPTargetID(targets []cdpTargetInfo) string {
	fallback := ""
	for _, t := range targets {
		if t.Type != "page" || t.ID == "" {
			continue
		}
		lower := strings.ToLower(t.URL + " " + t.Title)
		if strings.Contains(lower, "facebook.com") {
			return t.ID
		}
		if fallback == "" && t.URL != "about:blank" && !strings.HasPrefix(t.URL, "devtools://") {
			fallback = t.ID
		}
		if fallback == "" {
			fallback = t.ID
		}
	}
	return fallback
}

func cdpContextForEndpointTarget(ep cdpEndpoint, targetID string, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	if targetID == "" {
		return cdpclient.ContextForEndpoint(ep, timeout)
	}
	wsURL, err := cdpclient.BrowserWSFromEndpoint(ep)
	if err != nil {
		return nil, nil, err
	}
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	ctx, ctxCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(cdptarget.ID(targetID)))
	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	cancel := func() {
		timeoutCancel()
		ctxCancel()
		allocCancel()
	}
	return ctx, cancel, nil
}

func applyFacebookHumanChallengeDetection(snap *facebookSessionSnapshot, bodyText string) {
	if snap == nil {
		return
	}
	haystack := strings.ToLower(strings.Join([]string{
		snap.CurrentURL,
		snap.CurrentTitle,
		bodyText,
	}, " "))

	reason := ""
	switch {
	case textutil.ContainsAny(haystack, "checkpoint", "/checkpoint/", "checkpoint_src", "/sorry/"):
		reason = "facebook_checkpoint"
	case textutil.ContainsAny(haystack, "captcha", "recaptcha", "not a robot", "i'm not a robot", "robot check", "security check", "are you a robot", "not a bot"):
		reason = "facebook_captcha"
	case textutil.ContainsAny(haystack, "confirm your identity", "identity confirmation", "verify your identity", "xÃ¡c minh", "xÃ¡c nháº­n danh tÃ­nh", "kiá»ƒm tra báº£o máº­t"):
		reason = "facebook_identity_verification"
	case textutil.ContainsAny(haystack, "unusual activity", "suspicious activity", "automated behavior", "temporarily blocked"):
		reason = "facebook_risk_checkpoint"
	}
	if reason == "" {
		return
	}
	snap.Checkpoint = true
	snap.HumanRequired = true
	snap.HumanReason = reason
	if snap.CookieError == "" {
		snap.CookieError = "Meta requires manual verification before automation can continue"
	}
}


func cdpEndpointsForInstance(inst *browserworkspace.Instance) []cdpEndpoint {
	if inst == nil {
		return nil
	}
	endpoints := []cdpEndpoint{}
	if inst.CDPPort > 0 {
		endpoints = append(endpoints, cdpclient.EndpointFromPort(inst.CDPPort))
	}
	if ip := dockerContainerIP(inst); ip != "" {
		host := net.JoinHostPort(ip, "9222")
		endpoints = append(endpoints, cdpEndpoint{
			BaseURL: "http://" + host,
			WSHost:  host,
			Label:   "container " + host,
		})
	}
	return endpoints
}

func dockerContainerIP(inst *browserworkspace.Instance) string {
	if inst == nil || inst.ContainerID == "" {
		return ""
	}
	out, err := exec.Command("docker", "inspect", "--format={{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", inst.ContainerID).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func facebookCookiesFromInstance(inst *browserworkspace.Instance) (string, string, int, error) {
	if inst == nil {
		return "", "", 0, fmt.Errorf("browser instance is not running")
	}
	var errors []string
	for _, ep := range cdpEndpointsForInstance(inst) {
		fbUserID, cookiesJSON, cookieCount, err := facebookCookiesFromCDPEndpoint(ep)
		if err == nil {
			return fbUserID, cookiesJSON, cookieCount, nil
		}
		if isMissingFacebookUserCookie(err) {
			return "", "", cookieCount, err
		}
		errors = append(errors, fmt.Sprintf("%s: %v", ep.Label, err))
	}
	return "", "", 0, fmt.Errorf("no CDP endpoint succeeded: %s", strings.Join(errors, "; "))
}

func facebookCookiesFromCDPEndpoint(ep cdpEndpoint) (string, string, int, error) {
	targets, err := cdpclient.FetchTargetsFromEndpoint(ep)
	if err != nil {
		return "", "", 0, err
	}
	return facebookCookiesFromCDPEndpointTargets(ep, targets)
}

func facebookCookiesFromCDPEndpointTargets(ep cdpEndpoint, targets []cdpTargetInfo) (string, string, int, error) {
	targetID := bestFacebookCDPTargetID(targets)
	if targetID == "" {
		return "", "", 0, fmt.Errorf("no page target available for cookie capture")
	}
	ctx, cancel, err := cdpContextForEndpointTarget(ep, targetID, 8*time.Second)
	if err != nil {
		return "", "", 0, err
	}
	defer cancel()

	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, network.Enable(), chromedp.ActionFunc(func(ctx context.Context) error {
		c := chromedp.FromContext(ctx)
		if c == nil || c.Browser == nil {
			return fmt.Errorf("browser executor unavailable")
		}
		var e error
		cookies, e = storage.GetCookies().Do(cdp.WithExecutor(ctx, c.Browser))
		return e
	})); err != nil {
		return "", "", 0, err
	}

	type exportCookie struct {
		Name     string  `json:"name"`
		Value    string  `json:"value"`
		Domain   string  `json:"domain"`
		Path     string  `json:"path"`
		Expires  float64 `json:"expires,omitempty"`
		HTTPOnly bool    `json:"httpOnly"`
		Secure   bool    `json:"secure"`
	}
	out := make([]exportCookie, 0, len(cookies))
	var fbUserID string
	for _, ck := range cookies {
		if !isFacebookCookieDomain(ck.Domain) {
			continue
		}
		if ck.Name == "c_user" && ck.Value != "" {
			fbUserID = ck.Value
		}
		out = append(out, exportCookie{
			Name:     ck.Name,
			Value:    ck.Value,
			Domain:   ck.Domain,
			Path:     ck.Path,
			Expires:  float64(ck.Expires),
			HTTPOnly: bool(ck.HTTPOnly),
			Secure:   bool(ck.Secure),
		})
	}
	if fbUserID == "" {
		return "", "", len(out), fmt.Errorf("missing c_user cookie")
	}
	cookiesJSON, err := json.Marshal(out)
	if err != nil {
		return "", "", len(out), fmt.Errorf("serialize cookies: %w", err)
	}
	return fbUserID, string(cookiesJSON), len(out), nil
}

func isFacebookCookieDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	return d == "facebook.com" || d == ".facebook.com" || strings.HasSuffix(d, ".facebook.com")
}

func isCDPUnavailable(err error) bool {
	if err == nil {
		return false
	}
	return isCDPUnavailableMessage(err.Error())
}

func isCDPUnavailableMessage(message string) bool {
	msg := strings.ToLower(message)
	return strings.Contains(msg, "chrome not ready") ||
		strings.Contains(msg, "no cdp endpoint succeeded") ||
		strings.Contains(msg, "cdp target list unavailable") ||
		strings.Contains(msg, "/json/version") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "eof")
}

func isMissingFacebookUserCookie(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "missing c_user")
}

func (h *Handler) checkpointVerifier() session.CheckpointVerifier {
	if h.workspace == nil {
		return nil
	}
	return &workspaceCheckpointVerifier{handler: h}
}

type workspaceCheckpointVerifier struct {
	handler *Handler
}

// StillAtCheckpoint runs the same Facebook human-challenge detector
// that workspaceSyncSession uses, against the live CDP target. If the
// browser is no longer running we return (false, "", nil) so the
// resolve flow falls through to the state machine â€” the session row
// will be cleared regardless of whether Chrome is still up.
func (v *workspaceCheckpointVerifier) StillAtCheckpoint(ctx context.Context, accountID int64) (bool, string, error) {
	if v == nil || v.handler == nil || v.handler.workspace == nil {
		return false, "", nil
	}
	inst := v.handler.workspace.Get(accountID)
	if inst == nil || inst.CDPPort == 0 {
		return false, "", nil
	}
	snap, err := facebookSessionSnapshotFromInstance(inst)
	if err != nil || snap == nil {
		return false, "", err
	}
	if snap.Checkpoint || snap.HumanRequired {
		reason := snap.HumanReason
		if reason == "" && snap.CurrentURL != "" {
			reason = snap.CurrentURL
		}
		return true, reason, nil
	}
	return false, "", nil
}
