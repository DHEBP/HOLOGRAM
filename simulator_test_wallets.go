package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/walletapi"
)

// Pre-defined seeds from the original DERO simulator
// These are the same seeds everyone gets when running the simulator
// This ensures consistency across all simulator instances
var SimulatorWalletSeeds = []string{
	"171eeaa899e360bf1a8ada7627aaea9fdad7992463581d935a8838f16b1ff51a",
	"193faf64d79e9feca5fce8b992b4bb59b86c50f491e2dc475522764ca6666b6b",
	"2e49383ac5c938c268921666bccfcb5f0c4d43cd3ed125c6c9e72fc5620bc79b",
	"1c8ee58431e21d1ef022ccf1f53fec36f5e5851d662a3dd96ced3fc155445120",
	"19182604625563f3ff913bb8fb53b0ade2e0271ca71926edb98c8e39f057d557",
	"2a3beb8a57baa096512e85902bb5f1833f1f37e79f75227bbf57c4687bfbb002",
	"055e43ebff20efff612ba6f8128caf990f2bf89aeea91584e63179b9d43cd3ab",
	"2ccb7fc12e867796dd96e246aceff3fea1fdf78a28253c583017350034c31c81",
	"279533d87cc4c637bf853e630480da4ee9d4390a282270d340eac52a391fd83d",
	"03bae8b71519fe8ac3137a3c77d2b6a164672c8691f67bd97548cb6c6f868c67",
	"2b9022d0c5ee922439b0d67864faeced65ebce5f35d26e0ee0746554d395eb88",
	"1a63d5cf9955e8f3d6cecde4c9ecbd538089e608741019397824dc6a2e0bfcc1",
	"10900d25e7dc0cec35fcca9161831a02cb7ed513800368529ba8944eeca6e949",
	"2af6630905d73ee40864bd48339f297908a0731a6c4c6fa0a27ea574ac4e4733",
	"2ac9a8984c988fcb54b261d15bc90b5961d673bffa5ff41c8250c7e262cbd606",
	"040572cec23e6df4f686192b776c197a50591836a3dd02ba2e4a7b7474382ccd",
	"2b2b029cfbc5d08b5d661e6fa444102d387780bec088f4dd41a4a537bf9762af",
	"1812298da90ded6457b2a20fd52d09f639584fb470c715617db13959927be7f8",
	"1eee334e1f533aa1ac018124cf3d5efa20e52f54b05e475f6f2cff3476b4a92f",
	"2c34e7978ce249aebed33e14cdd5177921ecd78fbe58d33bbec21f22b80af7a5",
	"083e7fe96e8415ea119ec6c4d0ebe233e86b53bd4e2f7598505317efc23ae34b",
	"0fd7f8db0ed6cbe3bf300258619d8d4a2ff8132ef3c896f6e3fa65a6c92bdf9a",
}

// TestWallet represents a pre-seeded test wallet
type TestWallet struct {
	Index      int                    `json:"index"`
	Seed       string                 `json:"seed"`
	Address    string                 `json:"address"`
	Balance    uint64                 `json:"balance"`
	Locked     uint64                 `json:"locked"`
	Registered bool                   `json:"registered"`
	RPCPort    int                    `json:"rpcPort"`
	wallet     *walletapi.Wallet_Disk // internal, not exposed to JSON
}

// TestWalletManager manages all pre-seeded test wallets
type TestWalletManager struct {
	sync.RWMutex
	wallets     []*TestWallet
	walletsDir  string
	isSetup     bool
	logFunc     func(string)
}

const (
	TestWalletPassword    = "" // Empty password like original DERO simulator
	TestWalletRPCPortBase = 30000
)

// NewTestWalletManager creates a new test wallet manager
func NewTestWalletManager(logFunc func(string)) *TestWalletManager {
	return &TestWalletManager{
		wallets: make([]*TestWallet, 0, len(SimulatorWalletSeeds)),
		logFunc: logFunc,
	}
}

// log helper
func (twm *TestWalletManager) log(msg string) {
	if twm.logFunc != nil {
		twm.logFunc(msg)
	}
}

