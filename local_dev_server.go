package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LocalDevServer manages local TELA development preview
type LocalDevServer struct {
	app       *App
	server    *http.Server
	watcher   *fsnotify.Watcher
	directory string
	port      int
	url       string
	mu        sync.Mutex
	running   bool
}

// localDevServer singleton
var localDevServer *LocalDevServer
var localDevServerMu sync.Mutex

// getLocalDevServer returns the singleton instance
func getLocalDevServer(app *App) *LocalDevServer {
	localDevServerMu.Lock()
	defer localDevServerMu.Unlock()

	if localDevServer == nil {
		localDevServer = &LocalDevServer{app: app}
	}
	return localDevServer
}

// StartLocalDevServer starts serving files from a local directory
func (a *App) StartLocalDevServer(directory string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[START] Starting local dev server for: %s", directory))

	// Validate directory exists
	info, err := os.Stat(directory)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Directory not found: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Directory not found: %v", err),
		}
	}
	if !info.IsDir() {
		a.logToConsole("[ERR] Path is not a directory")
		return map[string]interface{}{
			"success": false,
			"error":   "Path is not a directory",
		}
	}

	// Check for index.html
	indexPath := filepath.Join(directory, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		a.logToConsole("[ERR] No index.html found in directory")
		return map[string]interface{}{
			"success": false,
			"error":   "No index.html found in directory. TELA apps must have an index.html file.",
		}
	}

	lds := getLocalDevServer(a)
	lds.mu.Lock()
	defer lds.mu.Unlock()

	// Stop existing server if running
	if lds.running {
		a.logToConsole("[SYNC] Stopping existing local dev server...")
		lds.stopInternal()
	}

	// Find available port
	port, err := findAvailablePort()
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Could not find available port: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Could not find available port: %v", err),
		}
	}

	// Create file server with CORS headers
	fs := http.FileServer(http.Dir(directory))
	handler := localDevCORSMiddleware(fs)

	// Create server
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	lds.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	lds.directory = directory
	lds.port = port
	lds.url = fmt.Sprintf("http://%s", addr)
	lds.app = a

	// Start server in goroutine
	go func() {
		a.logToConsole(fmt.Sprintf("[NET] Local dev server listening on %s", lds.url))
		if err := lds.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logToConsole(fmt.Sprintf("[ERR] Local dev server error: %v", err))
		}
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	lds.running = true

	// Start file watcher in background (non-blocking)
	go func() {
		if err := lds.startWatcher(); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] File watcher failed to start: %v (hot reload disabled)", err))
		}
	}()

	// Register in server registry
	serverRegistry.Lock()
	serverRegistry.servers["local-dev"] = &ActiveServer{
		Name:      "local-dev",
		Port:      port,
		URL:       lds.url,
		IsLocal:   true,
		Directory: directory,
	}
	serverRegistry.Unlock()

	a.logToConsole(fmt.Sprintf("[OK] Local dev server started at %s", lds.url))

	return map[string]interface{}{
		"success":   true,
		"url":       lds.url,
		"port":      port,
		"directory": directory,
		"message":   "Local dev server started",
	}
}

// StopLocalDevServer stops the local dev server
func (a *App) StopLocalDevServer() map[string]interface{} {
	lds := getLocalDevServer(a)
	lds.mu.Lock()
	defer lds.mu.Unlock()

	if !lds.running {
		return map[string]interface{}{
			"success": true,
			"message": "Server was not running",
		}
	}

	lds.stopInternal()
	a.logToConsole("[OK] Local dev server stopped")

	return map[string]interface{}{
		"success": true,
		"message": "Local dev server stopped",
	}
}

// stopInternal stops the server without locking (caller must hold lock)
func (lds *LocalDevServer) stopInternal() {
	// Stop file watcher
	if lds.watcher != nil {
		lds.watcher.Close()
		lds.watcher = nil
	}

	// Shutdown HTTP server
	if lds.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		lds.server.Shutdown(ctx)
		lds.server = nil
	}

	// Remove from registry
	serverRegistry.Lock()
	delete(serverRegistry.servers, "local-dev")
	serverRegistry.Unlock()

	lds.running = false
	lds.url = ""
	lds.port = 0
	lds.directory = ""
}

// GetLocalDevServerStatus returns the current status of the local dev server
func (a *App) GetLocalDevServerStatus() map[string]interface{} {
	lds := getLocalDevServer(a)
	lds.mu.Lock()
	defer lds.mu.Unlock()

	watcherActive := lds.watcher != nil

	return map[string]interface{}{
		"running":       lds.running,
		"url":           lds.url,
		"port":          lds.port,
		"directory":     lds.directory,
		"watcherActive": watcherActive,
	}
}

