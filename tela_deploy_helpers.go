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

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
)

// BatchDeployConfig holds the configuration for a batch deployment
type BatchDeployConfig struct {
	Files       []DOCInfo `json:"files"`
	IndexName   string    `json:"indexName"`
	IndexDURL   string    `json:"indexDurl"`
	Description string    `json:"description"`
	IconURL     string    `json:"iconUrl"`
	Ringsize    uint64    `json:"ringsize"`
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

// deployDOC installs a single prepared DOC and returns the SCID
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
		// This pattern mirrors the successful registration flow:
		// 1. Connect websocket
		// 2. Build and send transaction
		// 3. Disconnect IMMEDIATELY after send
		// 4. Wait for block confirmation via HTTP polling (no websocket needed)
		
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
		
		// Create default transfer (required for install)
		_, defaultDest := tela.GetDefaultNetworkAddress()
		transfers := []rpc.Transfer{{Destination: defaultDest, Amount: 0}}
		
		// Use a reasonable default gas fee for simulator (it's free anyway)
		gasFees := uint64(100000)
		
		// TEMPORARY CONNECTION: Connect, sync, build, send, disconnect
		a.logToConsole(fmt.Sprintf("[NET] Temporary connect for %s...", prepared.Original.Name))
		if err := walletapi.Connect(endpoint); err != nil {
			return "", fmt.Errorf("failed to connect to simulator daemon: %v", err)
		}
		
		// Sync wallet to get correct nonce
		if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] Pre-tx sync failed: %v", err))
		}
		time.Sleep(50 * time.Millisecond) // Brief settle time
		
		// Check wallet balance
		mature, locked := wallet.Get_Balance()
		a.logToConsole(fmt.Sprintf("[DEBUG] Wallet balance: mature=%d, locked=%d", mature, locked))
		if mature == 0 && locked == 0 {
			walletapi.Connected = false // Disconnect before error
			return "", fmt.Errorf("wallet has zero balance - registration may not have completed")
		}
		
		// Build transaction
		tx, buildErr := wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
		if buildErr != nil {
			errStr := buildErr.Error()
			a.logToConsole(fmt.Sprintf("[ERR] TransferPayload0 failed: %v", buildErr))
			
			// Retry once after sync and block wait
			if strings.Contains(errStr, "could not be built") || strings.Contains(errStr, "nonce") {
				a.logToConsole("[WARN] Retrying after sync and block wait...")
				walletapi.Connected = false // Disconnect
				
				// Wait for block via HTTP
				if err := a.waitForNewBlockWithHealthCheck(20 * time.Second); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Block wait failed: %v", err))
				}
				
				// Reconnect and retry
				if err := walletapi.Connect(endpoint); err != nil {
					return "", fmt.Errorf("reconnect failed: %v", err)
				}
				if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Retry sync failed: %v", err))
				}
				time.Sleep(100 * time.Millisecond)
				
				tx, buildErr = wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
				if buildErr != nil {
					walletapi.Connected = false
					return "", fmt.Errorf("transfer build failed on retry: %v", buildErr)
				}
			} else {
				walletapi.Connected = false
				return "", fmt.Errorf("transfer build error: %v", buildErr)
			}
		}
		
		if tx == nil {
			walletapi.Connected = false
			return "", fmt.Errorf("transaction is nil after build")
		}
		
		// Send transaction
		if err := wallet.SendTransaction(tx); err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] SendTransaction failed: %v", err))
			walletapi.Connected = false
			return "", fmt.Errorf("transaction dispatch error: %v", err)
		}
		
		txid = tx.GetHash().String()
		a.logToConsole(fmt.Sprintf("[OK] Transaction sent: %s", txid))
		
		// CRITICAL: Disconnect IMMEDIATELY after send to free daemon
		walletapi.Connected = false
		a.logToConsole("[NET] Disconnected after send (daemon freed)")
		
		// Wait for block confirmation via HTTP polling (no websocket needed)
		a.logToConsole("[WAIT] Waiting for block confirmation (HTTP polling)...")
		if err := a.waitForNewBlockWithHealthCheck(30 * time.Second); err != nil {
			// Check if daemon is still alive
			if a.daemonClient != nil {
				if _, rpcErr := a.daemonClient.GetInfo(); rpcErr != nil {
					return "", fmt.Errorf("daemon crashed while waiting for block: %v", rpcErr)
				}
			}
			a.logToConsole(fmt.Sprintf("[WARN] Block wait issue: %v. Continuing...", err))
		} else {
			a.logToConsole("[OK] Block confirmed")
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
func (a *App) createINDEX(wallet *walletapi.Wallet_Disk, config *BatchDeployConfig, docScids []string, ringsize uint64, isSimulator bool) (string, error) {
	a.logToConsole(fmt.Sprintf("[INDEX] [TELA] Creating INDEX with %d DOCs...", len(docScids)))

	index := tela.INDEX{
		DURL: config.IndexDURL,
		DOCs: docScids,
		Headers: tela.Headers{
			NameHdr:  config.IndexName,
			DescrHdr: config.Description,
			IconHdr:  config.IconURL,
		},
	}

	var txid string
	
	if isSimulator {
		// SIMULATOR MODE: Use TEMPORARY connect/disconnect (same as deployDOC)
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
		
		// Create default transfer
		_, defaultDest := tela.GetDefaultNetworkAddress()
		transfers := []rpc.Transfer{{Destination: defaultDest, Amount: 0}}
		gasFees := uint64(100000)
		
		// TEMPORARY CONNECTION: Connect, sync, build, send, disconnect
		a.logToConsole("[NET] Temporary connect for INDEX...")
		if err := walletapi.Connect(endpoint); err != nil {
			return "", fmt.Errorf("failed to connect for INDEX: %v", err)
		}
		
		// Sync wallet to get correct nonce
		if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] Pre-INDEX sync failed: %v", err))
		}
		time.Sleep(50 * time.Millisecond)
		
		// Check wallet balance
		mature, locked := wallet.Get_Balance()
		a.logToConsole(fmt.Sprintf("[DEBUG] Wallet balance before INDEX: mature=%d, locked=%d", mature, locked))
		
		if mature == 0 && locked == 0 {
			walletapi.Connected = false // Disconnect before retry loop
			a.logToConsole("[WARN] Zero balance - waiting for mining reward...")
			for attempt := 0; attempt < 3; attempt++ {
				if err := a.waitForNewBlockWithHealthCheck(15 * time.Second); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Block wait failed: %v", err))
				}
				// Reconnect briefly to check balance
				if err := walletapi.Connect(endpoint); err == nil {
					wallet.Sync_Wallet_Memory_With_Daemon()
					mature, locked = wallet.Get_Balance()
					walletapi.Connected = false
				}
				a.logToConsole(fmt.Sprintf("[DEBUG] Balance after block %d: mature=%d, locked=%d", attempt+1, mature, locked))
				if mature > 0 {
					break
				}
			}
			// Final reconnect for transaction
			if err := walletapi.Connect(endpoint); err != nil {
				return "", fmt.Errorf("reconnect failed after balance wait: %v", err)
			}
			wallet.Sync_Wallet_Memory_With_Daemon()
		}
		
		// Build transaction
		tx, buildErr := wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
		if buildErr != nil {
			errStr := buildErr.Error()
			a.logToConsole(fmt.Sprintf("[ERR] TransferPayload0 failed for INDEX: %v", buildErr))
			
			// Retry once
			if strings.Contains(errStr, "could not be built") || strings.Contains(errStr, "nonce") {
				a.logToConsole("[WARN] Retrying INDEX build after sync and block wait...")
				walletapi.Connected = false
				
				if err := a.waitForNewBlockWithHealthCheck(20 * time.Second); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Block wait failed: %v", err))
				}
				
				if err := walletapi.Connect(endpoint); err != nil {
					return "", fmt.Errorf("reconnect failed: %v", err)
				}
				wallet.Sync_Wallet_Memory_With_Daemon()
				time.Sleep(100 * time.Millisecond)
				
				tx, buildErr = wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
				if buildErr != nil {
					walletapi.Connected = false
					return "", fmt.Errorf("INDEX transfer build failed on retry: %v", buildErr)
				}
			} else {
				walletapi.Connected = false
				return "", fmt.Errorf("INDEX transfer build error: %v", buildErr)
			}
		}
		
		if tx == nil {
			walletapi.Connected = false
			return "", fmt.Errorf("INDEX transaction is nil")
		}
		
		// Send transaction
		if err := wallet.SendTransaction(tx); err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] SendTransaction failed for INDEX: %v", err))
			walletapi.Connected = false
			return "", fmt.Errorf("INDEX transaction dispatch error: %v", err)
		}
		
		txid = tx.GetHash().String()
		a.logToConsole(fmt.Sprintf("[OK] INDEX transaction sent: %s", txid))
		
		// CRITICAL: Disconnect IMMEDIATELY
		walletapi.Connected = false
		a.logToConsole("[NET] Disconnected (batch complete)")
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
	
	pollInterval := 500 * time.Millisecond // Poll every 500ms (simulator auto-mines fast)
	timeoutTime := time.Now().Add(timeout)
	
	for time.Now().Before(timeoutTime) {
		time.Sleep(pollInterval)
		
		// Check daemon via HTTP RPC (not websocket)
		if a.daemonClient != nil {
			info, err := a.daemonClient.GetInfo()
			if err != nil {
				// Daemon not responding - might have crashed
				return fmt.Errorf("daemon RPC not responding (may have crashed): %v", err)
			}
			
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

