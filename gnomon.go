package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/civilware/Gnomon/indexer"
	"github.com/civilware/Gnomon/storage"
	"github.com/civilware/Gnomon/structures"
	"github.com/deroproject/derohe/globals"
)

// GnomonClient manages the Gnomon indexer for TELA content discovery
type GnomonClient struct {
	Indexer          *indexer.Indexer
	fastsync         bool
	parallelBlocks   int
	dbPath           string
	dbType           string
	running          bool
	disableFastsync  bool  // Temporary flag to disable fastsync for next start (used after resync)
	startFromHeight  int64 // If > 0, start indexing from this height instead of 0 or current
	appsLoaded       bool  // True when GetDiscoveredApps() has completed at least once
}

const maxParallelBlocks = 10

// TELA search filter - matches contracts with owner initialization
const gnomonSearchFilter = `Function init() Uint64
10 IF EXISTS("owner") == 0 THEN GOTO 30
20 RETURN 1
30 STORE("owner", address())`

// NewGnomonClient creates a new Gnomon client
func NewGnomonClient(dbType string) *GnomonClient {
	if dbType == "" {
		dbType = "gravdb" // Default to GravDB
	}

	return &GnomonClient{
		fastsync:       true,
		parallelBlocks: 5,
		dbType:         dbType,
		running:        false,
	}
}

// Start initializes and starts the Gnomon indexer
func (g *GnomonClient) Start(endpoint string, network string) error {
	if g.running {
		return fmt.Errorf("gnomon already running")
	}

	// Strip http:// or https:// prefix - Gnomon's indexer.Connect() adds "ws://" internally
	// So we need to pass just "host:port" to avoid "ws://http://host:port/ws"
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// Determine data path based on network
	// Use UserHomeDir instead of Getwd for packaged macOS apps
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create network-specific path in ~/.dero/hologram/datashards/
	baseDir := filepath.Join(homeDir, ".dero", "hologram", "datashards")
	basePath := filepath.Join(baseDir, "gnomon")
	switch network {
	case "simulator":
		basePath = filepath.Join(baseDir, "gnomon_simulator")
	case "mainnet":
		basePath = filepath.Join(baseDir, "gnomon_mainnet")
	}

	g.dbPath = basePath

	// Ensure directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return fmt.Errorf("failed to create gnomon directory: %w", err)
	}

	// Initialize storage backends
	boltDB, boltErr := storage.NewBBoltDB(basePath, "gnomon")
	gravDB, gravErr := storage.NewGravDB(basePath, "25ms")

	var height int64
	switch g.dbType {
	case "boltdb":
		if boltErr != nil {
			if !strings.HasPrefix(boltErr.Error(), "[") {
				boltErr = fmt.Errorf("[NewBBoltDB] %s", boltErr)
			}
			return boltErr
		}

		height, err = boltDB.GetLastIndexHeight()
		if err != nil {
			height = 0
		}
	default: // gravdb
		if gravErr != nil {
			return fmt.Errorf("[NewGravDB] %s", gravErr)
		}

		height, err = gravDB.GetLastIndexHeight()
		if err != nil {
			height = 0
		}
	}

	// Known exclusions (if any)
	exclusions := []string{"bb43c3eb626ee767c9f305772a6666f7c7300441a0ad8538a0799eb4f12ebcd2"}
	
	// Search filter for TELA apps
	filter := []string{gnomonSearchFilter}

	// Fastsync configuration
	// For simulator mode, disable fastsync to ensure we index from block 0
	// This is important because simulator chains are small and we need to find
	// all deployed contracts, not just new ones
	useFastsync := g.fastsync
	forceFastsync := true
	if network == "simulator" {
		useFastsync = false
		forceFastsync = false
	}
	
	// If disableFastsync flag is set (e.g., after a resync), disable fastsync
	// This ensures we index from the stored height (or 0 if DB was cleaned)
	if g.disableFastsync {
		useFastsync = false
		forceFastsync = false
		g.disableFastsync = false // Reset the flag after use
	}
	
	// If a specific start height is set, use it instead of the stored height
	if g.startFromHeight > 0 {
		height = g.startFromHeight
		g.startFromHeight = 0 // Reset after use
	}
	
	config := &structures.FastSyncConfig{
		Enabled:           useFastsync,
		SkipFSRecheck:     false,
		ForceFastSync:     forceFastsync,
		ForceFastSyncDiff: 100,
		NoCode:            false,
	}

	// Create indexer
	g.Indexer = indexer.NewIndexer(
		gravDB,
		boltDB,
		g.dbType,
		filter,
		height,
		endpoint,
		"daemon",
		false, // closeondisconnect
		false, // runtime mode
		config,
		exclusions,
	)

	// Initialize logging
	indexer.InitLog(globals.Arguments, os.Stdout)

	// Start indexer in background
	go g.Indexer.StartDaemonMode(g.parallelBlocks)

	g.running = true

	return nil
}

