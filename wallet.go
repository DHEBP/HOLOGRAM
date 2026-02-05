package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WalletManager handles wallet operations
type WalletManager struct {
	sync.RWMutex
	wallet        *walletapi.Wallet_Disk
	walletPath    string
	isOpen        bool
	recentWallets []string
}

// NewWalletManager creates a new wallet manager
func NewWalletManager() *WalletManager {
	return &WalletManager{
		recentWallets: make([]string, 0),
	}
}

// Global wallet manager instance
var walletManager = NewWalletManager()

// walletapi uses global connectivity; start it once per process.
var walletConnectivityOnce sync.Once

func normalizeDaemonEndpointForWallet(endpoint string) string {
	// walletapi.Wallet_* typically expects host:port (no scheme) here.
	// walletapi.Connect() can handle schemes, but SetDaemonAddress is used elsewhere.
	e := strings.TrimSpace(endpoint)
	switch {
	case strings.HasPrefix(e, "http://"):
		return strings.TrimPrefix(e, "http://")
	case strings.HasPrefix(e, "https://"):
		return strings.TrimPrefix(e, "https://")
	case strings.HasPrefix(e, "ws://"):
		return strings.TrimPrefix(e, "ws://")
	case strings.HasPrefix(e, "wss://"):
		return strings.TrimPrefix(e, "wss://")
	default:
		return e
	}
}

func (a *App) ensureWalletDaemonConnectivity(endpoint string) {
	// Ensure walletapi has an active daemon endpoint and is connected.
	// Keep_Connectivity() will continue to retry and keep the connection alive.
	if endpoint == "" {
		endpoint = "127.0.0.1:10102"
	}

	if err := walletapi.Connect(endpoint); err != nil {
		a.logToConsole(fmt.Sprintf("[WARN] Wallet daemon connect failed: %v - will retry in background", err))
		// Emit event to notify frontend of connection issue
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "wallet:daemon_connection_warning", map[string]interface{}{
				"error":    err.Error(),
				"endpoint": endpoint,
				"message":  "Wallet daemon connection failed. Retrying in background...",
			})
		}
	}

	walletConnectivityOnce.Do(func() {
		go walletapi.Keep_Connectivity()
		a.logToConsole("[NET] Wallet daemon connectivity loop started")
	})
}

// OpenWallet opens a DERO wallet file
func (a *App) OpenWallet(filePath, password string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	// If just a name is provided (no path separators), construct full path
	// This matches the behavior of CreateWallet for consistency
	if !strings.Contains(filePath, string(filepath.Separator)) && !strings.Contains(filePath, "/") {
		// Clean the name - remove any .db extension if user added it
		name := strings.TrimSuffix(filePath, ".db")
		// Construct path in wallets directory
		filePath = filepath.Join(getDatashardsDir(), "wallets", name+".db")
	}

	// Determine current network mode - check multiple ways for robustness
	currentNetwork := "mainnet"
	simArg := globals.Arguments["--simulator"]

	// Check if simulator (can be bool or interface{})
	if simArg == true || simArg == "true" || fmt.Sprintf("%v", simArg) == "true" {
		currentNetwork = "simulator"
	}

	a.logToConsole(fmt.Sprintf("[WALLET] Opening wallet: %s (network: %s)", filePath, currentNetwork))

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		a.logToConsole(fmt.Sprintf("[ERR] Wallet file not found: %s", filePath))
		return map[string]interface{}{
			"success": false,
			"error":   "Wallet file not found",
		}
	}

	// Check if this wallet was last used on a different network
	var networkWarning string
	existingData := loadRecentWalletsData()
	for _, w := range existingData {
		if w.Path == filePath {
			storedNetwork := w.Network

			// For legacy wallets without stored network, infer from address prefix
			if storedNetwork == "" && w.AddressPrefix != "" {
				if len(w.AddressPrefix) >= 4 {
					prefix := w.AddressPrefix[:4]
					if prefix == "dero" {
						storedNetwork = "mainnet"
					} else if prefix == "deto" {
						storedNetwork = "simulator" // Simulator wallets use deto1-style prefixes
					}
				}
			}

			if storedNetwork != "" && storedNetwork != currentNetwork {
				// Wallet was previously used on a different network
				if currentNetwork == "simulator" && storedNetwork == "mainnet" {
					networkWarning = "This wallet was last used on mainnet. In simulator mode, your mainnet balance will not be shown."
					a.logToConsole(fmt.Sprintf("[WARN] Opening mainnet wallet in simulator mode"))
				} else if storedNetwork == "simulator" && currentNetwork == "mainnet" {
					networkWarning = "This wallet was last used in simulator mode. Now connecting to mainnet."
					a.logToConsole("[WARN] Opening simulator wallet on mainnet")
				} else if storedNetwork != currentNetwork {
					// Generic mismatch warning
					networkWarning = fmt.Sprintf("This wallet was last used on %s. You are now on %s.", storedNetwork, currentNetwork)
					a.logToConsole(fmt.Sprintf("[WARN] Network mismatch: stored=%s, current=%s", storedNetwork, currentNetwork))
				}
			}
			break
		}
	}

	// Close existing wallet if open
	if walletManager.isOpen && walletManager.wallet != nil {
		walletManager.wallet.Close_Encrypted_Wallet()
		walletManager.isOpen = false
	}

	// Open the wallet
	wallet, err := walletapi.Open_Encrypted_Wallet(filePath, password)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Failed to open wallet: %v", err))
		return ErrorResponse(err)
	}

	walletManager.wallet = wallet
	walletManager.walletPath = filePath
	walletManager.isOpen = true

	// Set network mode (mainnet vs simulator) - MUST be called before GetAddress()
	wallet.SetNetwork(currentNetwork == "mainnet")

	// Get daemon endpoint from settings
	endpointRaw := "127.0.0.1:10102"
	if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		endpointRaw = ep
	}
	endpoint := normalizeDaemonEndpointForWallet(endpointRaw)

	// Connect wallet to daemon
	wallet.SetDaemonAddress(endpoint)
	a.ensureWalletDaemonConnectivity(endpointRaw)
	wallet.SetOnlineMode()
	
	// Track sync status for user feedback
	var syncWarning string
	if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
		a.logToConsole(fmt.Sprintf("[WARN] Initial wallet sync failed: %v - wallet may show outdated balance", err))
		syncWarning = "Initial sync failed. Balance may be outdated until sync completes."
	}

	// Get wallet info (now with correct network prefix)
	address := wallet.GetAddress().String()

	// Add to recent wallets with address info (updates network to current)
	addToRecentWalletsWithInfo(filePath, address)

	a.logToConsole(fmt.Sprintf("[OK] Wallet opened successfully: %s", address[:16]+"..."))

	result := map[string]interface{}{
		"success": true,
		"address": address,
		"message": "Wallet opened successfully",
	}

	// Include network warning if applicable
	if networkWarning != "" {
		result["networkWarning"] = networkWarning
	}
	
	// Include sync warning if applicable
	if syncWarning != "" {
		result["syncWarning"] = syncWarning
	}

	return result
}

// CloseWallet closes the currently open wallet
func (a *App) CloseWallet() map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	a.logToConsole("[WALLET] Closing wallet...")

	walletManager.wallet.Close_Encrypted_Wallet()
	walletManager.wallet = nil
	walletManager.isOpen = false
	walletManager.walletPath = ""

	a.logToConsole("[OK] Wallet closed successfully")

	return map[string]interface{}{
		"success": true,
		"message": "Wallet closed successfully",
	}
}

// GetWalletStatus returns the current wallet status
func (a *App) GetWalletStatus() map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": true,
			"isOpen":  false,
		}
	}

	wallet := walletManager.wallet
	address := wallet.GetAddress().String()

	return map[string]interface{}{
		"success": true,
		"isOpen":  true,
		"address": address,
		"path":    walletManager.walletPath,
	}
}

// GetBalance returns the wallet balance
func (a *App) GetBalance() map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	wallet := walletManager.wallet

	// Get mature (spendable) and locked balance
	mature, locked := wallet.Get_Balance()

	return map[string]interface{}{
		"success":       true,
		"balance":       mature,
		"lockedBalance": locked,
		"balanceHuman":  float64(mature) / 100000.0,
		"lockedHuman":   float64(locked) / 100000.0,
	}
}

