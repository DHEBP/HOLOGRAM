// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Settings Management - Extracted from app.go for organization
// Session 87: Domain splitting

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Settings that should be persisted to disk
// Not all settings need persistence - only user-configured values
var persistedSettingKeys = []string{
	"daemon_endpoint",
	"network",
	"min_rating",
	"block_malware",
	"show_nsfw",
	"auto_connect_ws",
	"gnomon_enabled",
	"integrated_wallet",
	"allow_github_check",
	"wizard_complete",
	"dev_support_enabled",
	"epoch_enabled",
}

// Settings Functions

func (a *App) GetSetting(key string) interface{} {
	if val, ok := a.settings[key]; ok {
		return val
	}
	return nil
}

// GetAllSettings returns all settings for frontend sync
func (a *App) GetAllSettings() map[string]interface{} {
	return a.settings
}

func (a *App) SetSetting(settingJSON string) map[string]interface{} {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(settingJSON), &data); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
	}

	for k, v := range data {
		a.settings[k] = v
		log.Printf("[Settings] Updated: %s = %v", k, v)
	}

	// Persist settings to disk
	a.saveSettings()

	return map[string]interface{}{
		"success": true,
		"message": "Settings updated",
	}
}

// saveSettings persists user-configured settings to disk
// Settings are saved to ~/.dero/hologram/datashards/settings/settings.json
func (a *App) saveSettings() {
	configDir := filepath.Join(getDatashardsDir(), "settings")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Printf("[Settings] Failed to create settings directory: %v", err)
		return
	}

	// Only persist specific settings, not all in-memory values
	toSave := make(map[string]interface{})
	for _, key := range persistedSettingKeys {
		if val, ok := a.settings[key]; ok {
			toSave[key] = val
		}
	}

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		log.Printf("[Settings] Failed to marshal settings: %v", err)
		return
	}

	settingsFile := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(settingsFile, data, 0600); err != nil {
		log.Printf("[Settings] Failed to save settings: %v", err)
	} else {
		log.Printf("[Settings] Saved settings to %s", settingsFile)
	}
}

// loadSettings loads persisted settings from disk and merges with defaults
// Call this during app startup after defaults are set
func (a *App) loadSettings() {
	settingsFile := filepath.Join(getDatashardsDir(), "settings", "settings.json")
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		// No settings file yet - this is normal on first run
		if !os.IsNotExist(err) {
			log.Printf("[Settings] Failed to read settings file: %v", err)
		}
		return
	}

	var loaded map[string]interface{}
	if err := json.Unmarshal(data, &loaded); err != nil {
		log.Printf("[Settings] Failed to parse settings file: %v", err)
		return
	}

	// Merge loaded settings into current settings (overwriting defaults)
	for key, val := range loaded {
		a.settings[key] = val
		log.Printf("[Settings] Loaded from disk: %s = %v", key, val)
	}

	log.Printf("[Settings] Loaded %d settings from %s", len(loaded), settingsFile)
}

// reconcileDaemonEndpoint ensures daemon_endpoint, daemonClient, and network are
// consistent after loading persisted settings. This handles the case where a user
// was previously on simulator (port 20000) but is now restarting on mainnet — the
// persisted daemon_endpoint would be stale and cause Gnomon/wallet/EPOCH to fail.
func (a *App) reconcileDaemonEndpoint() {
	loadedEndpoint, _ := a.settings["daemon_endpoint"].(string)
	loadedNetwork, _ := a.settings["network"].(string)

	// Step 1: Sync daemonClient with the loaded endpoint so the connection test
	// hits whatever the user had configured (not the hardcoded default).
	if loadedEndpoint != "" {
		a.daemonClient.SetEndpoint(loadedEndpoint)
	}

	// Step 2: Try to reach the daemon at the loaded endpoint.
	// If it responds, check chain height to infer the actual network.
	if err := a.daemonClient.TestConnection(); err == nil {
		info, infoErr := a.daemonClient.GetInfo()
		if infoErr == nil {
			if h, ok := info["height"].(float64); ok {
				chainHeight := int64(h)
				var inferredNetwork string
				if chainHeight > 10000 {
					inferredNetwork = "mainnet"
				} else if chainHeight > 0 {
					inferredNetwork = "simulator"
				}

				if inferredNetwork != "" && inferredNetwork != loadedNetwork {
					netConfig := GetNetworkConfig(NetworkMode(inferredNetwork))
					correctEndpoint := fmt.Sprintf("http://127.0.0.1:%d", netConfig.RPCPort)

					log.Printf("[Settings] Network reconciliation: persisted=%s, detected=%s — correcting endpoint %s → %s",
						loadedNetwork, inferredNetwork, loadedEndpoint, correctEndpoint)

					a.settings["network"] = inferredNetwork
					a.settings["daemon_endpoint"] = correctEndpoint
					a.daemonClient.SetEndpoint(correctEndpoint)

					nodeManager.Lock()
					nodeManager.networkMode = NetworkMode(inferredNetwork)
					nodeManager.rpcPort = netConfig.RPCPort
					nodeManager.p2pPort = netConfig.P2PPort
					nodeManager.getworkPort = netConfig.GetWorkPort
					nodeManager.Unlock()

					a.saveSettings()
					return
				}
			}
		}
	}

	// Step 3: Loaded endpoint is unreachable.
	// If persisted network is simulator, the daemon was likely a child process
	// from a previous session that is no longer running. Try falling back to
	// mainnet so the user isn't stuck on a dead endpoint.
	if loadedNetwork == "simulator" {
		mainnetConfig := GetNetworkConfig(NetworkMainnet)
		mainnetEndpoint := fmt.Sprintf("http://127.0.0.1:%d", mainnetConfig.RPCPort)

		a.daemonClient.SetEndpoint(mainnetEndpoint)
		if err := a.daemonClient.TestConnection(); err == nil {
			log.Printf("[Settings] Simulator unreachable — falling back to mainnet at %s", mainnetEndpoint)

			a.settings["network"] = "mainnet"
			a.settings["daemon_endpoint"] = mainnetEndpoint

			nodeManager.Lock()
			nodeManager.networkMode = NetworkMainnet
			nodeManager.rpcPort = mainnetConfig.RPCPort
			nodeManager.p2pPort = mainnetConfig.P2PPort
			nodeManager.getworkPort = mainnetConfig.GetWorkPort
			nodeManager.Unlock()

			a.saveSettings()
			return
		}
		// Neither simulator nor mainnet reachable — restore the loaded endpoint
		// so settings stay internally consistent.
		a.daemonClient.SetEndpoint(loadedEndpoint)
	}

	// For any network: if the endpoint doesn't match the persisted network's
	// expected port, correct the endpoint.
	if loadedNetwork != "" {
		netConfig := GetNetworkConfig(NetworkMode(loadedNetwork))
		expectedEndpoint := fmt.Sprintf("http://127.0.0.1:%d", netConfig.RPCPort)

		if loadedEndpoint != expectedEndpoint {
			log.Printf("[Settings] Endpoint/network mismatch: endpoint=%s but network=%s (expected %s) — correcting",
				loadedEndpoint, loadedNetwork, expectedEndpoint)

			a.settings["daemon_endpoint"] = expectedEndpoint
			a.daemonClient.SetEndpoint(expectedEndpoint)

			nodeManager.Lock()
			nodeManager.networkMode = NetworkMode(loadedNetwork)
			nodeManager.rpcPort = netConfig.RPCPort
			nodeManager.p2pPort = netConfig.P2PPort
			nodeManager.getworkPort = netConfig.GetWorkPort
			nodeManager.Unlock()

			a.saveSettings()
		}
	}
}
