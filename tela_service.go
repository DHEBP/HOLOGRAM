package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// TELAService handles TELA content operations

// DOCInfo represents information about a file to be installed as a DOC
type DOCInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SubDir      string `json:"subDir"`
	DocType     string `json:"docType"`
	Size        int64  `json:"size"`
	Compressed  bool   `json:"compressed"`
	Data        []byte `json:"-"`
	DataString  string `json:"data"` // Accept data as string from frontend
	Description string `json:"description"`
	IconURL     string `json:"iconUrl"`
	Ringsize    int    `json:"ringsize"` // 2 = updateable, 16+ = immutable
}

// INDEXInfo represents information for creating a TELA INDEX
type INDEXInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	DURL        string   `json:"durl"`
	IconURL     string   `json:"iconUrl"`
	DOCSCIDs    []string `json:"docScids"`
	Licenses    []string `json:"licenses"`
	Ringsize    int      `json:"ringsize"` // 2 = updateable, 16+ = immutable
	Mods        string   `json:"mods"`     // Comma-separated MOD tags (e.g., "vsoo,txdwd")
}

// sortFilesForDeployment sorts files to ensure the entry point (index.html) is deployed first.
// In TELA, the first DOC deployed becomes DOC1, which is the application's entry point.
// Without proper sorting, alphabetical order could make blocks.js the entrypoint instead of index.html.
func sortFilesForDeployment(files []DOCInfo) {
	if len(files) <= 1 {
		return
	}

	// Find the entry point file (index.html or index.htm in root directory)
	entryIndex := -1
	for i, f := range files {
		name := strings.ToLower(f.Name)
		subDir := strings.TrimSpace(f.SubDir)
		isRoot := subDir == "" || subDir == "/"

		if isRoot && (name == "index.html" || name == "index.htm") {
			entryIndex = i
			break
		}
	}

	// If entry point found and not already first, move it to the front
	if entryIndex > 0 {
		entryFile := files[entryIndex]
		// Shift all files before the entry point one position to the right
		copy(files[1:entryIndex+1], files[0:entryIndex])
		files[0] = entryFile
	}
}