// SyncWallet syncs the wallet with the daemon and waits for new blocks to be scanned
func (a *App) SyncWallet() map[string]interface{} {
	walletManager.RLock()
	if !walletManager.isOpen || walletManager.wallet == nil {
		walletManager.RUnlock()
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}
	wallet := walletManager.wallet
	walletManager.RUnlock()

	// Ensure wallet is online and connected to daemon (manual refresh should force this)
	endpointRaw := "127.0.0.1:10102"
	if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		endpointRaw = ep
	}
	endpoint := normalizeDaemonEndpointForWallet(endpointRaw)
	wallet.SetDaemonAddress(endpoint)
	a.ensureWalletDaemonConnectivity(endpointRaw)
	wallet.SetOnlineMode()

	// Force an immediate sync pass (otherwise height may never advance)
	if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Unable to sync wallet. Check your connection to the daemon.",
			"technicalError": err.Error(),
		}
	}

	// Get current heights
	walletHeight := wallet.Get_Height()
	daemonHeight := wallet.Get_Daemon_Height()

	a.logToConsole(fmt.Sprintf("[SYNC] Wallet height: %d, Daemon height: %d", walletHeight, daemonHeight))

	// If wallet is already synced, return immediately
	if daemonHeight == 0 {
		return map[string]interface{}{
			"success":      true,
			"synced":       false,
			"walletHeight": walletHeight,
			"daemonHeight": daemonHeight,
			"behindBlocks": int64(0),
			"message":      "Daemon not connected",
		}
	}

	if walletHeight >= daemonHeight {
		return map[string]interface{}{
			"success":      true,
			"synced":       true,
			"walletHeight": walletHeight,
			"daemonHeight": daemonHeight,
			"behindBlocks": int64(0),
			"message":      "Wallet is up to date",
		}
	}

	// Wallet is behind - wait for it to sync (up to 10 seconds)
	a.logToConsole("[SYNC] Wallet is behind daemon, waiting for sync...")
	
	maxWait := 10 * time.Second
	pollInterval := 500 * time.Millisecond
	startTime := time.Now()
	
	for time.Since(startTime) < maxWait {
		time.Sleep(pollInterval)
		
		walletManager.RLock()
		if walletManager.wallet == nil {
			walletManager.RUnlock()
			return map[string]interface{}{
				"success": false,
				"error":   "Wallet closed during sync",
			}
		}
		newHeight := walletManager.wallet.Get_Height()
		walletManager.RUnlock()
		
		if newHeight >= daemonHeight {
			a.logToConsole(fmt.Sprintf("[SYNC] Wallet synced to height %d", newHeight))
			return map[string]interface{}{
				"success":      true,
				"synced":       true,
				"walletHeight": newHeight,
				"daemonHeight": daemonHeight,
				"behindBlocks": int64(0),
				"message":      "Wallet synced successfully",
			}
		}
		
		// Log progress
		if newHeight > walletHeight {
			a.logToConsole(fmt.Sprintf("[SYNC] Progress: %d / %d", newHeight, daemonHeight))
			walletHeight = newHeight
		}
	}
	
	// Timeout - still syncing
	walletManager.RLock()
	finalHeight := wallet.Get_Height()
	walletManager.RUnlock()
	
	a.logToConsole(fmt.Sprintf("[SYNC] Sync timeout, wallet at %d / %d", finalHeight, daemonHeight))
	
	return map[string]interface{}{
		"success":      true,
		"synced":       false,
		"walletHeight": finalHeight,
		"daemonHeight": daemonHeight,
		"behindBlocks": int64(daemonHeight) - int64(finalHeight),
		"message":      fmt.Sprintf("Still syncing: %d / %d blocks", finalHeight, daemonHeight),
	}
}

// GetWalletSyncStatus returns the current sync status without waiting
func (a *App) GetWalletSyncStatus() map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	wallet := walletManager.wallet
	walletHeight := wallet.Get_Height()
	daemonHeight := wallet.Get_Daemon_Height()
	
	synced := daemonHeight > 0 && walletHeight >= daemonHeight

	return map[string]interface{}{
		"success":      true,
		"synced":       synced,
		"walletHeight": walletHeight,
		"daemonHeight": daemonHeight,
		"behindBlocks": int64(daemonHeight) - int64(walletHeight),
	}
}

// GetAddress returns the wallet address
func (a *App) GetAddress() map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	address := walletManager.wallet.GetAddress().String()

	return map[string]interface{}{
		"success": true,
		"address": address,
	}
}

// GetSeedPhrase returns the wallet's recovery seed phrase (password-protected)
func (a *App) GetSeedPhrase(password string) map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	// Verify password by attempting to re-open the wallet
	// This is a security check to ensure the user has the correct password
	tempWallet, err := walletapi.Open_Encrypted_Wallet(walletManager.walletPath, password)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Failed to verify password for seed phrase: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid password",
		}
	}
	// Close the temporary wallet immediately after verification
	tempWallet.Close_Encrypted_Wallet()

	// Get the seed phrase from the currently open wallet
	seed := walletManager.wallet.GetSeed()

	a.logToConsole("[OK] Seed phrase retrieved (password verified)")

	return map[string]interface{}{
		"success": true,
		"seed":    seed,
		"message": "Seed phrase retrieved successfully",
	}
}

// GetWalletKeys returns the wallet's secret and public keys (password-protected)
func (a *App) GetWalletKeys(password string) map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	// Verify password by attempting to re-open the wallet
	tempWallet, err := walletapi.Open_Encrypted_Wallet(walletManager.walletPath, password)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Failed to verify password for wallet keys: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid password",
		}
	}
	// Close the temporary wallet immediately after verification
	tempWallet.Close_Encrypted_Wallet()

	// Get the keys from the currently open wallet
	keys := walletManager.wallet.Get_Keys()

	// Format secret key (64 hex characters, matching Engram/dero-wallet-cli format)
	// Pad with zeros on the left, then take last 64 characters
	secretHex := keys.Secret.Text(16)
	paddedSecret := "0000000000000000000000000000000000000000000000" + secretHex
	secretKey := paddedSecret[len(paddedSecret)-64:]

	// Get public key
	publicKey := keys.Public.StringHex()

	a.logToConsole("[OK] Wallet keys retrieved (password verified)")

	return map[string]interface{}{
		"success":    true,
		"secretKey":  secretKey,
		"publicKey":  publicKey,
		"message":    "Wallet keys retrieved successfully",
	}
}

// GetIntegratedAddress generates an integrated address with optional destination port (payment ID)
// In DERO, "payment ID" is implemented as a destination port (uint64) embedded in the address.
// The resulting address changes from dero.../deto... to deroi.../detoi... format.
// Optional parameters: comment (string), amount (uint64 in atomic units)
func (a *App) GetIntegratedAddress(destinationPort uint64, comment string, amount uint64) map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	// Get the base address
	baseAddr := walletManager.wallet.GetAddress()

	// Build arguments for the integrated address
	var arguments rpc.Arguments

	// Add destination port (this is DERO's equivalent of payment ID)
	// Port 0 is valid and commonly used for simple transfers
	arguments = append(arguments, rpc.Argument{
		Name:     rpc.RPC_DESTINATION_PORT,
		DataType: rpc.DataUint64,
		Value:    destinationPort,
	})

	// Add optional comment/message
	if comment != "" {
		arguments = append(arguments, rpc.Argument{
			Name:     rpc.RPC_COMMENT,
			DataType: rpc.DataString,
			Value:    comment,
		})
	}

	// Add optional requested amount
	if amount > 0 {
		arguments = append(arguments, rpc.Argument{
			Name:     rpc.RPC_VALUE_TRANSFER,
			DataType: rpc.DataUint64,
			Value:    amount,
		})
	}

	// Clone the address and add arguments to create integrated address
	integratedAddr := baseAddr.Clone()
	integratedAddr.Arguments = arguments

	// Validate the integrated address can be encoded
	_, err := integratedAddr.MarshalText()
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERROR] Failed to create integrated address: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create integrated address: %v", err),
		}
	}

	a.logToConsole(fmt.Sprintf("[OK] Generated integrated address with port %d", destinationPort))

	return map[string]interface{}{
		"success":           true,
		"integratedAddress": integratedAddr.String(),
		"baseAddress":       baseAddr.String(),
		"destinationPort":   destinationPort,
		"comment":           comment,
		"amount":            amount,
	}
}

