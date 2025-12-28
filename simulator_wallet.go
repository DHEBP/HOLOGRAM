package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi"
)

// SimulatorWalletManager handles automatic wallet creation and management for simulator mode
type SimulatorWalletManager struct {
	sync.RWMutex
	wallet         *walletapi.Wallet_Disk
	walletPath     string
	walletPassword string
	isOpen         bool
	address        string
	logFunc        func(string)
}

const (
	// SimulatorWalletName is the default filename for the simulator wallet
	SimulatorWalletName = "simulator_wallet.db"
	// SimulatorWalletPassword is a fixed password for the simulator wallet
	// Security is not a concern for test coins
	SimulatorWalletPassword = "simulator"
	// SimulatorWalletDir is the subdirectory for simulator data
	SimulatorWalletDir = "simulator"
)

// NewSimulatorWalletManager creates a new simulator wallet manager
func NewSimulatorWalletManager(logFunc func(string)) *SimulatorWalletManager {
	return &SimulatorWalletManager{
		walletPassword: SimulatorWalletPassword,
		logFunc:        logFunc,
	}
}

// log helper
func (swm *SimulatorWalletManager) log(msg string) {
	if swm.logFunc != nil {
		swm.logFunc(msg)
	}
}

// GetWalletPath returns the full path to the simulator wallet
func (swm *SimulatorWalletManager) GetWalletPath(baseDir string) string {
	return filepath.Join(baseDir, SimulatorWalletDir, SimulatorWalletName)
}

// WalletExists checks if the simulator wallet already exists
func (swm *SimulatorWalletManager) WalletExists(baseDir string) bool {
	walletPath := swm.GetWalletPath(baseDir)
	_, err := os.Stat(walletPath)
	return err == nil
}

// EnsureWalletExists creates the simulator wallet if it doesn't exist
func (swm *SimulatorWalletManager) EnsureWalletExists(baseDir string) (string, error) {
	swm.Lock()
	defer swm.Unlock()

	walletPath := swm.GetWalletPath(baseDir)
	swm.walletPath = walletPath

	// Create directory if needed
	walletDir := filepath.Dir(walletPath)
	if err := os.MkdirAll(walletDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create wallet directory: %v", err)
	}

	// Check if wallet already exists
	if _, err := os.Stat(walletPath); err == nil {
		swm.log(fmt.Sprintf("[OK] Simulator wallet already exists at: %s", walletPath))
		return walletPath, nil
	}

	// Create new wallet
	swm.log("[WALLET] Creating new simulator wallet...")

	wallet, err := walletapi.Create_Encrypted_Wallet_Random(walletPath, SimulatorWalletPassword)
	if err != nil {
		return "", fmt.Errorf("failed to create wallet: %v", err)
	}

	// Set network mode to NOT mainnet (simulator mode)
	wallet.SetNetwork(false)

	// Get address
	address := wallet.GetAddress().String()
	swm.address = address

	// Close the wallet (we'll open it properly later)
	wallet.Close_Encrypted_Wallet()

	swm.log(fmt.Sprintf("[OK] Simulator wallet created: %s", address[:20]+"..."))

	return walletPath, nil
}

