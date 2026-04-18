package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// NetworkFilter implements Privacy Mode - blocking non-DERO network connections

type NetworkFilter struct {
	sync.RWMutex
	enabled      bool
	allowedHosts []string
	blockedCount int64
	allowedCount int64
	connectionLog []ConnectionLogEntry
}

type ConnectionLogEntry struct {
	Timestamp int64  `json:"timestamp"`
	URL       string `json:"url"`
	Host      string `json:"host"`
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason"`
}

// Global network filter instance
var networkFilter = &NetworkFilter{
	enabled: false,
	allowedHosts: []string{
		"127.0.0.1",
		"localhost",
		"0.0.0.0",
		"::1",
	},
	connectionLog: make([]ConnectionLogEntry, 0),
}

// SetCypherpunkMode enables or disables Cypherpunk Mode
func (a *App) SetCypherpunkMode(enabled bool) map[string]interface{} {
	networkFilter.Lock()
	defer networkFilter.Unlock()

	networkFilter.enabled = enabled

	if enabled {
		a.logToConsole("[SHIELD] Privacy Mode ENABLED - Only DERO/localhost connections allowed")
	} else {
		a.logToConsole("[NET] Privacy Mode DISABLED - All connections allowed")
	}

	return map[string]interface{}{
		"success": true,
		"enabled": enabled,
		"message": func() string {
			if enabled {
				return "Privacy Mode enabled - non-DERO connections will be blocked"
			}
			return "Privacy Mode disabled - all connections allowed"
		}(),
	}
}

// GetCypherpunkMode returns the current Cypherpunk Mode status
func (a *App) GetCypherpunkMode() bool {
	networkFilter.RLock()
	defer networkFilter.RUnlock()
	return networkFilter.enabled
}

// GetNetworkFilterStatus returns detailed network filter status
func (a *App) GetNetworkFilterStatus() map[string]interface{} {
	networkFilter.RLock()
	defer networkFilter.RUnlock()

	return map[string]interface{}{
		"success":      true,
		"enabled":      networkFilter.enabled,
		"allowedHosts": networkFilter.allowedHosts,
		"blockedCount": networkFilter.blockedCount,
		"allowedCount": networkFilter.allowedCount,
	}
}

// IsRequestAllowed checks if a request URL is allowed under current settings
func (a *App) IsRequestAllowed(urlStr string) map[string]interface{} {
	allowed, reason := checkRequestAllowed(urlStr)

	return map[string]interface{}{
		"success": true,
		"url":     urlStr,
		"allowed": allowed,
		"reason":  reason,
	}
}

// AddAllowedHost adds a host to the allowed list
func (a *App) AddAllowedHost(host string) map[string]interface{} {
	networkFilter.Lock()
	defer networkFilter.Unlock()

	// Check if already exists
	for _, h := range networkFilter.allowedHosts {
		if h == host {
			return map[string]interface{}{
				"success": true,
				"message": "Host already in allowed list",
			}
		}
	}

	networkFilter.allowedHosts = append(networkFilter.allowedHosts, host)
	a.logToConsole("[OK] Added to allowed hosts: " + host)

	return map[string]interface{}{
		"success": true,
		"host":    host,
		"message": "Host added to allowed list",
	}
}

// RemoveAllowedHost removes a host from the allowed list
func (a *App) RemoveAllowedHost(host string) map[string]interface{} {
	networkFilter.Lock()
	defer networkFilter.Unlock()

	// Don't allow removing localhost entries
	if host == "127.0.0.1" || host == "localhost" || host == "::1" {
		return map[string]interface{}{
			"success": false,
			"error":   "Cannot remove localhost entries",
		}
	}

	newList := []string{}
	removed := false
	for _, h := range networkFilter.allowedHosts {
		if h != host {
			newList = append(newList, h)
		} else {
			removed = true
		}
	}

	if removed {
		networkFilter.allowedHosts = newList
		a.logToConsole("[OK] Removed from allowed hosts: " + host)
		return map[string]interface{}{
			"success": true,
			"host":    host,
			"message": "Host removed from allowed list",
		}
	}

	return map[string]interface{}{
		"success": false,
		"error":   "Host not found in allowed list",
	}
}