// Stop closes the Gnomon indexer
func (g *GnomonClient) Stop() {
	if g.Indexer != nil {
		g.Indexer.Close()
		g.Indexer = nil
		g.running = false
		g.appsLoaded = false // Reset apps loaded state
	}
}

// SetDisableFastsync sets a flag to disable fastsync on the next start
// This is used after a resync to ensure we index from block 0
func (g *GnomonClient) SetDisableFastsync(disable bool) {
	g.disableFastsync = disable
}

// SetStartFromHeight sets a specific height to start indexing from
// This is useful for resyncing recent contracts without indexing the entire chain
func (g *GnomonClient) SetStartFromHeight(height int64) {
	g.startFromHeight = height
}

// SetAppsLoaded sets the appsLoaded flag (called by App.GetDiscoveredApps)
func (g *GnomonClient) SetAppsLoaded(loaded bool) {
	g.appsLoaded = loaded
}

// IsAppsLoaded returns whether apps have been loaded at least once
func (g *GnomonClient) IsAppsLoaded() bool {
	return g.appsLoaded
}

// IsRunning returns whether Gnomon is running
func (g *GnomonClient) IsRunning() bool {
	return g.running && g.Indexer != nil
}

// GetStatus returns the current indexing status
func (g *GnomonClient) GetStatus() map[string]interface{} {
	if !g.IsRunning() {
		return map[string]interface{}{
			"running":        false,
			"connecting":     false,
			"indexed_height": 0,
			"chain_height":   0,
			"progress":       0.0,
		}
	}

	indexed := g.Indexer.LastIndexedHeight
	chain := g.Indexer.ChainHeight

	// If chain height is 0, Gnomon is still trying to connect to the daemon
	// This happens when the connection loop in StartDaemonMode is retrying
	connecting := chain == 0

	progress := 0.0
	if chain > 0 {
		progress = (float64(indexed) / float64(chain)) * 100.0
	}

	return map[string]interface{}{
		"running":        true,
		"connecting":     connecting,
		"indexed_height": indexed,
		"chain_height":   chain,
		"progress":       progress,
		"db_type":        g.dbType,
		"db_path":        g.dbPath,
		"apps_loaded":    g.appsLoaded,
	}
}

// GetAllOwnersAndSCIDs returns all indexed smart contracts
func (g *GnomonClient) GetAllOwnersAndSCIDs() map[string]string {
	if !g.IsRunning() {
		return make(map[string]string)
	}

	switch g.Indexer.DBType {
	case "gravdb":
		return g.Indexer.GravDBBackend.GetAllOwnersAndSCIDs()
	case "boltdb":
		return g.Indexer.BBSBackend.GetAllOwnersAndSCIDs()
	default:
		return make(map[string]string)
	}
}

// GetAllSCIDVariableDetails returns all variables for a smart contract
func (g *GnomonClient) GetAllSCIDVariableDetails(scid string) []*structures.SCIDVariable {
	if !g.IsRunning() {
		return nil
	}

	switch g.Indexer.DBType {
	case "gravdb":
		return g.Indexer.GravDBBackend.GetAllSCIDVariableDetails(scid)
	case "boltdb":
		return g.Indexer.BBSBackend.GetAllSCIDVariableDetails(scid)
	default:
		return nil
	}
}