// SplitIntegratedAddress decodes an integrated address and returns its components
// This is useful for understanding what data is embedded in an integrated address
func (a *App) SplitIntegratedAddress(integratedAddress string) map[string]interface{} {
	// Parse the address
	addr, err := rpc.NewAddress(integratedAddress)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Invalid address format: %v", err),
		}
	}

	result := map[string]interface{}{
		"success":      true,
		"baseAddress":  addr.BaseAddress().String(),
		"isIntegrated": addr.IsIntegratedAddress(),
		"isMainnet":    addr.IsMainnet(),
	}

	// If it's an integrated address, extract the embedded data
	if addr.IsIntegratedAddress() {
		// Extract destination port (payment ID)
		if addr.Arguments.Has(rpc.RPC_DESTINATION_PORT, rpc.DataUint64) {
			result["destinationPort"] = addr.Arguments.Value(rpc.RPC_DESTINATION_PORT, rpc.DataUint64).(uint64)
		}

		// Extract comment if present
		if addr.Arguments.Has(rpc.RPC_COMMENT, rpc.DataString) {
			result["comment"] = addr.Arguments.Value(rpc.RPC_COMMENT, rpc.DataString).(string)
		}

		// Extract requested amount if present
		if addr.Arguments.Has(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64) {
			result["amount"] = addr.Arguments.Value(rpc.RPC_VALUE_TRANSFER, rpc.DataUint64).(uint64)
		}

		// Extract expiry if present
		if addr.Arguments.Has(rpc.RPC_EXPIRY, rpc.DataTime) {
			result["expiry"] = addr.Arguments.Value(rpc.RPC_EXPIRY, rpc.DataTime)
		}

		// Extract needs replyback flag if present
		if addr.Arguments.Has(rpc.RPC_NEEDS_REPLYBACK_ADDRESS, rpc.DataUint64) {
			result["needsReplyback"] = true
		}

		// Include raw arguments count
		result["argumentCount"] = len(addr.Arguments)
	}

	return result
}

// ListRecentWallets returns the list of recently opened wallets
func (a *App) ListRecentWallets() []string {
	walletManager.RLock()
	defer walletManager.RUnlock()
	return walletManager.recentWallets
}

// Transfer sends DERO to another address
func (a *App) Transfer(destination string, amount uint64, paymentID string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	wallet := walletManager.wallet

	// Validate destination address
	if destination == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Destination address is required",
		}
	}

	// Validate amount
	if amount == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "Amount must be greater than 0",
		}
	}

	destPreview := destination
	if len(destination) > 16 {
		destPreview = destination[:16] + "..."
	}
	a.logToConsole(fmt.Sprintf("[Transfer] Initiating transfer: %d atomic units to %s", amount, destPreview))

	// Build the transfer
	transfers := []rpc.Transfer{
		{
			Destination: destination,
			Amount:      amount,
		},
	}

	// Handle payment ID if provided (integrated address or separate)
	if paymentID != "" {
		// Payment IDs are typically embedded in integrated addresses
		// For now, log it - full implementation would handle this
		a.logToConsole(fmt.Sprintf("[Transfer] Payment ID provided: %s", paymentID))
	}

	// Execute transfer with ringsize 16 (standard), no SC arguments
	tx, err := wallet.TransferPayload0(transfers, 16, false, rpc.Arguments{}, 0, false)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[Transfer] Failed: %s", err.Error()))
		return map[string]interface{}{
			"success":        false,
			"error":          FriendlyError(err),
			"technicalError": err.Error(),
		}
	}

	txid := tx.GetHash().String()
	a.logToConsole(fmt.Sprintf("[Transfer] Success! TXID: %s", txid))

	return map[string]interface{}{
		"success": true,
		"txid":    txid,
		"hex":     fmt.Sprintf("%x", tx.Serialize()),
	}
}

// GetTransactionHistory returns recent transactions with optional labels
func (a *App) GetTransactionHistory(limit int) map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	if limit <= 0 {
		limit = 50
	}

	// Get DERO transactions (zero SCID = native DERO)
	var scid crypto.Hash
	// Show_Transfers(scid, coinbase, incoming, outgoing, min_height, max_height, sender, receiver, dstport, srcport)
	rpcEntries := walletManager.wallet.Show_Transfers(scid, true, true, true, 0, 0, "", "", 0, 0)

	// Load transaction labels for quick lookup
	labelMap := getTransactionLabelsMap()

	// Convert to frontend-friendly format
	entries := make([]map[string]interface{}, 0, len(rpcEntries))
	for _, e := range rpcEntries {
		entry := map[string]interface{}{
			"txid":        e.TXID,
			"height":      e.Height,
			"topoheight":  e.TopoHeight,
			"amount":      e.Amount,
			"incoming":    e.Incoming,
			"coinbase":    e.Coinbase,
			"destination": e.Destination,
			"timestamp":   e.Time.Unix(),
		}
		// Include label if one exists for this transaction
		if label, ok := labelMap[e.TXID]; ok && label != "" {
			entry["label"] = label
		}
		entries = append(entries, entry)
	}

	// Limit results (return most recent)
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	return map[string]interface{}{
		"success":      true,
		"transactions": entries,
		"count":        len(entries),
	}
}

// GetWalletMiningEarnings returns coinbase (mining reward) transactions from wallet
// This filters the transaction history to show only mining rewards
func (a *App) GetWalletMiningEarnings(limit int) map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success":  false,
			"error":    "No wallet is currently open",
			"earnings": []map[string]interface{}{},
		}
	}

	if limit <= 0 {
		limit = 100
	}

	// Get DERO transactions with coinbase filter
	var scid crypto.Hash
	// Show_Transfers(scid, coinbase, incoming, outgoing, min_height, max_height, sender, receiver, dstport, srcport)
	// Only get coinbase transactions (coinbase=true, incoming=true, outgoing=false)
	rpcEntries := walletManager.wallet.Show_Transfers(scid, true, true, false, 0, 0, "", "", 0, 0)

	// Filter for coinbase only and convert to frontend-friendly format
	earnings := make([]map[string]interface{}, 0)
	var totalAmount uint64 = 0
	var blocksCount int = 0
	var minisCount int = 0

	for _, e := range rpcEntries {
		if !e.Coinbase {
			continue // Skip non-mining transactions
		}

		entry := map[string]interface{}{
			"txid":      e.TXID,
			"height":    e.Height,
			"amount":    e.Amount,
			"timestamp": e.Time.Unix(),
		}

		// Try to determine if block or miniblock based on amount
		// Full blocks have higher rewards than miniblocks
		// This is a heuristic - full blocks typically have 2+ DERO, minis less
		rewardType := "miniblock"
		if e.Amount >= 200000 { // 2 DERO = 200000 atomic units (DERO has 5 decimal places)
			rewardType = "block"
			blocksCount++
		} else {
			minisCount++
		}
		entry["type"] = rewardType
		totalAmount += e.Amount

		earnings = append(earnings, entry)
	}

	// Limit results (return most recent)
	if limit > 0 && len(earnings) > limit {
		earnings = earnings[len(earnings)-limit:]
	}

	return map[string]interface{}{
		"success":       true,
		"earnings":      earnings,
		"count":         len(earnings),
		"total_amount":  totalAmount,
		"formatted":     formatDEROAmount(totalAmount),
		"blocks_count":  blocksCount,
		"minis_count":   minisCount,
	}
}

// GetMiningEarningsSummary returns a summary of mining earnings without full list
func (a *App) GetMiningEarningsSummary() map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success":      false,
			"error":        "No wallet is currently open",
			"total_amount": uint64(0),
		}
	}

	// Get all coinbase transactions
	var scid crypto.Hash
	rpcEntries := walletManager.wallet.Show_Transfers(scid, true, true, false, 0, 0, "", "", 0, 0)

	var totalAmount uint64 = 0
	var blocksCount int = 0
	var minisCount int = 0
	var latestHeight uint64 = 0
	var earliestHeight uint64 = 0

	for _, e := range rpcEntries {
		if !e.Coinbase {
			continue
		}

		totalAmount += e.Amount

		// Determine type
		if e.Amount >= 200000 {
			blocksCount++
		} else {
			minisCount++
		}

		// Track height range
		if earliestHeight == 0 || e.Height < earliestHeight {
			earliestHeight = e.Height
		}
		if e.Height > latestHeight {
			latestHeight = e.Height
		}
	}

	return map[string]interface{}{
		"success":         true,
		"total_amount":    totalAmount,
		"formatted":       formatDEROAmount(totalAmount),
		"blocks_count":    blocksCount,
		"minis_count":     minisCount,
		"total_count":     blocksCount + minisCount,
		"earliest_height": earliestHeight,
		"latest_height":   latestHeight,
	}
}

