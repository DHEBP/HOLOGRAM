// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Offline-First TELA Browser - Cache apps for local-first browsing

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deroproject/graviton"
)

const (
	TreeCachedApps     = "cached_apps"      // App metadata
	TreeCachedContent  = "cached_content"   // Actual content
	TreeCacheManifest  = "cache_manifest"   // Manifest for each cached app
	TreeCacheStats     = "cache_stats"      // Cache usage statistics
)

// CachedApp represents a fully cached TELA app
type CachedApp struct {
	SCID          string            `json:"scid"`
	Name          string            `json:"name"`
	Author        string            `json:"author"`
	Description   string            `json:"description"`
	IconURL       string            `json:"icon_url,omitempty"`
	Category      string            `json:"category,omitempty"`
	Version       int               `json:"version"`
	CachedAt      time.Time         `json:"cached_at"`
	LastAccessed  time.Time         `json:"last_accessed"`
	LastUpdated   time.Time         `json:"last_updated"`
	TotalSize     int64             `json:"total_size"`       // Total bytes cached
	FileCount     int               `json:"file_count"`       // Number of files
	IsComplete    bool              `json:"is_complete"`      // All files cached
	SupportsEpoch bool              `json:"supports_epoch"`
	Files         []CachedFile      `json:"files,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// CachedFile represents a single cached file within an app
type CachedFile struct {
	Path        string    `json:"path"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	Hash        string    `json:"hash"`       // Content hash for verification
	CachedAt    time.Time `json:"cached_at"`
	SCID        string    `json:"scid,omitempty"` // Source SCID for the content
}

// CacheManifest tracks what files are cached for an app
type CacheManifest struct {
	AppSCID      string                 `json:"app_scid"`
	Files        map[string]CachedFile  `json:"files"` // path -> file info
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	TotalSize    int64                  `json:"total_size"`
	IsComplete   bool                   `json:"is_complete"`
}

// CacheStats tracks overall cache usage
type CacheStats struct {
	TotalApps       int       `json:"total_apps"`
	TotalFiles      int       `json:"total_files"`
	TotalSize       int64     `json:"total_size"`
	LastCleanup     time.Time `json:"last_cleanup"`
	CacheHits       int64     `json:"cache_hits"`
	CacheMisses     int64     `json:"cache_misses"`
	BytesSaved      int64     `json:"bytes_saved"` // Estimated network savings
}

// OfflineCache provides local-first content caching using Graviton
type OfflineCache struct {
	sync.RWMutex
	store     *graviton.Store
	logFn     func(string)
	basePath  string
	maxSize   int64  // Maximum cache size in bytes
	isEnabled bool
}

// NewOfflineCache creates a new offline cache service
func NewOfflineCache(logFn func(string)) (*OfflineCache, error) {
	wd, _ := os.Getwd()
	cachePath := filepath.Join(wd, "datashards", "offline_cache")
	_ = os.MkdirAll(cachePath, 0755)

	store, err := graviton.NewDiskStore(cachePath)
	if err != nil {
		store, err = graviton.NewMemStore()
		if err != nil {
			return nil, fmt.Errorf("failed to create offline cache store: %v", err)
		}
		if logFn != nil {
			logFn("[WARN] Offline cache using in-memory store (data will not persist)")
		}
	}

	cache := &OfflineCache{
		store:     store,
		logFn:     logFn,
		basePath:  cachePath,
		maxSize:   500 * 1024 * 1024, // 500MB default limit
		isEnabled: true,
	}

	if logFn != nil {
		logFn("[PKG] Offline cache service initialized")
	}

	return cache, nil
}

// Close closes the cache store
func (c *OfflineCache) Close() {
	if c.store != nil {
		c.store.Close()
	}
}

func (c *OfflineCache) log(msg string) {
	if c.logFn != nil {
		c.logFn(msg)
	}
}

// IsEnabled returns whether offline caching is enabled
func (c *OfflineCache) IsEnabled() bool {
	c.RLock()
	defer c.RUnlock()
	return c.isEnabled
}