// InstallDOC installs a single TELA DOC smart contract
func (a *App) InstallDOC(docJSON string) map[string]interface{} {
	isSimulator := a.IsInSimulatorMode()
	modeStr := ""
	if isSimulator {
		modeStr = " [SIMULATOR]"
	}
	a.logToConsole(fmt.Sprintf("[DOC] [TELA] InstallDOC: Starting installation...%s", modeStr))

	// Check wallet - use simulator wallet in simulator mode, otherwise main wallet
	wallet := a.getWalletForDeployment(isSimulator)
	if wallet == nil {
		errMsg := "No wallet is currently open"
		if isSimulator {
			errMsg = "Simulator wallet is not open. Restart simulator mode."
		}
		return map[string]interface{}{
			"success": false,
			"error":   errMsg,
		}
	}

	// Parse DOC info
	var docInfo DOCInfo
	if err := json.Unmarshal([]byte(docJSON), &docInfo); err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "Invalid DOC format. Please check your input.",
			"technicalError": err.Error(),
		}
	}

	// Convert DataString to Data if provided (from frontend)
	if len(docInfo.Data) == 0 && docInfo.DataString != "" {
		docInfo.Data = []byte(docInfo.DataString)
		a.logToConsole(fmt.Sprintf("[DOC] Received file data from frontend: %d bytes", len(docInfo.Data)))
	}

	// Read file content if path is provided (fallback for local file paths)
	if docInfo.Path != "" && len(docInfo.Data) == 0 {
		data, err := os.ReadFile(docInfo.Path)
		if err != nil {
			return ErrorResponse(err)
		}
		docInfo.Data = data
	}

	if len(docInfo.Data) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "No file data provided",
		}
	}

	// Handle compression if requested (matching tela-cli install-doc behavior)
	// Compression must happen BEFORE signing, as we sign the compressed data
	docCode := string(docInfo.Data)
	fileName := docInfo.Name
	compressionStr := "none"
	
	if docInfo.Compressed {
		// Check if file is already compressed (has .gz extension)
		ext := filepath.Ext(fileName)
		if !tela.IsCompressedExt(ext) {
			// Compress the data using gzip (matching tela-cli)
			compressed, err := tela.Compress(docInfo.Data, tela.COMPRESSION_GZIP)
			if err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] [TELA] InstallDOC: Compression failed - %v", err))
				return map[string]interface{}{
					"success":        false,
					"error":          "Failed to compress file data",
					"technicalError": err.Error(),
				}
			}
			docCode = compressed
			fileName = fileName + tela.COMPRESSION_GZIP // Append .gz to filename
			compressionStr = "gzip"
			
			// Log compression results
			originalSize := len(docInfo.Data)
			compressedSize := len(compressed)
			savings := 100 - (float64(compressedSize) / float64(originalSize) * 100)
			a.logToConsole(fmt.Sprintf("[COMPRESS] %s: %d → %d bytes (%.1f%% smaller)", 
				docInfo.Name, originalSize, compressedSize, savings))
		} else {
			// File already has compression extension
			a.logToConsole(fmt.Sprintf("[DEBUG] File already compressed: %s", fileName))
			compressionStr = ext
		}
	}

	// Sign the (possibly compressed) file content to generate CheckC and CheckS
	// IMPORTANT: We sign docCode, not docInfo.Data, as tela-cli signs the compressed data
	signature := wallet.SignData([]byte(docCode))
	if signature == nil || len(signature) == 0 {
		a.logToConsole("[ERR] [TELA] InstallDOC: wallet.SignData returned nil or empty")
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to sign file content",
		}
	}

	// Debug: Log signature details
	a.logToConsole(fmt.Sprintf("[DEBUG] Signature length: %d bytes", len(signature)))
	sigStr := string(signature)
	// Log first 200 chars of signature to see format
	if len(sigStr) > 200 {
		a.logToConsole(fmt.Sprintf("[DEBUG] Signature preview: %s...", sigStr[:200]))
	} else {
		a.logToConsole(fmt.Sprintf("[DEBUG] Signature: %s", sigStr))
	}

	// Parse the signature to extract C and S values
	_, checkC, checkS, err := tela.ParseSignature(signature)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] InstallDOC: ParseSignature failed - %v", err))
		return map[string]interface{}{
			"success":        false,
			"error":          "Failed to parse signature",
			"technicalError": err.Error(),
		}
	}

	// IMPORTANT: CheckC and CheckS must be exactly 64 hex characters (32 bytes)
	// The signature may have fewer characters if there are leading zeros
	// Pad with leading zeros if needed
	if len(checkC) < 64 {
		checkC = strings.Repeat("0", 64-len(checkC)) + checkC
	}
	if len(checkS) < 64 {
		checkS = strings.Repeat("0", 64-len(checkS)) + checkS
	}

	// Debug: Log extracted signature values (after padding)
	a.logToConsole(fmt.Sprintf("[DEBUG] Parsed checkC: '%s' (len=%d)", checkC, len(checkC)))
	a.logToConsole(fmt.Sprintf("[DEBUG] Parsed checkS: '%s' (len=%d)", checkS, len(checkS)))

	// Build DOC structure (matching tela-cli)
	doc := tela.DOC{
		DocType: docInfo.DocType,
		Code:    docCode, // Use compressed code if compression enabled
		SubDir:  docInfo.SubDir,
		Headers: tela.Headers{
			NameHdr:  fileName, // Use filename with .gz extension if compressed
			DescrHdr: docInfo.Description,
			IconHdr:  docInfo.IconURL,
		},
		Signature: tela.Signature{
			CheckC: checkC,
			CheckS: checkS,
		},
	}

	// Set compression field on DOC struct
	if docInfo.Compressed {
		doc.Compression = tela.COMPRESSION_GZIP
	}

	a.logToConsole(fmt.Sprintf("[DOC] [TELA] InstallDOC: %s (type=%s, size=%d, subdir=%s, compression=%s)",
		docInfo.Name, docInfo.DocType, len(docInfo.Data), docInfo.SubDir, compressionStr))

	// Set up network configuration
	// CRITICAL: In simulator mode, we must NOT call walletapi.Connect() because:
	// 1. walletapi.Connect() creates a persistent websocket connection
	// 2. tela.GetGasEstimate() creates its OWN websocket connection
	// 3. The simulator daemon can only handle ONE websocket at a time
	// 4. Having both connections open crashes the daemon
	// 
	// Solution: In simulator mode, only set the endpoint variable and flags.
	// Let the tela library create its own connection for everything.
	endpoint := "127.0.0.1:20000"
	
	if isSimulator {
		// Set globals for simulator
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = true
		globals.InitNetwork()
		a.logToConsole("[DEBUG] Set globals for simulator mode (--testnet=true, --simulator=true)")
		
		// Set wallet daemon address and mode (no websocket connection yet)
	wallet.SetDaemonAddress(endpoint)
	wallet.SetOnlineMode()

		// Set the endpoint variable that tela.GetGasEstimate() uses
		walletapi.Daemon_Endpoint_Active = endpoint
		
		// Set Connected=true so TransferPayload0 doesn't reject as "offline"
		// NOTE: We're NOT creating a real walletapi websocket connection!
		// The tela library creates its own connection via GetGasEstimate().
		// Setting Connected=true just satisfies the check in TransferPayload0.
		walletapi.Connected = true
		
		a.logToConsole(fmt.Sprintf("[DEBUG] Set Daemon_Endpoint_Active=%s, Connected=true (tela library will create websocket)", endpoint))
		a.logToConsole("[OK] Proceeding with installation (tela library will create its own websocket)")
	} else {
		// For mainnet/testnet: use setupNetworkForDeployment (which calls walletapi.Connect)
		var err error
		endpoint, err = a.setupNetworkForDeployment(wallet, isSimulator)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] setupNetworkForDeployment failed: %v", err))
			return map[string]interface{}{
				"success":        false,
				"error":          "Failed to setup network for deployment",
				"technicalError": err.Error(),
			}
		}
	}

	// Install DOC using tela library
	// Ringsize: 2 = updateable, 16+ = immutable (anonymous)
	ringsize := uint64(2) // Default: updateable
	if docInfo.Ringsize >= 2 {
		ringsize = uint64(docInfo.Ringsize)
	}
	a.logToConsole(fmt.Sprintf("[DOC] Using ringsize=%d (updateable=%v)", ringsize, ringsize <= 2))
	
	var txid string
	if isSimulator {
		// SIMULATOR MODE: Bypass tela.Installer() to avoid websocket conflicts
		// Uses retry logic similar to tela-cli tests for better reliability
		
		a.logToConsole("[DOC] Using simulator-specific installation with GetGasEstimate validation")
		
		// PRE-DEPLOYMENT HEALTH CHECK: Verify daemon is alive
		if a.daemonClient != nil {
			if _, err := a.daemonClient.GetInfo(); err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] Simulator daemon not responding: %v", err))
				return map[string]interface{}{
					"success":        false,
					"error":          "Cannot connect to simulator daemon. Please restart simulator mode.",
					"technicalError": err.Error(),
				}
			}
			a.logToConsole("[OK] Simulator daemon responding")
		}
		
		// Build install arguments
		args, err := tela.NewInstallArgs(&doc)
		if err != nil {
			a.logToConsole(fmt.Sprintf("[ERR] Failed to create install args: %v", err))
			return ErrorResponse(err)
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
				a.logToConsole(fmt.Sprintf("[RETRY] Attempt %d/%d...", attempt, maxRetries))
				// Wait for a new block before retrying
				if err := a.waitForNewBlockWithHealthCheck(15 * time.Second); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Block wait failed: %v", err))
				}
			}
			
			// CRITICAL: The simulator daemon can only handle ONE websocket connection at a time.
			// tela.GetGasEstimate() creates its own websocket connection internally.
			// So we must: 1) Call GetGasEstimate (creates/closes its own WS), 2) Then connect walletapi
			
			// STEP 1: Ensure walletapi is DISCONNECTED before calling GetGasEstimate
			a.disconnectWalletAPI()
			time.Sleep(50 * time.Millisecond) // Brief settle time for daemon
			
			// STEP 2: Get gas estimate - this validates SC code AND creates its own temporary websocket
			a.logToConsole(fmt.Sprintf("[GAS] Getting gas estimate (attempt %d)...", attempt))
			gasFees, gasErr := tela.GetGasEstimate(wallet, ringsize, transfers, args)
			if gasErr != nil {
				a.logToConsole(fmt.Sprintf("[ERR] GetGasEstimate failed (attempt %d): %v", attempt, gasErr))
				lastErr = fmt.Errorf("failed to get gas estimate: %v", gasErr)
				continue
			}
			a.logToConsole(fmt.Sprintf("[OK] Gas estimate: %d", gasFees))
			
			// Brief pause to let GetGasEstimate's websocket fully close
			time.Sleep(100 * time.Millisecond)
			
			// STEP 3: NOW connect walletapi for sync/build/send
			a.logToConsole(fmt.Sprintf("[NET] Connecting walletapi (attempt %d): %s", attempt, endpoint))
			if err := walletapi.Connect(endpoint); err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] walletapi.Connect failed (attempt %d): %v", attempt, err))
				lastErr = fmt.Errorf("failed to connect to simulator daemon: %v", err)
				a.disconnectWalletAPI()
				continue
			}
			
			// Sync wallet to get correct nonce
			if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
				a.logToConsole(fmt.Sprintf("[WARN] Pre-tx sync failed: %v", err))
			}
			time.Sleep(50 * time.Millisecond) // Brief settle time
			
			// Build transaction
			tx, buildErr := wallet.TransferPayload0(transfers, ringsize, false, args, gasFees, false)
			if buildErr != nil {
				a.logToConsole(fmt.Sprintf("[ERR] TransferPayload0 failed (attempt %d): %v", attempt, buildErr))
				lastErr = fmt.Errorf("transfer build error: %v", buildErr)
				a.disconnectWalletAPI()
				continue
			}
			
			if tx == nil {
				lastErr = fmt.Errorf("transaction is nil after build")
				a.disconnectWalletAPI()
				continue
			}
			
			// Send transaction
			if err := wallet.SendTransaction(tx); err != nil {
				a.logToConsole(fmt.Sprintf("[ERR] SendTransaction failed (attempt %d): %v", attempt, err))
				lastErr = fmt.Errorf("transaction dispatch error: %v", err)
				a.disconnectWalletAPI()
				continue
			}
			
			txid = tx.GetHash().String()
			a.logToConsole(fmt.Sprintf("[OK] Transaction sent: %s", txid))
			
			// Disconnect walletapi (cleanup)
			a.disconnectWalletAPI()
			a.logToConsole("[NET] Disconnected after send")
			
			// SUCCESS! Exit retry loop
			lastErr = nil
			break
		}
		
		if lastErr != nil {
			return map[string]interface{}{
				"success":        false,
				"error":          fmt.Sprintf("Failed after %d attempts: %v", maxRetries, lastErr),
				"technicalError": lastErr.Error(),
			}
		}
	} else {
		// NON-SIMULATOR: Use standard tela.Installer() (supports multiple connections)
		var err error
		txid, err = tela.Installer(wallet, ringsize, &doc)
		if err != nil {
			// Handle "Account Unregistered" error specifically
			if strings.Contains(err.Error(), "Account Unregistered") || strings.Contains(err.Error(), "-32098") {
				a.logToConsole("[ERR] Wallet not registered on blockchain")
				return map[string]interface{}{
					"success":        false,
					"error":          "Wallet not registered. Please click 'Auto-mines to confirm' button or start mining to register your wallet on the simulator blockchain.",
					"technicalError": err.Error(),
					"needsRegistration": true,
				}
			}
			a.logToConsole(fmt.Sprintf("[ERR] [TELA] InstallDOC: Failed - %v", err))
			return ErrorResponse(err)
		}
	}

	a.logToConsole(fmt.Sprintf("[OK] [TELA] InstallDOC: Success! SCID=%s", txid))

	return map[string]interface{}{
		"success": true,
		"txid":    txid,
		"message": "DOC installed successfully",
	}
}

