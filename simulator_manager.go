package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deroproject/derohe/globals"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SimulatorManager provides unified control for all simulator operations
type SimulatorManager struct {
	sync.RWMutex
	app           *App
	walletManager *SimulatorWalletManager // Manages all 22 pre-seeded test wallets
	isInitialized bool
	isStarting    bool
	baseDir       string
}

// SimulatorStatus represents the current state of the simulator
type SimulatorStatus struct {
	IsInitialized bool                   `json:"isInitialized"`
	IsStarting    bool                   `json:"isStarting"`
	DaemonRunning bool                   `json:"daemonRunning"`
	WalletOpen    bool                   `json:"walletOpen"`
	WalletAddress string                 `json:"walletAddress"`
	Balance       uint64                 `json:"balance"`
	BlockHeight   int64                  `json:"blockHeight"`
	RpcEndpoint   string                 `json:"rpcEndpoint"`
	Extra         map[string]interface{} `json:"extra"`
}

// NewSimulatorManager creates a new simulator manager
func NewSimulatorManager(app *App) *SimulatorManager {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".dero", "hologram")

	return &SimulatorManager{
		app:           app,
		walletManager: NewSimulatorWalletManager(app.logToConsole),
		baseDir:       baseDir,
	}
}

// ================== Main Simulator Operations ==================

