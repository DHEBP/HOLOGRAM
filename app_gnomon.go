// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Gnomon & App Discovery - Extracted from app.go for organization
// Session 87: Domain splitting

package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// Gnomon Functions

func (a *App) StartGnomon() map[string]interface{} {
	a.logToConsole("[START] Starting Gnomon indexer...")

	endpoint := "http://127.0.0.1:10102"
	if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		endpoint = ep
	}

	network := "mainnet"
	if net, ok := a.settings["network"].(string); ok && net != "" {
		network = net
	}

	err := a.gnomonClient.Start(endpoint, network)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Gnomon start failed: %v", err))
		return ErrorResponse(err)
	}

	a.logToConsole("[OK] Gnomon indexer started successfully")
	a.settings["gnomon_enabled"] = true

	return map[string]interface{}{
		"success": true,
		"message": "Gnomon indexer started",
	}
}

func (a *App) StopGnomon() map[string]interface{} {
	a.logToConsole("[STOP] Stopping Gnomon indexer...")

	a.gnomonClient.Stop()
	a.settings["gnomon_enabled"] = false

	a.logToConsole("[OK] Gnomon indexer stopped")

	return map[string]interface{}{
		"success": true,
		"message": "Gnomon indexer stopped",
	}
}

func (a *App) GetGnomonStatus() map[string]interface{} {
	status := a.gnomonClient.GetStatus()
	return map[string]interface{}{
		"success": true,
		"status":  status,
	}
}

// SetGnomonAutostart enables or disables automatic Gnomon startup
func (a *App) SetGnomonAutostart(enabled bool) map[string]interface{} {
	a.settings["gnomon_autostart"] = enabled

	if enabled {
		a.logToConsole("[GNOMON] Auto-start enabled - Gnomon will start automatically on app launch")
	} else {
		a.logToConsole("[GNOMON] Auto-start disabled")
	}

	return map[string]interface{}{
		"success": true,
		"enabled": enabled,
		"message": fmt.Sprintf("Gnomon auto-start %s", map[bool]string{true: "enabled", false: "disabled"}[enabled]),
	}
}

// GetGnomonAutostart returns the current auto-start setting
func (a *App) GetGnomonAutostart() bool {
	if autostart, ok := a.settings["gnomon_autostart"].(bool); ok {
		return autostart
	}
	return false
}

// SearchByKey searches all indexed SCIDs for those containing a specific key
func (a *App) SearchByKey(key string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[...] Searching by key: %s", key))

	if !a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success": false,
			"error":   "Gnomon is not running. Start it in Settings to search.",
			"results": []map[string]interface{}{},
		}
	}

	if key == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Key cannot be empty",
			"results": []map[string]interface{}{},
		}
	}

	results := a.gnomonClient.SearchByKey(key)
	a.logToConsole(fmt.Sprintf("[OK] Found %d SCIDs containing key '%s'", len(results), key))

	return map[string]interface{}{
		"success": true,
		"key":     key,
		"results": results,
		"count":   len(results),
	}
}

// SearchByValue searches all indexed SCIDs for those containing a specific value
func (a *App) SearchByValue(value string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[...] Searching by value: %s", value))

	if !a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success": false,
			"error":   "Gnomon is not running. Start it in Settings to search.",
			"results": []map[string]interface{}{},
		}
	}

	if value == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Value cannot be empty",
			"results": []map[string]interface{}{},
		}
	}

	results := a.gnomonClient.SearchByValue(value)
	a.logToConsole(fmt.Sprintf("[OK] Found %d SCIDs containing value '%s'", len(results), value))

	return map[string]interface{}{
		"success": true,
		"value":   value,
		"results": results,
		"count":   len(results),
	}
}

// SearchCodeLine searches all indexed smart contracts for a specific line of code
func (a *App) SearchCodeLine(line string) map[string]interface{} {
	result := a.searchCodeLineWrapper("code:" + line)

	if result.Success {
		return map[string]interface{}{
			"success": true,
			"line":    line,
			"results": result.Data["results"],
			"count":   result.Data["count"],
		}
	}

	return map[string]interface{}{
		"success": false,
		"error":   result.Error,
		"results": []map[string]interface{}{},
	}
}