// GetSCIDValuesByKey returns values for a specific key in a smart contract
func (g *GnomonClient) GetSCIDValuesByKey(scid string, key interface{}) (valuesstring []string, valuesuint64 []uint64) {
	if !g.IsRunning() {
		return nil, nil
	}

	switch g.Indexer.DBType {
	case "gravdb":
		return g.Indexer.GravDBBackend.GetSCIDValuesByKey(scid, key, g.Indexer.ChainHeight, true)
	case "boltdb":
		return g.Indexer.BBSBackend.GetSCIDValuesByKey(scid, key, g.Indexer.ChainHeight, true)
	default:
		return nil, nil
	}
}

// GetSCIDKeysByValue returns keys for a specific value in a smart contract
func (g *GnomonClient) GetSCIDKeysByValue(scid string, value interface{}) (valuesstring []string, valuesuint64 []uint64) {
	if !g.IsRunning() {
		return nil, nil
	}

	switch g.Indexer.DBType {
	case "gravdb":
		return g.Indexer.GravDBBackend.GetSCIDKeysByValue(scid, value, g.Indexer.ChainHeight, true)
	case "boltdb":
		return g.Indexer.BBSBackend.GetSCIDKeysByValue(scid, value, g.Indexer.ChainHeight, true)
	default:
		return nil, nil
	}
}

// GetTELAApps returns all discovered TELA INDEX applications (filters out DOCs)
func (g *GnomonClient) GetTELAApps() []map[string]interface{} {
	apps := make([]map[string]interface{}, 0)

	if !g.IsRunning() {
		return apps
	}

	// Get all SCIDs
	scids := g.GetAllOwnersAndSCIDs()

	for scid, owner := range scids {

		var (
			// Get variables for this SCID
			vars = g.GetAllSCIDVariableDetails(scid)

			data = map[string]interface{}{"scid": scid, "owner": owner, "is_index": false}

			// Extract TELA-specific variables
			app, isIndex, _, _ = allocateData(vars, data)
		)

		// Only include INDEX contracts (apps with DOC references)
		// This filters out individual DOC files which can't be rendered standalone
		if isIndex {

			var (
				// Generate clean display name (prefer dURL when present)
				displayName = ""

				// Get fields
				name, hasName        = app["name"].(string)
				description, hasDesc = app["description"].(string)
				url, hasURL          = app["url"].(string)

				// Helper function to check if a string is a URL/file path
				isURLFunc = func(s string) bool {
					if s == "" {
						return false
					}

					has := strings.Contains
					lower := strings.ToLower(s)
					return has(lower, "http") ||
						has(lower, "://") ||
						has(lower, ".png") ||
						has(lower, ".jpg") ||
						has(lower, ".jpeg") ||
						has(lower, ".svg") ||
						has(lower, ".gif") ||
						has(lower, ".ico") ||
						has(lower, "/ipfs/") ||
						has(lower, "/images/") ||
						has(lower, "/icons/") ||
						has(lower, "/assets/") ||
						has(lower, "gateway.") ||
						has(lower, "blob/") ||
						has(lower, "i.ibb.") ||
						has(lower, "bafybeih") ||
						has(lower, "avatars.") ||
						has(lower, "raw.github") ||
						has(lower, ".world/") ||
						has(lower, ".com/") ||
						has(lower, ".org/") ||
						has(lower, ".io/")
				}

				// Check both description and name for URLs
				isDescURL = hasDesc && isURLFunc(description)
				isNameURL = hasName && isURLFunc(name)
			)

			// Decision tree - prefer dURL if present
			if du, hasDU := app["durl"].(string); hasDU && du != "" {
				displayName = du
			} else if hasDesc && description != "" && !isDescURL {
				// Use description if it's NOT a URL
				displayName = description
			} else if hasName && name != "" && !isNameURL {
				// Use name if description was URL/empty and name is clean
				displayName = name
			} else if hasURL && url != "" {
				// Use cleaned dURL domain
				displayName = cleanupAppName(url)
			} else {
				// Nothing usable - generic name
				displayName = "TELA App"
			}

			// Limit to 40 characters for uniformity
			displayName = strings.TrimSpace(displayName)
			if len(displayName) > 40 {
				displayName = displayName[:37] + "..."
			}

			// Final paranoid safety check - if result still looks like URL, replace it
			if isURLFunc(displayName) {
				// It's STILL a URL after all that - use generic name
				if hasURL && url != "" {
					cleaned := cleanupAppName(url)
					// Triple-check the cleaned version
					if !isURLFunc(cleaned) && !strings.Contains(cleaned, "/") {
						displayName = cleaned
					} else {
						displayName = "TELA App"
					}
				} else {
					displayName = "TELA App"
				}
			}

			app["display_name"] = displayName
			apps = append(apps, app)
		}
	}

	return apps
}

