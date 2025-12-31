// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Settings Management - Extracted from app.go for organization
// Session 87: Domain splitting

package main

import (
	"encoding/json"
	"log"
)

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

	return map[string]interface{}{
		"success": true,
		"message": "Settings updated",
	}
}