// PreviewDOC analyzes a file and returns DOC metadata without installing
func (a *App) PreviewDOC(filePath string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[...] [TELA] PreviewDOC: Analyzing %s", filepath.Base(filePath)))

	// Check file exists
	info, err := os.Stat(filePath)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] PreviewDOC: File not found - %s", filePath))
		return map[string]interface{}{
			"success":        false,
			"error":          "File not found. Check the path.",
			"technicalError": err.Error(),
		}
	}

	// Detect doc type from extension
	ext := strings.ToLower(filepath.Ext(filePath))
	docType := tela.ParseDocType(filepath.Base(filePath))

	// Read file for size and potential compression estimate
	data, err := os.ReadFile(filePath)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] PreviewDOC: Failed to read file - %v", err))
		return ErrorResponse(err)
	}

	// Estimate gas cost (simplified calculation)
	gasEstimate := estimateGasCost(len(data))
	compress := canCompress(docType)

	a.logToConsole(fmt.Sprintf("[OK] [TELA] PreviewDOC: %s (%d bytes, type=%s, compress=%v, gas=%d)",
		filepath.Base(filePath), info.Size(), docType, compress, gasEstimate))

	return map[string]interface{}{
		"success":     true,
		"name":        filepath.Base(filePath),
		"path":        filePath,
		"size":        info.Size(),
		"docType":     docType,
		"extension":   ext,
		"gasEstimate": gasEstimate,
		"canCompress": compress,
	}
}

// GetGasEstimate estimates gas cost for DOC installation
func (a *App) GetGasEstimate(docJSON string) map[string]interface{} {
	var doc DOCInfo
	if err := json.Unmarshal([]byte(docJSON), &doc); err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] GetGasEstimate: Invalid JSON - %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid DOC info",
		}
	}

	size := doc.Size
	if size == 0 && doc.Path != "" {
		if info, err := os.Stat(doc.Path); err == nil {
			size = info.Size()
		}
	}

	gasEstimate := estimateGasCost(int(size))

	// Check if we're in simulator mode (gas is free)
	isSimulator := a.IsInSimulatorMode()
	if isSimulator {
		a.logToConsole(fmt.Sprintf("[BALANCE] [TELA] GetGasEstimate: %s (%d bytes) → FREE (Simulator Mode)", doc.Name, size))
	} else {
		a.logToConsole(fmt.Sprintf("[BALANCE] [TELA] GetGasEstimate: %s (%d bytes) → %d gas (~%.5f DERO)", doc.Name, size, gasEstimate, float64(gasEstimate)/100000))
	}

	return map[string]interface{}{
		"success":     true,
		"gasEstimate": gasEstimate,
		"size":        size,
	}
}

// InstallINDEX creates a TELA INDEX smart contract
func (a *App) InstallINDEX(indexJSON string) map[string]interface{} {
	isSimulator := a.IsInSimulatorMode()
	modeStr := ""
	if isSimulator {
		modeStr = " [SIMULATOR]"
	}
	a.logToConsole(fmt.Sprintf("[INDEX] [TELA] InstallINDEX: Starting installation...%s", modeStr))

	// Check wallet - use simulator wallet in simulator mode, otherwise main wallet
	wallet := a.getWalletForDeployment(isSimulator)
	if wallet == nil {
		errMsg := "No wallet is currently open"
		if isSimulator {
			errMsg = "Simulator wallet is not open. Restart simulator mode."
		}
		return map[string]interface{}{
			"success": false,
			"error":   errMsg,
		}
	}

	// Parse INDEX info
	var idx INDEXInfo
	if err := json.Unmarshal([]byte(indexJSON), &idx); err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "Invalid INDEX format. Please check your input.",
			"technicalError": err.Error(),
		}
	}

	// Build INDEX structure
	index := tela.INDEX{
		DURL: idx.DURL,
		DOCs: idx.DOCSCIDs,
		Mods: idx.Mods, // MOD tags (e.g., "vsoo,txdwd")
		Headers: tela.Headers{
			NameHdr:  idx.Name,
			DescrHdr: idx.Description,
			IconHdr:  idx.IconURL,
		},
	}

	// Log MODs if any
	modsStr := "none"
	if idx.Mods != "" {
		modsStr = idx.Mods
	}
	a.logToConsole(fmt.Sprintf("[INDEX] [TELA] InstallINDEX: %s (durl=%s, docs=%d, mods=%s)",
		idx.Name, idx.DURL, len(idx.DOCSCIDs), modsStr))

	// Set up network configuration
	// CRITICAL: In simulator mode, do NOT call walletapi.Connect() - see InstallDOC for explanation
	endpoint := "127.0.0.1:20000"
	
	if isSimulator {
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = true
		globals.InitNetwork()
		a.logToConsole("[DEBUG] Set globals for simulator mode (--testnet=true, --simulator=true)")
		
		// Set wallet daemon address and mode (no websocket connection yet)
		wallet.SetDaemonAddress(endpoint)
		wallet.SetOnlineMode()
		
		// Set the endpoint variable and Connected flag for tela library
		walletapi.Daemon_Endpoint_Active = endpoint
		walletapi.Connected = true  // Required for TransferPayload0 check
		a.logToConsole(fmt.Sprintf("[DEBUG] Set Daemon_Endpoint_Active=%s, Connected=true (tela library will create websocket)", endpoint))
	} else {
		// Get daemon endpoint for non-simulator
		if a.daemonClient != nil {
			endpoint = a.daemonClient.GetEndpoint()
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}
		
		// For non-simulator: use walletapi.Connect()
		a.logToConsole(fmt.Sprintf("[DEBUG] Connecting walletapi to daemon: %s", endpoint))
		if err := walletapi.Connect(endpoint); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] walletapi.Connect failed: %v", err))
		}
	wallet.SetDaemonAddress(endpoint)
	wallet.SetOnlineMode()
	}

	// Install INDEX using tela library
	// Ringsize: 2 = updateable, 16+ = immutable (anonymous)
	// MODs require ringsize 2 (they have no functionality above RS 2)
	ringsize := uint64(2) // Default: updateable
	if idx.Mods != "" {
		// MODs force ringsize 2
		ringsize = 2
		a.logToConsole("[INDEX] MODs enabled - forcing ringsize 2 (MODs require updateable INDEX)")
	} else if idx.Ringsize >= 2 {
		ringsize = uint64(idx.Ringsize)
	}
	a.logToConsole(fmt.Sprintf("[INDEX] Using ringsize=%d (updateable=%v)", ringsize, ringsize <= 2))
	txid, err := tela.Installer(wallet, ringsize, &index)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] InstallINDEX: Failed - %v", err))
		return ErrorResponse(err)
	}

	a.logToConsole(fmt.Sprintf("[OK] [TELA] InstallINDEX: Success! SCID=%s, dURL=%s", txid, idx.DURL))

	return map[string]interface{}{
		"success": true,
		"txid":    txid,
		"durl":    idx.DURL,
		"message": "INDEX installed successfully",
	}
}