// CleanGnomonDB deletes the Gnomon database for a specific network
func (a *App) CleanGnomonDB(network string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("🗑️ Cleaning Gnomon DB for network: %s", network))

	if a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success": false,
			"error":   "Gnomon must be stopped before cleaning the database. Stop it first.",
		}
	}

	if network == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Network must be specified (mainnet, testnet, or simulator)",
		}
	}

	validNetworks := []string{"mainnet", "testnet", "simulator"}
	isValid := false
	for _, n := range validNetworks {
		if n == network {
			isValid = true
			break
		}
	}
	if !isValid {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid network. Must be mainnet, testnet, or simulator",
		}
	}

	err := a.gnomonClient.CleanDB(network)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] CleanGnomonDB failed: %v", err))
		return ErrorResponse(err)
	}

	a.logToConsole(fmt.Sprintf("[OK] Gnomon DB cleaned for %s", network))

	return map[string]interface{}{
		"success": true,
		"network": network,
		"message": fmt.Sprintf("Gnomon database for %s has been deleted. Restart Gnomon to re-sync.", network),
	}
}

// ResyncGnomon stops Gnomon, cleans the DB, and restarts it
func (a *App) ResyncGnomon() map[string]interface{} {
	network := a.getNetworkName()
	a.logToConsole(fmt.Sprintf("🔄 Resyncing Gnomon for %s...", network))

	if a.gnomonClient.IsRunning() {
		a.StopGnomon()
	}

	err := a.gnomonClient.CleanDB(network)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[WARN] Could not clean DB: %v", err))
	}

	result := a.StartGnomon()
	if success, ok := result["success"].(bool); ok && success {
		return map[string]interface{}{
			"success": true,
			"network": network,
			"message": fmt.Sprintf("Gnomon resync started for %s", network),
		}
	}

	return result
}

// getNetworkName returns the current network name
func (a *App) getNetworkName() string {
	if a.IsInSimulatorMode() {
		return "simulator"
	}
	if networkMode, ok := a.settings["network_mode"].(string); ok {
		return networkMode
	}
	return "mainnet"
}

// EnsureGnomonRunning starts Gnomon if not already running
func (a *App) EnsureGnomonRunning() map[string]interface{} {
	if a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success":        true,
			"alreadyRunning": true,
			"message":        "Gnomon is already running",
		}
	}

	a.logToConsole("[NET] Lazy-starting Gnomon indexer (on demand)...")
	return a.StartGnomon()
}

func (a *App) GetDiscoveredApps() map[string]interface{} {
	if !a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success": false,
			"error":   "Gnomon is not running",
			"apps":    []map[string]interface{}{},
		}
	}

	apps := a.gnomonClient.GetTELAApps()

	epochCount := 0

	for i, app := range apps {
		if scid, ok := app["scid"].(string); ok && scid != "" {
			rating, err := a.gnomonClient.GetRating(scid)
			if err == nil && rating != nil {
				apps[i]["rating"] = map[string]interface{}{
					"average":  rating.Average,
					"count":    rating.Count,
					"likes":    rating.Likes,
					"dislikes": rating.Dislikes,
				}
			}

			supportsEpoch := a.gnomonClient.CheckAppSupportsEpoch(scid)
			apps[i]["supports_epoch"] = supportsEpoch
			if supportsEpoch {
				apps[i]["epoch_badge"] = "💎 Supports Ecosystem"
				epochCount++
			}
		}
	}

	a.logToConsole(fmt.Sprintf("📱 Found %d TELA apps (%d with EPOCH support)", len(apps), epochCount))

	return map[string]interface{}{
		"success": true,
		"apps":    apps,
		"count":   len(apps),
	}
}

// GetTELALibraries returns all TELA content tagged as libraries
func (a *App) GetTELALibraries() map[string]interface{} {
	if !a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success":   false,
			"error":     "Gnomon is not running. Start it in Settings to discover libraries.",
			"libraries": []map[string]interface{}{},
		}
	}

	libs := a.gnomonClient.GetTELALibraries()

	for i, lib := range libs {
		if scid, ok := lib["scid"].(string); ok && scid != "" {
			rating, err := a.gnomonClient.GetRating(scid)
			if err == nil && rating != nil {
				libs[i]["rating"] = map[string]interface{}{
					"average":  rating.Average,
					"count":    rating.Count,
					"likes":    rating.Likes,
					"dislikes": rating.Dislikes,
				}
			}
		}
	}

	a.logToConsole(fmt.Sprintf("📚 Found %d TELA libraries", len(libs)))

	return map[string]interface{}{
		"success":   true,
		"libraries": libs,
		"count":     len(libs),
	}
}