// GetWalletsDir returns the directory for test wallets
func (twm *TestWalletManager) GetWalletsDir(baseDir string) string {
	return filepath.Join(baseDir, SimulatorWalletDir, "test_wallets")
}

// SetupTestWallets creates all 22 pre-seeded test wallets
// This matches the original DERO simulator behavior
func (twm *TestWalletManager) SetupTestWallets(baseDir string) error {
	twm.Lock()
	defer twm.Unlock()

	if twm.isSetup {
		twm.log("[INFO] Test wallets already set up")
		return nil
	}

	twm.walletsDir = twm.GetWalletsDir(baseDir)

	// Create wallets directory
	if err := os.MkdirAll(twm.walletsDir, 0755); err != nil {
		return fmt.Errorf("failed to create test wallets directory: %v", err)
	}

	twm.log(fmt.Sprintf("[WALLET] Setting up %d pre-seeded test wallets...", len(SimulatorWalletSeeds)))

	// Create each wallet from its seed
	for i, seed := range SimulatorWalletSeeds {
		wallet, err := twm.createWalletFromSeed(i, seed)
		if err != nil {
			twm.log(fmt.Sprintf("[ERR] Failed to create test wallet %d: %v", i, err))
			continue
		}
		twm.wallets = append(twm.wallets, wallet)
		twm.log(fmt.Sprintf("[OK] Test wallet %d: %s", i, wallet.Address[:20]+"..."))
	}

	twm.isSetup = true
	twm.log(fmt.Sprintf("[OK] Created %d test wallets", len(twm.wallets)))
	return nil
}

// createWalletFromSeed creates a single wallet from a hex seed
func (twm *TestWalletManager) createWalletFromSeed(index int, seedHex string) (*TestWallet, error) {
	// Decode seed from hex
	seedRaw, err := hex.DecodeString(seedHex)
	if err != nil || len(seedHex) >= 65 {
		return nil, fmt.Errorf("invalid seed: %v", err)
	}

	// Wallet filename
	filename := filepath.Join(twm.walletsDir, fmt.Sprintf("wallet_%d.db", index))

	// Delete existing wallet file to ensure fresh state
	os.Remove(filename)

	// Create wallet from seed
	wallet, err := walletapi.Create_Encrypted_Wallet(filename, TestWalletPassword, new(crypto.BNRed).SetBytes(seedRaw))
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %v", err)
	}

	// Set network mode (not mainnet for simulator)
	wallet.SetNetwork(false)

	// Save wallet
	wallet.Save_Wallet()

	address := wallet.GetAddress().String()

	testWallet := &TestWallet{
		Index:      index,
		Seed:       seedHex,
		Address:    address,
		Balance:    0,
		Locked:     0,
		Registered: false,
		RPCPort:    TestWalletRPCPortBase + index,
		wallet:     wallet,
	}

	return testWallet, nil
}

// RegisterAllWallets registers all test wallets on the blockchain
func (twm *TestWalletManager) RegisterAllWallets(daemonEndpoint string) error {
	twm.Lock()
	defer twm.Unlock()

	if len(twm.wallets) == 0 {
		return fmt.Errorf("no test wallets to register")
	}

	twm.log(fmt.Sprintf("[WALLET] Registering %d test wallets on blockchain...", len(twm.wallets)))

	// Connect walletapi for registration
	if err := walletapi.Connect(daemonEndpoint); err != nil {
		twm.log(fmt.Sprintf("[WARN] walletapi.Connect returned error: %v (continuing anyway)", err))
	}

	registeredCount := 0
	for i, tw := range twm.wallets {
		if tw.wallet == nil {
			continue
		}

		// Set daemon address and online mode
		tw.wallet.SetDaemonAddress(daemonEndpoint)
		tw.wallet.SetOnlineMode()

		// Get registration TX
		regTX := tw.wallet.GetRegistrationTX()
		if regTX == nil {
			twm.log(fmt.Sprintf("[INFO] Wallet %d: GetRegistrationTX returned nil (may already be registered)", i))
			tw.Registered = true
			registeredCount++
			continue
		}

		// Send registration TX
		if err := tw.wallet.SendTransaction(regTX); err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "already") || strings.Contains(errStr, "registered") {
				twm.log(fmt.Sprintf("[OK] Wallet %d: Already registered", i))
				tw.Registered = true
				registeredCount++
			} else {
				twm.log(fmt.Sprintf("[WARN] Wallet %d: Registration error: %v", i, err))
			}
		} else {
			twm.log(fmt.Sprintf("[OK] Wallet %d: Registration TX sent", i))
			tw.Registered = true
			registeredCount++
		}

		// Small delay between registrations
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for registrations to be confirmed
	twm.log("[WAIT] Waiting for registrations to be confirmed...")
	time.Sleep(2 * time.Second)

	// Sync balances
	twm.syncBalancesUnlocked()

	// Disconnect walletapi
	walletapi.Connected = false

	twm.log(fmt.Sprintf("[OK] Registered %d/%d test wallets", registeredCount, len(twm.wallets)))
	return nil
}

