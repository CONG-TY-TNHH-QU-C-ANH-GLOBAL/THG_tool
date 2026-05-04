package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func mustPairAndSave(serverURL, configPath, code, prompt string) connectorConfig {
	code = strings.TrimSpace(code)
	if code == "" {
		code = promptLine(prompt)
	}
	if code == "" {
		exitWithError("Pairing code is required", nil)
	}
	paired, err := pairConnector(serverURL, code)
	if err != nil {
		exitWithError("Pairing failed", err)
	}
	cfg := connectorConfig{
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
	return cfg
}

func normalizeServerURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "https://sale.thgfulfill.com"
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		exitWithError("Invalid THG server URL", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" {
		host := strings.ToLower(parsed.Hostname())
		if scheme != "http" || !isLocalDevelopmentHost(host) {
			exitWithError("THG Local Runtime requires HTTPS for production server URLs. HTTP is only allowed for localhost development.", nil)
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func isLocalDevelopmentHost(host string) bool {
	switch strings.Trim(strings.ToLower(host), "[]") {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
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