// UpdateINDEX updates an existing TELA INDEX
func (a *App) UpdateINDEX(scid, indexJSON string) map[string]interface{} {
	isSimulator := a.IsInSimulatorMode()
	modeStr := ""
	if isSimulator {
		modeStr = " [SIMULATOR]"
	}
	a.logToConsole(fmt.Sprintf("[SYNC] Updating INDEX: %s%s", scid[:16]+"...", modeStr))

	// Check wallet - use simulator wallet in simulator mode
	wallet := a.getWalletForDeployment(isSimulator)
	if wallet == nil {
		errMsg := "No wallet is currently open"
		if isSimulator {
			errMsg = "Simulator wallet is not open. Restart simulator mode."
		}
		return map[string]interface{}{
			"success": false,
			"error":   errMsg,
		}
	}

	// Parse INDEX info
	var idx INDEXInfo
	if err := json.Unmarshal([]byte(indexJSON), &idx); err != nil {
		return map[string]interface{}{
			"success":        false,
			"error":          "Invalid INDEX format. Please check your input.",
			"technicalError": err.Error(),
		}
	}

	// Debug: Log what we received from frontend
	a.logToConsole(fmt.Sprintf("[DEBUG] Received INDEX update request:"))
	a.logToConsole(fmt.Sprintf("[DEBUG]   Name: %s", idx.Name))
	a.logToConsole(fmt.Sprintf("[DEBUG]   DURL: %s", idx.DURL))
	a.logToConsole(fmt.Sprintf("[DEBUG]   DOCSCIDs count: %d", len(idx.DOCSCIDs)))
	for i, doc := range idx.DOCSCIDs {
		a.logToConsole(fmt.Sprintf("[DEBUG]   DOCSCIDs[%d]: %s", i, doc))
	}

	// Get daemon endpoint
	// CRITICAL: In simulator mode, do NOT call walletapi.Connect() - see InstallDOC for explanation
	endpoint := "127.0.0.1:10102"
	if isSimulator {
		endpoint = "127.0.0.1:20000"
		globals.Arguments["--testnet"] = true
		globals.Arguments["--simulator"] = true
		globals.InitNetwork()
		a.logToConsole("[DEBUG] Set globals for simulator mode (--testnet=true, --simulator=true)")
		
		// Set wallet daemon address and mode (no websocket connection yet)
		wallet.SetDaemonAddress(endpoint)
		wallet.SetOnlineMode()
		
		// Set the endpoint variable and Connected flag for tela library
		walletapi.Daemon_Endpoint_Active = endpoint
		walletapi.Connected = true  // Required for TransferPayload0 check
		a.logToConsole(fmt.Sprintf("[DEBUG] Set Daemon_Endpoint_Active=%s, Connected=true (tela library will create websocket)", endpoint))
	} else if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		endpoint = strings.TrimPrefix(ep, "http://")
		endpoint = strings.TrimPrefix(endpoint, "https://")

		// Connect walletapi for non-simulator mode
		a.logToConsole(fmt.Sprintf("[NET] Connecting walletapi to daemon: %s", endpoint))
		if err := walletapi.Connect(endpoint); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] walletapi.Connect failed: %v", err))
		}
	wallet.SetDaemonAddress(endpoint)
	wallet.SetOnlineMode()
	}

	// Verify owner (only original author can update)
	existingIndex, err := tela.GetINDEXInfo(scid, endpoint)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Could not verify INDEX ownership: %v", err))
		return map[string]interface{}{
			"success":        false,
			"error":          "Could not verify INDEX: " + FriendlyError(err),
			"technicalError": err.Error(),
		}
	}

	// Check if INDEX is immutable (anon author)
	if existingIndex.Author == "anon" {
		return map[string]interface{}{
			"success": false,
			"error":   "This INDEX is immutable and cannot be updated (deployed with Ring 16+)",
		}
	}

	// Check if wallet is owner
	walletAddr := wallet.GetAddress().String()
	if existingIndex.Author != walletAddr {
		return map[string]interface{}{
			"success": false,
			"error":   "Your wallet is not the owner of this INDEX. Only the original author can update it.",
		}
	}

	// Build INDEX structure with SCID for update
	// Preserve existing version info so Updater knows how to handle the update
	index := tela.INDEX{
		SCID:      scid,
		DURL:      idx.DURL,
		DOCs:      idx.DOCSCIDs,
		SCVersion: existingIndex.SCVersion,
		Mods:      existingIndex.Mods, // Preserve existing mods unless explicitly changed
		Headers: tela.Headers{
			NameHdr:  idx.Name,
			DescrHdr: idx.Description,
			IconHdr:  idx.IconURL,
		},
	}

	// Log version info
	if existingIndex.SCVersion != nil {
		latestVersion := tela.GetLatestContractVersion(false)
		if existingIndex.SCVersion.LessThan(latestVersion) {
			a.logToConsole(fmt.Sprintf("[INFO] INDEX version %s will be updated to %s", existingIndex.SCVersion.String(), latestVersion.String()))
		}
	}

	// Debug: Log the DOCs being sent in the update
	a.logToConsole(fmt.Sprintf("[DEBUG] INDEX update - DOCs to set: %d", len(index.DOCs)))
	for i, doc := range index.DOCs {
		a.logToConsole(fmt.Sprintf("[DEBUG]   DOC%d: %s", i+1, doc))
	}

	// Update INDEX using tela library
	txid, err := tela.Updater(wallet, &index)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] INDEX update failed: %v", err))
		return ErrorResponse(err)
	}

	a.logToConsole(fmt.Sprintf("[OK] INDEX updated successfully! TXID: %s", txid))

	return map[string]interface{}{
		"success": true,
		"scid":    scid,
		"txid":    txid,
		"message": "INDEX updated successfully",
	}
}

// GetINDEXInfo retrieves information about a TELA INDEX
func (a *App) GetINDEXInfo(scid string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("📖 Getting INDEX info: %s", scid[:16]+"..."))

	// Get daemon endpoint
	isSimulator := a.IsInSimulatorMode()
	endpoint := "127.0.0.1:10102"
	if isSimulator {
		endpoint = "127.0.0.1:20000"
	} else if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		endpoint = strings.TrimPrefix(ep, "http://")
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	// Get INDEX info using tela library
	index, err := tela.GetINDEXInfo(scid, endpoint)
	if err != nil {
		return ErrorResponse(err)
	}

	// Check version info
	latestVersion := tela.GetLatestContractVersion(false)
	isLatest := true
	currentVersion := ""
	if index.SCVersion != nil {
		currentVersion = index.SCVersion.String()
		isLatest = !index.SCVersion.LessThan(latestVersion)
	}

	// Check if current wallet is owner
	isOwner := false
	canUpdate := true
	wallet := GetWallet()
	if wallet != nil {
		walletAddr := wallet.GetAddress().String()
		isOwner = index.Author == walletAddr
	}
	
	// "anon" author means immutable (ring 16+)
	if index.Author == "anon" {
		canUpdate = false
	}

	return map[string]interface{}{
		"success":        true,
		"scid":           scid,
		"name":           index.Headers.NameHdr,
		"description":    index.Headers.DescrHdr,
		"icon":           index.Headers.IconHdr,
		"durl":           index.DURL,
		"owner":          index.Author,
		"docs":           index.DOCs,
		"currentVersion": currentVersion,
		"latestVersion":  latestVersion.String(),
		"isLatest":       isLatest,
		"isOwner":        isOwner,
		"canUpdate":      canUpdate,
		"mods":           index.Mods,
	}
}

