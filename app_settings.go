// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Settings Management - Extracted from app.go for organization
// Session 87: Domain splitting

package main

import (
	"encoding/json"
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