// GetTELALibraries returns all TELA content tagged as libraries (.lib suffix in dURL)
func (g *GnomonClient) GetTELALibraries() []map[string]interface{} {
	libs := make([]map[string]interface{}, 0)

	if !g.IsRunning() {
		return libs
	}

	// Get all SCIDs
	scids := g.GetAllOwnersAndSCIDs()

	for scid, owner := range scids {

		var (
			vars                 = g.GetAllSCIDVariableDetails(scid)
			params               = map[string]any{"scid": scid, "owner": owner, "is_index": false, "doc_count": 0}
			lib, _, _, hasLibTag = allocateData(vars, params)
		)

		// Only include content tagged as library
		if hasLibTag {
			libs = append(libs, lib)
		}
	}

	return libs
}

// SearchTELApps searches for TELA apps by name or description
func (g *GnomonClient) SearchTELApps(query string) []map[string]interface{} {
	allApps := g.GetTELAApps()
	results := make([]map[string]interface{}, 0)

	query = strings.ToLower(query)

	for _, app := range allApps {
		name := ""
		if n, ok := app["name"].(string); ok {
			name = strings.ToLower(n)
		}

		description := ""
		if d, ok := app["description"].(string); ok {
			description = strings.ToLower(d)
		}

		if strings.Contains(name, query) || strings.Contains(description, query) {
			results = append(results, app)
		}
	}

	return results
}

// LatestInteractionHeight returns the most recent interaction height for a SCID
func (g *GnomonClient) LatestInteractionHeight(scid string) int64 {
    if !g.IsRunning() {
        return 0
    }
    heights := g.Indexer.GravDBBackend.GetSCIDInteractionHeight(scid)
    var max int64 = 0
    for _, h := range heights {
        if h > max { max = h }
    }
    return max
}

// CheckAppSupportsEpoch determines if a TELA app supports EPOCH crowd mining
// Looks for EPOCH-related variables or functions in the smart contract
func (g *GnomonClient) CheckAppSupportsEpoch(scid string) bool {
    if !g.IsRunning() {
        return false
    }

    vars := g.GetAllSCIDVariableDetails(scid)
    if vars == nil {
        return false
    }

    // Check for EPOCH-related variables
    epochKeywords := []string{
        "epoch",
        "EPOCH",
        "epochEnabled",
        "epoch_enabled",
        "epochSupport",
        "crowd_mining",
        "crowdMining",
    }

    for _, v := range vars {
        key := fmt.Sprintf("%v", v.Key)
        keyLower := strings.ToLower(key)
        
        for _, keyword := range epochKeywords {
            if strings.Contains(keyLower, strings.ToLower(keyword)) {
                return true
            }
        }
    }

    return false
}

// GetTELAAppsWithEpochInfo returns all TELA apps with EPOCH support information
func (g *GnomonClient) GetTELAAppsWithEpochInfo() []map[string]interface{} {
    apps := g.GetTELAApps()
    
    for i, app := range apps {
        if scid, ok := app["scid"].(string); ok {
            supportsEpoch := g.CheckAppSupportsEpoch(scid)
            apps[i]["supports_epoch"] = supportsEpoch
            if supportsEpoch {
                apps[i]["epoch_badge"] = "EPOCH Enabled"
            }
        }
    }
    
    return apps
}