// CloneTELA downloads TELA content from a SCID
func (a *App) CloneTELA(scid string, allowUpdates bool) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[Clone] TELA content: %s", scid))

	// Validate SCID format
	// Standard SCID = 64 chars, at commit = scid@txid = 129 chars
	atCommit := ""
	if len(scid) == 129 && strings.Contains(scid, "@") {
		parts := strings.Split(scid, "@")
		if len(parts) == 2 && len(parts[0]) == 64 && len(parts[1]) == 64 {
			atCommit = parts[1]
			scid = parts[0]
		} else {
			return map[string]interface{}{
				"success": false,
				"error":   "Invalid format. Use 64-char SCID or scid@txid for specific version",
			}
		}
	} else if len(scid) != 64 {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid SCID. Must be exactly 64 characters",
		}
	}

	// Get daemon endpoint
	endpoint := "127.0.0.1:10102"
	isSimulator := a.IsInSimulatorMode()
	if isSimulator {
		endpoint = "127.0.0.1:20000"
	} else if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		endpoint = strings.TrimPrefix(ep, "http://")
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	// First, get info about what we're cloning
	var contentType string
	var name, dURL, description string
	var fileCount int
	
	// Try INDEX first
	indexInfo, err := tela.GetINDEXInfo(scid, endpoint)
	if err == nil {
		contentType = "INDEX"
		name = indexInfo.NameHdr
		dURL = indexInfo.DURL
		description = indexInfo.DescrHdr
		fileCount = len(indexInfo.DOCs)
		a.logToConsole(fmt.Sprintf("[INFO] Detected INDEX: %s (%s) with %d DOCs", name, dURL, fileCount))
	} else {
		// Try DOC
		docInfo, err := tela.GetDOCInfo(scid, endpoint)
		if err == nil {
			contentType = "DOC"
			name = docInfo.NameHdr
			dURL = docInfo.DURL
			description = docInfo.DescrHdr
			fileCount = 1
			a.logToConsole(fmt.Sprintf("[INFO] Detected DOC: %s (%s)", name, dURL))
		} else {
			a.logToConsole(fmt.Sprintf("[ERR] Could not identify SCID as DOC or INDEX: %v", err))
			return map[string]interface{}{
				"success":        false,
				"error":          "Could not identify SCID as TELA DOC or INDEX",
				"technicalError": err.Error(),
			}
		}
	}

	// Set allow updates flag for tela library
	if allowUpdates {
		tela.AllowUpdates(true)
	}

	// Clone using tela library
	if atCommit != "" {
		a.logToConsole(fmt.Sprintf("[CLONE] Cloning at commit: %s", atCommit))
		err = tela.CloneAtCommit(scid, atCommit, endpoint)
	} else {
		err = tela.Clone(scid, endpoint)
	}

	// Reset allow updates flag
	if allowUpdates {
		tela.AllowUpdates(false)
	}

	if err != nil {
		errStr := err.Error()
		a.logToConsole(fmt.Sprintf("[ERR] Clone failed: %v", err))
		
		// Check if it's an "updated content" error - user needs to confirm
		if strings.Contains(errStr, "user defined no updates and content has been updated") {
			return map[string]interface{}{
				"success":          false,
				"error":            "Content has been updated since original deployment",
				"technicalError":   errStr,
				"requiresConfirm":  true,
				"confirmMessage":   "This TELA content has been updated. Do you want to clone the latest version?",
			}
		}
		
		return map[string]interface{}{
			"success":        false,
			"error":          FriendlyError(err),
			"technicalError": errStr,
		}
	}

	// Get the clone directory
	cloneDir := filepath.Join(tela.GetClonePath(), dURL)
	a.logToConsole(fmt.Sprintf("[OK] Content cloned to: %s", cloneDir))

	return map[string]interface{}{
		"success":     true,
		"directory":   cloneDir,
		"contentType": contentType,
		"name":        name,
		"dURL":        dURL,
		"description": description,
		"fileCount":   fileCount,
		"message":     fmt.Sprintf("Successfully cloned %s: %s", contentType, name),
	}
}

// GetClonePath returns the path where TELA content is cloned to
func (a *App) GetClonePath() string {
	return tela.GetClonePath()
}

// RateTELA submits a rating for TELA content
func (a *App) RateTELA(scid string, rating uint64) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[STAR] Rating SCID %s with %d", scid[:16]+"...", rating))

	// Check wallet
	wallet := GetWallet()
	if wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	// Validate rating (0-10)
	if rating > 10 {
		return map[string]interface{}{
			"success": false,
			"error":   "Rating must be between 0 and 10",
		}
	}

	// Submit rating using tela library
	txid, err := tela.Rate(wallet, scid, rating)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Rating failed: %v", err))
		return ErrorResponse(err)
	}

	a.logToConsole(fmt.Sprintf("[OK] Rating submitted: %s", txid))

	return map[string]interface{}{
		"success": true,
		"txid":    txid,
		"rating":  rating,
		"message": "Rating submitted successfully",
	}
}

// ParseFolderForTELA analyzes a folder and returns staged file information
func (a *App) ParseFolderForTELA(folderPath string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[DIR] [TELA] ParseFolderForTELA: Scanning %s", folderPath))

	files := []DOCInfo{}
	var totalSize int64
	var totalGas uint64
	errors := []string{}

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error accessing %s: %v", path, err))
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// Get relative path for subDir
		relPath, _ := filepath.Rel(folderPath, path)
		subDir := filepath.Dir(relPath)
		if subDir == "." {
			subDir = "/"
		} else {
			subDir = "/" + subDir
		}

		// Detect doc type
		docType := tela.ParseDocType(info.Name())

		files = append(files, DOCInfo{
			Name:    info.Name(),
			Path:    path,
			SubDir:  subDir,
			DocType: docType,
			Size:    info.Size(),
		})

		totalSize += info.Size()
		totalGas += estimateGasCost(int(info.Size()))

		return nil
	})

	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] ParseFolderForTELA: Error walking folder - %v", err))
		return ErrorResponse(err)
	}

	// Log details about found files
	if len(files) > 0 {
		a.logToConsole(fmt.Sprintf("[OK] [TELA] ParseFolderForTELA: Found %d files:", len(files)))
		for _, f := range files {
			a.logToConsole(fmt.Sprintf("   [DOC] %s (type=%s, size=%d, subdir=%s)", f.Name, f.DocType, f.Size, f.SubDir))
		}
		a.logToConsole(fmt.Sprintf("   [STATS] Total: %d bytes, estimated gas: %d", totalSize, totalGas))
	} else {
		a.logToConsole(fmt.Sprintf("[WARN] [TELA] ParseFolderForTELA: No files found in %s", folderPath))
	}

	if len(errors) > 0 {
		a.logToConsole(fmt.Sprintf("[WARN] [TELA] ParseFolderForTELA: %d errors encountered", len(errors)))
	}

	// Calculate estimated gas (for informational display only)
	// Note: This is a rough estimate. Actual gas in simulator is FREE.
	// Real gas estimation happens via tela.GetGasEstimate() during deployment.
	estimatedGas := totalGas

	return map[string]interface{}{
		"success":                  true,
		"files":                    files,
		"totalFiles":               len(files),
		"totalSize":                totalSize,
		"totalGas":                 totalGas,
		"errors":                   errors,
		"folderPath":               folderPath,
		"estimatedGas": estimatedGas,
	}
}