// GetConnectionLog returns recent connection log entries
func (a *App) GetConnectionLog(limit int) map[string]interface{} {
	networkFilter.RLock()
	defer networkFilter.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	log := networkFilter.connectionLog
	if len(log) > limit {
		log = log[len(log)-limit:]
	}

	return map[string]interface{}{
		"success": true,
		"log":     log,
		"count":   len(log),
	}
}

// ClearConnectionLog clears the connection log
func (a *App) ClearConnectionLog() map[string]interface{} {
	networkFilter.Lock()
	defer networkFilter.Unlock()

	networkFilter.connectionLog = make([]ConnectionLogEntry, 0)
	networkFilter.blockedCount = 0
	networkFilter.allowedCount = 0

	return map[string]interface{}{
		"success": true,
		"message": "Connection log cleared",
	}
}

// Internal helper functions

func checkRequestAllowed(urlStr string) (bool, string) {
	networkFilter.RLock()
	enabled := networkFilter.enabled
	allowedHosts := networkFilter.allowedHosts
	networkFilter.RUnlock()

	// If privacy mode is disabled, allow everything
	if !enabled {
		return true, "Privacy Mode disabled"
	}

	// Parse URL
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false, "Invalid URL"
	}

	host := parsed.Hostname()
	if host == "" {
		host = parsed.Host
	}

	// Remove port if present
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}

	// Check against allowed hosts
	for _, allowed := range allowedHosts {
		if host == allowed {
			return true, "Host in allowed list"
		}
	}

	// Check for DERO-specific patterns
	if isDEROConnection(urlStr, host) {
		return true, "DERO network connection"
	}

	return false, "Blocked by Privacy Mode"
}

func isDEROConnection(urlStr, host string) bool {
	// Allow known DERO patterns
	deroPatterns := []string{
		"dero",
		"xswd",
		"10102", // default RPC port
		"10101", // default P2P port
		"44326", // XSWD port
	}

	lowerURL := strings.ToLower(urlStr)
	lowerHost := strings.ToLower(host)

	for _, pattern := range deroPatterns {
		if strings.Contains(lowerURL, pattern) || strings.Contains(lowerHost, pattern) {
			return true
		}
	}

	// Check if it's a blockchain RPC call pattern
	if strings.Contains(lowerURL, "json_rpc") || strings.Contains(lowerURL, "jsonrpc") {
		return true
	}

	// Allow WebSocket connections to localhost (for XSWD)
	if (strings.HasPrefix(lowerURL, "ws://127.0.0.1") || strings.HasPrefix(lowerURL, "ws://localhost")) {
		return true
	}

	return false
}

// LogConnection logs a connection attempt (called from request interceptor)
func logConnection(urlStr string, allowed bool, reason string) {
	networkFilter.Lock()
	defer networkFilter.Unlock()

	parsed, _ := url.Parse(urlStr)
	host := ""
	if parsed != nil {
		host = parsed.Hostname()
	}

	entry := ConnectionLogEntry{
		Timestamp: getCurrentTimestamp(),
		URL:       urlStr,
		Host:      host,
		Allowed:   allowed,
		Reason:    reason,
	}

	networkFilter.connectionLog = append(networkFilter.connectionLog, entry)

	// Keep only last 1000 entries
	if len(networkFilter.connectionLog) > 1000 {
		networkFilter.connectionLog = networkFilter.connectionLog[len(networkFilter.connectionLog)-1000:]
	}

	if allowed {
		networkFilter.allowedCount++
	} else {
		networkFilter.blockedCount++
	}
}

func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// Dedupe rapid identical host toasts (e.g. many assets from one domain).
var (
	blockToastMu       sync.Mutex
	blockToastLastHost string
	blockToastLastTime time.Time
)