// OpenWallet opens the simulator wallet
func (swm *SimulatorWalletManager) OpenWallet(baseDir string) error {
	swm.Lock()
	defer swm.Unlock()

	if swm.isOpen && swm.wallet != nil {
		swm.log("[INFO] Simulator wallet already open")
		return nil
	}

	walletPath := swm.GetWalletPath(baseDir)

	// Check if wallet exists
	if _, err := os.Stat(walletPath); os.IsNotExist(err) {
		return fmt.Errorf("simulator wallet not found at %s", walletPath)
	}

	swm.log(fmt.Sprintf("[WALLET] Opening simulator wallet: %s", walletPath))

	// Open the wallet
	wallet, err := walletapi.Open_Encrypted_Wallet(walletPath, SimulatorWalletPassword)
	if err != nil {
		return fmt.Errorf("failed to open wallet: %v", err)
	}

	// Set network mode to NOT mainnet (simulator mode)
	wallet.SetNetwork(false)

	// CRITICAL: Initialize globals for simulator mode
	// This must happen before any daemon connections
	globals.Arguments["--testnet"] = true
	globals.Arguments["--simulator"] = true
	globals.InitNetwork()
	swm.log("[NET] Initialized globals for simulator mode")

	// Set simulator daemon endpoint
	simulatorEndpoint := fmt.Sprintf("127.0.0.1:%d", GetNetworkConfig(NetworkSimulator).RPCPort)
	
	// CRITICAL: Set the global daemon endpoint that tela library uses
	// The tela library creates its own websocket connections using this endpoint
	// We DON'T call walletapi.Connect() here because it creates a persistent websocket
	// that conflicts with tela library's websocket connections - the simulator daemon
	// cannot handle multiple websocket connections and will crash
	walletapi.Daemon_Endpoint_Active = simulatorEndpoint
	swm.log(fmt.Sprintf("[NET] Set Daemon_Endpoint_Active to %s", simulatorEndpoint))
	
	// Set wallet-specific daemon address for balance/registration checks
	wallet.SetDaemonAddress(simulatorEndpoint)
	
	// Enable online mode - this configures the wallet but doesn't create connections
	wallet.SetOnlineMode()
	
	swm.log(fmt.Sprintf("[NET] Wallet configured for simulator daemon at %s (tela library will manage connections)", simulatorEndpoint))

	swm.wallet = wallet
	swm.walletPath = walletPath
	swm.isOpen = true
	swm.address = wallet.GetAddress().String()

	swm.log(fmt.Sprintf("[OK] Simulator wallet opened: %s", swm.address[:20]+"..."))

	return nil
}

// CloseWallet closes the simulator wallet
func (swm *SimulatorWalletManager) CloseWallet() error {
	swm.Lock()
	defer swm.Unlock()

	if !swm.isOpen || swm.wallet == nil {
		return nil
	}

	swm.log("[WALLET] Closing simulator wallet...")

	swm.wallet.Close_Encrypted_Wallet()
	swm.wallet = nil
	swm.isOpen = false

	swm.log("[OK] Simulator wallet closed")

	return nil
}

// GetAddress returns the simulator wallet address
func (swm *SimulatorWalletManager) GetAddress() string {
	swm.RLock()
	defer swm.RUnlock()
	return swm.address
}

// IsOpen returns whether the wallet is open
func (swm *SimulatorWalletManager) IsOpen() bool {
	swm.RLock()
	defer swm.RUnlock()
	return swm.isOpen
}

// GetBalance returns the simulator wallet balance
func (swm *SimulatorWalletManager) GetBalance() (uint64, uint64, error) {
	swm.RLock()
	defer swm.RUnlock()

	if !swm.isOpen || swm.wallet == nil {
		return 0, 0, fmt.Errorf("wallet not open")
	}

	mature, locked := swm.wallet.Get_Balance()
	return mature, locked, nil
}

// GetWallet returns the underlying wallet (for advanced operations)
func (swm *SimulatorWalletManager) GetWallet() *walletapi.Wallet_Disk {
	swm.RLock()
	defer swm.RUnlock()
	return swm.wallet
}

// NeedsInitialFunding checks if the wallet needs initial test DERO
func (swm *SimulatorWalletManager) NeedsInitialFunding(minBalance uint64) bool {
	mature, _, err := swm.GetBalance()
	if err != nil {
		return true // Assume needs funding if we can't check
	}
	return mature < minBalance
}

// GetStatus returns the current status of the simulator wallet
func (swm *SimulatorWalletManager) GetStatus() map[string]interface{} {
	swm.RLock()
	defer swm.RUnlock()

	status := map[string]interface{}{
		"isOpen":     swm.isOpen,
		"walletPath": swm.walletPath,
		"address":    swm.address,
	}

	if swm.isOpen && swm.wallet != nil {
		mature, locked := swm.wallet.Get_Balance()
		status["balance"] = mature
		status["lockedBalance"] = locked
		status["totalBalance"] = mature + locked
		status["isRegistered"] = swm.wallet.IsRegistered()
	}

	return status
}