// syncBalancesUnlocked syncs balances for all wallets (must hold lock)
func (twm *TestWalletManager) syncBalancesUnlocked() {
	for i, tw := range twm.wallets {
		if tw.wallet == nil {
			continue
		}

		if err := tw.wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
			twm.log(fmt.Sprintf("[WARN] Wallet %d: Sync failed: %v", i, err))
			continue
		}

		mature, locked := tw.wallet.Get_Balance()
		tw.Balance = mature
		tw.Locked = locked
		tw.Registered = tw.wallet.IsRegistered()
	}
}

// SyncBalances syncs balances for all wallets
func (twm *TestWalletManager) SyncBalances(daemonEndpoint string) error {
	twm.Lock()
	defer twm.Unlock()

	// Connect walletapi for sync
	if err := walletapi.Connect(daemonEndpoint); err != nil {
		twm.log(fmt.Sprintf("[WARN] walletapi.Connect returned error: %v", err))
	}

	twm.syncBalancesUnlocked()

	// Disconnect
	walletapi.Connected = false

	return nil
}

// GetWallets returns all test wallets (safe copy)
func (twm *TestWalletManager) GetWallets() []TestWallet {
	twm.RLock()
	defer twm.RUnlock()

	result := make([]TestWallet, len(twm.wallets))
	for i, tw := range twm.wallets {
		result[i] = TestWallet{
			Index:      tw.Index,
			Seed:       tw.Seed,
			Address:    tw.Address,
			Balance:    tw.Balance,
			Locked:     tw.Locked,
			Registered: tw.Registered,
			RPCPort:    tw.RPCPort,
		}
	}
	return result
}

// GetWallet returns a specific test wallet by index
func (twm *TestWalletManager) GetWallet(index int) *TestWallet {
	twm.RLock()
	defer twm.RUnlock()

	if index < 0 || index >= len(twm.wallets) {
		return nil
	}
	tw := twm.wallets[index]
	return &TestWallet{
		Index:      tw.Index,
		Seed:       tw.Seed,
		Address:    tw.Address,
		Balance:    tw.Balance,
		Locked:     tw.Locked,
		Registered: tw.Registered,
		RPCPort:    tw.RPCPort,
	}
}

// GetWalletByAddress returns a test wallet by address
func (twm *TestWalletManager) GetWalletByAddress(address string) *TestWallet {
	twm.RLock()
	defer twm.RUnlock()

	for _, tw := range twm.wallets {
		if tw.Address == address {
			return &TestWallet{
				Index:      tw.Index,
				Seed:       tw.Seed,
				Address:    tw.Address,
				Balance:    tw.Balance,
				Locked:     tw.Locked,
				Registered: tw.Registered,
				RPCPort:    tw.RPCPort,
			}
		}
	}
	return nil
}

// GetInternalWallet returns the internal wallet object for a test wallet
// Use with caution - this is for advanced operations like transactions
func (twm *TestWalletManager) GetInternalWallet(index int) *walletapi.Wallet_Disk {
	twm.RLock()
	defer twm.RUnlock()

	if index < 0 || index >= len(twm.wallets) {
		return nil
	}
	return twm.wallets[index].wallet
}