func emitPrivacyBlockedToast(ctx context.Context, urlStr, host, reason string) {
	if ctx == nil {
		return
	}
	displayHost := host
	if displayHost == "" {
		displayHost = urlStr
		if len(displayHost) > 64 {
			displayHost = displayHost[:61] + "…"
		}
	}

	blockToastMu.Lock()
	now := time.Now()
	if displayHost == blockToastLastHost && now.Sub(blockToastLastTime) < 3*time.Second {
		blockToastMu.Unlock()
		return
	}
	blockToastLastHost, blockToastLastTime = displayHost, now
	blockToastMu.Unlock()

	msg := fmt.Sprintf("Privacy Mode blocked a connection to %s.", displayHost)
	if reason != "" && reason != "Blocked by Privacy Mode" {
		msg = fmt.Sprintf("Privacy Mode blocked %s (%s).", displayHost, reason)
	}

	runtime.EventsEmit(ctx, "toast:show", map[string]interface{}{
		"type":    "warning",
		"message": msg,
	})
}

// RequestInterceptor evaluates Privacy Mode for a URL, logs it, and notifies the UI when blocked.
// Call from the frontend before opening external URLs (and eventually from native request hooks).
func (a *App) RequestInterceptor(urlStr string) map[string]interface{} {
	allowed, reason := checkRequestAllowed(urlStr)

	parsed, _ := url.Parse(urlStr)
	host := ""
	if parsed != nil {
		host = parsed.Hostname()
	}

	// Log the connection
	logConnection(urlStr, allowed, reason)

	if !allowed {
		a.logToConsole("[SHIELD] Blocked: " + urlStr + " (" + reason + ")")
		emitPrivacyBlockedToast(a.ctx, urlStr, host, reason)
	}

	return map[string]interface{}{
		"allowed": allowed,
		"reason":  reason,
	}
}

// OpenURLInBrowserIfAllowed runs Privacy Mode for remote URLs, then opens the system browser when allowed.
// file:// and wails:// skip the network policy (no outbound HTTP).
func (a *App) OpenURLInBrowserIfAllowed(urlStr string) map[string]interface{} {
	trimmed := strings.TrimSpace(urlStr)
	if trimmed == "" {
		return map[string]interface{}{"success": false, "error": "empty URL"}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return map[string]interface{}{"success": false, "error": "invalid URL"}
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "file" || scheme == "wails" {
		runtime.BrowserOpenURL(a.ctx, trimmed)
		return map[string]interface{}{"success": true, "allowed": true, "reason": "local scheme"}
	}

	res := a.RequestInterceptor(trimmed)
	allowed, _ := res["allowed"].(bool)
	if !allowed {
		reason, _ := res["reason"].(string)
		return map[string]interface{}{"success": false, "allowed": false, "reason": reason}
	}
	runtime.BrowserOpenURL(a.ctx, trimmed)
	reason, _ := res["reason"].(string)
	return map[string]interface{}{"success": true, "allowed": true, "reason": reason}
}

// GetActiveConnections returns information about active network connections
func (a *App) GetActiveConnections() map[string]interface{} {
	connections := []map[string]interface{}{}

	// XSWD connection
	connections = append(connections, map[string]interface{}{
		"name":      "XSWD (Wallet)",
		"type":      "websocket",
		"endpoint":  "ws://127.0.0.1:44326/xswd",
		"connected": a.xswdClient.IsConnected(),
		"direction": "outbound",
	})

	// Daemon RPC connection
	daemonEndpoint := "http://127.0.0.1:10102"
	if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		daemonEndpoint = ep
	}
	connections = append(connections, map[string]interface{}{
		"name":      "Daemon RPC",
		"type":      "http",
		"endpoint":  daemonEndpoint,
		"connected": a.daemonClient != nil,
		"direction": "outbound",
	})

	// Gnomon indexer
	connections = append(connections, map[string]interface{}{
		"name":      "Gnomon Indexer",
		"type":      "local",
		"endpoint":  "local",
		"connected": a.gnomonClient.IsRunning(),
		"direction": "internal",
	})

	// Node P2P (if embedded node is running)
	nodeManager.RLock()
	nodeRunning := nodeManager.isRunning
	nodeManager.RUnlock()

	if nodeRunning {
		connections = append(connections, map[string]interface{}{
			"name":      "Node P2P",
			"type":      "p2p",
			"endpoint":  "0.0.0.0:10101",
			"connected": true,
			"direction": "bidirectional",
		})
	}

	return map[string]interface{}{
		"success":     true,
		"connections": connections,
		"count":       len(connections),
	}
}

