// Copyright 2025 HOLOGRAM Project. All rights reserved.
// TELA Deploy Helpers - Extracted from tela_service.go for maintainability
// Session 87: Code restructuring

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
)

// DOC content validation constants (from tela-cli)
const (
	MAX_DOC_CODE_SIZE      = float64(18)    // DOC SC template file size is +1.2KB with headers
	MAX_DOC_INSTALL_SIZE   = float64(19.2)  // DOC SC total file size (including docCode) should be below this
	MAX_INDEX_INSTALL_SIZE = float64(11.64) // INDEX SC file size should be below this
)

// validateDocContent validates document content before deployment to prevent DVM crashes.
// This mirrors the validation done in tela-cli to catch issues before they reach the daemon.
func (a *App) validateDocContent(content string, fileName string) error {
	// Check for non-ASCII characters - these can crash the DVM
	for i, r := range content {
		if r > unicode.MaxASCII {
			// Find the position for helpful error message
			line := 1
			col := 1
			for j := 0; j < i; j++ {
				if content[j] == '\n' {
					line++
					col = 1
				} else {
					col++
				}
			}
			return fmt.Errorf("non-ASCII character '%c' (U+%04X) found in %s at line %d, column %d - this will crash the DVM", r, r, fileName, line, col)
		}
	}
	
	// Check content size
	contentSize := getCodeSizeInKB(content)
	if contentSize > MAX_DOC_CODE_SIZE {
		return fmt.Errorf("content size %.2fKB exceeds max %.2fKB for %s", contentSize, MAX_DOC_CODE_SIZE, fileName)
	}
	
	return nil
}

// getCodeSizeInKB calculates the size of code in KB, counting newlines (from tela-cli)
func getCodeSizeInKB(code string) float64 {
	newLines := strings.Count(code, "\n")
	return float64(len([]byte(code))+newLines) / 1024
}

// getSimulatorTransferDestination returns a safe destination address for simulator transfers
// that is guaranteed to be different from the sender's address AND is registered on the blockchain.
// 
// The issue: wallet #0's address is the same as tela.GetDefaultNetworkAddress() for simulator mode.
// If we deploy from wallet #0 to wallet #0, we get "Sending to self is not supported".
// If we use an unregistered address, we get "Account Unregistered".
// 
// Solution: Use a REGISTERED wallet from the simulator wallet manager that's different from the sender.
func (a *App) getSimulatorTransferDestination(senderAddress string) string {
	// Get the default simulator destination
	_, defaultDest := tela.GetDefaultNetworkAddress()
	
	// Check if sender is the same as default destination (wallet #0)
	senderIsDefault := senderAddress != "" && len(senderAddress) >= 20 && strings.HasPrefix(defaultDest, senderAddress[:20])
	
	if !senderIsDefault {
		// Sender is not wallet #0, so we can use the default destination
		return defaultDest
	}
	
	// Sender IS wallet #0 - we need to find a different REGISTERED wallet
	a.logToConsole("[DEBUG] Sender is default simulator address, looking for alternate registered destination...")
	
	// Try to get wallet #1 from the simulator wallet manager
	if a.simulatorManager != nil && a.simulatorManager.walletManager != nil {
		wallet1 := a.simulatorManager.walletManager.GetWallet(1)
		if wallet1 != nil && wallet1.Address != "" && wallet1.Registered {
			a.logToConsole(fmt.Sprintf("[DEBUG] Using registered wallet #1 as destination: %s...", wallet1.Address[:20]))
			return wallet1.Address
		}
		
		// Wallet #1 not registered, try other wallets
		for i := 2; i < 22; i++ {
			wallet := a.simulatorManager.walletManager.GetWallet(i)
			if wallet != nil && wallet.Address != "" && wallet.Registered && wallet.Address != senderAddress {
				a.logToConsole(fmt.Sprintf("[DEBUG] Using registered wallet #%d as destination: %s...", i, wallet.Address[:20]))
				return wallet.Address
			}
		}
		
		a.logToConsole("[WARN] No registered alternate wallet found, using default (may fail)")
	} else {
		a.logToConsole("[WARN] Simulator wallet manager not available, using default destination")
	}
	
	return defaultDest
}