// ResolveName tries to resolve a human-friendly TELA app name to a SCID using the Gnomon index.
// Matching strategy (strict first, then relaxed):
// 1) Exact match on display_name (case-insensitive)
// 2) Exact match on name (case-insensitive)
// 3) Prefix match on display_name/name if unique
func (g *GnomonClient) ResolveName(name string) (string, bool) {
    if !g.IsRunning() {
        return "", false
    }

    target := strings.ToLower(strings.TrimSpace(name))
    if target == "" {
        return "", false
    }

    apps := g.GetTELAApps()

    // exact matches first
    for _, app := range apps {
        if dn, ok := app["display_name"].(string); ok && strings.ToLower(dn) == target {
            if scid, ok := app["scid"].(string); ok && scid != "" {
                return scid, true
            }
        }
        if n, ok := app["name"].(string); ok && strings.ToLower(n) == target {
            if scid, ok := app["scid"].(string); ok && scid != "" {
                return scid, true
            }
        }
    }

    // prefix match (collect candidates)
    candidates := make([]string, 0)
    for _, app := range apps {
        if dn, ok := app["display_name"].(string); ok && strings.HasPrefix(strings.ToLower(dn), target) {
            if scid, ok := app["scid"].(string); ok && scid != "" {
                candidates = append(candidates, scid)
            }
        } else if n, ok := app["name"].(string); ok && strings.HasPrefix(strings.ToLower(n), target) {
            if scid, ok := app["scid"].(string); ok && scid != "" {
                candidates = append(candidates, scid)
            }
        }
    }
    if len(candidates) == 1 {
        return candidates[0], true
    }
    return "", false
}

// ResolveDURL resolves an exact dURL (case-insensitive) to a SCID, or returns false
// Handles both with and without "dero://" prefix
func (g *GnomonClient) ResolveDURL(durl string) (string, bool) {
    if !g.IsRunning() { return "", false }
    target := strings.ToLower(strings.TrimSpace(durl))
    if target == "" { return "", false }
    
    // Normalize: remove dero:// prefix if present
    targetNorm := target
    if strings.HasPrefix(targetNorm, "dero://") {
        targetNorm = targetNorm[7:]
    }
    
    apps := g.GetTELAApps()
    for _, app := range apps {
        if du, ok := app["durl"].(string); ok {
            // Normalize stored dURL too
            duNorm := strings.ToLower(strings.TrimSpace(du))
            if strings.HasPrefix(duNorm, "dero://") {
                duNorm = duNorm[7:]
            }
            
            if duNorm == targetNorm {
                if scid, ok := app["scid"].(string); ok && scid != "" {
                    return scid, true
                }
            }
        }
    }
    return "", false
}

// GetRating fetches rating data for a SCID from Gnomon indexed data
func (g *GnomonClient) GetRating(scid string) (*RatingResult, error) {
	if !g.IsRunning() {
		return nil, fmt.Errorf("gnomon is not running")
	}

	// Get all variables for this SCID
	vars := g.GetAllSCIDVariableDetails(scid)
	if len(vars) == 0 {
		// No data indexed yet, return empty result
		return &RatingResult{
			SCID:     scid,
			Ratings:  make([]Rating, 0),
			Likes:    0,
			Dislikes: 0,
			Average:  0.0,
			Count:    0,
		}, nil
	}

	result := &RatingResult{
		SCID:    scid,
		Ratings: make([]Rating, 0),
		Likes:   0,
		Dislikes: 0,
		Average: 0.0,
		Count:   0,
	}

	// Parse variables
	for _, v := range vars {
		var (
			key, _, value = parseVars(v)
			decoded       = decodeHexIfNeeded(value)
		)

		switch key {
		case "likes":
			// Parse likes count
			if val, err := parseUint64Safe(decoded); err == nil {
				result.Likes = val
			}

		case "dislikes":
			// Parse dislikes count
			if val, err := parseUint64Safe(decoded); err == nil {
				result.Dislikes = val
			}

		default:
			// Check if this is a rating (key is a DERO address)
			if strings.HasPrefix(strings.ToLower(key), "dero") {
				// Parse rating string (format: "rating_height")

				parts := strings.Split(decoded, "_")
				if len(parts) < 2 {
					continue
				}

				ratingNum, err := parseUint64Safe(parts[0])
				if err != nil || ratingNum > 99 {
					continue
				}

				heightNum, err := parseUint64Safe(parts[1])
				if err != nil {
					continue
				}

				result.Ratings = append(result.Ratings, Rating{
					Address: key,
					Rating:  ratingNum,
					Height:  heightNum,
				})
			}
		}
	}

	// Calculate average from categories (first digit of rating)
	if len(result.Ratings) > 0 {
		var sum uint64
		for _, r := range result.Ratings {
			category := r.Rating / 10 // Extract category (0-9)
			sum += category
		}
		result.Average = float64(sum) / float64(len(result.Ratings))
		result.Count = len(result.Ratings)
	}

	return result, nil
}