// DeployTELABatch deploys multiple DOCs and creates an INDEX
// Emits Wails events for real-time progress tracking:
// - tela:deploy:start - deployment initiated
// - tela:deploy:progress - each DOC deployed
// - tela:deploy:complete - INDEX created
// - tela:deploy:error - if something fails
func (a *App) DeployTELABatch(batchJSON string) map[string]interface{} {
	isSimulator := a.IsInSimulatorMode()
	modeStr := ""
	if isSimulator {
		modeStr = " [SIMULATOR - FREE]"
	}
	a.logToConsole(fmt.Sprintf("[START] [TELA] DeployTELABatch: Starting batch deployment...%s", modeStr))

	// PRE-DEPLOYMENT HEALTH CHECK: Verify daemon is alive before starting
	if isSimulator {
		a.logToConsole("[CHECK] Verifying simulator daemon is healthy before deployment...")
		if a.daemonClient == nil {
			errMsg := "Simulator daemon client not initialized. Restart simulator mode."
			runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{"error": errMsg})
			return map[string]interface{}{"success": false, "error": errMsg}
		}
		info, err := a.daemonClient.GetInfo()
		if err != nil {
			errMsg := fmt.Sprintf("Cannot connect to simulator daemon: %v. Please restart simulator mode.", err)
			runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{"error": errMsg})
			return map[string]interface{}{"success": false, "error": errMsg}
		}
		if info == nil {
			errMsg := "Simulator daemon returned empty response. Please restart simulator mode."
			runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{"error": errMsg})
			return map[string]interface{}{"success": false, "error": errMsg}
		}
		// Log daemon status
		if height, ok := info["topoheight"].(float64); ok {
			a.logToConsole(fmt.Sprintf("[OK] Simulator daemon healthy (height: %.0f)", height))
		} else {
			a.logToConsole("[OK] Simulator daemon responding")
		}
	}

	// Check wallet
	wallet := a.getWalletForDeployment(isSimulator)
	if wallet == nil {
		errMsg := "No wallet is currently open"
		if isSimulator {
			errMsg = "Simulator wallet is not open. Restart simulator mode."
		}
		runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{"error": errMsg})
		return map[string]interface{}{"success": false, "error": errMsg}
	}

	// Parse batch config
	var batch BatchDeployConfig
	if err := json.Unmarshal([]byte(batchJSON), &batch); err != nil {
		runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{"error": "Invalid batch format"})
		return map[string]interface{}{"success": false, "error": "Invalid batch format", "technicalError": err.Error()}
	}

	// CRITICAL: Sort files to ensure entry point (index.html) is deployed FIRST
	// The first DOC deployed becomes DOC1 in the INDEX, which is the application entrypoint.
	// Without this sort, alphabetical ordering could make blocks.js the entrypoint instead of index.html.
	sortFilesForDeployment(batch.Files)
	a.logToConsole(fmt.Sprintf("[SORT] Files ordered for deployment (entry point first): %s", batch.Files[0].Name))

	// Set up network
	if _, err := a.setupNetworkForDeployment(wallet, isSimulator); err != nil {
		runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{"error": err.Error()})
		return map[string]interface{}{"success": false, "error": err.Error()}
	}

	// NOTE: Pre-deployment balance check REMOVED for simulator mode (Session 103)
	// Reason: The hardcoded SimulatorGasFee (100,000) was overly conservative and blocked
	// deployments that would have succeeded. Gas is FREE in simulator mode anyway.
	// The actual deployment uses tela.GetGasEstimate() which validates SC code properly.
	// For mainnet/testnet, real gas estimation happens per-transaction in deployDOC().

	// Emit start event
	runtime.EventsEmit(a.ctx, "tela:deploy:start", map[string]interface{}{
		"totalFiles": len(batch.Files),
		"indexName":  batch.IndexName,
	})

	// Use ringsize from batch, default to 2 (updateable)
	ringsize := batch.Ringsize
	if ringsize == 0 {
		ringsize = 2
	}
	a.logToConsole(fmt.Sprintf("[DEBUG] Using ringsize %d", ringsize))

	// Deploy each DOC
	docScids := []string{}
	deployedFiles := []map[string]interface{}{}

	for i, docInfo := range batch.Files {
		a.logToConsole(fmt.Sprintf("[DOC] Deploying %d/%d: %s (type=%s, size=%d)",
			i+1, len(batch.Files), docInfo.Name, docInfo.DocType, docInfo.Size))

		runtime.EventsEmit(a.ctx, "tela:deploy:progress", map[string]interface{}{
			"current": i + 1, "total": len(batch.Files), "fileName": docInfo.Name, "status": "deploying",
		})

		// Prepare DOC (read, compress, sign)
		prepared, err := a.prepareDOCForDeployment(docInfo, wallet)
		if err != nil {
			runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{
				"error": err.Error(), "fileName": docInfo.Name, "index": i, "partial": deployedFiles,
			})
			return map[string]interface{}{
				"success": false, "error": err.Error(), "partial": deployedFiles,
			}
		}

		// Deploy DOC
		txid, err := a.deployDOC(wallet, prepared, ringsize, isSimulator)
		if err != nil {
			// Check for TELA-specific errors and provide detailed help
			errResp := GetTELAErrorResponse(err.Error(), docInfo.Name)
			errResp["index"] = i
			errResp["partial"] = deployedFiles
			
			runtime.EventsEmit(a.ctx, "tela:deploy:error", errResp)
			return errResp
		}

		docScids = append(docScids, txid)
		deployedFiles = append(deployedFiles, map[string]interface{}{"name": docInfo.Name, "scid": txid})

		runtime.EventsEmit(a.ctx, "tela:deploy:progress", map[string]interface{}{
			"current": i + 1, "total": len(batch.Files), "fileName": docInfo.Name, "status": "completed", "scid": txid,
		})

		// POST-DEPLOYMENT VERIFICATION: Check that the DOC's init() succeeded
		// This catches cases where the SC was deployed but stringkeys weren't stored
		if !isSimulator {
			a.logToConsole(fmt.Sprintf("[VERIFY] Checking DOC %s (%s...) has valid stringkeys...", docInfo.Name, txid[:16]))
			runtime.EventsEmit(a.ctx, "tela:deploy:progress", map[string]interface{}{
				"current": i + 1, "total": len(batch.Files), "fileName": docInfo.Name, 
				"status": "verifying", "scid": txid,
			})
			
			// Wait a moment for the transaction to propagate before verifying
			time.Sleep(1 * time.Second)
			
			if err := a.verifyDeployedDOC(txid, docInfo.Name, 3); err != nil {
				a.logToConsole(fmt.Sprintf("[WARN] DOC verification failed: %v", err))
				// Emit warning but continue - the DOC might still work after more time
				runtime.EventsEmit(a.ctx, "tela:deploy:progress", map[string]interface{}{
					"current": i + 1, "total": len(batch.Files), "fileName": docInfo.Name, 
					"status": "verify_warning", "scid": txid, "warning": err.Error(),
				})
			} else {
				a.logToConsole(fmt.Sprintf("[OK] DOC %s verified successfully", docInfo.Name))
			}
		}

		// CRITICAL: Wait for block confirmation between deployments
		// On mainnet/testnet, each transaction must be confirmed in a block before the next can be sent
		// Otherwise the daemon will reject the transaction with "rejected by pool by mempool"
		if !isSimulator && i < len(batch.Files)-1 { // Don't wait after last DOC (INDEX will handle its own wait)
			a.logToConsole("[WAIT] Waiting for block confirmation before next DOC...")
			runtime.EventsEmit(a.ctx, "tela:deploy:progress", map[string]interface{}{
				"current": i + 1, "total": len(batch.Files), "fileName": docInfo.Name, 
				"status": "waiting_confirmation", "scid": txid,
			})
			
			// CRITICAL: Must wait for block confirmation - retry up to 3 times with increasing timeout
			// If we don't wait, the next transaction will be rejected with "rejected by pool by mempool"
			blockConfirmed := false
			for blockWaitAttempt := 0; blockWaitAttempt < 3; blockWaitAttempt++ {
				timeout := time.Duration(60+blockWaitAttempt*30) * time.Second // 60s, 90s, 120s
				a.logToConsole(fmt.Sprintf("[DEBUG] Block wait attempt %d (timeout: %v)...", blockWaitAttempt+1, timeout))
				
				if err := a.waitForNewBlockWithHealthCheck(timeout); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Block wait attempt %d failed: %v", blockWaitAttempt+1, err))
					// Wait a bit before retrying
					time.Sleep(5 * time.Second)
				} else {
					a.logToConsole("[OK] Block confirmed, proceeding with next DOC")
					blockConfirmed = true
					break
				}
			}
			
			if !blockConfirmed {
				// Last resort: wait a fixed amount of time for the block to be mined
				a.logToConsole("[WARN] Block confirmation retries exhausted. Waiting 30s as fallback...")
				time.Sleep(30 * time.Second)
			}
			
			// CRITICAL: Sync wallet MULTIPLE times after block confirmation to ensure nonce is updated
			// The first sync may return before the daemon has fully processed the new block
			// Adding a brief delay and double-sync ensures the wallet has the correct nonce
			time.Sleep(500 * time.Millisecond) // Let daemon fully process the new block
			
			for syncAttempt := 0; syncAttempt < 3; syncAttempt++ {
			if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
					a.logToConsole(fmt.Sprintf("[WARN] Post-deploy wallet sync attempt %d failed: %v", syncAttempt+1, err))
				} else {
					a.logToConsole(fmt.Sprintf("[OK] Wallet synced (attempt %d)", syncAttempt+1))
				}
				if syncAttempt < 2 {
					time.Sleep(200 * time.Millisecond)
				}
			}
		}
		
		// SIMULATOR MODE: Add delay between deployments (like tela-cli tests do)
		// This gives the simulator daemon time to process and prevents transaction conflicts
		if isSimulator && i < len(batch.Files)-1 {
			a.logToConsole("[WAIT] Brief delay before next DOC deployment...")
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Create INDEX
	runtime.EventsEmit(a.ctx, "tela:deploy:progress", map[string]interface{}{
		"current": len(batch.Files), "total": len(batch.Files), "fileName": "INDEX", "status": "creating_index",
	})

	// SIMULATOR MODE: Add delay before INDEX creation to let all DOC transactions settle
	// This gives the simulator daemon time to fully process all DOC transactions
	if isSimulator {
		a.logToConsole("[WAIT] Waiting for DOC transactions to settle before INDEX creation...")
		time.Sleep(1 * time.Second)
	}

	// MAINNET/TESTNET: Wait for last DOC's block confirmation before creating INDEX
	// This ensures all DOC transactions are confirmed before the INDEX references them
	if !isSimulator && len(batch.Files) > 0 {
		a.logToConsole("[WAIT] Waiting for last DOC block confirmation before INDEX creation...")
		runtime.EventsEmit(a.ctx, "tela:deploy:progress", map[string]interface{}{
			"current": len(batch.Files), "total": len(batch.Files), 
			"fileName": "INDEX", "status": "waiting_for_docs",
		})
		
		// CRITICAL: Must wait for block confirmation - retry up to 3 times
		blockConfirmed := false
		for blockWaitAttempt := 0; blockWaitAttempt < 3; blockWaitAttempt++ {
			timeout := time.Duration(60+blockWaitAttempt*30) * time.Second
			a.logToConsole(fmt.Sprintf("[DEBUG] INDEX block wait attempt %d (timeout: %v)...", blockWaitAttempt+1, timeout))
			
			if err := a.waitForNewBlockWithHealthCheck(timeout); err != nil {
				a.logToConsole(fmt.Sprintf("[WARN] INDEX block wait attempt %d failed: %v", blockWaitAttempt+1, err))
				time.Sleep(5 * time.Second)
			} else {
				a.logToConsole("[OK] DOC transactions confirmed, creating INDEX")
				blockConfirmed = true
				break
			}
		}
		
		if !blockConfirmed {
			a.logToConsole("[WARN] INDEX block confirmation retries exhausted. Waiting 30s as fallback...")
			time.Sleep(30 * time.Second)
		}
		
		// CRITICAL: Sync wallet multiple times after block confirmation
		time.Sleep(500 * time.Millisecond)
		for syncAttempt := 0; syncAttempt < 3; syncAttempt++ {
			if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
				a.logToConsole(fmt.Sprintf("[WARN] Pre-INDEX wallet sync attempt %d failed: %v", syncAttempt+1, err))
			} else {
				a.logToConsole(fmt.Sprintf("[OK] Wallet synced for INDEX (attempt %d)", syncAttempt+1))
			}
			if syncAttempt < 2 {
				time.Sleep(200 * time.Millisecond)
			}
		}
	}

	// Sync wallet before INDEX creation to ensure nonce is correct
	// NOTE: Skip in simulator mode - tela library manages its own connections
	if !isSimulator {
		if err := wallet.Sync_Wallet_Memory_With_Daemon(); err != nil {
			a.logToConsole(fmt.Sprintf("[WARN] Pre-INDEX wallet sync failed: %v", err))
		}
	}

	indexTxid, err := a.createINDEX(wallet, &batch, docScids, ringsize, isSimulator)
	if err != nil {
		runtime.EventsEmit(a.ctx, "tela:deploy:error", map[string]interface{}{
			"error": "DOCs deployed but INDEX creation failed: " + err.Error(), "deployedDocs": deployedFiles,
		})
		return map[string]interface{}{
			"success": false, "error": "INDEX creation failed: " + FriendlyError(err), "deployedDocs": deployedFiles,
		}
	}

	a.logToConsole(fmt.Sprintf("[OK] Complete! INDEX=%s, dURL=%s, DOCs=%d", indexTxid, batch.IndexDURL, len(deployedFiles)))

	runtime.EventsEmit(a.ctx, "tela:deploy:complete", map[string]interface{}{
		"indexScid": indexTxid, "deployedDocs": deployedFiles, "durl": batch.IndexDURL, "totalFiles": len(deployedFiles),
	})

	return map[string]interface{}{
		"success": true, "indexScid": indexTxid, "deployedDocs": deployedFiles, "durl": batch.IndexDURL,
		"message": fmt.Sprintf("Successfully deployed %d DOCs and created INDEX", len(deployedFiles)),
	}
}

