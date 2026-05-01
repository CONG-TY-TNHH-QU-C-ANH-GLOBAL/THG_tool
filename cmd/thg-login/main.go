package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

const capabilitiesJSON = `{"local_chrome":true,"browser_control":"user_device","screen_capture":"planned","extension_bridge":"planned"}`

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

func main() {
	defaultServer := os.Getenv("THG_SERVER_URL")
	if defaultServer == "" {
		defaultServer = "https://sale.thgfulfill.com"
	}

	serverFlag := flag.String("server", defaultServer, "THG server URL")
	pairFlag := flag.String("pair", "", "one-time pairing code from the dashboard")
	resetFlag := flag.Bool("reset", false, "remove saved connector token and pair again")
	onceFlag := flag.Bool("once", false, "send one heartbeat then exit")
	flag.Parse()

	serverURL := normalizeServerURL(*serverFlag)
	configPath := connectorConfigPath()

	fmt.Println("==================================================")
	fmt.Println("        THG LOCAL CONNECTOR")
	fmt.Println("==================================================")
	fmt.Println("This app pairs your real desktop Chrome with THG.")
	fmt.Println("Keep it running while the dashboard automation is active.")
	fmt.Println()
	fmt.Println("Server:", serverURL)
	fmt.Println("Config:", configPath)
	fmt.Println()

	if *resetFlag {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			exitWithError("Could not reset connector config", err)
		}
		fmt.Println("Saved connector token removed. Pairing will start again.")
	}

	cfg, err := loadConnectorConfig(configPath)
	if err != nil {
		exitWithError("Could not read connector config", err)
	}
	if cfg.DeviceToken == "" {
		code := strings.TrimSpace(*pairFlag)
		if code == "" {
			code = promptLine("Enter pairing code from Browser workspace: ")
		}
		if code == "" {
			exitWithError("Pairing code is required", nil)
		}
		paired, err := pairConnector(serverURL, code)
		if err != nil {
			exitWithError("Pairing failed", err)
		}
		cfg = connectorConfig{
			ServerURL:     serverURL,
			DeviceToken:   paired.DeviceToken,
			ConnectorID:   paired.Connector.ID,
			ConnectorName: paired.Connector.Name,
			WSPath:        defaultString(paired.WSPath, "/ws/agent"),
			APIBase:       defaultString(paired.APIBase, "/api"),
			PairedAt:      time.Now().UTC(),
		}
		if err := saveConnectorConfig(configPath, cfg); err != nil {
			exitWithError("Could not save connector token", err)
		}
		fmt.Printf("Paired: %s (device #%d)\n", cfg.ConnectorName, cfg.ConnectorID)
		fmt.Println("The dashboard will use the saved device token from now on.")
		fmt.Println()
	} else if cfg.ServerURL != "" {
		serverURL = normalizeServerURL(cfg.ServerURL)
		fmt.Printf("Using saved connector: %s (device #%d)\n\n", cfg.ConnectorName, cfg.ConnectorID)
	}

	if err := sendHeartbeat(serverURL, cfg.DeviceToken); err != nil {
		exitWithError("Heartbeat failed", err)
	}
	fmt.Println("Connector is online. You can return to the dashboard Browser tab.")
	if *onceFlag {
		return
	}
	runHeartbeatLoop(serverURL, cfg.DeviceToken)
}

func normalizeServerURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "https://sale.thgfulfill.com"
	}
	return strings.TrimRight(value, "/")
}

func promptLine(label string) string {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func connectorConfigPath() string {
	root, err := os.UserConfigDir()
	if err != nil || root == "" {
		root = "."
	}
	return filepath.Join(root, "THG Local Connector", "config.json")
}

func loadConnectorConfig(path string) (connectorConfig, error) {
	var cfg connectorConfig
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func saveConnectorConfig(path string, cfg connectorConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func pairConnector(serverURL, code string) (*pairResponse, error) {
	hostname, _ := os.Hostname()
	body, _ := json.Marshal(map[string]any{
		"code":              code,
		"hostname":          hostname,
		"os":                runtime.GOOS + "/" + runtime.GOARCH,
		"version":           version,
		"capabilities_json": capabilitiesJSON,
		"stream_status":     "pairing",
	})
	resp, err := http.Post(serverURL+"/api/connectors/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out pairResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.DeviceToken) == "" {
		return nil, fmt.Errorf("server did not return a device token")
	}
	return &out, nil
}

func sendHeartbeat(serverURL, token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing saved device token; run with --reset and pair again")
	}
	hostname, _ := os.Hostname()
	body, _ := json.Marshal(map[string]any{
		"hostname":          hostname,
		"os":                runtime.GOOS + "/" + runtime.GOARCH,
		"version":           version,
		"kind":              "desktop_connector",
		"transport":         "local_chrome",
		"capabilities_json": capabilitiesJSON,
		"stream_status":     "online",
	})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/agent/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("device token was rejected; run with --reset and pair again")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func runHeartbeatLoop(serverURL, token string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	for {
		select {
		case <-ticker.C:
			if err := sendHeartbeat(serverURL, token); err != nil {
				fmt.Println("[warn] heartbeat failed:", err)
				continue
			}
			fmt.Println("heartbeat ok", time.Now().Format("15:04:05"))
		case <-stop:
			fmt.Println()
			fmt.Println("Connector stopped.")
			return
		}
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func exitWithError(message string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(1)
}
