// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Navigation & Bookmarks - Extracted from app.go for organization
// Session 87: Domain splitting

package main

import (
	"fmt"
	"log"
)

// Navigation Functions

func (a *App) Navigate(scid string) map[string]interface{} {
	// Accepts raw SCID or dero://name and resolves via Gnomon when needed
	input := scid
	resolved := input

	// If input looks like dero://<identifier>, strip scheme and try dURL first
	if len(input) > 7 && (input[:7] == "dero://") {
		name := input[7:]
		if cached, ok := a.getCachedDURLMapping(name); ok {
			resolved = cached
			a.logToConsole(fmt.Sprintf("🔎 Resolved dero://%s → %s (cache)", name, cached))

			// Refresh mapping in the background when possible
			if a.gnomonClient != nil && a.gnomonClient.IsRunning() {
				go func(n string) {
					if sc, ok := a.gnomonClient.ResolveDURL(n); ok {
						a.cacheDURLMapping(n, sc)
						return
					}
					if sc, ok := a.gnomonClient.ResolveName(n); ok {
						a.cacheDURLMapping(n, sc)
					}
				}(name)
			}
		} else if a.gnomonClient != nil && a.gnomonClient.IsRunning() {
			// Prefer exact dURL match
			if sc, ok := a.gnomonClient.ResolveDURL(name); ok {
				resolved = sc
				a.cacheDURLMapping(name, sc)
				a.logToConsole(fmt.Sprintf("🔎 Resolved dero://%s → %s", name, sc))
			} else if sc, ok := a.gnomonClient.ResolveName(name); ok {
				resolved = sc
				a.cacheDURLMapping(name, sc)
				a.logToConsole(fmt.Sprintf("🔎 Resolved name dero://%s → %s", name, sc))
			} else {
				a.logToConsole(fmt.Sprintf("[WARN]  Could not resolve dero://%s via Gnomon (name or dURL)", name))
			}
		} else {
			a.logToConsole("[WARN]  Gnomon not running; cannot resolve dero:// names")
		}
	}

	log.Printf("[LINK] Navigating to: %s", resolved)

	// Add to history (store user input and resolved target)
	a.history = append(a.history, resolved)

	return map[string]interface{}{
		"success": true,
		"scid":    resolved,
		"input":   input,
		"message": "Navigation initiated",
	}
}

func (a *App) GoBack() map[string]interface{} {
	log.Println("⬅️ Go back")
	return map[string]interface{}{"success": true, "message": "Back navigation"}
}

func (a *App) GoForward() map[string]interface{} {
	log.Println("➡️ Go forward")
	return map[string]interface{}{"success": true, "message": "Forward navigation"}
}

func (a *App) Reload() map[string]interface{} {
	log.Println("[SYNC] Reload page")
	return map[string]interface{}{"success": true, "message": "Page reload"}
}

// History Functions

func (a *App) GetHistory() []string {
	return a.history
}

func (a *App) ClearHistory() map[string]interface{} {
	a.history = make([]string, 0)
	return map[string]interface{}{
		"success": true,
		"message": "History cleared",
	}
}