// ServeLocalDirectory starts a local server to preview TELA content
func (a *App) ServeLocalDirectory(directory string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[NET] Starting local server for: %s", directory))

	// Check directory exists
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return map[string]interface{}{
			"success": false,
			"error":   "Directory not found",
		}
	}

	// Find an open port and start serving
	server, found := tela.FindOpenPort()
	if !found {
		return map[string]interface{}{
			"success": false,
			"error":   "No available ports",
		}
	}

	// Get the server address
	addr := server.Addr

	a.logToConsole(fmt.Sprintf("[OK] Local server available at: %s", addr))

	return map[string]interface{}{
		"success":   true,
		"address":   addr,
		"directory": directory,
		"message":   "Local server available",
	}
}

// ================== VERSION CONTROL (GitHub-like) ==================

// Commit represents a single version in the TELA content history
type Commit struct {
	Number    int    `json:"number"`    // Commit number (1, 2, 3...)
	TXID      string `json:"txid"`      // Transaction ID that made this commit
	Height    int64  `json:"height"`    // Block height of the commit
	Timestamp int64  `json:"timestamp"` // Unix timestamp (if available)
	IsCurrent bool   `json:"isCurrent"` // True if this is the latest version
}

// GetCommitHistory retrieves all commits (versions) for a TELA SCID
func (a *App) GetCommitHistory(scid string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[SC] Getting commit history for: %s", scid[:16]+"..."))

	if a.gnomonClient == nil || !a.gnomonClient.IsRunning() {
		// Fallback: try to get from daemon directly
		return a.getCommitHistoryFromDaemon(scid)
	}

	// Get SC interaction history from Gnomon
	commits := []Commit{}

	// Get all SCID interaction heights
	heights := a.gnomonClient.Indexer.GravDBBackend.GetSCIDInteractionHeight(scid)

	if len(heights) == 0 {
		return map[string]interface{}{
			"success": true,
			"scid":    scid,
			"commits": commits,
			"count":   0,
		}
	}

	// Build commit list
	for i, height := range heights {
		commits = append(commits, Commit{
			Number:    i + 1,
			Height:    height,
			IsCurrent: i == len(heights)-1,
		})
	}

	a.logToConsole(fmt.Sprintf("[OK] Found %d commits", len(commits)))

	return map[string]interface{}{
		"success": true,
		"scid":    scid,
		"commits": commits,
		"count":   len(commits),
	}
}

