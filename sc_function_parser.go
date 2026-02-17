package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"unicode"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/dvm"
	"github.com/deroproject/derohe/rpc"
)

// SCFunction represents a parsed smart contract function
type SCFunction struct {
	Name       string    `json:"name"`
	Params     []SCParam `json:"params"`
	ReturnType string    `json:"returnType"`
	UsesDERO   bool      `json:"usesDero"`  // DEROVALUE() detected
	UsesAsset  bool      `json:"usesAsset"` // ASSETVALUE() detected
	UsesSigner bool      `json:"usesSigner"` // SIGNER() detected - can't be anonymous
}

// SCParam represents a function parameter
type SCParam struct {
	Name     string `json:"name"`
	Type     string `json:"type"`     // "String" or "Uint64"
	DataType string `json:"dataType"` // "S" or "U" for XSWD
}

// ParseSCFunctions parses SC code and returns callable functions
func (a *App) ParseSCFunctions(scid string) map[string]interface{} {
	a.logToConsole(fmt.Sprintf("[SC] Parsing functions for: %s...", scid[:16]))

	// Get SC code from daemon
	codeResult := a.GetSCCode(scid)
	if success, ok := codeResult["success"].(bool); !ok || !success {
		return codeResult
	}

	code, ok := codeResult["code"].(string)
	if !ok || code == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Smart contract has no code",
		}
	}

	// Parse the DVM code
	sc, _, err := dvm.ParseSmartContract(code)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to parse smart contract: " + err.Error(),
		}
	}

	// Extract exported functions (start with uppercase)
	functions := []SCFunction{}

	for name, fn := range sc.Functions {
		// Check if exported (first char uppercase)
		runes := []rune(name)
		if len(runes) == 0 || !unicode.IsUpper(runes[0]) {
			continue // Skip private functions
		}

		scFn := SCFunction{
			Name:       name,
			Params:     []SCParam{},
			ReturnType: "Uint64", // DVM functions return Uint64
		}

		// Extract parameters
		for _, param := range fn.Params {
			paramType := "String"
			dataType := "S"
			if param.Type == dvm.Uint64 {
				paramType = "Uint64"
				dataType = "U"
			}

			scFn.Params = append(scFn.Params, SCParam{
				Name:     param.Name,
				Type:     paramType,
				DataType: dataType,
			})
		}

		// Scan function lines for special keywords
		for _, line := range fn.Lines {
			for _, token := range line {
				switch token {
				case "DEROVALUE":
					scFn.UsesDERO = true
				case "ASSETVALUE":
					scFn.UsesAsset = true
				case "SIGNER":
					scFn.UsesSigner = true
				}
			}
		}

		functions = append(functions, scFn)
	}

	// Sort functions alphabetically
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Name < functions[j].Name
	})

	a.logToConsole(fmt.Sprintf("[OK] Found %d exported functions", len(functions)))

	return map[string]interface{}{
		"success":   true,
		"functions": functions,
		"count":     len(functions),
		"scid":      scid,
	}
}

// ValidateSCCode parses raw DVM-BASIC code and returns validation result with extracted functions.
// This allows pre-deploy validation without hitting the daemon or spending DERO.
func (a *App) ValidateSCCode(code string) map[string]interface{} {
	if code == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Smart contract code cannot be empty",
		}
	}

	sc, _, err := dvm.ParseSmartContract(code)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Parse error: " + err.Error(),
		}
	}

	functions := []SCFunction{}
	hasInitialize := false

	for name, fn := range sc.Functions {
		runes := []rune(name)
		if len(runes) == 0 || !unicode.IsUpper(runes[0]) {
			continue
		}

		if name == "Initialize" {
			hasInitialize = true
		}

		scFn := SCFunction{
			Name:       name,
			Params:     []SCParam{},
			ReturnType: "Uint64",
		}

		for _, param := range fn.Params {
			paramType := "String"
			dataType := "S"
			if param.Type == dvm.Uint64 {
				paramType = "Uint64"
				dataType = "U"
			}
			scFn.Params = append(scFn.Params, SCParam{
				Name:     param.Name,
				Type:     paramType,
				DataType: dataType,
			})
		}

		for _, line := range fn.Lines {
			for _, token := range line {
				switch token {
				case "DEROVALUE":
					scFn.UsesDERO = true
				case "ASSETVALUE":
					scFn.UsesAsset = true
				case "SIGNER":
					scFn.UsesSigner = true
				}
			}
		}

		functions = append(functions, scFn)
	}

	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Name < functions[j].Name
	})

	a.logToConsole(fmt.Sprintf("[SC] Validated code: %d functions, Initialize=%v", len(functions), hasInitialize))

	return map[string]interface{}{
		"success":       true,
		"functions":     functions,
		"count":         len(functions),
		"hasInitialize": hasInitialize,
	}
}

