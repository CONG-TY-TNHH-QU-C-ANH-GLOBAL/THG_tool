package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	defaultServer := os.Getenv("THG_SERVER_URL")
	if defaultServer == "" {
		defaultServer = "https://sale.thgfulfill.com"
	}

	serverFlag := flag.String("server", defaultServer, "THG server URL")
	pairFlag := flag.String("pair", "", "one-time pairing code from the dashboard")
	resetFlag := flag.Bool("reset", false, "remove saved connector token and pair again")
	onceFlag := flag.Bool("once", false, "send one heartbeat then exit")
	noChromeFlag := flag.Bool("no-chrome", false, "only report connector heartbeat; do not open or inspect local Chrome")
	chromePortFlag := flag.Int("chrome-port", 9222, "local Chrome DevTools port")
	flag.Parse()

	serverURL := normalizeServerURL(*serverFlag)
	configPath := connectorConfigPath()

	fmt.Println("==================================================")
	fmt.Println("        THG LOCAL CONNECTOR")
	fmt.Println("==================================================")
	fmt.Println("Local login first: THG opens a Chrome profile on this device for Facebook login/checkpoint.")
	fmt.Println("After Facebook is ready, the Browser dashboard observes and controls automation through this Runtime.")
	fmt.Println("When Facebook login succeeds, the local Chrome window is moved away so the dashboard becomes the main workspace.")
	fmt.Println("THG does not ask for your Facebook password or upload your Facebook password.")
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
		cfg = mustPairAndSave(serverURL, configPath, strings.TrimSpace(*pairFlag), "Enter pairing code from Browser workspace: ")
	} else if cfg.ServerURL != "" {
		serverURL = normalizeServerURL(cfg.ServerURL)
		fmt.Printf("Using saved connector: %s (device #%d)\n\n", cfg.ConnectorName, cfg.ConnectorID)
	}

	if err := sendHeartbeat(serverURL, cfg.DeviceToken, chromeSnapshot{Status: streamStatusConnectorOnline}); err != nil {
		if isDeviceTokenRejected(err) {
			printDeviceTokenRejected(err)
			if removeErr := os.Remove(configPath); removeErr != nil && !os.IsNotExist(removeErr) {
				exitWithError("Could not reset rejected connector config", removeErr)
			}
			fmt.Println("Old connector config removed. Create a new pairing code in the Browser dashboard.")
			cfg = mustPairAndSave(serverURL, configPath, strings.TrimSpace(*pairFlag), "Enter new pairing code: ")
			if err := sendHeartbeat(serverURL, cfg.DeviceToken, chromeSnapshot{Status: streamStatusConnectorOnline}); err != nil {
				exitWithError("Heartbeat failed after re-pairing", err)
			}
		} else {
			fmt.Println("[warn] initial heartbeat failed:", err)
		}
	}
	fmt.Println("Connector is online. You can return to the dashboard Browser tab.")
	if *onceFlag {
		return
	}
	if *noChromeFlag {
		runHeartbeatLoop(serverURL, cfg.DeviceToken, nil)
		return
	}
	runConnectorLoop(serverURL, cfg.DeviceToken, *chromePortFlag)
}