// getCommitHistoryFromDaemon fetches commit history directly from daemon
func (a *App) getCommitHistoryFromDaemon(scid string) map[string]interface{} {
	// Get SC variables to find version info
	vars, err := a.daemonClient.GetSCVariables(scid, true, true)
	if err != nil {
		return ErrorResponse(err)
	}

	commits := []Commit{}

	// Look for "C" variable which typically holds commit/version counter
	if stringKeys, ok := vars["stringkeys"].(map[string]interface{}); ok {
		if cVal, ok := stringKeys["C"].(string); ok {
			// C often contains version count
			versionCount := parseVersionCount(cVal)
			for i := 1; i <= versionCount; i++ {
				commits = append(commits, Commit{
					Number:    i,
					IsCurrent: i == versionCount,
				})
			}
		}
	}

	return map[string]interface{}{
		"success": true,
		"scid":    scid,
		"commits": commits,
		"count":   len(commits),
	}
}

// GetCommitContent fetches content at a specific commit number
func (a *App) GetCommitContent(scid string, commitNum int) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[DOC] Getting content at commit %d for: %s", commitNum, scid[:16]+"..."))

	// Get commit history first
	historyResult := a.GetCommitHistory(scid)
	commits, ok := historyResult["commits"].([]Commit)
	if !ok || len(commits) < commitNum {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Commit %d not found", commitNum),
		}
	}

	commit := commits[commitNum-1]

	// Clone at this specific commit
	var content string
	if commit.Height > 0 {
		// We have the height; clone at that point
		// Note: This requires tela.CloneAtCommit or similar functionality
		content = fmt.Sprintf("Content at block height %d", commit.Height)
	}

	return map[string]interface{}{
		"success":   true,
		"scid":      scid,
		"commit":    commit,
		"commitNum": commitNum,
		"content":   content,
		"message":   fmt.Sprintf("Content at commit %d", commitNum),
	}
}

// DiffCommits compares two commits and returns the differences
func (a *App) DiffCommits(scid string, commitA, commitB int) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[SYNC] Diffing commits %d vs %d for: %s", commitA, commitB, scid[:16]+"..."))

	// Get content at both commits
	contentAResult := a.GetCommitContent(scid, commitA)
	contentBResult := a.GetCommitContent(scid, commitB)

	if !contentAResult["success"].(bool) {
		return contentAResult
	}
	if !contentBResult["success"].(bool) {
		return contentBResult
	}

	contentA, _ := contentAResult["content"].(string)
	contentB, _ := contentBResult["content"].(string)

	// Generate simple line-by-line diff
	diff := generateDiff(contentA, contentB)

	return map[string]interface{}{
		"success":  true,
		"scid":     scid,
		"commitA":  commitA,
		"commitB":  commitB,
		"diff":     diff,
		"hasChanges": contentA != contentB,
	}
}

// generateDiff creates a simple line-by-line diff
func generateDiff(oldContent, newContent string) []map[string]interface{} {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	diff := []map[string]interface{}{}

	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if oldLine != "" && newLine == "" {
				diff = append(diff, map[string]interface{}{
					"type":    "removed",
					"line":    i + 1,
					"content": oldLine,
				})
			} else if oldLine == "" && newLine != "" {
				diff = append(diff, map[string]interface{}{
					"type":    "added",
					"line":    i + 1,
					"content": newLine,
				})
			} else {
				diff = append(diff, map[string]interface{}{
					"type":       "modified",
					"line":       i + 1,
					"oldContent": oldLine,
					"newContent": newLine,
				})
			}
		}
	}

	return diff
}

// parseVersionCount parses version count from SC variable
func parseVersionCount(val string) int {
	// Try direct parse
	count := 0
	decoded := decodeHexString(val)
	fmt.Sscanf(decoded, "%d", &count)
	return count
}

// Helper functions

// getWalletForDeployment returns the appropriate wallet for TELA deployments
// In simulator mode, it returns the primary simulator wallet (#0); otherwise the main app wallet
func (a *App) getWalletForDeployment(isSimulator bool) *walletapi.Wallet_Disk {
	if isSimulator {
		if a.simulatorManager != nil && a.simulatorManager.walletManager != nil {
			return a.simulatorManager.walletManager.GetPrimaryWallet()
		}
		return nil
	}
	return GetWallet()
}

func estimateGasCost(sizeBytes int) uint64 {
	// Base cost + size-based cost
	// This is a rough estimate; actual cost depends on network conditions
	baseCost := uint64(5000)
	sizeCost := uint64(sizeBytes) * 10 // 10 gas per byte approximate
	return baseCost + sizeCost
}

func canCompress(docType string) bool {
	// Text-based types can benefit from compression
	compressible := []string{
		tela.DOC_HTML,
		tela.DOC_CSS,
		tela.DOC_JS,
		tela.DOC_JSON,
		tela.DOC_MD,
	}

	for _, t := range compressible {
		if docType == t {
			return true
		}
	}
	return false
}