// CreateWallet creates a new wallet file
// If filePath is just a name (no path separators), it will be created in datashards/wallets/
func (a *App) CreateWallet(filePath, password string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	// If just a name is provided (no path separators), construct full path
	if !strings.Contains(filePath, string(filepath.Separator)) && !strings.Contains(filePath, "/") {
		// Clean the name - remove any .db extension if user added it
		name := strings.TrimSuffix(filePath, ".db")
		// Construct path in wallets directory
		filePath = filepath.Join(getDatashardsDir(), "wallets", name+".db")
	}

	a.logToConsole(fmt.Sprintf("[WALLET] Creating new wallet: %s", filePath))

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "A wallet with this name already exists",
		}
	}

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "Failed to create wallet directory",
			"technicalError": err.Error(),
		}
	}

	// Create new wallet
	wallet, err := walletapi.Create_Encrypted_Wallet_Random(filePath, password)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Failed to create wallet: %v", err))
		return ErrorResponse(err)
	}

	// Get the seed phrase for backup
	seed := wallet.GetSeed()

	// Close the wallet (user should open it explicitly)
	wallet.Close_Encrypted_Wallet()

	a.logToConsole("[OK] Wallet created successfully")

	return map[string]interface{}{
		"success": true,
		"seed":    seed,
		"message": "Wallet created successfully. SAVE YOUR SEED PHRASE!",
	}
}

// RestoreWallet restores a wallet from seed phrase
// If filePath is just a name (no path separators), it will be created in datashards/wallets/
func (a *App) RestoreWallet(filePath, password, seed string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	// If just a name is provided (no path separators), construct full path
	if !strings.Contains(filePath, string(filepath.Separator)) && !strings.Contains(filePath, "/") {
		// Clean the name - remove any .db extension if user added it
		name := strings.TrimSuffix(filePath, ".db")
		// Construct path in wallets directory
		filePath = filepath.Join(getDatashardsDir(), "wallets", name+".db")
	}

	a.logToConsole("[WALLET] Restoring wallet from seed...")

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "A wallet with this name already exists",
		}
	}

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "Failed to create wallet directory",
			"technicalError": err.Error(),
		}
	}

	// Restore wallet from seed
	wallet, err := walletapi.Create_Encrypted_Wallet_From_Recovery_Words(filePath, password, seed)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Failed to restore wallet: %v", err))
		return ErrorResponse(err)
	}

	address := wallet.GetAddress().String()
	wallet.Close_Encrypted_Wallet()

	a.logToConsole(fmt.Sprintf("[OK] Wallet restored successfully: %s", address[:16]+"..."))

	return map[string]interface{}{
		"success": true,
		"address": address,
		"message": "Wallet restored successfully",
	}
}

// IsWalletOpen returns whether a wallet is currently open
func (a *App) IsWalletOpen() bool {
	walletManager.RLock()
	defer walletManager.RUnlock()
	return walletManager.isOpen
}

// GetWallet returns the current wallet instance (for internal use)
func GetWallet() *walletapi.Wallet_Disk {
	walletManager.RLock()
	defer walletManager.RUnlock()
	if walletManager.isOpen {
		return walletManager.wallet
	}
	return nil
}

// Helper function to add wallet to recent list
func addToRecentWallets(path string) {
	// Remove if already exists
	newRecent := []string{path}
	for _, p := range walletManager.recentWallets {
		if p != path {
			newRecent = append(newRecent, p)
		}
	}

	// Keep only last 5
	if len(newRecent) > 5 {
		newRecent = newRecent[:5]
	}

	walletManager.recentWallets = newRecent

	// Save to settings file
	saveRecentWallets(newRecent)
}

func saveRecentWallets(wallets []string) {
	configDir := filepath.Join(getDatashardsDir(), "settings")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Printf("Failed to create settings directory: %v", err)
		return
	}

	data, err := json.Marshal(wallets)
	if err != nil {
		log.Printf("Failed to marshal recent wallets: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(configDir, "recent_wallets.json"), data, 0600); err != nil {
		log.Printf("Failed to save recent wallets: %v", err)
	}
}

func loadRecentWallets() []string {
	configFile := filepath.Join(getDatashardsDir(), "settings", "recent_wallets.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return []string{}
	}

	var wallets []string
	if err := json.Unmarshal(data, &wallets); err != nil {
		return []string{}
	}

	return wallets
}

// Initialize recent wallets on startup
func init() {
	walletManager.recentWallets = loadRecentWallets()
}

// ApproveWalletConnection signals that the user has approved a dApp connection
func (a *App) ApproveWalletConnection() map[string]interface{} {
	a.logToConsole("[OK] Wallet connection approved by user")
	return map[string]interface{}{"success": true}
}