// decodeHexIfNeeded decodes a hex string if it looks like hex, otherwise returns as-is
func decodeHexIfNeeded(s string) string {
	// If already a number string, return it
	if _, err := parseUint64Safe(s); err == nil {
		return s
	}
	// Try hex decoding
	return decodeHexString(s)
}

// parseUint64Safe safely parses a string to uint64
func parseUint64Safe(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	return strconv.ParseUint(s, 10, 64)
}

// SearchByKey searches all indexed SCIDs for those containing a specific key store
// Returns SCIDs with the key's values
func (g *GnomonClient) SearchByKey(key string) []map[string]interface{} {
	results := make([]map[string]interface{}, 0)

	if !g.IsRunning() {
		return results
	}

	// Get all SCIDs
	scids := g.GetAllOwnersAndSCIDs()

	for scid, owner := range scids {
		// Check if this SCID has the key
		valuesString, valuesUint64 := g.GetSCIDValuesByKey(scid, key)

		if len(valuesString) > 0 || len(valuesUint64) > 0 {
			var (
				// Get additional info (dURL, name)
				vars            = g.GetAllSCIDVariableDetails(scid)
				params          = map[string]any{"scid": scid, "owner": owner, "key": key}
				result, _, _, _ = allocateData(vars, params)
			)

			// Add found values
			if len(valuesString) > 0 {
				result["values_string"] = valuesString
			}
			if len(valuesUint64) > 0 {
				result["values_uint64"] = valuesUint64
			}

			results = append(results, result)
		}
	}

	return results
}

// SearchByValue searches all indexed SCIDs for those containing a specific value store
// Returns SCIDs with the value's keys
func (g *GnomonClient) SearchByValue(value interface{}) []map[string]interface{} {
	results := make([]map[string]interface{}, 0)

	if !g.IsRunning() {
		return results
	}

	// Get all SCIDs
	scids := g.GetAllOwnersAndSCIDs()

	for scid, owner := range scids {
		// Check if this SCID has the value
		keysString, keysUint64 := g.GetSCIDKeysByValue(scid, value)

		if len(keysString) > 0 || len(keysUint64) > 0 {
			var (
				params = map[string]any{"scid": scid, "owner": owner, "value": value}

				// Get additional info (dURL, name)
				vars = g.GetAllSCIDVariableDetails(scid)

				result, _, _, _ = allocateData(vars, params)
			)
			// Add found keys
			if len(keysString) > 0 {
				result["keys_string"] = keysString
			}
			if len(keysUint64) > 0 {
				result["keys_uint64"] = keysUint64
			}

			results = append(results, result)
		}
	}

	return results
}

// SearchCodeLine returns all indexed SCIDs for code searching
// Note: Code search requires daemon calls - this just returns SCIDs for the caller to check
// The actual code fetching/searching is done by the App layer which has daemon access
func (g *GnomonClient) SearchCodeLine(line string) []map[string]interface{} {
	results := make([]map[string]interface{}, 0)

	if !g.IsRunning() || line == "" {
		return results
	}

	// Get all SCIDs with metadata
	scids := g.GetAllOwnersAndSCIDs()

	for scid, owner := range scids {
		var (
			vars            = g.GetAllSCIDVariableDetails(scid)
			params          = map[string]any{"scid": scid, "owner": owner}
			result, _, _, _ = allocateData(vars, params)
		)
		results = append(results, result)
	}

	return results
}

