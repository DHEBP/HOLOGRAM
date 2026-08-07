// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Navigation & History - Extracted from app.go for organization
// Session 87: Domain splitting

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/civilware/tela"
)

// pickServableSCID chooses which deployment of a dURL to open.
//
// A name maps to many SCIDs, ranked best-first by Gnomon interaction height.
// That ranking has no idea whether a contract can actually be served: it sends
// telatomicswaps.tela to a V5.3 build that does not match TELA-INDEX-1, so the
// browser drops to srcdoc with limited wallet and script access, while the
// publisher's conforming V5.6 sits one entry away under the same name.
//
// So walk the ranking and take the first contract that validates. If none do,
// return the top candidate unchanged - this can improve the choice, never
// worsen it. servable is injected so the decision is testable without a daemon.
//
// Cost is one extra validation call in the common case, because the top
// candidate usually validates and the walk stops there.
func pickServableSCID(candidates []string, servable func(string) bool) (scid string, skipped int, ok bool) {
	if len(candidates) == 0 {
		return "", 0, false
	}

	for i, c := range candidates {
		if servable(c) {
			return c, i, true
		}
	}

	return candidates[0], 0, true
}

// resolveServableDURL turns a dURL into the SCID HOLOGRAM will actually serve.
//
// Use this everywhere a dURL becomes a SCID. There are five such places, and
// when only the navigation one preferred a servable contract the browser
// resolved telatomicswaps.tela to a conforming deployment and then fetched the
// non-conforming one anyway, because FetchByDURL resolved a second time on its
// own. A name has to mean the same contract in every path.
func (a *App) resolveServableDURL(name string) (string, bool) {
	if a.gnomonClient == nil || !a.gnomonClient.IsRunning() {
		return "", false
	}

	endpoint := a.telaEndpoint()
	scid, skipped, ok := pickServableSCID(a.gnomonClient.ResolveDURLAll(name), func(c string) bool {
		_, err := tela.GetINDEXInfo(c, endpoint)
		return err == nil
	})
	if !ok {
		return "", false
	}

	if skipped > 0 {
		a.logToConsole(fmt.Sprintf("[Search] dero://%s → %s (skipped %d deployment(s) that do not parse as TELA-INDEX-1)", name, scid, skipped))
	}
	return scid, true
}

// Navigation Functions

func (a *App) Navigate(scid string) map[string]interface{} {
	// Accepts raw SCID or dero://name and resolves via Gnomon when needed
	input := scid
	resolved := input

	// If input looks like dero://<identifier>, strip scheme and try dURL first
	if len(input) > 7 && (input[:7] == "dero://") {
		name := input[7:]
		// Prefer live Gnomon resolution first so stale cache entries do not win.
		if a.gnomonClient != nil && a.gnomonClient.IsRunning() {
			if sc, ok := a.resolveServableDURL(name); ok {
				resolved = sc
				a.cacheDURLMapping(name, sc)
				a.logToConsole(fmt.Sprintf("[Search] Resolved dero://%s → %s", name, sc))
			} else if sc, ok := a.gnomonClient.ResolveName(name); ok {
				resolved = sc
				a.cacheDURLMapping(name, sc)
				a.logToConsole(fmt.Sprintf("[Search] Resolved name dero://%s → %s", name, sc))
			} else if cached, ok := a.getCachedDURLMapping(name); ok {
				resolved = cached
				a.logToConsole(fmt.Sprintf("[Search] Resolved dero://%s → %s (cache fallback)", name, cached))
			} else {
				a.logToConsole(fmt.Sprintf("[WARN]  Could not resolve dero://%s via Gnomon (name or dURL)", name))
			}
		} else if cached, ok := a.getCachedDURLMapping(name); ok {
			resolved = cached
			a.logToConsole(fmt.Sprintf("[Search] Resolved dero://%s → %s (cache)", name, cached))
		} else {
			a.logToConsole("[WARN]  Gnomon not running and no cached dURL mapping available")
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

func (a *App) captureLaunchURLFromArgs(args []string) {
	for _, raw := range args {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "dero://") {
			a.launchURLMu.Lock()
			a.launchURL = trimmed
			a.launchURLMu.Unlock()
			a.logToConsole(fmt.Sprintf("[LINK] Startup deep link captured: %s", trimmed))
			return
		}
	}
}

func (a *App) ConsumeLaunchURL() string {
	a.launchURLMu.Lock()
	defer a.launchURLMu.Unlock()

	url := strings.TrimSpace(a.launchURL)
	a.launchURL = ""
	return url
}

func (a *App) GoBack() map[string]interface{} {
	log.Println("⬅️ Go back")
	return map[string]interface{}{"success": true, "message": "Back navigation"}
}

func (a *App) GoForward() map[string]interface{} {
	log.Println("[Nav] Go forward")
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
