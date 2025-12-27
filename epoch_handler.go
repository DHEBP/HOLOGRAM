package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/civilware/epoch"
)

// EpochHandler manages EPOCH (Event-Driven Propagation of Crowd Hashing) for HOLOGRAM.
// EPOCH allows TELA apps to request hash computations as a non-intrusive form of developer support.
// Default: ON (opt-out model)
type EpochHandler struct {
	sync.RWMutex

	enabled    bool   // User setting: is EPOCH enabled (default: true)
	maxHashes  int    // Max hashes per request (user-configurable)
	maxThreads int    // Max threads for EPOCH (user-configurable)
	address    string // Reward address (defaults to app developer or configured)

	// Rate limiting per app
	rateLimits    map[string]*rateLimitEntry
	rateLimitLock sync.Mutex

	// Logging callback
	logFn func(string)
}

type rateLimitEntry struct {
	lastRequest time.Time
	hashCount   uint64
	window      time.Duration
}

// EpochStats represents EPOCH session statistics for frontend display
type EpochStats struct {
	Active       bool    `json:"active"`
	Enabled      bool    `json:"enabled"`
	Hashes       uint64  `json:"hashes"`
	HashesStr    string  `json:"hashes_str"`
	MiniBlocks   int     `json:"miniblocks"`
	Version      string  `json:"version"`
	MaxHashes    int     `json:"max_hashes"`
	MaxThreads   int     `json:"max_threads"`
	Address      string  `json:"address"`
	IsProcessing bool    `json:"is_processing"`
}

// EpochResult represents the result of an EPOCH hash attempt
type EpochResult struct {
	Success    bool    `json:"success"`
	Hashes     uint64  `json:"hashes"`
	Submitted  int     `json:"submitted"`
	Duration   int64   `json:"duration_ms"`
	HashPerSec float64 `json:"hash_per_sec"`
	Error      string  `json:"error,omitempty"`
}

const (
	// Default settings for EPOCH
	DEFAULT_EPOCH_MAX_HASHES  = 100  // Conservative default for per-request limit
	DEFAULT_EPOCH_MAX_THREADS = 2    // Conservative CPU usage
	
	// Rate limiting
	RATE_LIMIT_WINDOW        = 10 * time.Second // Window for rate limiting
	RATE_LIMIT_MAX_HASHES    = 500              // Max hashes per app per window
	
	// EPOCH Developer Support Address
	// This is the default address where EPOCH mining rewards are sent.
	// EPOCH is for developer/ecosystem support - NOT user earnings (that's what the background miner is for).
	// This can be overridden per-app if the app specifies their own developer address.
	DEFAULT_EPOCH_DEVELOPER_ADDRESS = "dero1qyqu6kdla44msn0ky5skpv4fahj2ay80ycjpz27kgc4wf7jk4ys0kqq6s36fh"
)

// NewEpochHandler creates a new EPOCH handler with sensible defaults
func NewEpochHandler(logFn func(string)) *EpochHandler {
	return &EpochHandler{
		enabled:    true, // DEFAULT ON (opt-out model)
		maxHashes:  DEFAULT_EPOCH_MAX_HASHES,
		maxThreads: DEFAULT_EPOCH_MAX_THREADS,
		rateLimits: make(map[string]*rateLimitEntry),
		logFn:      logFn,
	}
}

// log helper
func (e *EpochHandler) log(msg string) {
	if e.logFn != nil {
		e.logFn(msg)
	}
}