// InternalWalletCall executes a wallet method directly using the embedded wallet
func (a *App) InternalWalletCall(method string, params map[string]interface{}, password string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	// If wallet not open, try to open it if we have path and password
	if !walletManager.isOpen || walletManager.wallet == nil {
		if walletManager.walletPath != "" && password != "" {
			a.logToConsole("[WALLET] Unlocking wallet for transaction...")
			var err error
			// Re-open wallet
			walletManager.wallet, err = walletapi.Open_Encrypted_Wallet(walletManager.walletPath, password)
			if err != nil {
				return map[string]interface{}{"success": false, "error": FriendlyError(err), "technicalError": err.Error()}
			}
			
			walletManager.isOpen = true
			walletManager.wallet.SetNetwork(!a.IsInSimulatorMode())
			
			// Set daemon endpoint
			endpointRaw := "127.0.0.1:10102"
			if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
				endpointRaw = ep
			}
			endpoint := normalizeDaemonEndpointForWallet(endpointRaw)
			walletManager.wallet.SetDaemonAddress(endpoint)
			a.ensureWalletDaemonConnectivity(endpointRaw)
			walletManager.wallet.SetOnlineMode()
		} else {
			return map[string]interface{}{"success": false, "error": "Wallet not open"}
		}
	}

	wallet := walletManager.wallet
	a.logToConsole(fmt.Sprintf("[FAST] Internal wallet call: %s", method))

	// Handle methods
	switch method {
	case "GetAddress", "DERO.GetAddress":
		// Return the wallet address
		address := wallet.GetAddress().String()
		return map[string]interface{}{
			"success": true,
			"result":  map[string]string{"address": address},
		}
		
	case "GetBalance", "DERO.GetBalance":
		// Return wallet balance
		balance, lockedBalance := wallet.Get_Balance()
		return map[string]interface{}{
			"success": true,
			"result":  map[string]uint64{"balance": balance, "unlocked_balance": balance - lockedBalance, "locked_balance": lockedBalance},
		}
		
	case "GetHeight", "DERO.GetHeight":
		// Return wallet height
		height := wallet.Get_Height()
		return map[string]interface{}{
			"success": true,
			"result":  map[string]uint64{"height": height},
		}
		
	case "transfer", "Transfer", "DERO.Transfer":
		// Parse transfers (with burn support for dev donations)
		var transfers []rpc.Transfer
		if t, ok := params["transfers"].([]interface{}); ok {
			for _, item := range t {
				if tf, ok := item.(map[string]interface{}); ok {
					amount := uint64(0)
					if a, ok := tf["amount"].(float64); ok {
						amount = uint64(a)
					}
					
					burn := uint64(0)
					if b, ok := tf["burn"].(float64); ok {
						burn = uint64(b)
					}
					
					dest := ""
					if d, ok := tf["destination"].(string); ok {
						dest = d
					}
					
					// Include transfer if it has destination, amount, or burn
					if dest != "" || amount > 0 || burn > 0 {
						transfers = append(transfers, rpc.Transfer{
							Destination: dest,
							Amount:      amount,
							Burn:        burn,
						})
					}
				}
			}
		} else {
			// Try single destination/amount if provided at top level
			amount := uint64(0)
			if a, ok := params["amount"].(float64); ok {
				amount = uint64(a)
			}
			dest := ""
			if d, ok := params["destination"].(string); ok {
				dest = d
			}
			
			if dest != "" {
				transfers = append(transfers, rpc.Transfer{
					Destination: dest,
					Amount:      amount,
				})
			}
		}
		
		// Check if this transfer includes SC call parameters (scid + sc_rpc)
		// Some dApps (like Villager) send SC calls via the transfer method
		scArgs := rpc.Arguments{}
		scid := ""
		if s, ok := params["scid"].(string); ok {
			scid = s
		}
		
		// If scid and sc_rpc are present, parse SC arguments
		if scid != "" {
			if args, ok := params["sc_rpc"].([]interface{}); ok && len(args) > 0 {
				hasEntrypointInScRpc := false
				entrypointFromScRpc := ""
				
				for _, arg := range args {
					if a, ok := arg.(map[string]interface{}); ok {
						name, _ := a["name"].(string)
						type_, _ := a["datatype"].(string)
						val := a["value"]
						
						if name != "" {
							// Track if entrypoint is in sc_rpc
							if name == "entrypoint" {
								hasEntrypointInScRpc = true
								if ep, ok := val.(string); ok {
									entrypointFromScRpc = ep
								}
							}
							
							switch type_ {
							case "U":
								if v, ok := val.(float64); ok {
									scArgs = append(scArgs, rpc.Argument{Name: name, DataType: "U", Value: uint64(v)})
								}
							case "S":
								if v, ok := val.(string); ok {
									scArgs = append(scArgs, rpc.Argument{Name: name, DataType: "S", Value: v})
								}
							case "H":
								// Handle hash type (SCID)
								if v, ok := val.(string); ok {
									scArgs = append(scArgs, rpc.Argument{Name: name, DataType: "H", Value: crypto.HashHexToHash(v)})
								}
							}
						}
					}
				}
				
				// Determine entrypoint: check sc_rpc first, then separate param
				entrypoint := entrypointFromScRpc
				if entrypoint == "" {
					if ep, ok := params["entrypoint"].(string); ok {
						entrypoint = ep
					}
				}
				
				// For SC invocation, SCACTION and SCID must be prepended if entrypoint exists
				if entrypoint != "" || hasEntrypointInScRpc {
					// Check if SCACTION and SCID are already in scArgs (avoid duplicates)
					hasSCACTION := false
					hasSCID := false
					for _, arg := range scArgs {
						if arg.Name == rpc.SCACTION {
							hasSCACTION = true
						}
						if arg.Name == rpc.SCID {
							hasSCID = true
						}
					}
					
					// Prepend SCACTION and SCID if not already present
					newArgs := rpc.Arguments{}
					if !hasSCACTION {
						newArgs = append(newArgs, rpc.Argument{Name: rpc.SCACTION, DataType: "U", Value: uint64(rpc.SC_CALL)})
					}
					if !hasSCID {
						newArgs = append(newArgs, rpc.Argument{Name: rpc.SCID, DataType: "H", Value: crypto.HashHexToHash(scid)})
					}
					
					// Add entrypoint if it was specified as a separate param (not in sc_rpc)
					if entrypoint != "" && !hasEntrypointInScRpc {
						newArgs = append(newArgs, rpc.Argument{Name: "entrypoint", DataType: "S", Value: entrypoint})
					}
					
					// Prepend new args to existing scArgs
					scArgs = append(newArgs, scArgs...)
				}
				
				scidPreview := scid
				if len(scid) > 16 {
					scidPreview = scid[:16] + "..."
				}
				a.logToConsole(fmt.Sprintf("[XSWD] Transfer with SC call detected: scid=%s, entrypoint=%s", scidPreview, entrypoint))
			}
		}
		
		// For pure transfers (no SC), we still need at least one transfer
		if len(transfers) == 0 && len(scArgs) == 0 {
			return map[string]interface{}{"success": false, "error": "Please specify a transfer amount and destination, or a smart contract call."}
		}
		
		// Execute transfer (with optional SC arguments)
		tx, err := wallet.TransferPayload0(transfers, 16, false, scArgs, 0, false)
		if err != nil {
			return map[string]interface{}{"success": false, "error": FriendlyError(err), "technicalError": err.Error()}
		}
		
		return map[string]interface{}{
			"success": true,
			"txid":    tx.GetHash().String(),
			"hex":     fmt.Sprintf("%x", tx.Serialize()),
		}

	case "scinvoke", "SC_Invoke", "DERO.SC_Invoke":
		scid := ""
		if s, ok := params["scid"].(string); ok {
			scid = s
		}
		
		if scid == "" {
			return map[string]interface{}{"success": false, "error": "Smart Contract ID (SCID) is required for this operation."}
		}
		
		// Parse SC arguments and track if entrypoint is in sc_rpc
		scArgs := rpc.Arguments{}
		hasEntrypointInScRpc := false
		entrypointFromScRpc := ""
		
		if args, ok := params["sc_rpc"].([]interface{}); ok {
			for _, arg := range args {
				if a, ok := arg.(map[string]interface{}); ok {
					name, _ := a["name"].(string)
					type_, _ := a["datatype"].(string)
					val := a["value"]
					
					if name != "" {
						// Track if entrypoint is in sc_rpc (feed.tela sends it this way)
						if name == "entrypoint" {
							hasEntrypointInScRpc = true
							if ep, ok := val.(string); ok {
								entrypointFromScRpc = ep
							}
						}
						
						switch type_ {
						case "U":
							if v, ok := val.(float64); ok {
								scArgs = append(scArgs, rpc.Argument{Name: name, DataType: "U", Value: uint64(v)})
							}
						case "S":
							if v, ok := val.(string); ok {
								scArgs = append(scArgs, rpc.Argument{Name: name, DataType: "S", Value: v})
							}
						case "H":
							// Handle hash type (SCID)
							if v, ok := val.(string); ok {
								scArgs = append(scArgs, rpc.Argument{Name: name, DataType: "H", Value: crypto.HashHexToHash(v)})
							}
						}
					}
				}
			}
		}
		
		// Determine entrypoint: check sc_rpc first, then separate param
		entrypoint := entrypointFromScRpc
		if entrypoint == "" {
			if ep, ok := params["entrypoint"].(string); ok {
				entrypoint = ep
			}
		}
		
		// For SC invocation, SCACTION and SCID must be prepended if entrypoint exists
		// This is required regardless of where entrypoint was specified
		if entrypoint != "" || hasEntrypointInScRpc {
			// Check if SCACTION and SCID are already in scArgs (avoid duplicates)
			hasSCACTION := false
			hasSCID := false
			for _, arg := range scArgs {
				if arg.Name == rpc.SCACTION {
					hasSCACTION = true
				}
				if arg.Name == rpc.SCID {
					hasSCID = true
				}
			}
			
			// Prepend SCACTION and SCID if not already present
			newArgs := rpc.Arguments{}
			if !hasSCACTION {
				newArgs = append(newArgs, rpc.Argument{Name: rpc.SCACTION, DataType: "U", Value: uint64(rpc.SC_CALL)})
			}
			if !hasSCID {
				newArgs = append(newArgs, rpc.Argument{Name: rpc.SCID, DataType: "H", Value: crypto.HashHexToHash(scid)})
			}
			
			// Add entrypoint if it was specified as a separate param (not in sc_rpc)
			if entrypoint != "" && !hasEntrypointInScRpc {
				newArgs = append(newArgs, rpc.Argument{Name: "entrypoint", DataType: "S", Value: entrypoint})
			}
			
			// Prepend new args to existing scArgs
			scArgs = append(newArgs, scArgs...)
		}
		
		// Check for transfers attached to SC call (including burns for dev donations)
		var transfers []rpc.Transfer
		if t, ok := params["transfers"].([]interface{}); ok {
			for _, item := range t {
				if tf, ok := item.(map[string]interface{}); ok {
					amt := uint64(0)
					if a, ok := tf["amount"].(float64); ok {
						amt = uint64(a)
					}
					
					burn := uint64(0)
					if b, ok := tf["burn"].(float64); ok {
						burn = uint64(b)
					}
					
					dest := ""
					if d, ok := tf["destination"].(string); ok {
						dest = d
					}
					
					// Include transfer if it has amount OR burn (burn is used for dev donations)
					if amt > 0 || burn > 0 {
						transfers = append(transfers, rpc.Transfer{
							Destination: dest,
							Amount:      amt,
							Burn:        burn,
						})
					}
				}
			}
		}

		// Execute SC Invoke
		// Note: WalletAPI might differ on SC invoke method signature
		// Using TransferPayload0 with SC args
		// For SC invoke, destination is usually random burn address or handled via rpc args
		
		// For SC invocation, we generally use TransferPayload0 with Arguments
		// If we are sending DERO to the SC, we include it in transfers
		
		tx, err := wallet.TransferPayload0(transfers, 16, false, scArgs, 0, false)
		if err != nil {
			return map[string]interface{}{"success": false, "error": FriendlyError(err), "technicalError": err.Error()}
		}
		
		return map[string]interface{}{
			"success": true,
			"txid":    tx.GetHash().String(),
			"hex":     fmt.Sprintf("%x", tx.Serialize()),
		}
		
	case "GetTrackedAssets", "gettrackedassets":
		// Return tracked asset balances
		// For the internal wallet, we return DERO balance at minimum
		// The wallet's Balance map is private, so we access what we can
		balance, lockedBalance := wallet.Get_Balance()
		
		// SCID "0000...0000" (zero SCID) represents native DERO
		zeroScid := "0000000000000000000000000000000000000000000000000000000000000000"
		
		balances := map[string]uint64{
			zeroScid: balance,
		}
		
		// Check for only_positive_balances param
		onlyPositive := true // default
		if opb, ok := params["only_positive_balances"].(bool); ok {
			onlyPositive = opb
		}
		
		// Filter zero balances if only_positive_balances is true
		if onlyPositive && balance == 0 {
			balances = map[string]uint64{}
		}
		
		return map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"balances":        balances,
				"locked_balance":  lockedBalance,
			},
		}
		
	default:
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("Method '%s' is not available. Use XSWD for this operation.", method)}
	}
}