// GetRandomSmartContracts returns a random sample of smart contracts
func (a *App) GetRandomSmartContracts(limit int) map[string]interface{} {
	if limit <= 0 {
		limit = 10
	}

	if !a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success": false,
			"error":   "Gnomon is not running. Start it in Settings to discover contracts.",
		}
	}

	allSCs := a.gnomonClient.GetAllOwnersAndSCIDs()

	if len(allSCs) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "No smart contracts found in index.",
		}
	}

	type scInfo struct {
		scid  string
		owner string
	}

	scs := make([]scInfo, 0, len(allSCs))
	for scid, owner := range allSCs {
		scs = append(scs, scInfo{scid, owner})
	}

	rand.Seed(time.Now().UnixNano())
	for i := len(scs) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		scs[i], scs[j] = scs[j], scs[i]
	}

	if len(scs) > limit {
		scs = scs[:limit]
	}

	results := make([]map[string]interface{}, len(scs))
	for i, sc := range scs {
		results[i] = map[string]interface{}{
			"scid":  sc.scid,
			"owner": sc.owner,
		}
	}

	a.logToConsole(fmt.Sprintf("🎲 Random SC discovery: Found %d contracts (from %d total)", len(results), len(allSCs)))

	return map[string]interface{}{
		"success":   true,
		"contracts": results,
		"total":     len(allSCs),
	}
}

func (a *App) SearchApps(query string) map[string]interface{} {
	if !a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success": false,
			"error":   "Gnomon is not running",
			"apps":    []map[string]interface{}{},
		}
	}

	go a.BuildTextIndex()

	ranked := a.SearchTextIndex(query)
	results := make([]map[string]interface{}, 0)

	if len(ranked) > 0 {
		appList := a.gnomonClient.GetTELAApps()
		scidToApp := make(map[string]map[string]interface{}, len(appList))
		for _, app := range appList {
			if sc, ok := app["scid"].(string); ok {
				scidToApp[sc] = app
			}
		}
		for _, sc := range ranked {
			if app, ok := scidToApp[sc]; ok {
				results = append(results, app)
			}
		}
	}

	if len(results) == 0 {
		results = a.gnomonClient.SearchTELApps(query)
	}

	a.logToConsole(fmt.Sprintf("[...] Search for '%s' returned %d results", query, len(results)))

	return map[string]interface{}{
		"success": true,
		"query":   query,
		"results": results,
		"count":   len(results),
	}
}

func (a *App) GetAppDetails(scid string) map[string]interface{} {
	if !a.gnomonClient.IsRunning() {
		return map[string]interface{}{
			"success": false,
			"error":   "Gnomon is not running",
		}
	}

	vars := a.gnomonClient.GetAllSCIDVariableDetails(scid)

	details := map[string]interface{}{
		"scid": scid,
	}

	for _, v := range vars {
		key := fmt.Sprintf("%v", v.Key)
		value := fmt.Sprintf("%v", v.Value)

		switch key {
		case "nameHdr":
			details["name"] = decodeHexString(value)
		case "descrHdr":
			details["description"] = decodeHexString(value)
		case "dURL":
			du := decodeHexString(value)
			details["url"] = du
			details["durl"] = du
		case "iconURLHdr":
			details["icon"] = decodeHexString(value)
		case "owner":
			details["owner"] = value
		}
	}

	return map[string]interface{}{
		"success": true,
		"details": details,
	}
}