// CleanDB deletes the Gnomon database for a specific network
// Must stop Gnomon first before calling this
func (g *GnomonClient) CleanDB(network string) error {
	if g.IsRunning() {
		return fmt.Errorf("gnomon must be stopped before cleaning database")
	}

	// Determine data path based on network
	// Use UserHomeDir instead of Getwd for packaged macOS apps
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".dero", "hologram", "datashards")
	var dbPath string
	switch strings.ToLower(network) {
	case "mainnet":
		dbPath = filepath.Join(baseDir, "gnomon_mainnet")
	case "simulator":
		dbPath = filepath.Join(baseDir, "gnomon_simulator")
	default:
		// Legacy path (just "gnomon" folder)
		dbPath = filepath.Join(baseDir, "gnomon")
	}

	// Check if path exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database path does not exist: %s", dbPath)
	}

	// Remove the directory
	if err := os.RemoveAll(dbPath); err != nil {
		return fmt.Errorf("failed to remove database: %w", err)
	}

	return nil
}

// GetMyDOCs returns all DOCs owned by the specified wallet address
// If docType is non-empty, filters by that specific document type
func (g *GnomonClient) GetMyDOCs(walletAddress string, docType string) []map[string]interface{} {
	results := make([]map[string]interface{}, 0)

	if !g.IsRunning() || walletAddress == "" {
		return results
	}

	// Get all SCIDs
	scids := g.GetAllOwnersAndSCIDs()

	for scid, owner := range scids {
		// Filter by owner
		if !strings.EqualFold(owner, walletAddress) {
			continue
		}
		var (
			vars = g.GetAllSCIDVariableDetails(scid)

			// Check if this is a DOC (has docType variable)
			params           = map[string]any{"scid": scid, "owner": owner, "type": "DOC"}
			doc, _, isDOC, _ = allocateData(vars, params)
		)

		// Skip if not a DOC
		if !isDOC {
			continue
		}

		// Filter by docType if specified
		scidDocType, ok := doc["docType"]
		if docType != "" && ok && scidDocType == docType {
			continue
		}

		// Generate display name
		results = append(results, doc)
	}

	return results
}

// GetMyINDEXes returns all INDEXes owned by the specified wallet address
func (g *GnomonClient) GetMyINDEXes(walletAddress string) []map[string]interface{} {
	results := make([]map[string]interface{}, 0)

	if !g.IsRunning() || walletAddress == "" {
		return results
	}

	// Get all SCIDs
	scids := g.GetAllOwnersAndSCIDs()

	for scid, owner := range scids {
		// Filter by owner
		if !strings.EqualFold(owner, walletAddress) {
			continue
		}

		var (
			vars = g.GetAllSCIDVariableDetails(scid)
			// Check if this is an INDEX (has DOC1 or more DOC references)
			params               = map[string]interface{}{"scid": scid, "owner": owner, "type": "INDEX"}
			index, isINDEX, _, _ = allocateData(vars, params)
		)

		// Skip if not an INDEX
		if !isINDEX {
			continue
		}

		results = append(results, index)
	}

	return results
}

// GetAllDOCTypes returns all unique docType values from indexed DOCs
func (g *GnomonClient) GetAllDOCTypes() []string {
	types := make(map[string]bool)

	if !g.IsRunning() {
		return []string{}
	}

	scids := g.GetAllOwnersAndSCIDs()

	for scid := range scids {
		vars := g.GetAllSCIDVariableDetails(scid)
		for _, v := range vars {
			var (
				key, present, value = parseVars(v)
				isDoc               = key == "docType"
			)
			if isDoc && present {
				docType := value
				if docType != "" {
					types[docType] = true
				}
			}
		}
	}

	result := make([]string, 0, len(types))
	for t := range types {
		result = append(result, t)
	}
	return result
}