// BatchDeployConfig holds the configuration for a batch deployment
type BatchDeployConfig struct {
	Files       []DOCInfo `json:"files"`
	IndexName   string    `json:"indexName"`
	IndexDURL   string    `json:"indexDurl"`
	Description string    `json:"description"`
	IconURL     string    `json:"iconUrl"`
	Ringsize    uint64    `json:"ringsize"`
	Mods        string    `json:"mods"` // Comma-separated MOD tags (e.g., "vsoo,txdwd")
}

// DeployedFile represents a successfully deployed DOC
type DeployedFile struct {
	Name string `json:"name"`
	SCID string `json:"scid"`
}

// PreparedDOC represents a DOC ready for deployment
type PreparedDOC struct {
	DOC      *tela.DOC
	FileName string
	Original DOCInfo
}

// setupNetworkForDeployment configures network and wallet for TELA deployment
// NOTE: For simulator mode, we DO NOT keep websocket open - the simulator daemon
// crashes when persistent connections are maintained. Instead, each transaction
// uses a temporary connect/send/disconnect pattern.
func (a *App) setupNetworkForDeployment(wallet *walletapi.Wallet_Disk, isSimulator bool) (string, error) {
	endpoint := "127.0.0.1:20000"

	// PRE-DEPLOYMENT HEALTH CHECK: Verify daemon is alive before starting
	if isSimulator {
		a.logToConsole("[CHECK] Verifying simulator daemon is healthy...")
		if a.daemonClient != nil {
			info, err := a.daemonClient.GetInfo()
			if err != nil {
				return "", fmt.Errorf("simulator daemon is not responding - please restart simulator mode: %v", err)
			}
			if info == nil {
				return "", fmt.Errorf("simulator daemon returned empty response - please restart simulator mode")
			}
			// Log daemon status
			if height, ok := info["topoheight"].(float64); ok {
				a.logToConsole(fmt.Sprintf("[OK] Simulator daemon healthy (height: %.0f)", height))
			} else {
				a.logToConsole("[OK] Simulator daemon responding")
			}
		}
	}

	if isSimulator {
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = true
		globals.InitNetwork()
		a.logToConsole("[DEBUG] Set globals for simulator mode")

		// Store endpoint for later use but DON'T connect yet
		// Each transaction will temporarily connect/disconnect
		walletapi.Daemon_Endpoint_Active = endpoint
		a.logToConsole(fmt.Sprintf("[NET] Simulator endpoint configured: %s (temporary connect per tx)", endpoint))
	} else {
		// Get daemon endpoint for non-simulator
		if a.daemonClient != nil {
			endpoint = a.daemonClient.GetEndpoint()
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}

		// Non-simulator: Connect walletapi normally
		a.logToConsole(fmt.Sprintf("[DEBUG] Connecting walletapi to daemon: %s", endpoint))
		if err := walletapi.Connect(endpoint); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] walletapi.Connect failed: %v", err))
		}
	}

	wallet.SetDaemonAddress(endpoint)
	wallet.SetOnlineMode()

	// For simulator: Do an initial sync via temporary connection
	if isSimulator {
		a.logToConsole("[SYNC] Initial wallet sync (temporary connection)...")
		if err := walletapi.Connect(endpoint); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] Temporary connect failed: %v", err))
		} else {
			if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
				a.logToConsole(fmt.Sprintf("[WARN] Wallet sync failed: %v", err))
			} else {
				a.logToConsole("[OK] Wallet synced with daemon")
			}
			// Disconnect immediately to free daemon
			walletapi.Connected = false
			a.logToConsole("[NET] Disconnected after initial sync")
		}
	} else {
		// Non-simulator: Normal sync with persistent connection
		a.logToConsole("[SYNC] Syncing wallet with daemon to update nonce...")
		if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] Wallet sync failed: %v (continuing anyway)", err))
		} else {
			a.logToConsole("[OK] Wallet synced with daemon")
		}
	}

	return endpoint, nil
}