// SetEnabled enables or disables offline caching
func (c *OfflineCache) SetEnabled(enabled bool) {
	c.Lock()
	c.isEnabled = enabled
	c.Unlock()
}

// SetMaxSize sets the maximum cache size
func (c *OfflineCache) SetMaxSize(bytes int64) {
	c.Lock()
	c.maxSize = bytes
	c.Unlock()
}

// ==================== App Caching ====================

// CacheApp downloads and caches a complete TELA app
func (c *OfflineCache) CacheApp(scid string, appData *CachedApp, files map[string][]byte) error {
	c.Lock()
	defer c.Unlock()

	if !c.isEnabled {
		return fmt.Errorf("offline cache is disabled")
	}

	// Check if we have space
	stats, _ := c.getStats()
	totalNewSize := int64(0)
	for _, content := range files {
		totalNewSize += int64(len(content))
	}

	if stats.TotalSize+totalNewSize > c.maxSize {
		// Try to free space
		if err := c.evictOldest(totalNewSize); err != nil {
			return fmt.Errorf("insufficient cache space: %v", err)
		}
	}

	// Store app metadata
	ss, err := c.store.LoadSnapshot(0)
	if err != nil {
		return err
	}

	// Update app metadata
	appData.CachedAt = time.Now()
	appData.LastUpdated = time.Now()
	appData.TotalSize = totalNewSize
	appData.FileCount = len(files)
	appData.IsComplete = true

	// Store files
	contentTree, _ := ss.GetTree(TreeCachedContent)
	manifest := CacheManifest{
		AppSCID:   scid,
		Files:     make(map[string]CachedFile),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	for path, content := range files {
		// Store content
		key := fmt.Sprintf("%s:%s", scid, path)
		if err := contentTree.Put([]byte(key), content); err != nil {
			return err
		}

		// Record in manifest
		cachedFile := CachedFile{
			Path:     path,
			Size:     int64(len(content)),
			CachedAt: time.Now(),
		}
		manifest.Files[path] = cachedFile
		manifest.TotalSize += int64(len(content))
		appData.Files = append(appData.Files, cachedFile)
	}

	manifest.IsComplete = true

	// Store manifest
	manifestTree, _ := ss.GetTree(TreeCacheManifest)
	manifestData, _ := json.Marshal(manifest)
	if err := manifestTree.Put([]byte(scid), manifestData); err != nil {
		return err
	}

	// Store app metadata
	appsTree, _ := ss.GetTree(TreeCachedApps)
	appDataBytes, _ := json.Marshal(appData)
	if err := appsTree.Put([]byte(scid), appDataBytes); err != nil {
		return err
	}

	// Commit all changes
	_, err = graviton.Commit(contentTree, manifestTree, appsTree)
	if err != nil {
		return err
	}

	c.log(fmt.Sprintf("[PKG] Cached app %s (%d files, %s)", scid[:16], len(files), formatBytes(totalNewSize)))
	return nil
}

// GetCachedContent retrieves content from cache
func (c *OfflineCache) GetCachedContent(scid, path string) ([]byte, bool, error) {
	c.RLock()
	defer c.RUnlock()

	ss, err := c.store.LoadSnapshot(0)
	if err != nil {
		return nil, false, err
	}

	contentTree, _ := ss.GetTree(TreeCachedContent)
	key := fmt.Sprintf("%s:%s", scid, path)
	content, err := contentTree.Get([]byte(key))
	if err != nil || content == nil {
		c.incrementMiss()
		return nil, false, nil
	}

	c.incrementHit()
	c.updateLastAccessed(scid)

	return content, true, nil
}

// IsAppCached checks if an app is fully cached
func (c *OfflineCache) IsAppCached(scid string) (bool, *CachedApp, error) {
	c.RLock()
	defer c.RUnlock()

	ss, err := c.store.LoadSnapshot(0)
	if err != nil {
		return false, nil, err
	}

	appsTree, _ := ss.GetTree(TreeCachedApps)
	data, err := appsTree.Get([]byte(scid))
	if err != nil || data == nil {
		return false, nil, nil
	}

	var app CachedApp
	if err := json.Unmarshal(data, &app); err != nil {
		return false, nil, err
	}

	return app.IsComplete, &app, nil
}

// GetCachedApps returns all cached apps
func (c *OfflineCache) GetCachedApps() ([]CachedApp, error) {
	c.RLock()
	defer c.RUnlock()

	ss, err := c.store.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	appsTree, _ := ss.GetTree(TreeCachedApps)
	cursor := appsTree.Cursor()

	apps := []CachedApp{}
	for k, v, err := cursor.First(); err == nil; k, v, err = cursor.Next() {
		if k == nil {
			break
		}

		var app CachedApp
		if json.Unmarshal(v, &app) == nil {
			apps = append(apps, app)
		}
	}

	return apps, nil
}

// RemoveCachedApp removes an app from cache
func (c *OfflineCache) RemoveCachedApp(scid string) error {
	c.Lock()
	defer c.Unlock()

	ss, err := c.store.LoadSnapshot(0)
	if err != nil {
		return err
	}

	// Get manifest to know which files to delete
	manifestTree, _ := ss.GetTree(TreeCacheManifest)
	manifestData, err := manifestTree.Get([]byte(scid))

	if err == nil && manifestData != nil {
		var manifest CacheManifest
		if json.Unmarshal(manifestData, &manifest) == nil {
			// Delete all cached files
			contentTree, _ := ss.GetTree(TreeCachedContent)
			for path := range manifest.Files {
				key := fmt.Sprintf("%s:%s", scid, path)
				contentTree.Delete([]byte(key))
			}
			graviton.Commit(contentTree)
		}
	}

	// Delete manifest
	manifestTree.Delete([]byte(scid))

	// Delete app metadata
	appsTree, _ := ss.GetTree(TreeCachedApps)
	appsTree.Delete([]byte(scid))

	_, err = graviton.Commit(manifestTree, appsTree)
	if err != nil {
		return err
	}

	c.log(fmt.Sprintf("🗑️ Removed cached app %s", scid[:16]))
	return nil
}

// ==================== Cache Statistics ====================

// GetCacheStats returns cache statistics
func (c *OfflineCache) GetCacheStats() (*CacheStats, error) {
	c.RLock()
	defer c.RUnlock()
	return c.getStats()
}

func (c *OfflineCache) getStats() (*CacheStats, error) {
	ss, err := c.store.LoadSnapshot(0)
	if err != nil {
		return nil, err
	}

	// Try to load existing stats
	statsTree, _ := ss.GetTree(TreeCacheStats)
	data, _ := statsTree.Get([]byte("global"))

	stats := &CacheStats{}
	if data != nil {
		json.Unmarshal(data, stats)
	}

	// Recalculate from actual data
	appsTree, _ := ss.GetTree(TreeCachedApps)
	cursor := appsTree.Cursor()

	stats.TotalApps = 0
	stats.TotalFiles = 0
	stats.TotalSize = 0

	for k, v, err := cursor.First(); err == nil; k, v, err = cursor.Next() {
		if k == nil {
			break
		}

		var app CachedApp
		if json.Unmarshal(v, &app) == nil {
			stats.TotalApps++
			stats.TotalFiles += app.FileCount
			stats.TotalSize += app.TotalSize
		}
	}

	return stats, nil
}

func (c *OfflineCache) updateStats(stats *CacheStats) error {
	ss, err := c.store.LoadSnapshot(0)
	if err != nil {
		return err
	}

	statsTree, _ := ss.GetTree(TreeCacheStats)
	data, _ := json.Marshal(stats)
	if err := statsTree.Put([]byte("global"), data); err != nil {
		return err
	}

	_, err = graviton.Commit(statsTree)
	return err
}

func (c *OfflineCache) incrementHit() {
	stats, _ := c.getStats()
	if stats != nil {
		stats.CacheHits++
		c.updateStats(stats)
	}
}

func (c *OfflineCache) incrementMiss() {
	stats, _ := c.getStats()
	if stats != nil {
		stats.CacheMisses++
		c.updateStats(stats)
	}
}

func (c *OfflineCache) updateLastAccessed(scid string) {
	ss, _ := c.store.LoadSnapshot(0)
	appsTree, _ := ss.GetTree(TreeCachedApps)

	data, err := appsTree.Get([]byte(scid))
	if err != nil || data == nil {
		return
	}

	var app CachedApp
	if json.Unmarshal(data, &app) == nil {
		app.LastAccessed = time.Now()
		newData, _ := json.Marshal(app)
		appsTree.Put([]byte(scid), newData)
		graviton.Commit(appsTree)
	}
}

// ==================== Cache Maintenance ====================

// evictOldest removes oldest cached apps to free space
func (c *OfflineCache) evictOldest(bytesNeeded int64) error {
	apps, err := c.GetCachedApps()
	if err != nil {
		return err
	}

	// Sort by last accessed (oldest first)
	for i := 0; i < len(apps); i++ {
		for j := i + 1; j < len(apps); j++ {
			if apps[j].LastAccessed.Before(apps[i].LastAccessed) {
				apps[i], apps[j] = apps[j], apps[i]
			}
		}
	}

	bytesFreed := int64(0)
	for _, app := range apps {
		if bytesFreed >= bytesNeeded {
			break
		}
		c.RemoveCachedApp(app.SCID)
		bytesFreed += app.TotalSize
		c.log(fmt.Sprintf("🗑️ Evicted %s to free %s", app.SCID[:16], formatBytes(app.TotalSize)))
	}

	if bytesFreed < bytesNeeded {
		return fmt.Errorf("could only free %s of %s needed", formatBytes(bytesFreed), formatBytes(bytesNeeded))
	}

	return nil
}

// CleanupOldApps removes apps not accessed within the specified duration
func (c *OfflineCache) CleanupOldApps(maxAge time.Duration) (int, int64, error) {
	c.Lock()
	defer c.Unlock()

	apps, err := c.GetCachedApps()
	if err != nil {
		return 0, 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	removedCount := 0
	bytesFreed := int64(0)

	for _, app := range apps {
		if app.LastAccessed.Before(cutoff) {
			if err := c.RemoveCachedApp(app.SCID); err == nil {
				removedCount++
				bytesFreed += app.TotalSize
			}
		}
	}

	// Update stats
	stats, _ := c.getStats()
	if stats != nil {
		stats.LastCleanup = time.Now()
		c.updateStats(stats)
	}

	c.log(fmt.Sprintf("🧹 Cache cleanup: removed %d apps, freed %s", removedCount, formatBytes(bytesFreed)))
	return removedCount, bytesFreed, nil
}

// ClearCache removes all cached content
func (c *OfflineCache) ClearCache() error {
	c.Lock()
	defer c.Unlock()

	apps, err := c.GetCachedApps()
	if err != nil {
		return err
	}

	for _, app := range apps {
		c.RemoveCachedApp(app.SCID)
	}

	c.log("🗑️ Cache cleared")
	return nil
}

// ==================== Helper Functions ====================

// formatBytes formats bytes to human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ==================== App Bindings ====================

// PrefetchApp downloads and caches a TELA app for offline use
func (a *App) PrefetchApp(scid string) map[string]interface{} {
	if a.offlineCache == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Offline cache not initialized",
		}
	}

	// Check if already cached
	isCached, cachedApp, _ := a.offlineCache.IsAppCached(scid)
	if isCached {
		return map[string]interface{}{
			"success":       true,
			"already_cached": true,
			"app":           cachedApp,
		}
	}

	// Get app from blockchain
	telaContent, err := a.FetchTELAContent(scid)
	if err != nil {
		return ErrorResponse(err)
	}

	// Extract files from content
	files := make(map[string][]byte)
	if telaContent.HTML != "" {
		files["index.html"] = []byte(telaContent.HTML)
	}
	// Add CSS and JS files
	for name, css := range telaContent.CSSByName {
		files[name] = []byte(css)
	}
	for name, js := range telaContent.JSByName {
		files[name] = []byte(js)
	}

	// Create app metadata from Meta field
	appData := &CachedApp{
		SCID: scid,
	}
	if meta := telaContent.Meta; meta != nil {
		if name, ok := meta["name"].(string); ok {
			appData.Name = name
		}
		if author, ok := meta["author"].(string); ok {
			appData.Author = author
		}
		if desc, ok := meta["description"].(string); ok {
			appData.Description = desc
		}
	}

	// Check if app supports EPOCH
	epochCheck := a.CheckAppSupportsEpoch(scid)
	if supports, ok := epochCheck["supports_epoch"].(bool); ok {
		appData.SupportsEpoch = supports
	}

	// Cache the app
	if err := a.offlineCache.CacheApp(scid, appData, files); err != nil {
		return ErrorResponse(err)
	}

	return map[string]interface{}{
		"success": true,
		"app":     appData,
		"message": fmt.Sprintf("Cached %s for offline use", appData.Name),
	}
}

