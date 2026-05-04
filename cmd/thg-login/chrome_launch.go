package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
)

func chromeWebSocketURL(devtoolsURL string) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(devtoolsURL + "/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Chrome DevTools returned %d", resp.StatusCode)
	}
	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.WebSocketDebuggerURL) == "" {
		return "", fmt.Errorf("Chrome DevTools URL is empty")
	}
	return payload.WebSocketDebuggerURL, nil
}

func waitChromeWebSocketURL(devtoolsURL string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		wsURL, err := chromeWebSocketURL(devtoolsURL)
		if err == nil {
			return wsURL, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Chrome DevTools did not become ready")
	}
	return "", lastErr
}

func launchChrome(port int, userDataDir string) (int, error) {
	chromePath, err := findChromePath()
	if err != nil {
		return 0, err
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-save-password-bubble",
		"--disable-features=PasswordManagerOnboarding,PasswordManagerEnableSaving,PasswordLeakDetection",
		"--force-device-scale-factor=1",
		"--high-dpi-support=1",
		"--window-size=1365,900",
		"https://www.facebook.com",
	}
	if shouldHideChromeWindow() {
		args = append([]string{"--window-position=-32000,-32000"}, args...)
	}
	if userDataDir != "" {
		if err := os.MkdirAll(userDataDir, 0700); err != nil {
			return 0, err
		}
		if err := configureChromeProfile(userDataDir); err != nil {
			fmt.Printf("[warn] could not write Chrome profile preferences: %v\n", err)
		}
		args = append([]string{fmt.Sprintf("--user-data-dir=%s", userDataDir)}, args...)
	}
	cmd := exec.Command(chromePath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	if cmd.Process == nil {
		return 0, nil
	}
	return cmd.Process.Pid, nil
}

func configureChromeProfile(userDataDir string) error {
	defaultDir := filepath.Join(userDataDir, "Default")
	if err := os.MkdirAll(defaultDir, 0700); err != nil {
		return err
	}
	prefsPath := filepath.Join(defaultDir, "Preferences")
	prefs := map[string]any{}
	if data, err := os.ReadFile(prefsPath); err == nil && len(bytes.TrimSpace(data)) > 0 {
		_ = json.Unmarshal(data, &prefs)
	}
	prefs["credentials_enable_service"] = false
	profile, _ := prefs["profile"].(map[string]any)
	if profile == nil {
		profile = map[string]any{}
	}
	profile["password_manager_enabled"] = false
	prefs["profile"] = profile
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(prefsPath, data, 0600)
}

func shouldHideChromeWindow() bool {
	visible := strings.TrimSpace(strings.ToLower(os.Getenv("THG_CHROME_VISIBLE")))
	headlessStream := strings.TrimSpace(strings.ToLower(os.Getenv("THG_CHROME_HEADLESS_STREAM")))
	return visible == "0" || visible == "false" || visible == "no" || visible == "off" ||
		headlessStream == "1" || headlessStream == "true" || headlessStream == "yes" || headlessStream == "on"
}

func keepLocalChromeVisibleAfterLogin() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("THG_KEEP_CHROME_VISIBLE_AFTER_LOGIN")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func hideChromeWindowAfterLogin(ctx context.Context, pid int) error {
	var failures []string
	hidden := false
	windowID, _, err := cdpbrowser.GetWindowForTarget().Do(ctx)
	if err == nil {
		if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateMinimized}).Do(ctx); err == nil {
			hidden = true
		} else {
			failures = append(failures, "cdp minimize: "+err.Error())
		}
		_ = cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateNormal}).Do(ctx)
		if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{
			Left:   -32000,
			Top:    -32000,
			Width:  1365,
			Height: 900,
		}).Do(ctx); err == nil {
			hidden = true
		} else {
			failures = append(failures, "cdp offscreen: "+err.Error())
		}
	} else {
		failures = append(failures, "cdp window: "+err.Error())
	}
	if err := hideLocalChromeProcessWindow(pid); err == nil {
		hidden = true
	} else if pid > 0 {
		failures = append(failures, "native hide: "+err.Error())
	}
	if hidden {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(failures, "; "))
}

func showChromeWindowForLogin(ctx context.Context, pid int) error {
	var failures []string
	shown := false
	if err := showLocalChromeProcessWindow(pid); err == nil {
		shown = true
	} else if pid > 0 {
		failures = append(failures, "native show: "+err.Error())
	}
	windowID, _, err := cdpbrowser.GetWindowForTarget().Do(ctx)
	if err != nil {
		if shown {
			return nil
		}
		return err
	}
	if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateNormal}).Do(ctx); err == nil {
		shown = true
	} else {
		failures = append(failures, "cdp normal: "+err.Error())
	}
	if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{
		Left:   80,
		Top:    60,
		Width:  1365,
		Height: 900,
	}).Do(ctx); err == nil {
		shown = true
	} else {
		failures = append(failures, "cdp bounds: "+err.Error())
	}
	if shown {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(failures, "; "))
}

func findChromePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CHROME_PATH")); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	candidates := chromePathCandidates()
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, string(os.PathSeparator)) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("Google Chrome was not found")
}

func chromePathCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
			"chrome.exe",
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"google-chrome",
			"chromium",
		}
	default:
		return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
	}
}

func chromeUserDataDir(accountID int64) string {
	if dir := strings.TrimSpace(os.Getenv("THG_CHROME_USER_DATA_DIR")); dir != "" {
		return dir
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	name := "account-" + strconv.FormatInt(accountID, 10)
	if accountID <= 0 {
		name = "default"
	}
	return filepath.Join(root, "THG Local Connector", "chrome-profiles", name)
}