// RefreshLocalDevServer triggers a manual refresh event for the local dev server
func (a *App) RefreshLocalDevServer() map[string]interface{} {
	lds := getLocalDevServer(a)
	lds.mu.Lock()
	defer lds.mu.Unlock()

	if !lds.running {
		return map[string]interface{}{
			"success": false,
			"error":   "Local dev server is not running",
		}
	}

	// Emit reload event
	runtime.EventsEmit(a.ctx, "localdev:reload", map[string]interface{}{
		"file":      "manual",
		"timestamp": time.Now().Unix(),
	})

	a.logToConsole("[SYNC] Manual refresh triggered")

	return map[string]interface{}{
		"success": true,
		"message": "Refresh triggered",
	}
}

// ================== Helper Functions ==================

// findAvailablePort finds an available port starting from 8080
func findAvailablePort() (int, error) {
	// Try ports in the 8080-9000 range
	for port := 8080; port < 9000; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range 8080-9000")
}

// localDevCORSMiddleware adds CORS headers and proper MIME types for local development
func localDevCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for development
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Ensure proper MIME types for common web files
		ext := filepath.Ext(r.URL.Path)
		switch ext {
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".js", ".mjs":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case ".json":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".html", ".htm":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}

		next.ServeHTTP(w, r)
	})
}

// ================== File Watcher (Hot Reload) ==================

// startWatcher initializes the file watcher for hot reload
func (lds *LocalDevServer) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	lds.watcher = watcher

	// Watch the directory recursively
	watchCount := 0
	err = filepath.Walk(lds.directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if info.IsDir() {
			// Skip hidden directories
			name := info.Name()
			if len(name) > 0 && name[0] == '.' {
				return filepath.SkipDir
			}
			// Skip common non-source directories
			if name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			if err := watcher.Add(path); err == nil {
				watchCount++
			}
		}
		return nil
	})
	if err != nil {
		watcher.Close()
		return fmt.Errorf("failed to setup watcher: %w", err)
	}

	// Start watching goroutine
	go lds.watchLoop()

	lds.app.logToConsole(fmt.Sprintf("👀 File watcher active - watching %d directories", watchCount))
	return nil
}

// watchLoop handles file system events
func (lds *LocalDevServer) watchLoop() {
	// Debounce timer to prevent rapid-fire events
	var debounceTimer *time.Timer
	debounceDelay := 300 * time.Millisecond

	for {
		select {
		case event, ok := <-lds.watcher.Events:
			if !ok {
				return // Watcher closed
			}

			// Only trigger on write/create events
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Check if it's a relevant file type
				ext := filepath.Ext(event.Name)
				if isWatchedExtension(ext) {
					// Debounce: reset timer on each event
					if debounceTimer != nil {
						debounceTimer.Stop()
					}

					// Capture file name for closure
					fileName := event.Name

					debounceTimer = time.AfterFunc(debounceDelay, func() {
						lds.mu.Lock()
						running := lds.running
						app := lds.app
						lds.mu.Unlock()

						if running && app != nil && app.ctx != nil {
							baseName := filepath.Base(fileName)
							app.logToConsole(fmt.Sprintf("[SYNC] File changed: %s", baseName))

							// Emit reload event to frontend
							runtime.EventsEmit(app.ctx, "localdev:reload", map[string]interface{}{
								"file":      baseName,
								"fullPath":  fileName,
								"timestamp": time.Now().Unix(),
							})
						}
					})
				}
			}

		case err, ok := <-lds.watcher.Errors:
			if !ok {
				return // Watcher closed
			}
			if lds.app != nil {
				lds.app.logToConsole(fmt.Sprintf("[WARN] File watcher error: %v", err))
			}
		}
	}
}

// isWatchedExtension checks if a file extension should trigger reload
func isWatchedExtension(ext string) bool {
	watched := map[string]bool{
		".html": true,
		".htm":  true,
		".css":  true,
		".js":   true,
		".mjs":  true,
		".json": true,
		".svg":  true,
		".png":  true,
		".jpg":  true,
		".jpeg": true,
		".gif":  true,
		".webp": true,
		".ico":  true,
		".woff": true,
		".woff2": true,
		".ttf":  true,
	}
	return watched[ext]
}