// Initialize starts the EPOCH connection to the DERO node.
// Called automatically on app startup if enabled.
// IMPORTANT: EPOCH is for developer/ecosystem support. Rewards go to the developer address,
// NOT the user's wallet (that's what the background miner is for).
func (e *EpochHandler) Initialize(address, daemonEndpoint string) error {
	e.Lock()
	defer e.Unlock()

	if !e.enabled {
		e.log("[EPOCH] Developer support is disabled")
		return nil
	}

	if epoch.IsActive() {
		e.log("[EPOCH] Already active")
		return nil
	}

	// Use default developer address if none provided
	// EPOCH rewards should go to developers/ecosystem, not users
	if address == "" {
		address = DEFAULT_EPOCH_DEVELOPER_ADDRESS
		e.log("[EPOCH] Using default developer support address")
	}
	e.address = address

	// Configure EPOCH
	epoch.SetMaxThreads(e.maxThreads)
	if err := epoch.SetMaxHashes(e.maxHashes); err != nil {
		e.log(fmt.Sprintf("[WARN] EPOCH: Could not set max hashes: %v", err))
	}

	e.log(fmt.Sprintf("[EPOCH] Connecting to daemon %s...", daemonEndpoint))

	// Start EPOCH connection
	err := epoch.StartGetWork(address, daemonEndpoint)
	if err != nil {
		e.log(fmt.Sprintf("[ERR] EPOCH: Connection failed: %v", err))
		return err
	}

	// Wait for first job (up to 10 seconds)
	if err := epoch.JobIsReady(10 * time.Second); err != nil {
		e.log(fmt.Sprintf("[WARN] EPOCH: No job received within timeout: %v", err))
		// Don't return error - connection is still active, job may come later
		return nil
	}

	// Option 3: Wait for successful job before logging "active"
	// Check if we can get a valid session after a short delay to ensure miner is registered
	// This helps avoid logging "active" when miner isn't registered yet
	go func() {
		// Wait 30 seconds to allow miner registration
		time.Sleep(30 * time.Second)

		// Check if EPOCH is still active (user might have disabled it)
		if !epoch.IsActive() {
			return
		}

		// Try to get a session - if successful, we can consider it "active"
		session, err := epoch.GetSession(5 * time.Second)
		if err == nil && session.Version != "" {
			e.log("[OK] EPOCH: Developer support active")
		} else {
			// Connected but waiting for miner registration
			e.log("[WARN] EPOCH: Connected but waiting for miner registration (may take up to 15 minutes)")
		}
	}()

	return nil
}

// Shutdown stops the EPOCH connection
func (e *EpochHandler) Shutdown() {
	e.Lock()
	defer e.Unlock()

	if epoch.IsActive() {
		epoch.StopGetWork()
		e.log("[EPOCH] Developer support stopped")
	}
}

// SetEnabled toggles EPOCH on/off
func (e *EpochHandler) SetEnabled(enabled bool) {
	e.Lock()
	e.enabled = enabled
	e.Unlock()

	if !enabled && epoch.IsActive() {
		epoch.StopGetWork()
		e.log("[EPOCH] Developer support disabled by user")
	}
}

// IsEnabled returns whether EPOCH is enabled in settings
func (e *EpochHandler) IsEnabled() bool {
	e.RLock()
	defer e.RUnlock()
	return e.enabled
}

// IsActive returns whether EPOCH connection is active
func (e *EpochHandler) IsActive() bool {
	return epoch.IsActive()
}

// SetMaxHashes updates the per-request hash limit
func (e *EpochHandler) SetMaxHashes(max int) error {
	e.Lock()
	defer e.Unlock()

	if max < 1 || max > epoch.LIMIT_MAX_HASHES {
		return fmt.Errorf("max hashes must be between 1 and %d", epoch.LIMIT_MAX_HASHES)
	}

	e.maxHashes = max
	return epoch.SetMaxHashes(max)
}

// SetMaxThreads updates the thread count
func (e *EpochHandler) SetMaxThreads(threads int) {
	e.Lock()
	defer e.Unlock()

	e.maxThreads = threads
	epoch.SetMaxThreads(threads)
}

// GetStats returns current EPOCH session statistics
func (e *EpochHandler) GetStats() EpochStats {
	e.RLock()
	enabled := e.enabled
	maxHashes := e.maxHashes
	maxThreads := e.maxThreads
	address := e.address
	e.RUnlock()

	stats := EpochStats{
		Enabled:    enabled,
		MaxHashes:  maxHashes,
		MaxThreads: maxThreads,
		Address:    address,
	}

	if epoch.IsActive() {
		stats.Active = true
		stats.IsProcessing = epoch.IsProcessing()

		session, err := epoch.GetSession(2 * time.Second)
		if err == nil {
			stats.Hashes = session.Hashes
			stats.HashesStr = epoch.HashesToString(session.Hashes)
			stats.MiniBlocks = session.MiniBlocks
			stats.Version = session.Version
		}
	}

	return stats
}