// GetCachedApps returns all apps available offline
func (a *App) GetCachedApps() map[string]interface{} {
	if a.offlineCache == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Offline cache not initialized",
		}
	}

	apps, err := a.offlineCache.GetCachedApps()
	if err != nil {
		return ErrorResponse(err)
	}

	return map[string]interface{}{
		"success": true,
		"apps":    apps,
		"count":   len(apps),
	}
}

// IsAppCachedOffline checks if an app is available offline
func (a *App) IsAppCachedOffline(scid string) map[string]interface{} {
	if a.offlineCache == nil {
		return map[string]interface{}{
			"success": false,
			"cached":  false,
		}
	}

	isCached, app, _ := a.offlineCache.IsAppCached(scid)
	return map[string]interface{}{
		"success": true,
		"cached":  isCached,
		"app":     app,
	}
}

// RemoveCachedApp removes an app from offline cache
func (a *App) RemoveCachedApp(scid string) map[string]interface{} {
	if a.offlineCache == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Offline cache not initialized",
		}
	}

	if err := a.offlineCache.RemoveCachedApp(scid); err != nil {
		return ErrorResponse(err)
	}

	return map[string]interface{}{
		"success": true,
	}
}

// GetOfflineCacheStats returns cache statistics
func (a *App) GetOfflineCacheStats() map[string]interface{} {
	if a.offlineCache == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Offline cache not initialized",
		}
	}

	stats, err := a.offlineCache.GetCacheStats()
	if err != nil {
		return ErrorResponse(err)
	}

	return map[string]interface{}{
		"success":       true,
		"stats":         stats,
		"max_size":      a.offlineCache.maxSize,
		"max_size_str":  formatBytes(a.offlineCache.maxSize),
		"used_size_str": formatBytes(stats.TotalSize),
		"usage_percent": float64(stats.TotalSize) / float64(a.offlineCache.maxSize) * 100,
	}
}

// SetOfflineCacheEnabled enables or disables offline caching
func (a *App) SetOfflineCacheEnabled(enabled bool) map[string]interface{} {
	if a.offlineCache == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Offline cache not initialized",
		}
	}

	a.offlineCache.SetEnabled(enabled)
	return map[string]interface{}{
		"success": true,
		"enabled": enabled,
	}
}

// ClearOfflineCache removes all cached content
func (a *App) ClearOfflineCache() map[string]interface{} {
	if a.offlineCache == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Offline cache not initialized",
		}
	}

	if err := a.offlineCache.ClearCache(); err != nil {
		return ErrorResponse(err)
	}

	return map[string]interface{}{
		"success": true,
		"message": "Cache cleared",
	}
}