// cleanupAppName cleans up app names for better display
func cleanupAppName(name string) string {
	original := name
	
	// Remove common URL prefixes
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimPrefix(name, "www.")
	
	// If it looks like a URL or long path, extract domain name only
	if strings.Contains(name, "/") || strings.Contains(name, ".") {
		parts := strings.Split(name, "/")
		
		// Get domain (first part before /)
		domain := parts[0]
		
		// Extract main domain name (remove subdomains and TLD for cleaner look)
		domainParts := strings.Split(domain, ".")
		if len(domainParts) >= 2 {
			// For things like "gateway.pinata.cloud" -> "Pinata"
			// For "raw.githubusercontent.com" -> "Github"
			// For "avatars.githubusercontent.com" -> "Github"
			
			mainPart := ""
			if len(domainParts) >= 3 {
				// Use second-to-last part (before TLD)
				mainPart = domainParts[len(domainParts)-2]
			} else {
				// Use first part
				mainPart = domainParts[0]
			}
			
			// Special handling for known services
			if strings.Contains(domain, "github") {
				mainPart = "GitHub"
			} else if strings.Contains(domain, "pinata") {
				mainPart = "Pinata"
			} else if strings.Contains(domain, "dero") {
				mainPart = "DERO"
			} else if strings.Contains(domain, "loc.gov") {
				mainPart = "Library of Congress"
			} else {
				// Capitalize first letter
				if len(mainPart) > 0 {
					mainPart = strings.ToUpper(string(mainPart[0])) + mainPart[1:]
				}
			}
			
			return mainPart
		}
	}
	
	// If not a URL, just clean it up
	name = strings.TrimSpace(name)
	
	// Limit length to 40 characters for uniformity
	if len(name) > 40 {
		return name[:37] + "..."
	}
	
	// If we couldn't clean it up, return first 40 chars of original
	if name == "" && len(original) > 0 {
		if len(original) > 40 {
			return original[:37] + "..."
		}
		return original
	}
	
	return name
}

func parseVars(v *structures.SCIDVariable) (key string, present bool, val string) {
	return fmt.Sprintf("%v", v.Key), v.Value != nil, fmt.Sprintf("%v", v.Value)
}

func allocateData(
	vars []*structures.SCIDVariable,
	params map[string]any,
) (
	data map[string]any, isIndex, isDOC, hasLibTag bool,
) {

	data = params

	var (
		docSCIDs = make([]string, 0)
		docCount = 0
	)

	for _, v := range vars {
		var (
			key, present, value = parseVars(v)
			decodedValue        = decodeHexString(value)
		)

		switch {
		// V2 headers (TELA standard) - check first
		case key == "var_header_name":
			if present {
				data["name"] = decodedValue
			}
		case key == "var_header_description":
			if present {
				data["description"] = decodedValue
			}
		case key == "var_header_icon":
			if present {
				data["icon"] = decodedValue
			}
		// V1 headers (ART-NFA standard) - fallback if V2 not set
		case key == "nameHdr":
			if present && data["name"] == nil {
				data["name"] = decodedValue
			}
		case key == "descrHdr":
			if present && data["description"] == nil {
				data["description"] = decodedValue
			}
		case key == "iconURLHdr":
			if present && data["icon"] == nil {
				data["icon"] = decodedValue
			}
		case key == "dURL":
			if present {
				du := decodedValue
				data["url"] = du
				data["durl"] = du
				// Check for .lib suffix
				if strings.HasSuffix(du, ".lib") {
					hasLibTag = true
				}
			}
		case key == "docType":
			// This is a DOC (single file library)
			data["type"] = "DOC"
			isDOC = true
			if present {
				data["docType"] = value
			}
		case strings.HasPrefix(key, "DOC") && len(key) <= 5:
			// Mark as TELA INDEX if it has DOC references
			data["is_index"] = true
			data["type"] = "INDEX"
			isIndex = true
			docCount++
			if docCount > 0 {
				docSCIDs = append(docSCIDs, value)
				data["doc_count"] = docCount
				data["doc_scids"] = docSCIDs
			}
		}
	}

	// Set type if not already set
	if _, hasType := data["type"]; !hasType {
		if _, hasDocType := data["docType"]; hasDocType {
			data["type"] = "DOC"
		} else {
			data["type"] = "SC"
		}
	}

	displayName := ""
	if durl, ok := data["durl"].(string); ok && durl != "" {
		displayName = durl
	} else if name, ok := data["name"].(string); ok && name != "" {
		displayName = name
	} else if isIndex {
		displayName = "INDEX"
	} else if isDOC {
		displayName = "DOC"
	} else if hasLibTag {
		displayName = "TELA Library"
	}

	data["display_name"] = displayName

	return data, isIndex, isDOC, hasLibTag
}