// InvokeSCFunctionParams represents the parameters for invoking an SC function
type InvokeSCFunctionParams struct {
	SCID        string                 `json:"scid"`
	Function    string                 `json:"function"`
	Params      map[string]interface{} `json:"params"`      // Function parameters
	DeroAmount  uint64                 `json:"deroAmount"`  // DERO to send (atomic units)
	AssetSCID   string                 `json:"assetScid"`   // Token SCID if sending asset
	AssetAmount uint64                 `json:"assetAmount"` // Token amount
	Anonymous   bool                   `json:"anonymous"`   // Use ringsize 16
}

// InvokeSCFunction builds and sends an SC invocation transaction
func (a *App) InvokeSCFunction(paramsJSON string) map[string]interface{} {
	var params InvokeSCFunctionParams
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid parameters: " + err.Error(),
		}
	}

	a.logToConsole(fmt.Sprintf("[SC] Invoking %s on %s...", params.Function, params.SCID[:16]))

	// Check wallet
	wallet := GetWallet()
	if wallet == nil {
		// Try XSWD if no local wallet
		if a.xswdClient != nil && a.xswdClient.IsConnected() {
			return a.invokeSCViaXSWD(params)
		}
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet available. Open a wallet or connect via XSWD.",
		}
	}

	// Build SC arguments
	scArgs := rpc.Arguments{
		{Name: rpc.SCACTION, DataType: rpc.DataUint64, Value: uint64(rpc.SC_CALL)},
		{Name: rpc.SCID, DataType: rpc.DataHash, Value: crypto.HashHexToHash(params.SCID)},
		{Name: "entrypoint", DataType: rpc.DataString, Value: params.Function},
	}

	// Add function parameters
	for name, value := range params.Params {
		switch v := value.(type) {
		case string:
			scArgs = append(scArgs, rpc.Argument{
				Name:     name,
				DataType: rpc.DataString,
				Value:    v,
			})
		case float64: // JSON numbers come as float64
			scArgs = append(scArgs, rpc.Argument{
				Name:     name,
				DataType: rpc.DataUint64,
				Value:    uint64(v),
			})
		case int:
			scArgs = append(scArgs, rpc.Argument{
				Name:     name,
				DataType: rpc.DataUint64,
				Value:    uint64(v),
			})
		}
	}

	// Build transfers
	transfers := []rpc.Transfer{}

	// Get random destination for the transfer
	randos := wallet.Random_ring_members(crypto.ZEROHASH)
	if len(randos) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "Could not get ring members - check daemon connection",
		}
	}
	destination := randos[0]
	if destination == wallet.GetAddress().String() && len(randos) > 1 {
		destination = randos[1]
	}

	// Add DERO transfer if needed
	if params.DeroAmount > 0 {
		transfers = append(transfers, rpc.Transfer{
			Destination: destination,
			Amount:      0,
			Burn:        params.DeroAmount,
		})
	}

	// Add asset transfer if needed
	if params.AssetSCID != "" && params.AssetAmount > 0 {
		transfers = append(transfers, rpc.Transfer{
			Destination: destination,
			SCID:        crypto.HashHexToHash(params.AssetSCID),
			Amount:      0,
			Burn:        params.AssetAmount,
		})
	}

	// If no transfers, add a minimal one
	if len(transfers) == 0 {
		transfers = append(transfers, rpc.Transfer{
			Destination: destination,
			Amount:      0,
		})
	}

	// Set ringsize
	ringsize := uint64(2)
	if params.Anonymous {
		ringsize = 16
	}

	// Build and send transaction
	tx, err := wallet.TransferPayload0(transfers, ringsize, false, scArgs, 0, false)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Transaction failed: " + err.Error(),
		}
	}

	if err := wallet.SendTransaction(tx); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to send transaction: " + err.Error(),
		}
	}

	txid := tx.GetHash().String()
	a.logToConsole(fmt.Sprintf("[OK] SC invoked successfully: %s...", txid[:16]))

	return map[string]interface{}{
		"success":  true,
		"txid":     txid,
		"function": params.Function,
		"message":  "Smart contract function called successfully",
	}
}