// StartSimulatorMode is the ONE-CLICK simulator activation
// It handles everything: daemon, wallet, initial funding
func (sm *SimulatorManager) StartSimulatorMode() map[string]interface{} {
	sm.Lock()
	if sm.isStarting {
		sm.Unlock()
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator is already starting",
		}
	}
	if sm.isInitialized {
		sm.Unlock()
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator is already running",
		}
	}
	sm.isStarting = true
	sm.Unlock()

	defer func() {
		sm.Lock()
		sm.isStarting = false
		sm.Unlock()
	}()

	sm.app.logToConsole("[SIM] Starting Simulator Mode...")
	
	// CRITICAL: Set globals for simulator mode EARLY
	// This must happen before any walletapi.Connect() calls throughout the simulator lifecycle
	// The walletapi checks globals.IsMainnet() which compares Config.Name
	// InitNetwork() sets Config based on --testnet flag, so we must set BOTH flags
	globals.Arguments["--simulator"] = true
	globals.Arguments["--testnet"] = true // Required for InitNetwork() to set Config = Testnet
	globals.InitNetwork()
	sm.app.logToConsole("[SIM] Set globals for simulator mode (--simulator=true, --testnet=true)")
	
	// Emit initial progress event
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    0,
			"message": "Starting simulator mode...",
			"status":  "starting",
		})
	}

	// Step 0: Check current connection status and save for rollback
	sm.app.logToConsole("[...] Step 0: Checking current node connection...")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    0,
			"message": "Checking current node connection...",
			"status":  "checking",
		})
	}
	currentStatus := sm.app.GetNodeStatus()
	wasExternalConnected := false
	previousEndpoint := ""
	previousNetworkMode := ""
	previousDaemonClient := sm.app.daemonClient
	
	// Safely check if external node is connected (isExternal might not exist in map)
	if isExt, ok := currentStatus["isExternal"].(bool); ok && isExt {
		wasExternalConnected = true
		if rpcPort, ok := currentStatus["rpcPort"].(int); ok {
			previousEndpoint = fmt.Sprintf("http://127.0.0.1:%d", rpcPort)
		}
		// Get current network mode
		netMode := sm.app.GetNetworkMode()
		if net, ok := netMode["network"].(string); ok {
			previousNetworkMode = net
		}
		sm.app.logToConsole(fmt.Sprintf("[WARN] External %s node detected at %s", previousNetworkMode, previousEndpoint))
		sm.app.logToConsole("[INFO] Switching to Simulator will disconnect from external node")
		sm.app.logToConsole("[INFO] If simulator fails, connection will be restored automatically")
	}

	// Step 1: Check simulator binary (not regular derod!)
	sm.app.logToConsole("[PKG] Step 1: Checking simulator binary...")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    1,
			"message": "Checking simulator binary...",
			"status":  "checking",
		})
	}
	binaryPath := GetSimulatorBinaryPath()
	binaryName := getSimulatorBinaryName() // Platform-specific name
	if binaryPath == "" {
		sm.app.logToConsole("[ERR] Simulator binary not found")
		sm.app.logToConsole(fmt.Sprintf("[INFO] Simulator requires %s binary", binaryName))
		sm.app.logToConsole("[INFO] Option 1: Run 'make all' to build from source (recommended)")
		sm.app.logToConsole(fmt.Sprintf("[INFO] Option 2: Download from DERO releases and place in ~/.dero/hologram/derod/{version}/%s", binaryName))
		errorMsg := fmt.Sprintf("Simulator binary (%s) not found. Run 'make all' to build from source, or download from DERO releases.", binaryName)
		if sm.app.ctx != nil {
			wailsRuntime.EventsEmit(sm.app.ctx, "simulator:error", map[string]interface{}{
				"error":      errorMsg,
				"binaryName": binaryName,
				"step":       "check_binary",
			})
		}
		return map[string]interface{}{
			"success":    false,
			"error":      errorMsg,
			"binaryName": binaryName,
			"step":       "check_binary",
		}
	}
	sm.app.logToConsole(fmt.Sprintf("[OK] Simulator binary found: %s", binaryPath))
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    1,
			"message": "Simulator binary found",
			"status":  "complete",
		})
	}

	// Step 2: Start daemon in simulator mode
	sm.app.logToConsole("[START] Step 2: Starting simulator daemon...")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    2,
			"message": "Starting simulator daemon...",
			"status":  "starting",
		})
	}
	
	// First set network mode (this will change daemonClient endpoint)
	modeResult := sm.app.SetNetworkMode("simulator")
	if !modeResult["success"].(bool) {
		// Rollback: restore previous connection
		if wasExternalConnected && previousDaemonClient != nil {
			sm.app.daemonClient = previousDaemonClient
			sm.app.logToConsole("[RESTORE] Rolled back to previous connection")
		}
		if sm.app.ctx != nil {
			wailsRuntime.EventsEmit(sm.app.ctx, "simulator:error", map[string]interface{}{
				"error": "Failed to set network mode",
				"step":  "set_network_mode",
			})
		}
		return map[string]interface{}{
			"success":        false,
			"error":          "Failed to set network mode",
			"technicalError": fmt.Sprintf("%v", modeResult["error"]),
			"step":           "set_network_mode",
		}
	}

	// Enable mining server for the simulator with wallet #0 as the reward address
	// This ensures wallet #0 accumulates mining rewards for deployments
	wallet0Address := ""
	if sm.walletManager != nil {
		wallet0 := sm.walletManager.GetWallet(0)
		if wallet0 != nil && wallet0.Address != "" {
			wallet0Address = wallet0.Address
			sm.app.logToConsole(fmt.Sprintf("[MINE] Mining rewards will go to wallet #0: %s...", wallet0Address[:20]))
		}
	}
	// If we don't have wallet #0 address yet, use the default simulator address
	if wallet0Address == "" {
		wallet0Address = "deto1qyvyeyzrcm2fzf6kyq7egkes2ufgny5xn77y6typhfx9s7w3mvyd5qqynr5hx"
		sm.app.logToConsole("[MINE] Mining rewards will go to default simulator wallet #0")
	}
	sm.app.SetNodeMiningConfig(true, wallet0Address, 0)

	// Start the node
	startResult := sm.app.StartNodeWithNetwork(sm.baseDir, "simulator")
	if !startResult["success"].(bool) {
		// Rollback: restore previous connection if we had one
		if wasExternalConnected && previousDaemonClient != nil {
			sm.app.logToConsole("[RESTORE] Simulator failed to start, restoring previous connection...")
			sm.app.daemonClient = previousDaemonClient
			// Restore network mode
			if previousNetworkMode != "" {
				sm.app.SetNetworkMode(previousNetworkMode)
			}
			sm.app.logToConsole(fmt.Sprintf("[OK] Restored connection to external %s node", previousNetworkMode))
		}
		if sm.app.ctx != nil {
			wailsRuntime.EventsEmit(sm.app.ctx, "simulator:error", map[string]interface{}{
				"error": "Failed to start simulator daemon",
				"step":  "start_daemon",
			})
		}
		return map[string]interface{}{
			"success":        false,
			"error":          "Failed to start simulator daemon",
			"technicalError": fmt.Sprintf("%v", startResult["error"]),
			"step":           "start_daemon",
			"rolledBack":     wasExternalConnected,
		}
	}
	sm.app.logToConsole("[OK] Simulator daemon started")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    2,
			"message": "Simulator daemon started",
			"status":  "complete",
		})
	}

	// Step 3: Wait for daemon to be ready
	sm.app.logToConsole("[WAIT] Step 3: Waiting for daemon to be ready...")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    3,
			"message": "Waiting for daemon to be ready...",
			"status":  "waiting",
		})
	}
	if err := sm.waitForDaemon(30 * time.Second); err != nil {
		// Rollback: restore previous connection if we had one
		if wasExternalConnected && previousDaemonClient != nil {
			sm.app.logToConsole("[RESTORE] Simulator daemon failed to become ready, restoring previous connection...")
			sm.app.daemonClient = previousDaemonClient
			// Restore network mode
			if previousNetworkMode != "" {
				sm.app.SetNetworkMode(previousNetworkMode)
			}
			sm.app.logToConsole(fmt.Sprintf("[OK] Restored connection to external %s node", previousNetworkMode))
		}
		if sm.app.ctx != nil {
			wailsRuntime.EventsEmit(sm.app.ctx, "simulator:error", map[string]interface{}{
				"error": FriendlyError(err),
				"step":  "wait_daemon",
			})
		}
		return map[string]interface{}{
			"success":        false,
			"error":          FriendlyError(err),
			"technicalError": err.Error(),
			"step":           "wait_daemon",
			"rolledBack":     wasExternalConnected,
		}
	}
	sm.app.logToConsole("[OK] Daemon is ready")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    3,
			"message": "Daemon is ready",
			"status":  "complete",
		})
	}

	// Step 4: Set up pre-seeded test wallets (same as original DERO simulator)
	sm.app.logToConsole("[WALLET] Step 4: Setting up pre-seeded test wallets...")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    4,
			"message": "Setting up test wallets...",
			"status":  "setting_up",
		})
	}
	
	if err := sm.walletManager.SetupWallets(sm.baseDir); err != nil {
		if sm.app.ctx != nil {
			wailsRuntime.EventsEmit(sm.app.ctx, "simulator:error", map[string]interface{}{
				"error": FriendlyError(err),
				"step":  "setup_wallets",
			})
		}
		return map[string]interface{}{
			"success":        false,
			"error":          FriendlyError(err),
			"technicalError": err.Error(),
			"step":           "setup_wallets",
		}
	}
	
	// Register all test wallets on blockchain
	sm.app.logToConsole("[WALLET] Registering test wallets on blockchain...")
	endpoint := fmt.Sprintf("127.0.0.1:%d", GetNetworkConfig(NetworkSimulator).RPCPort)
	if err := sm.walletManager.RegisterAllWallets(endpoint); err != nil {
		sm.app.logToConsole(fmt.Sprintf("[WARN] Failed to register test wallets: %v", err))
	} else {
		sm.app.logToConsole(fmt.Sprintf("[OK] %d pre-seeded test wallets ready", sm.walletManager.Count()))
	}
	
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    4,
			"message": fmt.Sprintf("%d test wallets ready", sm.walletManager.Count()),
			"status":  "complete",
		})
	}

	// Step 5: Configure Gnomon for simulator network (optional)
	sm.app.logToConsole("[...] Step 5: Configuring Gnomon for simulator...")
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:progress", map[string]interface{}{
			"step":    5,
			"message": "Configuring Gnomon for simulator...",
			"status":  "configuring",
		})
	}
	// Gnomon will automatically use the correct endpoint when we update daemon client
	// The gnomon data directory will be separate for simulator

	// Mark as initialized
	sm.Lock()
	sm.isInitialized = true
	sm.Unlock()

	sm.app.logToConsole("[OK] Simulator Mode activated successfully!")
	sm.app.logToConsole("[INFO] The simulator has built-in auto-mining - transactions are confirmed automatically")
	
	// Get final status
	netConfig := GetNetworkConfig(NetworkSimulator)
	
	// Emit completion event
	if sm.app.ctx != nil {
		wailsRuntime.EventsEmit(sm.app.ctx, "simulator:complete", map[string]interface{}{
			"success":        true,
			"message":        "Simulator mode activated successfully!",
			"walletAddress":  sm.walletManager.GetPrimaryAddress(),
			"walletCount":    sm.walletManager.Count(),
			"rpcEndpoint":    fmt.Sprintf("http://127.0.0.1:%d", netConfig.RPCPort),
		})
	}
	
	return map[string]interface{}{
		"success":       true,
		"message":       "Simulator mode activated",
		"walletAddress": sm.walletManager.GetPrimaryAddress(),
		"walletCount":   sm.walletManager.Count(),
		"rpcEndpoint":   fmt.Sprintf("http://127.0.0.1:%d", netConfig.RPCPort),
		"getworkPort":   netConfig.GetWorkPort,
	}
}