// prepareDOCForDeployment reads, compresses, and signs a file for DOC deployment
func (a *App) prepareDOCForDeployment(docInfo DOCInfo, wallet *walletapi.Wallet_Disk) (*PreparedDOC, error) {
	// Read file data
	data, err := os.ReadFile(docInfo.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Validate docType is accepted
	if !tela.IsAcceptedLanguage(docInfo.DocType) {
		return nil, fmt.Errorf("invalid docType %q for %s - must be one of: TELA-HTML-1, TELA-JS-1, TELA-CSS-1, TELA-JSON-1, TELA-MD-1, TELA-GO-1, TELA-STATIC-1", docInfo.DocType, docInfo.Name)
	}

	// Handle compression if requested
	docCode := string(data)
	fileName := docInfo.Name

	if docInfo.Compressed {
		ext := filepath.Ext(fileName)
		if !tela.IsCompressedExt(ext) {
			compressed, err := tela.Compress(data, tela.COMPRESSION_GZIP)
			if err != nil {
				return nil, fmt.Errorf("compression failed: %w", err)
			}
			docCode = compressed
			fileName = fileName + tela.COMPRESSION_GZIP

			// Log compression results
			originalSize := len(data)
			compressedSize := len(compressed)
			savings := 100 - (float64(compressedSize) / float64(originalSize) * 100)
			a.logToConsole(fmt.Sprintf("[COMPRESS] %s: %d → %d bytes (%.1f%% smaller)",
				docInfo.Name, originalSize, compressedSize, savings))
		}
	}

	// CRITICAL: Validate content BEFORE signing and deployment
	// This prevents DVM crashes from non-ASCII characters or oversized content
	if err := a.validateDocContent(docCode, docInfo.Name); err != nil {
		return nil, fmt.Errorf("content validation failed: %w", err)
	}

	// Sign the (possibly compressed) file content
	signature := wallet.SignData([]byte(docCode))
	if signature == nil || len(signature) == 0 {
		return nil, fmt.Errorf("wallet.SignData returned nil")
	}

	a.logToConsole(fmt.Sprintf("[DEBUG] %s signature length: %d bytes", docInfo.Name, len(signature)))

	_, checkC, checkS, err := tela.ParseSignature(signature)
	if err != nil {
		return nil, fmt.Errorf("ParseSignature failed: %w", err)
	}

	// Pad checkC and checkS to 64 chars
	checkC = padHex64(checkC)
	checkS = padHex64(checkS)

	a.logToConsole(fmt.Sprintf("[DEBUG] %s checkC: '%s' (len=%d)", docInfo.Name, checkC, len(checkC)))
	a.logToConsole(fmt.Sprintf("[DEBUG] %s checkS: '%s' (len=%d)", docInfo.Name, checkS, len(checkS)))

	// Build DOC
	doc := &tela.DOC{
		DocType: docInfo.DocType,
		Code:    docCode,
		SubDir:  docInfo.SubDir,
		Headers: tela.Headers{
			NameHdr:  fileName,
			DescrHdr: docInfo.Description,
		},
		Signature: tela.Signature{
			CheckC: checkC,
			CheckS: checkS,
		},
	}

	if docInfo.Compressed {
		doc.Compression = tela.COMPRESSION_GZIP
	}

	return &PreparedDOC{
		DOC:      doc,
		FileName: fileName,
		Original: docInfo,
	}, nil
}

// disconnectWalletAPI properly disconnects the walletapi and ensures no lingering connections
// Setting walletapi.Connected = false is not enough - we need to ensure the websocket is actually closed
func (a *App) disconnectWalletAPI() {
	walletapi.Connected = false
	// Give a brief moment for any pending operations to complete
	time.Sleep(10 * time.Millisecond)
}

// SimulatorGasFee is the gas fee per transaction in simulator mode
// Each DOC deployment costs this amount from the wallet balance
const SimulatorGasFee = uint64(100000)

// CheckBalanceForBatchDeployment checks if the wallet has sufficient balance
// for deploying the specified number of files (DOCs + 1 INDEX)
func (a *App) CheckBalanceForBatchDeployment(wallet *walletapi.Wallet_Disk, fileCount int, isSimulator bool) (bool, uint64, uint64, error) {
	if !isSimulator {
		// Non-simulator mode doesn't need pre-check (uses real gas estimation)
		return true, 0, 0, nil
	}

	// Calculate required balance: (fileCount DOCs + 1 INDEX) * gas fee
	requiredBalance := uint64(fileCount+1) * SimulatorGasFee

	// Get current balance via temporary connection
	endpoint := walletapi.Daemon_Endpoint_Active
	if endpoint == "" || endpoint == " " {
		return false, 0, requiredBalance, fmt.Errorf("daemon endpoint not configured")
	}

	// Temporarily connect to check balance
	if err := walletapi.Connect(endpoint); err != nil {
		return false, 0, requiredBalance, fmt.Errorf("failed to connect to daemon: %v", err)
	}

	// Sync wallet to get current balance
	if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
		walletapi.Connected = false
		return false, 0, requiredBalance, fmt.Errorf("failed to sync wallet: %v", err)
	}

	mature, _ := wallet.Get_Balance()

	// Disconnect
	walletapi.Connected = false

	if mature < requiredBalance {
		return false, mature, requiredBalance, nil
	}

	return true, mature, requiredBalance, nil
}