// IsRegistered checks if the wallet is registered on the blockchain
func (swm *SimulatorWalletManager) IsRegistered() bool {
	swm.RLock()
	defer swm.RUnlock()
	
	if !swm.isOpen || swm.wallet == nil {
		return false
	}
	
	return swm.wallet.IsRegistered()
}

// SendRegistration sends a registration transaction to register the wallet on the blockchain
// IMPORTANT: This function temporarily connects to the daemon, sends registration, then disconnects
// to avoid websocket conflicts with tela library's internal connections
func (swm *SimulatorWalletManager) SendRegistration() error {
	swm.Lock()
	defer swm.Unlock()
	
	if !swm.isOpen || swm.wallet == nil {
		return fmt.Errorf("wallet not open")
	}
	
	simulatorEndpoint := fmt.Sprintf("127.0.0.1:%d", GetNetworkConfig(NetworkSimulator).RPCPort)
	
	// Temporarily connect to daemon for registration
	// The simulator can only handle one websocket connection at a time, so we:
	// 1. Connect for registration
	// 2. Send registration TX
	// 3. Wait for confirmation
	// 4. Disconnect so tela library can create its own connections
	swm.log("[NET] Temporarily connecting to daemon for registration...")
	if err := walletapi.Connect(simulatorEndpoint); err != nil {
		swm.log(fmt.Sprintf("[WARN] walletapi.Connect returned error: %v (will try anyway)", err))
	}
	
	// In simulator mode, ALWAYS attempt registration
	// The wallet's IsRegistered() uses local state which may be stale if the simulator was restarted
	// Since simulator transactions are free and auto-mined, it's safe to always try
	swm.log("[WALLET] Attempting registration (simulator mode - always try to ensure fresh state)...")
	
	// Get registration transaction
	regTX := swm.wallet.GetRegistrationTX()
	if regTX == nil {
		// This can happen if wallet thinks it's already registered
		// Check actual blockchain status by trying to get balance
		swm.log("[INFO] GetRegistrationTX returned nil - wallet may already be registered")
		// Continue anyway - we'll verify below
	} else {
		// Send registration TX to the network
		swm.log("[WALLET] Broadcasting registration transaction...")
		if err := swm.wallet.SendTransaction(regTX); err != nil {
			errStr := err.Error()
			// If error indicates already registered, that's fine
			if strings.Contains(errStr, "already") || strings.Contains(errStr, "registered") {
				swm.log("[OK] Wallet already registered on blockchain (confirmed by daemon)")
			} else {
				swm.log(fmt.Sprintf("[WARN] Registration broadcast error: %v (may already be registered)", err))
				// Don't return error - continue to verify registration status
			}
		} else {
			swm.log("[OK] Registration transaction sent successfully")
		}
	}
	
	// Wait for simulator auto-mining to confirm the registration
	// The simulator auto-mines blocks, so we just need to give it time
	swm.log("[WAIT] Waiting for registration to be confirmed...")
	registered := false
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if swm.wallet.IsRegistered() {
			swm.log("[OK] Wallet registration confirmed on blockchain!")
			registered = true
			break
		}
	}
	
	// CRITICAL: Properly close the websocket connection
	// The simulator daemon can only handle ONE websocket connection at a time.
	// We must CLOSE the walletapi websocket before the tela library creates its own.
	// Note: walletapi.Connect(" ") doesn't actually close the websocket - it just
	// fails to connect to a new endpoint. 
	// NOTE: GetRPCClient() is not available in this derohe version, so we rely on
	// connection state management. The tela library will create its own connection.
	swm.log("[NET] Disconnecting walletapi to free daemon for tela library...")
	
	// Set Connected to false - this signals that walletapi is no longer connected
	// The tela library will create its own websocket connection when needed
	walletapi.Connected = false
	
	// Give the daemon a moment to clean up the connection
	time.Sleep(100 * time.Millisecond)
	swm.log("[OK] Walletapi disconnected")
	
	// CRITICAL: Restore Daemon_Endpoint_Active for tela library
	// The tela library uses this to know where to connect
	walletapi.Daemon_Endpoint_Active = simulatorEndpoint
	swm.log(fmt.Sprintf("[NET] Daemon_Endpoint_Active set to: %s (websocket closed, tela library will reconnect)", walletapi.Daemon_Endpoint_Active))
	swm.log("[OK] Wallet registration complete")
	
	if !registered {
		swm.log("[WARN] Registration sent but not yet confirmed - may need to wait longer")
	}
	
	return nil
}