// StopSimulatorMode stops all simulator services
func (sm *SimulatorManager) StopSimulatorMode() map[string]interface{} {
	sm.Lock()
	defer sm.Unlock()

	if !sm.isInitialized {
		return map[string]interface{}{
			"success": true,
			"message": "Simulator was not running",
		}
	}

	sm.app.logToConsole("[STOP] Stopping Simulator Mode...")

	// Step 1: Close all wallets
	if sm.walletManager != nil {
		sm.walletManager.CloseAll()
	}

	// Step 2: Stop daemon
	stopResult := sm.app.StopNode()
	if !stopResult["success"].(bool) {
		sm.app.logToConsole(fmt.Sprintf("[WARN] Warning stopping daemon: %v", stopResult["error"]))
	}

	sm.isInitialized = false

	sm.app.logToConsole("[OK] Simulator Mode stopped")

	return map[string]interface{}{
		"success": true,
		"message": "Simulator mode stopped",
	}
}

// GetStatus returns the current simulator status
func (sm *SimulatorManager) GetStatus() SimulatorStatus {
	sm.RLock()
	defer sm.RUnlock()

	status := SimulatorStatus{
		IsInitialized: sm.isInitialized,
		IsStarting:    sm.isStarting,
		Extra:         make(map[string]interface{}),
	}

	// Check daemon status
	nodeStatus := sm.app.GetNodeStatus()
	if isRunning, ok := nodeStatus["isRunning"].(bool); ok {
		status.DaemonRunning = isRunning
	}
	if height, ok := nodeStatus["topoHeight"].(int64); ok {
		status.BlockHeight = height
	}

	// Check wallet status (using primary wallet = wallet #0)
	if sm.walletManager != nil {
		status.WalletOpen = sm.walletManager.IsSetup()
		status.WalletAddress = sm.walletManager.GetPrimaryAddress()
		if balance, _, err := sm.walletManager.GetPrimaryBalance(); err == nil {
			status.Balance = balance
		}
		status.Extra["testWalletsCount"] = sm.walletManager.Count()
		status.Extra["testWalletsReady"] = sm.walletManager.IsSetup()
	}

	// RPC endpoint
	netConfig := GetNetworkConfig(NetworkSimulator)
	status.RpcEndpoint = fmt.Sprintf("http://127.0.0.1:%d", netConfig.RPCPort)

	return status
}