// SelectWalletFile opens a file dialog to select a wallet file
func (a *App) SelectWalletFile() string {
	selection, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Wallet File",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "DERO Wallet (*.db)",
				Pattern:     "*.db",
			},
			{
				DisplayName: "All Files (*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		log.Printf("Error opening file dialog: %v", err)
		return ""
	}
	return selection
}

// WalletInfo represents information about a wallet for the frontend
type WalletInfo struct {
	Path          string `json:"path"`
	Filename      string `json:"filename"`
	AddressPrefix string `json:"addressPrefix"`
	LastUsed      int64  `json:"lastUsed"`
	IsCurrent     bool   `json:"isCurrent"`
	Network       string `json:"network"` // "mainnet" or "simulator"
}

// SwitchWallet closes the current wallet and opens a different one
func (a *App) SwitchWallet(filePath, password string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	a.logToConsole(fmt.Sprintf("[SYNC] Switching wallet to: %s", filepath.Base(filePath)))

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return map[string]interface{}{
			"success": false,
			"error":   "Wallet file not found",
		}
	}

	// Close existing wallet if open
	if walletManager.isOpen && walletManager.wallet != nil {
		a.logToConsole("[WALLET] Closing current wallet...")
		walletManager.wallet.Close_Encrypted_Wallet()
		walletManager.isOpen = false
		walletManager.wallet = nil
	}

	// Open the new wallet
	wallet, err := walletapi.Open_Encrypted_Wallet(filePath, password)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Failed to switch wallet: %v", err))
		return ErrorResponse(err)
	}

	walletManager.wallet = wallet
	walletManager.walletPath = filePath
	walletManager.isOpen = true

	// Set network mode (mainnet vs simulator) - MUST be called before GetAddress()
	wallet.SetNetwork(!a.IsInSimulatorMode())

	// Get daemon endpoint from settings
	endpointRaw := "127.0.0.1:10102"
	if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		endpointRaw = ep
	}
	endpoint := normalizeDaemonEndpointForWallet(endpointRaw)

	// Connect wallet to daemon
	wallet.SetDaemonAddress(endpoint)
	a.ensureWalletDaemonConnectivity(endpointRaw)
	wallet.SetOnlineMode()
	if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
		a.logToConsole(fmt.Sprintf("[WARN] Initial wallet sync failed: %v", err))
	}

	// Add to recent wallets with updated timestamp (now with correct network prefix)
	addToRecentWalletsWithInfo(filePath, wallet.GetAddress().String())

	address := wallet.GetAddress().String()
	mature, locked := wallet.Get_Balance()

	a.logToConsole(fmt.Sprintf("[OK] Switched to wallet: %s", address[:16]+"..."))

	return map[string]interface{}{
		"success":       true,
		"address":       address,
		"balance":       mature,
		"lockedBalance": locked,
		"balanceHuman":  float64(mature) / 100000.0,
		"message":       "Wallet switched successfully",
	}
}

// GetRecentWalletsWithInfo returns recent wallets with additional metadata
func (a *App) GetRecentWalletsWithInfo() []WalletInfo {
	walletManager.RLock()
	defer walletManager.RUnlock()

	// Load the extended wallet info
	infos := loadRecentWalletsWithInfo()

	// Mark current wallet
	for i := range infos {
		if walletManager.isOpen && infos[i].Path == walletManager.walletPath {
			infos[i].IsCurrent = true
		}
	}

	return infos
}

// Extended wallet info storage
type recentWalletData struct {
	Path          string `json:"path"`
	AddressPrefix string `json:"addressPrefix"`
	LastUsed      int64  `json:"lastUsed"`
	Network       string `json:"network"` // "mainnet" or "simulator"
}

func addToRecentWalletsWithInfo(path, address string) {
	// Load existing data
	existing := loadRecentWalletsData()

	// Determine current network mode - check multiple ways for robustness
	network := "mainnet"
	simArg := globals.Arguments["--simulator"]

	if simArg == true || simArg == "true" || fmt.Sprintf("%v", simArg) == "true" {
		network = "simulator"
	}

	// Create new entry
	newEntry := recentWalletData{
		Path:          path,
		AddressPrefix: "",
		LastUsed:      nowUnix(),
		Network:       network,
	}
	if len(address) >= 16 {
		newEntry.AddressPrefix = address[:16] + "..."
	}

	// Remove existing entry for same path and add new one at front
	newData := []recentWalletData{newEntry}
	for _, e := range existing {
		if e.Path != path {
			newData = append(newData, e)
		}
	}

	// Keep only last 10
	if len(newData) > 10 {
		newData = newData[:10]
	}

	// Save
	saveRecentWalletsData(newData)

	// Also update the simple list for backward compatibility
	simplePaths := make([]string, len(newData))
	for i, d := range newData {
		simplePaths[i] = d.Path
	}
	walletManager.recentWallets = simplePaths
}

func loadRecentWalletsData() []recentWalletData {
	configFile := filepath.Join(getDatashardsDir(), "settings", "recent_wallets_info.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		// Try to migrate from old format
		oldWallets := loadRecentWallets()
		if len(oldWallets) > 0 {
			result := make([]recentWalletData, len(oldWallets))
			for i, p := range oldWallets {
				result[i] = recentWalletData{
					Path:          p,
					AddressPrefix: "",
					LastUsed:      0,
				}
			}
			return result
		}
		return []recentWalletData{}
	}

	var wallets []recentWalletData
	if err := json.Unmarshal(data, &wallets); err != nil {
		return []recentWalletData{}
	}

	return wallets
}

func saveRecentWalletsData(wallets []recentWalletData) {
	configDir := filepath.Join(getDatashardsDir(), "settings")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Printf("Failed to create settings directory: %v", err)
		return
	}

	data, err := json.Marshal(wallets)
	if err != nil {
		log.Printf("Failed to marshal recent wallets data: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(configDir, "recent_wallets_info.json"), data, 0600); err != nil {
		log.Printf("Failed to save recent wallets data: %v", err)
	}
}

func loadRecentWalletsWithInfo() []WalletInfo {
	data := loadRecentWalletsData()
	result := make([]WalletInfo, len(data))
	for i, d := range data {
		// Default to mainnet if not set (for backward compatibility)
		network := d.Network
		if network == "" {
			// Infer from address prefix if possible
			if len(d.AddressPrefix) > 4 {
				if d.AddressPrefix[:4] == "deto" {
					network = "simulator"
				} else {
					network = "mainnet"
				}
			} else {
				network = "mainnet"
			}
		}
		result[i] = WalletInfo{
			Path:          d.Path,
			Filename:      filepath.Base(d.Path),
			AddressPrefix: d.AddressPrefix,
			LastUsed:      d.LastUsed,
			IsCurrent:     false,
			Network:       network,
		}
	}
	return result
}

// RemoveRecentWallet removes a wallet from the recent wallets list by path
func (a *App) RemoveRecentWallet(path string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	// Load existing data
	existing := loadRecentWalletsData()

	// Filter out the wallet to remove
	var filtered []recentWalletData
	found := false
	for _, w := range existing {
		if w.Path != path {
			filtered = append(filtered, w)
		} else {
			found = true
		}
	}

	if !found {
		return map[string]interface{}{
			"success": false,
			"error":   "Wallet not found in recent list",
		}
	}

	// Save filtered list
	saveRecentWalletsData(filtered)

	// Update in-memory list
	simplePaths := make([]string, len(filtered))
	for i, w := range filtered {
		simplePaths[i] = w.Path
	}
	walletManager.recentWallets = simplePaths

	return map[string]interface{}{
		"success": true,
	}
}

// ClearRecentWallets removes all wallets from the recent list
func (a *App) ClearRecentWallets() map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	// Clear the data file
	saveRecentWalletsData([]recentWalletData{})

	// Clear in-memory list
	walletManager.recentWallets = []string{}

	return map[string]interface{}{
		"success": true,
	}
}

func nowUnix() int64 {
	return time.Now().Unix()
}

// GetCurrentWalletPath returns the path of the currently open wallet
func (a *App) GetCurrentWalletPath() string {
	walletManager.RLock()
	defer walletManager.RUnlock()
	if walletManager.isOpen {
		return walletManager.walletPath
	}
	return ""
}

// TrackedToken represents a user-tracked token
type TrackedToken struct {
	SCID    string `json:"scid"`
	Name    string `json:"name"`
	Symbol  string `json:"symbol"`
	AddedAt int64  `json:"addedAt"`
}