// CloseAll closes all test wallets
func (twm *TestWalletManager) CloseAll() {
	twm.Lock()
	defer twm.Unlock()

	for _, tw := range twm.wallets {
		if tw.wallet != nil {
			tw.wallet.Close_Encrypted_Wallet()
			tw.wallet = nil
		}
	}

	twm.wallets = make([]*TestWallet, 0)
	twm.isSetup = false
	twm.log("[OK] All test wallets closed")
}

// IsSetup returns whether test wallets are set up
func (twm *TestWalletManager) IsSetup() bool {
	twm.RLock()
	defer twm.RUnlock()
	return twm.isSetup
}

// Count returns the number of test wallets
func (twm *TestWalletManager) Count() int {
	twm.RLock()
	defer twm.RUnlock()
	return len(twm.wallets)
}

// ================== App API Functions ==================

// GetSimulatorTestWallets returns all pre-seeded test wallets
func (a *App) GetSimulatorTestWallets() map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.testWalletManager == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator not initialized",
			"wallets": []TestWallet{},
		}
	}

	wallets := a.simulatorManager.testWalletManager.GetWallets()
	return map[string]interface{}{
		"success": true,
		"count":   len(wallets),
		"wallets": wallets,
	}
}

// GetSimulatorTestWallet returns a specific test wallet by index
func (a *App) GetSimulatorTestWallet(index int) map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.testWalletManager == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator not initialized",
		}
	}

	wallet := a.simulatorManager.testWalletManager.GetWallet(index)
	if wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Wallet %d not found", index),
		}
	}

	return map[string]interface{}{
		"success": true,
		"wallet":  wallet,
	}
}

// SyncSimulatorTestWallets syncs balances for all test wallets
func (a *App) SyncSimulatorTestWallets() map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.testWalletManager == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator not initialized",
		}
	}

	endpoint := fmt.Sprintf("127.0.0.1:%d", GetNetworkConfig(NetworkSimulator).RPCPort)
	if err := a.simulatorManager.testWalletManager.SyncBalances(endpoint); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
	}

	wallets := a.simulatorManager.testWalletManager.GetWallets()
	return map[string]interface{}{
		"success": true,
		"count":   len(wallets),
		"wallets": wallets,
	}
}

// OpenSimulatorTestWallet opens a test wallet by index and sets it as the active wallet
func (a *App) OpenSimulatorTestWallet(index int) map[string]interface{} {
	if a.simulatorManager == nil || a.simulatorManager.testWalletManager == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Simulator not initialized",
		}
	}

	// Get the internal wallet object
	internalWallet := a.simulatorManager.testWalletManager.GetInternalWallet(index)
	if internalWallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Test wallet %d not found", index),
		}
	}

	// Get wallet info
	walletInfo := a.simulatorManager.testWalletManager.GetWallet(index)
	if walletInfo == nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Test wallet %d info not found", index),
		}
	}

	// Use the global wallet manager (same as OpenWallet)
	walletManager.Lock()
	defer walletManager.Unlock()

	// Close existing wallet if open
	if walletManager.isOpen && walletManager.wallet != nil {
		walletManager.wallet.Close_Encrypted_Wallet()
		walletManager.isOpen = false
	}

	// Set the test wallet as the active wallet
	walletManager.wallet = internalWallet
	walletManager.walletPath = fmt.Sprintf("TestWallet_%d (Simulator)", index)
	walletManager.isOpen = true

	// Connect to daemon
	endpoint := fmt.Sprintf("127.0.0.1:%d", GetNetworkConfig(NetworkSimulator).RPCPort)
	internalWallet.SetDaemonAddress(endpoint)
	internalWallet.SetOnlineMode()

	// Sync wallet
	if err := internalWallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
		a.logToConsole(fmt.Sprintf("[WARN] Failed to sync test wallet: %v", err))
	}

	// Get updated balance
	mature, locked := internalWallet.Get_Balance()

	a.logToConsole(fmt.Sprintf("[OK] Opened test wallet #%d: %s", index, walletInfo.Address[:20]+"..."))

	return map[string]interface{}{
		"success": true,
		"address": walletInfo.Address,
		"path":    walletManager.walletPath,
		"balance": mature,
		"locked":  locked,
		"index":   index,
	}
}