// deployDOC installs a single prepared DOC and returns the SCID
// For simulator mode, uses retry logic similar to tela-cli tests
func (a *App) deployDOC(wallet *walletapi.Wallet_Disk, prepared *PreparedDOC, ringsize uint64, isSimulator bool) (string, error) {
	// Pre-flight check for simulator mode
	if isSimulator {
		a.logToConsole(fmt.Sprintf("[DEBUG] Pre-flight: Daemon_Endpoint_Active = '%s'", walletapi.Daemon_Endpoint_Active))
		if walletapi.Daemon_Endpoint_Active == "" || walletapi.Daemon_Endpoint_Active == " " {
			return "", fmt.Errorf("daemon endpoint is invalid - please restart simulator mode")
		}
	}

	a.logToConsole(fmt.Sprintf("[DEBUG] Calling tela.Installer for %s...", prepared.Original.Name))

	var txid string
	
	if isSimulator {
		// SIMULATOR MODE: Use TEMPORARY connect/disconnect for each transaction
		// The simulator daemon CRASHES when websocket connections are kept open persistently.
		// Pattern: Connect → Sync → Build → Send → Disconnect → Wait for block (HTTP)
		
		endpoint := walletapi.Daemon_Endpoint_Active
		if endpoint == "" || endpoint == " " {
			return "", fmt.Errorf("daemon endpoint is invalid - please restart simulator mode")
		}
		
		// Build install arguments (no connection needed for this)
		args, err := tela.NewInstallArgs(prepared.DOC)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] Failed to create install args for %s: %v", prepared.Original.Name, err))
			return "", err
		}
		
		// Create transfer with safe destination (avoids "Sending to self" error)
		senderAddr := wallet.GetAddress().String()
		destAddr := a.getSimulatorTransferDestination(senderAddr)
		transfers := []rpc.Transfer{{Destination: destAddr, Amount: 0}}
		
		// RETRY LOOP: Similar to tela-cli tests, retry up to 3 times
		const maxRetries = 3
		var lastErr error
		
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if attempt > 1 {
				a.logToConsole(fmt.Sprintf("[RETRY] Attempt %d/%d for %s...", attempt, maxRetries, prepared.Original.Name))
				// Wait for a new block before retrying (like tela-cli tests do)
				if err := a.waitForNewBlockWithHealthCheck(15 * time.Second); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Block wait failed: %v", err))
				}
			}
			
			// TEMPORARY CONNECTION: Connect, sync, build, send, disconnect
			a.logToConsole(fmt.Sprintf("[NET] Temporary connect for %s (attempt %d)...", prepared.Original.Name, attempt))
			if err := walletapi.Connect(endpoint); err != nil {
				lastErr = fmt.Errorf("failed to connect to simulator daemon: %v", err)
				a.disconnectWalletAPI()
				continue
			}
			
			// Sync wallet to get correct nonce
			if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
				a.logToConsole(fmt.Sprintf("[WARN] Pre-tx sync failed: %v", err))
			}
			time.Sleep(100 * time.Millisecond) // Brief settle time (increased from 50ms)
			
			// Check wallet balance
			mature, locked := wallet.Get_Balance()
			a.logToConsole(fmt.Sprintf("[DEBUG] Wallet balance: mature=%d, locked=%d", mature, locked))
			if mature == 0 && locked == 0 {
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("wallet has zero balance - registration may not have completed")
				continue
			}
			
			// Get gas estimate from daemon - this validates the SC code before building the transaction
			// This is critical: tela-cli uses GetGasEstimate which validates the SC on the daemon
			// If the SC code is invalid, this will return an error BEFORE we crash the daemon
			gasFees, gasErr := tela.GetGasEstimate(wallet, ringsize, transfers, args)
			if gasErr != nil {
				a.logToConsole(fmt.Sprintf("[ERR] GetGasEstimate failed (attempt %d): %v", attempt, gasErr))
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("gas estimate error (SC validation failed): %v", gasErr)
				continue
			}
			a.logToConsole(fmt.Sprintf("[DEBUG] Gas estimate: %d", gasFees))
			
			// Build transaction
			tx, buildErr := wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
			if buildErr != nil {
				a.logToConsole(fmt.Sprintf("[ERR] TransferPayload0 failed (attempt %d): %v", attempt, buildErr))
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("transfer build error: %v", buildErr)
				continue
			}
			
			if tx == nil {
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("transaction is nil after build")
				continue
			}
			
			// Send transaction
			if err := wallet.SendTransaction(tx); err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] SendTransaction failed (attempt %d): %v", attempt, err))
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("transaction dispatch error: %v", err)
				continue
			}
			
			txid = tx.GetHash().String()
			a.logToConsole(fmt.Sprintf("[OK] Transaction sent: %s", txid))
			
			// CRITICAL: Disconnect IMMEDIATELY after send to free daemon
			a.disconnectWalletAPI()
			a.logToConsole("[NET] Disconnected after send (daemon freed)")
			
			// Wait for block confirmation via HTTP polling (no websocket needed)
			a.logToConsole("[WAIT] Waiting for block confirmation (HTTP polling)...")
			if err := a.waitForNewBlockWithHealthCheck(30 * time.Second); err != nil {
				// Check if daemon is still alive
				if a.daemonClient != nil {
					if _, rpcErr := a.daemonClient.GetInfo(); rpcErr != nil {
						return "", fmt.Errorf("daemon connection lost while waiting for block confirmation: %v", err)
					}
				}
				a.logToConsole(fmt.Sprintf("[WARN] Block wait issue: %v. Continuing...", err))
			} else {
				a.logToConsole("[OK] Block confirmed")
			}
			
			// SUCCESS! Exit retry loop
			lastErr = nil
			break
		}
		
		if lastErr != nil {
			return "", fmt.Errorf("failed after %d attempts: %v", maxRetries, lastErr)
		}
		
	} else {
		// NON-SIMULATOR: Use standard tela.Installer()
		var err error
		txid, err = tela.Installer(wallet, ringsize, prepared.DOC)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] tela.Installer failed for %s: %v", prepared.Original.Name, err))
			a.logToConsole(fmt.Sprintf("[DEBUG] Current Daemon_Endpoint_Active: '%s'", walletapi.Daemon_Endpoint_Active))
			return "", err
		}
	}

	a.logToConsole(fmt.Sprintf("[OK] tela.Installer succeeded for %s: SCID=%s", prepared.Original.Name, txid))
	return txid, nil
}