// HandleRequest processes an EPOCH request from a TELA app via XSWD.
// This is the main entry point for dApps to request hash computations.
// HOLOGRAM enforces its own limits regardless of what the app requests.
func (e *EpochHandler) HandleRequest(requestedHashes int, appSCID string) EpochResult {
	e.RLock()
	enabled := e.enabled
	maxAllowed := e.maxHashes
	e.RUnlock()

	result := EpochResult{}

	// Check if enabled
	if !enabled {
		result.Error = "Developer support is disabled"
		return result
	}

	// Check if active
	if !epoch.IsActive() {
		result.Error = "EPOCH not connected"
		return result
	}

	// Enforce HOLOGRAM's limits (not the app's request)
	if requestedHashes > maxAllowed {
		requestedHashes = maxAllowed
	}
	if requestedHashes < 1 {
		requestedHashes = 1
	}

	// Rate limiting per app
	if e.isRateLimited(appSCID, uint64(requestedHashes)) {
		result.Error = "Rate limited"
		return result
	}

	// Attempt hashes
	epochResult, err := epoch.AttemptHashes(requestedHashes)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Record for rate limiting
	e.recordRequest(appSCID, epochResult.Hashes)

	result.Success = true
	result.Hashes = epochResult.Hashes
	result.Submitted = epochResult.Submitted
	result.Duration = epochResult.Duration
	result.HashPerSec = epochResult.HashPerSec

	if epochResult.Submitted > 0 {
		e.log(fmt.Sprintf("[EPOCH] Found %d miniblock(s) for app %s!", epochResult.Submitted, appSCID[:16]))
	}

	return result
}

// isRateLimited checks if an app has exceeded its rate limit
func (e *EpochHandler) isRateLimited(appSCID string, requestedHashes uint64) bool {
	e.rateLimitLock.Lock()
	defer e.rateLimitLock.Unlock()

	now := time.Now()
	entry, exists := e.rateLimits[appSCID]

	if !exists {
		// First request from this app
		return false
	}

	// Check if window has expired
	if now.Sub(entry.lastRequest) > entry.window {
		// Window expired, reset
		entry.hashCount = 0
		entry.lastRequest = now
		return false
	}

	// Check if adding these hashes would exceed limit
	if entry.hashCount+requestedHashes > RATE_LIMIT_MAX_HASHES {
		return true
	}

	return false
}

// recordRequest records a request for rate limiting
func (e *EpochHandler) recordRequest(appSCID string, hashes uint64) {
	e.rateLimitLock.Lock()
	defer e.rateLimitLock.Unlock()

	now := time.Now()
	entry, exists := e.rateLimits[appSCID]

	if !exists {
		e.rateLimits[appSCID] = &rateLimitEntry{
			lastRequest: now,
			hashCount:   hashes,
			window:      RATE_LIMIT_WINDOW,
		}
		return
	}

	// Check if window has expired
	if now.Sub(entry.lastRequest) > entry.window {
		entry.hashCount = hashes
		entry.lastRequest = now
	} else {
		entry.hashCount += hashes
	}
}

// GetConfig returns the current EPOCH configuration
func (e *EpochHandler) GetConfig() map[string]interface{} {
	e.RLock()
	defer e.RUnlock()

	return map[string]interface{}{
		"enabled":     e.enabled,
		"max_hashes":  e.maxHashes,
		"max_threads": e.maxThreads,
		"address":     e.address,
	}
}

// SetConfig updates EPOCH configuration
func (e *EpochHandler) SetConfig(enabled bool, maxHashes, maxThreads int) {
	e.Lock()
	e.enabled = enabled
	if maxHashes > 0 && maxHashes <= epoch.LIMIT_MAX_HASHES {
		e.maxHashes = maxHashes
		epoch.SetMaxHashes(maxHashes)
	}
	if maxThreads > 0 {
		e.maxThreads = maxThreads
		epoch.SetMaxThreads(maxThreads)
	}
	e.Unlock()

	if !enabled && epoch.IsActive() {
		epoch.StopGetWork()
	}
}