// IsReady returns true if the simulator is fully initialized and ready to use
func (sm *SimulatorManager) IsReady() bool {
	sm.RLock()
	defer sm.RUnlock()

	if !sm.isInitialized {
		return false
	}

	// Check daemon
	nodeStatus := sm.app.GetNodeStatus()
	if isRunning, ok := nodeStatus["isRunning"].(bool); !ok || !isRunning {
		return false
	}

	// Check wallets are set up
	if sm.walletManager == nil || !sm.walletManager.IsSetup() {
		return false
	}

	return true
}

// waitForDaemon waits for the daemon to be ready
func (sm *SimulatorManager) waitForDaemon(timeout time.Duration) error {
	start := time.Now()
	pollInterval := 500 * time.Millisecond

	for time.Since(start) < timeout {
		info, err := sm.app.daemonClient.GetInfo()
		if err == nil && info != nil {
			// Daemon is responding
			return nil
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("daemon did not become ready within %v", timeout)
}


// ResetSimulator clears all simulator data and starts fresh
func (sm *SimulatorManager) ResetSimulator() map[string]interface{} {
	sm.app.logToConsole("[RESET] ========== RESETTING SIMULATOR ==========")

	// Stop simulator if running
	if sm.isInitialized {
		sm.app.logToConsole("[RESET] Stopping simulator...")
		sm.StopSimulatorMode()
	} else {
		sm.app.logToConsole("[RESET] Simulator was not running")
	}

	// Wait for services to stop
	sm.app.logToConsole("[RESET] Waiting for services to stop...")
	time.Sleep(2 * time.Second)

	// Delete simulator data directory (wallets)
	simulatorDir := filepath.Join(sm.baseDir, SimulatorWalletDir)
	sm.app.logToConsole(fmt.Sprintf("[RESET] Deleting wallet data: %s", simulatorDir))
	if err := os.RemoveAll(simulatorDir); err != nil {
		sm.app.logToConsole(fmt.Sprintf("[WARN] Failed to delete simulator wallet data: %v", err))
	} else {
		sm.app.logToConsole("[OK] Simulator wallet data cleared")
	}

	// Delete the blockchain data directory
	blockchainDir := filepath.Join(sm.baseDir, "simulator")
	sm.app.logToConsole(fmt.Sprintf("[RESET] Deleting blockchain data: %s", blockchainDir))
	if err := os.RemoveAll(blockchainDir); err != nil {
		sm.app.logToConsole(fmt.Sprintf("[WARN] Failed to delete blockchain data: %v", err))
	} else {
		sm.app.logToConsole("[OK] Blockchain data cleared")
	}

	// Delete simulator gnomon data
	gnomonSimDir := filepath.Join(sm.baseDir, "gnomon_simulator")
	sm.app.logToConsole(fmt.Sprintf("[RESET] Deleting gnomon data: %s", gnomonSimDir))
	if err := os.RemoveAll(gnomonSimDir); err != nil {
		sm.app.logToConsole(fmt.Sprintf("[WARN] Failed to delete gnomon data: %v", err))
	} else {
		sm.app.logToConsole("[OK] Gnomon data cleared")
	}

	sm.app.logToConsole("[RESET] ========== STARTING FRESH SIMULATOR ==========")

	// Restart simulator
	return sm.StartSimulatorMode()
}

// ================== App API Functions ==================

// StartSimulatorMode starts the one-click simulator mode
func (a *App) StartSimulatorMode() map[string]interface{} {
	// Initialize simulator manager if needed
	if a.simulatorManager == nil {
		a.simulatorManager = NewSimulatorManager(a)
	}

	return a.simulatorManager.StartSimulatorMode()
}

// StopSimulatorMode stops the simulator
func (a *App) StopSimulatorMode() map[string]interface{} {
	if a.simulatorManager == nil {
		return map[string]interface{}{
			"success": true,
			"message": "Simulator was not running",
		}
	}

	return a.simulatorManager.StopSimulatorMode()
}

// GetSimulatorStatus returns the current simulator status
func (a *App) GetSimulatorStatus() map[string]interface{} {
	if a.simulatorManager == nil {
		return map[string]interface{}{
			"success":       true,
			"isInitialized": false,
			"isStarting":    false,
			"daemonRunning": false,
			"walletOpen":    false,
		}
	}

	status := a.simulatorManager.GetStatus()
	return map[string]interface{}{
		"success":       true,
		"isInitialized": status.IsInitialized,
		"isStarting":    status.IsStarting,
		"daemonRunning": status.DaemonRunning,
		"walletOpen":    status.WalletOpen,
		"walletAddress": status.WalletAddress,
		"balance":       status.Balance,
		"balanceDERO":   float64(status.Balance) / 1e12,
		"blockHeight":   status.BlockHeight,
		"rpcEndpoint":   status.RpcEndpoint,
		"miningStats":   status.Extra["miningStats"],
	}
}

// IsSimulatorReady checks if the simulator is fully ready to use
func (a *App) IsSimulatorReady() map[string]interface{} {
	if a.simulatorManager == nil {
		return map[string]interface{}{
			"success": true,
			"ready":   false,
			"reason":  "Simulator not initialized",
		}
	}

	ready := a.simulatorManager.IsReady()
	reason := ""
	if !ready {
		status := a.simulatorManager.GetStatus()
		if !status.DaemonRunning {
			reason = "Daemon not running"
		} else if !status.WalletOpen {
			reason = "Wallet not open"
		} else {
			reason = "Unknown"
		}
	}

	return map[string]interface{}{
		"success": true,
		"ready":   ready,
		"reason":  reason,
	}
}

// ResetSimulator clears all simulator data and starts fresh
func (a *App) ResetSimulator() map[string]interface{} {
	if a.simulatorManager == nil {
		a.simulatorManager = NewSimulatorManager(a)
	}

	return a.simulatorManager.ResetSimulator()
}