// createINDEX creates a TELA INDEX from deployed DOC SCIDs
// For simulator mode, uses retry logic similar to tela-cli tests
func (a *App) createINDEX(wallet *walletapi.Wallet_Disk, config *BatchDeployConfig, docScids []string, ringsize uint64, isSimulator bool) (string, error) {
	// Log MODs if any
	modsStr := "none"
	if config.Mods != "" {
		modsStr = config.Mods
	}
	a.logToConsole(fmt.Sprintf("[INDEX] [TELA] Creating INDEX with %d DOCs, mods=%s", len(docScids), modsStr))

	index := tela.INDEX{
		DURL: config.IndexDURL,
		DOCs: docScids,
		Mods: config.Mods, // MOD tags (e.g., "vsoo,txdwd")
		Headers: tela.Headers{
			NameHdr:  config.IndexName,
			DescrHdr: config.Description,
			IconHdr:  config.IconURL,
		},
	}

	var txid string
	
	if isSimulator {
		// SIMULATOR MODE: Use TEMPORARY connect/disconnect with retry logic
		// The simulator daemon CRASHES with persistent websocket connections
		
		endpoint := walletapi.Daemon_Endpoint_Active
		if endpoint == "" || endpoint == " " {
			return "", fmt.Errorf("daemon endpoint is invalid - please restart simulator mode")
		}
		
		// Build install arguments for INDEX (no connection needed)
		args, err := tela.NewInstallArgs(&index)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] Failed to create INDEX install args: %v", err))
			return "", err
		}
		
		// Create transfer with safe destination (avoids "Sending to self" error)
		senderAddr := wallet.GetAddress().String()
		destAddr := a.getSimulatorTransferDestination(senderAddr)
		transfers := []rpc.Transfer{{Destination: destAddr, Amount: 0}}
		
		// RETRY LOOP: Similar to tela-cli tests, retry up to 3 times
		const maxRetries = 3
		var lastErr error
		
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if attempt > 1 {
				a.logToConsole(fmt.Sprintf("[RETRY] INDEX attempt %d/%d...", attempt, maxRetries))
				// Wait for a new block before retrying
				if err := a.waitForNewBlockWithHealthCheck(15 * time.Second); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Block wait failed: %v", err))
				}
			}
			
			// TEMPORARY CONNECTION: Connect, sync, build, send, disconnect
			a.logToConsole(fmt.Sprintf("[NET] Temporary connect for INDEX (attempt %d)...", attempt))
			if err := walletapi.Connect(endpoint); err != nil {
				lastErr = fmt.Errorf("failed to connect for INDEX: %v", err)
				a.disconnectWalletAPI()
				continue
			}
			
			// Sync wallet to get correct nonce
			if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
				a.logToConsole(fmt.Sprintf("[WARN] Pre-INDEX sync failed: %v", err))
			}
			time.Sleep(100 * time.Millisecond) // Settle time
			
			// Check wallet balance
			mature, locked := wallet.Get_Balance()
			a.logToConsole(fmt.Sprintf("[DEBUG] Wallet balance before INDEX: mature=%d, locked=%d", mature, locked))
			
			if mature == 0 && locked == 0 {
				a.disconnectWalletAPI()
				a.logToConsole("[WARN] Zero balance - waiting for mining reward...")
				for balanceWait := 0; balanceWait < 3; balanceWait++ {
					if err := a.waitForNewBlockWithHealthCheck(15 * time.Second); err != nil {
						a.logToConsole(fmt.Sprintf("[WARN] Block wait failed: %v", err))
					}
					// Reconnect briefly to check balance
					if err := walletapi.Connect(endpoint); err == nil {
						wallet.Sync_Wallet_Memory_With_Daemon()
						mature, locked = wallet.Get_Balance()
						a.disconnectWalletAPI()
					}
					a.logToConsole(fmt.Sprintf("[DEBUG] Balance after block %d: mature=%d, locked=%d", balanceWait+1, mature, locked))
					if mature > 0 {
						break
					}
				}
				// Final reconnect for transaction
				if err := walletapi.Connect(endpoint); err != nil {
					lastErr = fmt.Errorf("reconnect failed after balance wait: %v", err)
					continue
				}
				wallet.Sync_Wallet_Memory_With_Daemon()
				time.Sleep(100 * time.Millisecond)
			}
			
			// Get gas estimate from daemon - validates the SC code before building
			gasFees, gasErr := tela.GetGasEstimate(wallet, ringsize, transfers, args)
			if gasErr != nil {
				a.logToConsole(fmt.Sprintf("[ERR] GetGasEstimate failed for INDEX (attempt %d): %v", attempt, gasErr))
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("gas estimate error (INDEX validation failed): %v", gasErr)
				continue
			}
			a.logToConsole(fmt.Sprintf("[DEBUG] INDEX gas estimate: %d", gasFees))
			
			// Build transaction
			tx, buildErr := wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
			if buildErr != nil {
				a.logToConsole(fmt.Sprintf("[ERR] TransferPayload0 failed for INDEX (attempt %d): %v", attempt, buildErr))
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("INDEX transfer build error: %v", buildErr)
				continue
			}
			
			if tx == nil {
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("INDEX transaction is nil")
				continue
			}
			
			// Send transaction
			if err := wallet.SendTransaction(tx); err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] SendTransaction failed for INDEX (attempt %d): %v", attempt, err))
				a.disconnectWalletAPI()
				lastErr = fmt.Errorf("INDEX transaction dispatch error: %v", err)
				continue
			}
			
			txid = tx.GetHash().String()
			a.logToConsole(fmt.Sprintf("[OK] INDEX transaction sent: %s", txid))
			
			// CRITICAL: Disconnect IMMEDIATELY
			a.disconnectWalletAPI()
			a.logToConsole("[NET] Disconnected (batch complete)")
			
			// SUCCESS! Exit retry loop
			lastErr = nil
			break
		}
		
		if lastErr != nil {
			return "", fmt.Errorf("INDEX creation failed after %d attempts: %v", maxRetries, lastErr)
		}
		
	} else {
		// NON-SIMULATOR: Use standard tela.Installer()
		var err error
		txid, err = tela.Installer(wallet, ringsize, &index)
		if err != nil {
			return "", fmt.Errorf("INDEX creation failed: %w", err)
		}
	}

	a.logToConsole(fmt.Sprintf("[OK] INDEX created: SCID=%s, dURL=%s", txid, config.IndexDURL))
	return txid, nil
}