// GetTrackedTokens returns the list of user-tracked tokens with balances
func (a *App) GetTrackedTokens() map[string]interface{} {
	// Load tracked tokens from settings
	tokens := loadTrackedTokens()

	// If we have a local wallet open, get balances
	walletManager.RLock()
	localWalletOpen := walletManager.isOpen && walletManager.wallet != nil
	var walletAddress string
	if localWalletOpen {
		walletAddress = walletManager.wallet.GetAddress().String()
	}
	walletManager.RUnlock()

	result := make([]map[string]interface{}, 0)

	// Always include native DERO first if wallet is open
	if localWalletOpen {
		walletManager.RLock()
		mature, _ := walletManager.wallet.Get_Balance()
		walletManager.RUnlock()

		result = append(result, map[string]interface{}{
			"scid":    "0000000000000000000000000000000000000000000000000000000000000000",
			"name":    "DERO",
			"symbol":  "DERO",
			"balance": mature,
			"native":  true,
		})
	}

	// For each tracked token, try to get balance from Gnomon or SC query
	for _, token := range tokens {
		tokenData := map[string]interface{}{
			"scid":    token.SCID,
			"name":    token.Name,
			"symbol":  token.Symbol,
			"balance": uint64(0),
			"native":  false,
		}

		// Try to get balance from Gnomon if running
		if a.gnomonClient != nil && a.gnomonClient.IsRunning() && walletAddress != "" {
			// Look up balance in SC variables
			vars := a.gnomonClient.GetAllSCIDVariableDetails(token.SCID)
			for _, v := range vars {
				key := fmt.Sprintf("%v", v.Key)
				// Token balances are typically stored as address keys
				if key == walletAddress {
					if balance, ok := v.Value.(uint64); ok {
						tokenData["balance"] = balance
					} else if balance, ok := v.Value.(float64); ok {
						tokenData["balance"] = uint64(balance)
					}
				}
				// Also try common naming patterns
				if token.Name == "" && key == "nameHdr" {
					tokenData["name"] = decodeHexString(fmt.Sprintf("%v", v.Value))
				}
				if token.Symbol == "" && key == "symbolHdr" {
					tokenData["symbol"] = decodeHexString(fmt.Sprintf("%v", v.Value))
				}
			}
		}

		result = append(result, tokenData)
	}

	return map[string]interface{}{
		"success": true,
		"tokens":  result,
		"count":   len(result),
	}
}

// AddTrackedToken adds a token SCID to the tracked list
func (a *App) AddTrackedToken(scid, name, symbol string) map[string]interface{} {
	// Validate SCID format (64 hex chars)
	if len(scid) != 64 {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid SCID format - must be 64 hexadecimal characters",
		}
	}

	// Check if already tracked
	tokens := loadTrackedTokens()
	for _, t := range tokens {
		if t.SCID == scid {
			return map[string]interface{}{
				"success": false,
				"error":   "Token already tracked",
			}
		}
	}

	// Try to fetch token metadata from Gnomon
	if a.gnomonClient != nil && a.gnomonClient.IsRunning() {
		vars := a.gnomonClient.GetAllSCIDVariableDetails(scid)
		for _, v := range vars {
			key := fmt.Sprintf("%v", v.Key)
			if name == "" && key == "nameHdr" {
				name = decodeHexString(fmt.Sprintf("%v", v.Value))
			}
			if symbol == "" && key == "symbolHdr" {
				symbol = decodeHexString(fmt.Sprintf("%v", v.Value))
			}
		}
	}

	// Add to tracked list
	newToken := TrackedToken{
		SCID:    scid,
		Name:    name,
		Symbol:  symbol,
		AddedAt: time.Now().Unix(),
	}
	tokens = append(tokens, newToken)
	saveTrackedTokens(tokens)

	a.logToConsole(fmt.Sprintf("📌 Added tracked token: %s (%s)", name, scid[:16]+"..."))

	return map[string]interface{}{
		"success": true,
		"token":   newToken,
		"message": "Token added to portfolio",
	}
}

// RemoveTrackedToken removes a token from the tracked list
func (a *App) RemoveTrackedToken(scid string) map[string]interface{} {
	tokens := loadTrackedTokens()
	newTokens := make([]TrackedToken, 0)
	found := false

	for _, t := range tokens {
		if t.SCID != scid {
			newTokens = append(newTokens, t)
		} else {
			found = true
		}
	}

	if !found {
		return map[string]interface{}{
			"success": false,
			"error":   "Token not found in tracked list",
		}
	}

	saveTrackedTokens(newTokens)

	return map[string]interface{}{
		"success": true,
		"message": "Token removed from portfolio",
	}
}

// TransferToken sends a token (non-native asset) to another address
func (a *App) TransferToken(scid, destination string, amount uint64, password string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		// Try to reopen with password
		if walletManager.walletPath != "" && password != "" {
			var err error
			walletManager.wallet, err = walletapi.Open_Encrypted_Wallet(walletManager.walletPath, password)
			if err != nil {
				return map[string]interface{}{"success": false, "error": FriendlyError(err), "technicalError": err.Error()}
			}
			walletManager.isOpen = true
			endpoint := "127.0.0.1:10102"
			if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
				endpoint = ep
				if len(endpoint) > 7 && endpoint[:7] == "http://" {
					endpoint = endpoint[7:]
				}
			}
			walletManager.wallet.SetDaemonAddress(endpoint)
		} else {
			return map[string]interface{}{"success": false, "error": "Please open a wallet first."}
		}
	}

	wallet := walletManager.wallet

	a.logToConsole(fmt.Sprintf("[Transfer] Transferring %d units of token %s to %s", amount, scid[:16]+"...", destination[:16]+"..."))

	// Build transfer with asset (token)
	// For DERO tokens, transfers include the SCID as the asset
	transfers := []rpc.Transfer{
		{
			Destination: destination,
			Amount:      0,         // DERO amount (0 for pure token transfer)
			Burn:        amount,    // Token amount to transfer
			SCID:        crypto.HashHexToHash(scid),
		},
	}

	// Execute transfer
	tx, err := wallet.TransferPayload0(transfers, 16, false, rpc.Arguments{}, 0, false)
	if err != nil {
		return ErrorResponse(err)
	}

	a.logToConsole(fmt.Sprintf("[OK] Token transfer successful! TXID: %s", tx.GetHash().String()))

	return map[string]interface{}{
		"success": true,
		"txid":    tx.GetHash().String(),
		"message": "Token transfer sent successfully",
	}
}

// Helper functions for tracked tokens storage
func loadTrackedTokens() []TrackedToken {
	configFile := filepath.Join(getDatashardsDir(), "settings", "tracked_tokens.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return []TrackedToken{}
	}

	var tokens []TrackedToken
	if err := json.Unmarshal(data, &tokens); err != nil {
		return []TrackedToken{}
	}

	return tokens
}

func saveTrackedTokens(tokens []TrackedToken) {
	configDir := filepath.Join(getDatashardsDir(), "settings")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Printf("Failed to create settings directory: %v", err)
		return
	}

	data, err := json.Marshal(tokens)
	if err != nil {
		log.Printf("Failed to marshal tracked tokens: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(configDir, "tracked_tokens.json"), data, 0600); err != nil {
		log.Printf("Failed to save tracked tokens: %v", err)
	}
}

// ============================================
// ADDRESS BOOK FUNCTIONS
// ============================================