// GetAppRating fetches rating data for a SCID
func (a *App) GetAppRating(scid string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[STATS] Fetching rating for: %s", scid[:16]+"..."))

	result, err := a.GetRatingResultForSCID(scid)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[WARN]  Rating fetch failed: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
	}

	badge := GetRatingBadgeHTML(result)

	category := ""
	color := "#6b7280"
	categoryNum := uint64(0)

	if result.Count > 0 {
		category, _, categoryNum, _ = ParseRating(uint64(result.Average * 10))
		color = GetRatingColor(categoryNum)
	}

	a.logToConsole(fmt.Sprintf("[OK] Rating: %.1f/10 (%d ratings)", result.Average, result.Count))

	return map[string]interface{}{
		"success":     true,
		"scid":        scid,
		"average":     result.Average,
		"count":       result.Count,
		"likes":       result.Likes,
		"dislikes":    result.Dislikes,
		"badge":       badge,
		"category":    category,
		"categoryNum": categoryNum,
		"color":       color,
		"hasRatings":  result.Count > 0,
	}
}

// GetNameSuggestions returns name suggestions for autocomplete
func (a *App) GetNameSuggestions(prefix string) map[string]interface{} {
	const maxSuggestions = 10
	out := make([]map[string]string, 0)

	if a.gnomonClient == nil || !a.gnomonClient.IsRunning() {
		return map[string]interface{}{"success": true, "suggestions": out}
	}

	p := prefix
	if len(p) > 7 && p[:7] == "dero://" {
		p = p[7:]
	}
	p = strings.TrimSpace(strings.ToLower(p))
	apps := a.gnomonClient.GetTELAApps()

	if p == "" {
		type entry struct {
			sc, du, dn, nm string
			h              int64
		}
		list := make([]entry, 0, len(apps))
		for _, app := range apps {
			scid, _ := app["scid"].(string)
			durl, _ := app["durl"].(string)
			dn, _ := app["display_name"].(string)
			nm, _ := app["name"].(string)
			h := a.gnomonClient.LatestInteractionHeight(scid)
			list = append(list, entry{sc: scid, du: durl, dn: dn, nm: nm, h: h})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].h > list[j].h })
		for i := 0; i < len(list) && len(out) < maxSuggestions; i++ {
			name := list[i].du
			if name == "" {
				if list[i].dn != "" {
					name = list[i].dn
				} else {
					name = list[i].nm
				}
			}
			if name == "" {
				continue
			}
			avg := ""
			if rr, err := a.gnomonClient.GetRating(list[i].sc); err == nil && rr != nil && rr.Count > 0 {
				avg = fmt.Sprintf("%.1f", rr.Average)
			}
			out = append(out, map[string]string{
				"name":   name,
				"scid":   list[i].sc,
				"avg":    avg,
				"height": fmt.Sprintf("%d", list[i].h),
			})
		}
		return map[string]interface{}{"success": true, "suggestions": out, "count": len(out)}
	}

	for _, app := range apps {
		scid, _ := app["scid"].(string)
		durl, _ := app["durl"].(string)
		dn, _ := app["display_name"].(string)
		n, _ := app["name"].(string)

		if durl != "" && strings.HasPrefix(strings.ToLower(durl), p) {
			avg := ""
			if rr, err := a.gnomonClient.GetRating(scid); err == nil && rr != nil && rr.Count > 0 {
				avg = fmt.Sprintf("%.1f", rr.Average)
			}
			out = append(out, map[string]string{
				"name":   durl,
				"scid":   scid,
				"avg":    avg,
				"height": fmt.Sprintf("%d", a.gnomonClient.LatestInteractionHeight(scid)),
			})
			if len(out) >= maxSuggestions {
				break
			}
			continue
		}

		cand := dn
		if cand == "" {
			cand = n
		}
		if cand == "" {
			continue
		}
		lower := strings.ToLower(cand)
		if strings.HasPrefix(lower, p) {
			avg := ""
			if rr, err := a.gnomonClient.GetRating(scid); err == nil && rr != nil && rr.Count > 0 {
				avg = fmt.Sprintf("%.1f", rr.Average)
			}
			out = append(out, map[string]string{
				"name":   cand,
				"scid":   scid,
				"avg":    avg,
				"height": fmt.Sprintf("%d", a.gnomonClient.LatestInteractionHeight(scid)),
			})
			if len(out) >= maxSuggestions {
				break
			}
		}
	}

	return map[string]interface{}{
		"success":     true,
		"suggestions": out,
		"count":       len(out),
	}
}