// padHex64 pads a hex string to 64 characters with leading zeros
func padHex64(s string) string {
	if len(s) < 64 {
		return strings.Repeat("0", 64-len(s)) + s
	}
	return s
}

// waitForNewBlockWithHealthCheck waits for a new block using HTTP polling (safer than websocket subscriptions)
// This prevents hanging indefinitely if the daemon crashes and detects crashes early
// IMPORTANT: Uses HTTP RPC polling instead of walletapi.WaitNewHeightBlock() which can cause
// websocket issues with the simulator daemon
func (a *App) waitForNewBlockWithHealthCheck(timeout time.Duration) error {
	// Get current height
	startHeight := int64(0)
	if a.daemonClient != nil {
		info, err := a.daemonClient.GetInfo()
		if err != nil {
			return fmt.Errorf("failed to get initial height: %v", err)
		}
		// Extract topoheight from map
		if th, ok := info["topoheight"].(float64); ok {
			startHeight = int64(th)
		} else if th, ok := info["topoheight"].(int64); ok {
			startHeight = th
		}
		a.logToConsole(fmt.Sprintf("[DEBUG] Current height: %d, waiting for new block...", startHeight))
	} else {
		return fmt.Errorf("daemon client not available")
	}
	
	// Start with faster polling to detect daemon crashes quickly
	// Then slow down after confirming daemon is alive
	fastPollInterval := 200 * time.Millisecond  // Fast initial polls
	normalPollInterval := 500 * time.Millisecond // Normal interval
	timeoutTime := time.Now().Add(timeout)
	pollCount := 0
	consecutiveFailures := 0
	
	for time.Now().Before(timeoutTime) {
		pollCount++
		// Use fast polling for first 5 polls (1 second), then normal
		interval := normalPollInterval
		if pollCount <= 5 {
			interval = fastPollInterval
		}
		time.Sleep(interval)
		
		// Check daemon via HTTP RPC (not websocket)
		if a.daemonClient != nil {
			info, err := a.daemonClient.GetInfo()
			if err != nil {
				consecutiveFailures++
				// If we get 2 consecutive failures, daemon is likely crashed
				if consecutiveFailures >= 2 {
					return fmt.Errorf("daemon crashed - please restart simulator mode (error: %v)", err)
				}
				continue
			}
			consecutiveFailures = 0 // Reset on success
			
			// Extract topoheight from map
			var currentHeight int64
			if th, ok := info["topoheight"].(float64); ok {
				currentHeight = int64(th)
			} else if th, ok := info["topoheight"].(int64); ok {
				currentHeight = th
			}
			
			if currentHeight > startHeight {
				a.logToConsole(fmt.Sprintf("[OK] New block detected: %d → %d", startHeight, currentHeight))
				return nil // New block found!
			}
		}
	}
	
	return fmt.Errorf("timeout after %v waiting for new block", timeout)
}

