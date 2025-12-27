// Copyright 2025 HOLOGRAM Project. All rights reserved.
// TELA Deploy Helpers - Extracted from tela_service.go for maintainability
// Session 87: Code restructuring

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
func (a *App) setupNetworkForDeployment(wallet *walletapi.Wallet_Disk, isSimulator bool) (string, error) {
	endpoint := "127.0.0.1:20000"

	if isSimulator {
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = true
		globals.InitNetwork()
		a.logToConsole("[DEBUG] Set globals for simulator mode")

		// CRITICAL: Connect walletapi for transaction building
		a.logToConsole(fmt.Sprintf("[NET] Connecting walletapi to simulator daemon: %s", endpoint))
		if err := walletapi.Connect(endpoint); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] walletapi.Connect failed: %v (continuing anyway)", err))
		} else {
			a.logToConsole("[OK] walletapi connected to simulator daemon")
		}
	} else {
		// Get daemon endpoint for non-simulator
		if a.daemonClient != nil {
			endpoint = a.daemonClient.GetEndpoint()
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}

		// Connect walletapi for transaction building
		a.logToConsole(fmt.Sprintf("[DEBUG] Connecting walletapi to daemon: %s", endpoint))
		if err := walletapi.Connect(endpoint); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] walletapi.Connect failed: %v", err))
		}
	}

	wallet.SetDaemonAddress(endpoint)
	wallet.SetOnlineMode()

	// CRITICAL: Sync wallet with daemon to get correct nonce
	// Without this, the wallet's local nonce may be stale (e.g., 24 when daemon expects 71)
	// which causes "nonce verification failed" errors
	a.logToConsole("[SYNC] Syncing wallet with daemon to update nonce...")
	if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
		a.logToConsole(fmt.Sprintf("[WARN] Wallet sync failed: %v (continuing anyway)", err))
	} else {
		a.logToConsole("[OK] Wallet synced with daemon")
	}

	// CRITICAL for SIMULATOR: Keep the websocket OPEN for batch operations
	// We bypass tela.Installer() in simulator mode (which would create its own websocket)
	// and use direct TransferPayload0/SendTransaction calls instead.
	// This allows us to maintain ONE persistent connection for all transactions.
	// The websocket should be closed after all transactions are complete.
	if isSimulator {
		// Keep the flags set
		walletapi.Daemon_Endpoint_Active = endpoint
		a.logToConsole("[NET] Keeping walletapi websocket open for batch transactions")
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
		// SIMULATOR MODE: Bypass tela.Installer() to avoid websocket conflicts
		// The issue is:
		// - tela.GetGasEstimate() creates its own websocket (in tela.client.WS)
		// - wallet.TransferPayload0() needs walletapi's websocket (walletapi.rpc_client.WS)
		// - These are DIFFERENT websockets, and simulator can only handle ONE
		// 
		// Solution: In simulator mode (where gas is FREE), we:
		// 1. Use the EXISTING persistent websocket (opened by setupNetworkForDeployment)
		// 2. Build install args using tela.NewInstallArgs()
		// 3. Use a fixed gas value (skip GetGasEstimate entirely)
		// 4. Call TransferPayload0 and SendTransaction directly
		// 5. Sync wallet after each tx to update nonce
		// 6. Keep websocket open for next transaction
		
		// Build install arguments
		args, err := tela.NewInstallArgs(prepared.DOC)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] Failed to create install args for %s: %v", prepared.Original.Name, err))
			return "", err
		}
		
		// Use existing websocket connection (opened by setupNetworkForDeployment)
		// Only reconnect if not connected
		if !walletapi.Connected {
			endpoint := walletapi.Daemon_Endpoint_Active
			a.logToConsole(fmt.Sprintf("[NET] Reconnecting walletapi: %s", endpoint))
			if err := walletapi.Connect(endpoint); err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] walletapi.Connect failed: %v", err))
				return "", fmt.Errorf("failed to connect to simulator daemon: %v", err)
			}
		}
		
		// Create default transfer (required for install)
		_, defaultDest := tela.GetDefaultNetworkAddress()
		transfers := []rpc.Transfer{{Destination: defaultDest, Amount: 0}}
		
		// Use a reasonable default gas fee for simulator (it's free anyway)
		gasFees := uint64(100000)
		
		// Build transaction
		tx, err := wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] TransferPayload0 failed for %s: %v", prepared.Original.Name, err))
			return "", fmt.Errorf("transfer build error: %v", err)
		}
		
		// Send transaction
		if err := wallet.SendTransaction(tx); err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] SendTransaction failed for %s: %v", prepared.Original.Name, err))
			return "", fmt.Errorf("transaction dispatch error: %v", err)
		}
		
		txid = tx.GetHash().String()
		
		// CRITICAL: Sync wallet to update nonce for next transaction
		// This must happen WHILE websocket is still connected
		if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] Post-tx wallet sync failed for %s: %v", prepared.Original.Name, err))
		}
		
		// Keep websocket open for next transaction (don't close)
		
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
		// SIMULATOR MODE: Same approach as deployDOC
		// Use existing websocket connection, close at the end (this is the last operation)
		
		// Build install arguments for INDEX
		args, err := tela.NewInstallArgs(&index)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] Failed to create INDEX install args: %v", err))
			return "", err
		}
		
		// Use existing websocket connection (opened by setupNetworkForDeployment)
		// Only reconnect if not connected
		if !walletapi.Connected {
			endpoint := walletapi.Daemon_Endpoint_Active
			a.logToConsole(fmt.Sprintf("[NET] Reconnecting walletapi for INDEX: %s", endpoint))
			if err := walletapi.Connect(endpoint); err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] walletapi.Connect failed for INDEX: %v", err))
				return "", fmt.Errorf("failed to connect to simulator daemon: %v", err)
			}
		}
		
		// Create default transfer
		_, defaultDest := tela.GetDefaultNetworkAddress()
		transfers := []rpc.Transfer{{Destination: defaultDest, Amount: 0}}
		
		// Use default gas fee (free in simulator)
		gasFees := uint64(100000)
		
		// Build transaction
		tx, err := wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] TransferPayload0 failed for INDEX: %v", err))
			return "", fmt.Errorf("INDEX transfer build error: %v", err)
		}
		
		// Send transaction
		if err := wallet.SendTransaction(tx); err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] SendTransaction failed for INDEX: %v", err))
			return "", fmt.Errorf("INDEX transaction dispatch error: %v", err)
		}
		
		txid = tx.GetHash().String()
		
		// Close websocket - this is the LAST operation, so cleanup now
		a.logToConsole("[NET] Closing walletapi websocket (batch complete)")
		if rpcClient := walletapi.GetRPCClient(); rpcClient != nil && rpcClient.WS != nil {
			rpcClient.WS.Close()
		}
		walletapi.Connected = false
		
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