// invokeSCViaXSWD invokes SC function via XSWD connection
func (a *App) invokeSCViaXSWD(params InvokeSCFunctionParams) map[string]interface{} {
	// Build XSWD-compatible request
	scRPC := []map[string]interface{}{
		{"name": "entrypoint", "datatype": "S", "value": params.Function},
	}

	for name, value := range params.Params {
		switch v := value.(type) {
		case string:
			scRPC = append(scRPC, map[string]interface{}{
				"name": name, "datatype": "S", "value": v,
			})
		case float64:
			scRPC = append(scRPC, map[string]interface{}{
				"name": name, "datatype": "U", "value": uint64(v),
			})
		}
	}

	xswdParams := map[string]interface{}{
		"scid":   params.SCID,
		"sc_rpc": scRPC,
	}

	// Add transfers if DERO/asset amounts
	if params.DeroAmount > 0 || params.AssetAmount > 0 {
		transfers := []map[string]interface{}{}
		if params.DeroAmount > 0 {
			transfers = append(transfers, map[string]interface{}{
				"burn": params.DeroAmount,
			})
		}
		if params.AssetSCID != "" && params.AssetAmount > 0 {
			transfers = append(transfers, map[string]interface{}{
				"scid": params.AssetSCID,
				"burn": params.AssetAmount,
			})
		}
		xswdParams["transfers"] = transfers
	}

	if params.Anonymous {
		xswdParams["ringsize"] = 16
	}

	result, err := a.xswdClient.Call("scinvoke", xswdParams)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "XSWD call failed: " + err.Error(),
		}
	}

	// Extract txid from result if available
	txid := ""
	if resultMap, ok := result.(map[string]interface{}); ok {
		if t, ok := resultMap["txid"].(string); ok {
			txid = t
		}
	}

	return map[string]interface{}{
		"success":  true,
		"txid":     txid,
		"result":   result,
		"function": params.Function,
		"message":  "Smart contract function called via XSWD",
	}
}

// InstallSmartContract deploys a new smart contract to mainnet
func (a *App) InstallSmartContract(code string, anonymous bool) map[string]interface{} {
	a.logToConsole("[SC] Installing smart contract...")

	// Check wallet
	wallet := GetWallet()
	if wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet available. Open a wallet first.",
		}
	}

	// Validate the code
	if code == "" {
		return map[string]interface{}{
			"success": false,
			"error":   "Smart contract code cannot be empty",
		}
	}

	// Parse to validate
	_, _, err := dvm.ParseSmartContract(code)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Invalid smart contract code: " + err.Error(),
		}
	}

	// Build SC install arguments
	scArgs := rpc.Arguments{
		{Name: rpc.SCACTION, DataType: rpc.DataUint64, Value: uint64(rpc.SC_INSTALL)},
		{Name: rpc.SCCODE, DataType: rpc.DataString, Value: code},
	}

	// Get random destination for the transfer
	randos := wallet.Random_ring_members(crypto.ZEROHASH)
	if len(randos) == 0 {
		return map[string]interface{}{
			"success": false,
			"error":   "Could not get ring members - check daemon connection",
		}
	}
	destination := randos[0]
	if destination == wallet.GetAddress().String() && len(randos) > 1 {
		destination = randos[1]
	}

	// Build transfer
	transfers := []rpc.Transfer{
		{
			Destination: destination,
			Amount:      0,
		},
	}

	// Set ringsize
	ringsize := uint64(2)
	if anonymous {
		ringsize = 16
	}

	// Build and send transaction
	tx, err := wallet.TransferPayload0(transfers, ringsize, false, scArgs, 0, false)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Transaction failed: " + err.Error(),
		}
	}

	if err := wallet.SendTransaction(tx); err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to send transaction: " + err.Error(),
		}
	}

	txid := tx.GetHash().String()
	a.logToConsole(fmt.Sprintf("[OK] SC installed! TXID: %s", txid[:16]))

	return map[string]interface{}{
		"success": true,
		"txid":    txid,
		"message": "Smart contract installed successfully. The SCID will be available once the transaction is confirmed.",
	}
}