// AddressBookEntry represents a saved contact
type AddressBookEntry struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Address   string `json:"address"`
	Notes     string `json:"notes"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// GetAddressBook returns all saved contacts
func (a *App) GetAddressBook() map[string]interface{} {
	contacts := loadAddressBook()
	return map[string]interface{}{
		"success":  true,
		"contacts": contacts,
		"count":    len(contacts),
	}
}

// AddContact adds a new contact to the address book
func (a *App) AddContact(label, address, notes string) map[string]interface{} {
	// Validate address format
	if !strings.HasPrefix(address, "dero1") && !strings.HasPrefix(address, "deto1") {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid DERO address format",
		}
	}

	if label == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Label is required",
		}
	}

	contacts := loadAddressBook()

	// Check for duplicate address
	for _, c := range contacts {
		if c.Address == address {
			return map[string]interface{}{
				"success": false,
				"error":   "Address already exists in address book",
			}
		}
	}

	// Generate unique ID
	id := fmt.Sprintf("contact_%d", time.Now().UnixNano())

	newContact := AddressBookEntry{
		ID:        id,
		Label:     label,
		Address:   address,
		Notes:     notes,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	contacts = append(contacts, newContact)
	saveAddressBook(contacts)

	a.logToConsole(fmt.Sprintf("📒 Added contact: %s", label))

	return map[string]interface{}{
		"success": true,
		"contact": newContact,
		"message": "Contact added successfully",
	}
}

// UpdateContact updates an existing contact
func (a *App) UpdateContact(id, label, address, notes string) map[string]interface{} {
	contacts := loadAddressBook()
	found := false

	for i, c := range contacts {
		if c.ID == id {
			contacts[i].Label = label
			contacts[i].Address = address
			contacts[i].Notes = notes
			contacts[i].UpdatedAt = time.Now().Unix()
			found = true
			break
		}
	}

	if !found {
		return map[string]interface{}{
			"success": false,
			"error":   "Contact not found",
		}
	}

	saveAddressBook(contacts)

	return map[string]interface{}{
		"success": true,
		"message": "Contact updated successfully",
	}
}

// DeleteContact removes a contact from the address book
func (a *App) DeleteContact(id string) map[string]interface{} {
	contacts := loadAddressBook()
	newContacts := make([]AddressBookEntry, 0)
	found := false

	for _, c := range contacts {
		if c.ID != id {
			newContacts = append(newContacts, c)
		} else {
			found = true
		}
	}

	if !found {
		return map[string]interface{}{
			"success": false,
			"error":   "Contact not found",
		}
	}

	saveAddressBook(newContacts)

	return map[string]interface{}{
		"success": true,
		"message": "Contact deleted successfully",
	}
}

// Helper functions for address book storage
func loadAddressBook() []AddressBookEntry {
	configFile := filepath.Join(getDatashardsDir(), "settings", "address_book.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return []AddressBookEntry{}
	}

	var contacts []AddressBookEntry
	if err := json.Unmarshal(data, &contacts); err != nil {
		return []AddressBookEntry{}
	}

	return contacts
}

func saveAddressBook(contacts []AddressBookEntry) {
	configDir := filepath.Join(getDatashardsDir(), "settings")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Printf("Failed to create settings directory: %v", err)
		return
	}

	data, err := json.Marshal(contacts)
	if err != nil {
		log.Printf("Failed to marshal address book: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(configDir, "address_book.json"), data, 0600); err != nil {
		log.Printf("Failed to save address book: %v", err)
	}
}

// ============================================
// CHANGE WALLET PASSWORD
// ============================================

// ChangeWalletPassword changes the password for the currently open wallet
// Requires current password for verification before changing
func (a *App) ChangeWalletPassword(currentPassword, newPassword string) map[string]interface{} {
	walletManager.Lock()
	defer walletManager.Unlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	if currentPassword == "" || newPassword == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Current and new passwords are required",
		}
	}

	if len(newPassword) < 1 {
		return map[string]interface{}{
			"success": false,
			"error":   "New password cannot be empty",
		}
	}

	// Verify current password by attempting to re-open the wallet
	tempWallet, err := walletapi.Open_Encrypted_Wallet(walletManager.walletPath, currentPassword)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Password verification failed: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   "Current password is incorrect",
		}
	}
	// Close the temporary wallet immediately after verification
	tempWallet.Close_Encrypted_Wallet()

	// Change the password on the currently open wallet
	err = walletManager.wallet.Set_Encrypted_Wallet_Password(newPassword)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Failed to change password: %v", err))
		return ErrorResponse(err)
	}

	a.logToConsole("[OK] Wallet password changed successfully")

	return map[string]interface{}{
		"success": true,
		"message": "Wallet password changed successfully",
	}
}

// ============================================
// TRANSACTION LABELS
// ============================================

// TransactionLabel represents a user-defined label for a transaction
type TransactionLabel struct {
	TXID      string `json:"txid"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// SetTransactionLabel adds or updates a label for a transaction
func (a *App) SetTransactionLabel(txid, label string) map[string]interface{} {
	if txid == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Transaction ID is required",
		}
	}

	// Load existing labels
	labels := loadTransactionLabels()

	now := time.Now().Unix()

	// Check if label exists for this TXID
	found := false
	for i, l := range labels {
		if l.TXID == txid {
			if label == "" {
				// Remove label if empty
				labels = append(labels[:i], labels[i+1:]...)
			} else {
				// Update existing label
				labels[i].Label = label
				labels[i].UpdatedAt = now
			}
			found = true
			break
		}
	}

	// Add new label if not found and label is not empty
	if !found && label != "" {
		labels = append(labels, TransactionLabel{
			TXID:      txid,
			Label:     label,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	// Save labels
	saveTransactionLabels(labels)

	if label == "" {
		a.logToConsole(fmt.Sprintf("[TX] Removed label for transaction %s", txid[:16]+"..."))
	} else {
		a.logToConsole(fmt.Sprintf("[TX] Set label for transaction %s: %s", txid[:16]+"...", label))
	}

	return map[string]interface{}{
		"success": true,
		"message": "Transaction label saved",
	}
}

// GetTransactionLabel retrieves the label for a specific transaction
func (a *App) GetTransactionLabel(txid string) map[string]interface{} {
	if txid == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Transaction ID is required",
		}
	}

	labels := loadTransactionLabels()

	for _, l := range labels {
		if l.TXID == txid {
			return map[string]interface{}{
				"success": true,
				"label":   l.Label,
				"txid":    l.TXID,
			}
		}
	}

	return map[string]interface{}{
		"success": true,
		"label":   "",
		"txid":    txid,
	}
}

// GetAllTransactionLabels returns all transaction labels
func (a *App) GetAllTransactionLabels() map[string]interface{} {
	labels := loadTransactionLabels()

	// Convert to map for easy lookup
	labelMap := make(map[string]string)
	for _, l := range labels {
		labelMap[l.TXID] = l.Label
	}

	return map[string]interface{}{
		"success": true,
		"labels":  labelMap,
		"count":   len(labels),
	}
}

// DeleteTransactionLabel removes a label for a transaction
func (a *App) DeleteTransactionLabel(txid string) map[string]interface{} {
	return a.SetTransactionLabel(txid, "") // Setting empty label removes it
}

// Helper functions for transaction labels storage
func loadTransactionLabels() []TransactionLabel {
	configFile := filepath.Join(getDatashardsDir(), "settings", "transaction_labels.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return []TransactionLabel{}
	}

	var labels []TransactionLabel
	if err := json.Unmarshal(data, &labels); err != nil {
		return []TransactionLabel{}
	}

	return labels
}

func saveTransactionLabels(labels []TransactionLabel) {
	configDir := filepath.Join(getDatashardsDir(), "settings")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Printf("Failed to create settings directory: %v", err)
		return
	}

	data, err := json.Marshal(labels)
	if err != nil {
		log.Printf("Failed to marshal transaction labels: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(configDir, "transaction_labels.json"), data, 0600); err != nil {
		log.Printf("Failed to save transaction labels: %v", err)
	}
}

// getTransactionLabelsMap returns a map of txid -> label for quick lookup
func getTransactionLabelsMap() map[string]string {
	labels := loadTransactionLabels()
	labelMap := make(map[string]string)
	for _, l := range labels {
		labelMap[l.TXID] = l.Label
	}
	return labelMap
}

// ============================================
// MESSAGE SIGNING FUNCTIONS
// ============================================

// SignMessage signs a message with the wallet's private key
// Returns a PEM-encoded signature that can be verified
func (a *App) SignMessage(message string) map[string]interface{} {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	if message == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Message cannot be empty",
		}
	}

	wallet := walletManager.wallet

	// Sign the message - returns PEM encoded signature
	signature := wallet.SignData([]byte(message))
	if signature == nil || len(signature) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to sign message",
		}
	}

	address := wallet.GetAddress().String()

	a.logToConsole("✍️ Message signed successfully")

	return map[string]interface{}{
		"success":   true,
		"signature": string(signature), // PEM encoded string
		"address":   address,
		"message":   message,
	}
}

// VerifySignature verifies a PEM-encoded signed message
// The signature parameter should be the full PEM block from SignMessage
func (a *App) VerifySignature(signedData string) map[string]interface{} {
	if signedData == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Signed data cannot be empty",
		}
	}

	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	wallet := walletManager.wallet

	// CheckSignature takes the PEM data and returns signer, message, error
	signer, message, err := wallet.CheckSignature([]byte(signedData))
	if err != nil {
		return map[string]interface{}{
			"success": true,
			"valid":   false,
			"error":   fmt.Sprintf("Verification failed: %v", err),
		}
	}

	a.logToConsole("✓ Signature verified successfully")

	return map[string]interface{}{
		"success": true,
		"valid":   true,
		"signer":  signer.String(),
		"message": string(message),
	}
}