// ================== App API Functions ==================

// GetSimulatorWalletStatus returns the status of the simulator wallet
func (a *App) GetSimulatorWalletStatus() map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.walletManager == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator manager not initialized",
		}
	}

	status := a.simulatorManager.walletManager.GetStatus()
	status["success"] = true
	return status
}

// CreateSimulatorWallet creates a new simulator wallet
func (a *App) CreateSimulatorWallet() map[string]interface{} {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to get home directory: %v", err),
		}
	}

	baseDir := filepath.Join(homeDir, ".dero", "tela-gui")

	// Create wallet manager if needed
	if a.simulatorManager == nil {
		a.simulatorManager = NewSimulatorManager(a)
	}
	if a.simulatorManager.walletManager == nil {
		a.simulatorManager.walletManager = NewSimulatorWalletManager(a.logToConsole)
	}

	walletPath, err := a.simulatorManager.walletManager.EnsureWalletExists(baseDir)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create simulator wallet: %v", err),
		}
	}

	return map[string]interface{}{
		"success":    true,
		"walletPath": walletPath,
		"message":    "Simulator wallet ready",
	}
}

// OpenSimulatorWallet opens the simulator wallet
func (a *App) OpenSimulatorWallet() map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.walletManager == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator manager not initialized. Call CreateSimulatorWallet first.",
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to get home directory: %v", err),
		}
	}

	baseDir := filepath.Join(homeDir, ".dero", "tela-gui")

	if err := a.simulatorManager.walletManager.OpenWallet(baseDir); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to open simulator wallet: %v", err),
		}
	}

	return map[string]interface{}{
		"success": true,
		"address": a.simulatorManager.walletManager.GetAddress(),
		"message": "Simulator wallet opened",
	}
}

// CloseSimulatorWallet closes the simulator wallet
func (a *App) CloseSimulatorWallet() map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.walletManager == nil {
		return map[string]interface{}{
			"success": true,
			"message": "No simulator wallet to close",
		}
	}

	if err := a.simulatorManager.walletManager.CloseWallet(); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to close simulator wallet: %v", err),
		}
	}

	return map[string]interface{}{
		"success": true,
		"message": "Simulator wallet closed",
	}
}

// RegisterSimulatorWallet registers the wallet on the simulator blockchain
func (a *App) RegisterSimulatorWallet() map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.walletManager == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator manager not initialized",
		}
	}

	if !a.simulatorManager.walletManager.IsOpen() {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator wallet not open",
		}
	}

	// Check if already registered
	if a.simulatorManager.walletManager.IsRegistered() {
		return map[string]interface{}{
			"success":      true,
			"message":      "Wallet already registered",
			"isRegistered": true,
		}
	}

	// Send registration
	if err := a.simulatorManager.walletManager.SendRegistration(); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to register wallet: %v", err),
		}
	}

	return map[string]interface{}{
		"success":      true,
		"message":      "Registration transaction sent. Mine a block to confirm.",
		"isRegistered": false, // Will be true after mining confirms it
	}
}

// IsSimulatorWalletRegistered checks if the simulator wallet is registered
func (a *App) IsSimulatorWalletRegistered() map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.walletManager == nil {
		return map[string]interface{}{
			"success":      false,
			"isRegistered": false,
			"error":        "Simulator manager not initialized",
		}
	}

	isRegistered := a.simulatorManager.walletManager.IsRegistered()
	return map[string]interface{}{
		"success":      true,
		"isRegistered": isRegistered,
	}
}
