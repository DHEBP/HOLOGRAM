package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type XSWDPendingRequest struct {
	Method   string
	Params   map[string]interface{}
	RespChan chan interface{}
}

// Subscription event types
type SubscriptionType string

const (
	SubNewTopoheight SubscriptionType = "new_topoheight"
	SubNewBalance    SubscriptionType = "new_balance"
	SubNewEntry      SubscriptionType = "new_entry"
)

// ClientSubscriptions tracks what events a client is subscribed to
type ClientSubscriptions struct {
	NewTopoheight bool
	NewBalance    bool
	NewEntry      bool
}

type XSWDServer struct {
	app      *App
	server   *http.Server
	upgrader websocket.Upgrader
	clients  map[*websocket.Conn]bool
	lock     sync.RWMutex
	
	pendingRequests map[string]*XSWDPendingRequest
	pendingLock     sync.Mutex
	
	// Track client origins for permission checking
	clientOrigins map[*websocket.Conn]string
	
	// Track subscriptions per client
	clientSubscriptions map[*websocket.Conn]*ClientSubscriptions
	
	// Track last known values for change detection
	lastTopoheight int64
	lastBalance    uint64
	
	// Subscription event pusher
	stopPusher chan struct{}
	pusherRunning bool
}

func NewXSWDServer(app *App) *XSWDServer {
	return &XSWDServer{
		app:     app,
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		pendingRequests:     make(map[string]*XSWDPendingRequest),
		clientOrigins:       make(map[*websocket.Conn]string),
		clientSubscriptions: make(map[*websocket.Conn]*ClientSubscriptions),
		stopPusher:          make(chan struct{}),
	}
}

func (s *XSWDServer) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/xswd", s.handleWebSocket)

	s.server = &http.Server{
		Addr:    "127.0.0.1:44326",
		Handler: mux,
	}

	go func() {
		log.Println("[START] Starting internal XSWD server on 127.0.0.1:44326")
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ERR] XSWD server error: %v", err)
		}
	}()
	
	// Start subscription event pusher
	go s.startSubscriptionPusher()
}

func (s *XSWDServer) Stop() {
	// Stop subscription pusher
	if s.pusherRunning {
		close(s.stopPusher)
		s.pusherRunning = false
	}
	
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
}

func (s *XSWDServer) IsRunning() bool {
	return s.server != nil
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *XSWDServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ERR] Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	s.lock.Lock()
	s.clients[conn] = true
	s.lock.Unlock()

	defer func() {
		s.lock.Lock()
		// Clean up client tracking
		origin := s.clientOrigins[conn]
		delete(s.clients, conn)
		delete(s.clientOrigins, conn)
		delete(s.clientSubscriptions, conn)
		s.lock.Unlock()
		
		// Mark client as inactive
		if pm := GetPermissionManager(); pm != nil && origin != "" {
			pm.SetActiveClient(origin, false)
		}
	}()

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if messageType != websocket.TextMessage {
			continue
		}

		log.Printf("[XSWD] Raw Message: %s", string(message))

		var req JSONRPCRequest
		if err := json.Unmarshal(message, &req); err != nil {
			log.Printf("[ERR] JSON Unmarshal error: %v", err)
			continue
		}

		go s.handleRequest(conn, req, message)
	}
}

func (s *XSWDServer) handleRequest(conn *websocket.Conn, req JSONRPCRequest, raw []byte) {
	var result interface{}
	var errRes *JSONRPCError

	// Log request method
	log.Printf("[XSWD] Request: %s", req.Method)

	// Some dApps send a handshake payload (application metadata) before JSON-RPC calls.
	if req.Method == "" {
		s.handleHandshake(conn, req, raw)
		return
	}

	// Auto-Login
	if req.Method == "Login" || req.Method == "DERO.Login" {
		result = "Logged in"
		s.sendResponse(conn, req.ID, result, nil)
		return
	}

	// Ping / Echo - always allowed
	if req.Method == "Ping" || req.Method == "DERO.Ping" {
		result = "Pong"
		s.sendResponse(conn, req.ID, result, nil)
		return
	}
	if req.Method == "Echo" || req.Method == "DERO.Echo" {
		params := []interface{}{}
		json.Unmarshal(req.Params, &params)
		result = params
		s.sendResponse(conn, req.ID, result, nil)
		return
	}

	// Get client origin for permission checking
	s.lock.RLock()
	origin := s.clientOrigins[conn]
	s.lock.RUnlock()

	// Check permissions for methods that require them
	pm := GetPermissionManager()
	requiredPerm := GetRequiredPermission(req.Method)

	// Handle permission-gated methods
	switch req.Method {
	case "GetAddress", "DERO.GetAddress":
		// Check if permission granted
		if pm != nil && origin != "" && !pm.HasPermission(origin, PermissionViewAddress) {
			errRes = &JSONRPCError{Code: -32003, Message: "Permission denied: view_address not granted"}
			s.sendResponse(conn, req.ID, nil, errRes)
			return
		}
		
		if !walletManager.isOpen {
			errRes = &JSONRPCError{Code: -32000, Message: "Wallet not open"}
		} else {
			addr := walletManager.wallet.GetAddress().String()
			result = map[string]string{"address": addr}
		}
		s.sendResponse(conn, req.ID, result, errRes)

	case "GetBalance", "DERO.GetBalance":
		// Check if permission granted
		if pm != nil && origin != "" && !pm.HasPermission(origin, PermissionViewBalance) {
			errRes = &JSONRPCError{Code: -32003, Message: "Permission denied: view_balance not granted"}
			s.sendResponse(conn, req.ID, nil, errRes)
			return
		}
		
		if !walletManager.isOpen {
			errRes = &JSONRPCError{Code: -32000, Message: "Wallet not open"}
		} else {
			m, l := walletManager.wallet.Get_Balance()
			result = map[string]uint64{"balance": m, "locked_balance": l}
		}
		s.sendResponse(conn, req.ID, result, errRes)
		
	case "transfer", "Transfer", "DERO.Transfer", "scinvoke", "SC_Invoke", "DERO.SC_Invoke":
		// Check if base permission granted (still requires per-TX approval)
		if pm != nil && origin != "" {
			if !pm.HasPermission(origin, requiredPerm) {
				errRes = &JSONRPCError{Code: -32003, Message: fmt.Sprintf("Permission denied: %s not granted", requiredPerm)}
				s.sendResponse(conn, req.ID, nil, errRes)
				return
			}
		}
		// Handle signing request (always requires user approval)
		s.handleSigningRequest(conn, req)

	// EPOCH Methods - Developer Support (no permission required, always allowed if enabled)
	case "AttemptEPOCH":
		// Get app SCID from params if available
		var params map[string]interface{}
		json.Unmarshal(req.Params, &params)
		hashes := 100 // default
		if h, ok := params["hashes"].(float64); ok {
			hashes = int(h)
		}
		appSCID := origin
		if scid, ok := params["scid"].(string); ok && scid != "" {
			appSCID = scid
		}
		
		epochResult := s.app.HandleEpochRequest(hashes, appSCID)
		if epochResult["success"] == true {
			result = map[string]interface{}{
				"epochHashes":        epochResult["hashes"],
				"epochSubmitted":     epochResult["submitted"],
				"epochDuration":      epochResult["duration_ms"],
				"epochHashPerSecond": epochResult["hash_per_sec"],
			}
		} else {
			errRes = &JSONRPCError{Code: -32000, Message: fmt.Sprintf("%v", epochResult["error"])}
		}
		s.sendResponse(conn, req.ID, result, errRes)

	case "GetMaxHashesEPOCH":
		stats := s.app.GetEpochStats()
		result = map[string]interface{}{
			"maxHashes": stats["max_hashes"],
		}
		s.sendResponse(conn, req.ID, result, errRes)

	case "GetSessionEPOCH":
		stats := s.app.GetEpochStats()
		if stats["active"] == true {
			result = map[string]interface{}{
				"sessionHashes":  stats["hashes"],
				"sessionMinis":   stats["miniblocks"],
				"sessionVersion": stats["version"],
			}
		} else {
			errRes = &JSONRPCError{Code: -32000, Message: "EPOCH is not active"}
		}
		s.sendResponse(conn, req.ID, result, errRes)

	case "GetAddressEPOCH":
		stats := s.app.GetEpochStats()
		if stats["active"] == true {
			result = map[string]interface{}{
				"epochAddress": stats["address"],
			}
		} else {
			errRes = &JSONRPCError{Code: -32000, Message: "EPOCH is not active"}
		}
		s.sendResponse(conn, req.ID, result, errRes)

	// Subscription support for real-time events
	case "Subscribe":
		var params map[string]interface{}
		json.Unmarshal(req.Params, &params)
		
		eventType, _ := params["event"].(string)
		if eventType == "" {
			errRes = &JSONRPCError{Code: -32602, Message: "Invalid params: 'event' required"}
			s.sendResponse(conn, req.ID, nil, errRes)
			return
		}
		
		// Initialize subscriptions for this client if not exists
		s.lock.Lock()
		if s.clientSubscriptions[conn] == nil {
			s.clientSubscriptions[conn] = &ClientSubscriptions{}
		}
		subs := s.clientSubscriptions[conn]
		
		switch SubscriptionType(eventType) {
		case SubNewTopoheight:
			subs.NewTopoheight = true
			log.Printf("[NET] Client subscribed to new_topoheight")
		case SubNewBalance:
			subs.NewBalance = true
			log.Printf("[NET] Client subscribed to new_balance")
		case SubNewEntry:
			subs.NewEntry = true
			log.Printf("[NET] Client subscribed to new_entry")
		default:
			s.lock.Unlock()
			errRes = &JSONRPCError{Code: -32602, Message: fmt.Sprintf("Unknown event type: %s", eventType)}
			s.sendResponse(conn, req.ID, nil, errRes)
			return
		}
		s.lock.Unlock()
		
		// Return success
		result = map[string]interface{}{
			"event":      eventType,
			"subscribed": true,
		}
		s.sendResponse(conn, req.ID, result, nil)

	// GetDaemon - Returns daemon endpoint for direct node communication
	// dApps use this to connect directly to the node for read operations (GetSC, tx tracking, etc.)
	// Returns just host:port - dApps are expected to add protocol prefix and /ws path themselves
	// This matches Engram's behavior
	case "GetDaemon", "DERO.GetDaemon":
		log.Printf("[XSWD] GetDaemon: request from origin=%q, reqID=%v", origin, req.ID)
		
		// Check if permission granted (requires view_address like other read methods)
		if pm != nil && origin != "" && !pm.HasPermission(origin, PermissionViewAddress) {
			log.Printf("[XSWD] GetDaemon: DENIED - origin=%q does not have view_address permission", origin)
			errRes = &JSONRPCError{Code: -32003, Message: "Permission denied: view_address not granted"}
			s.sendResponse(conn, req.ID, nil, errRes)
			return
		}
		
		// Determine endpoint based on mode:
		// 1. If simulator mode is active, use simulator daemon (port 20000)
		// 2. Otherwise, use configured daemon endpoint or default mainnet (port 10102)
		var endpoint string
		var endpointSource string
		
		if s.app != nil && s.app.simulatorManager != nil && s.app.simulatorManager.isInitialized {
			// Simulator mode active - use simulator daemon endpoint
			endpoint = "127.0.0.1:20000"
			endpointSource = "simulator"
			log.Printf("[XSWD] GetDaemon: Simulator mode active, using endpoint %s", endpoint)
		} else {
			// Normal mode - use configured endpoint or default
			endpoint = "127.0.0.1:10102"
			endpointSource = "default"
			if s.app != nil {
				if ep, ok := s.app.settings["daemon_endpoint"].(string); ok && ep != "" {
					endpoint = ep
					endpointSource = "settings"
				}
			}
		}
		
		// Log the raw endpoint before stripping
		log.Printf("[XSWD] GetDaemon: raw endpoint=%q source=%s", endpoint, endpointSource)
		
		// Strip http:// or https:// prefix if present - return just host:port
		if len(endpoint) > 7 && endpoint[:7] == "http://" {
			endpoint = endpoint[7:]
		} else if len(endpoint) > 8 && endpoint[:8] == "https://" {
			endpoint = endpoint[8:]
		}
		
		log.Printf("[XSWD] GetDaemon: RETURNING endpoint=%q (source=%s) to origin=%q", endpoint, endpointSource, origin)
		result = map[string]interface{}{"endpoint": endpoint}
		s.sendResponse(conn, req.ID, result, nil)

	default:
		log.Printf("[ERR] XSWD Method not found: %s", req.Method)
		errRes = &JSONRPCError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)}
		s.sendResponse(conn, req.ID, nil, errRes)
	}
}

func (s *XSWDServer) handleHandshake(conn *websocket.Conn, req JSONRPCRequest, raw []byte) {
	reqID := fmt.Sprintf("%v", req.ID)
	if reqID == "" {
		reqID = fmt.Sprintf("handshake_%d", time.Now().UnixNano())
	}

	// Parse handshake info
	info := map[string]interface{}{}
	if err := json.Unmarshal(raw, &info); err != nil {
		log.Printf("[WARN] Failed to parse handshake info: %v", err)
	}

	resChan := make(chan interface{})
	s.pendingLock.Lock()
	s.pendingRequests[reqID] = &XSWDPendingRequest{
		Method:   "handshake",
		Params:   info,
		RespChan: resChan,
	}
	s.pendingLock.Unlock()

	appName, _ := info["name"].(string)
	origin, _ := info["url"].(string)
	description, _ := info["description"].(string)
	
	// Parse requested permissions from handshake (if provided by dApp)
	// Format: {"permissions": ["view_address", "view_balance", ...]}
	requestedPerms := DefaultRequestedPermissions()
	if permsRaw, ok := info["permissions"].([]interface{}); ok {
		requestedPerms = []XSWDPermission{}
		for _, p := range permsRaw {
			if pStr, ok := p.(string); ok {
				requestedPerms = append(requestedPerms, XSWDPermission(pStr))
			}
		}
	}
	
	// Build permission info for frontend display
	permInfos := make([]map[string]interface{}, 0, len(requestedPerms))
	for _, p := range requestedPerms {
		pi := GetPermissionInfo(p)
		permInfos = append(permInfos, map[string]interface{}{
			"id":          string(pi.ID),
			"name":        pi.Name,
			"description": pi.Description,
			"alwaysAsk":   pi.AlwaysAsk,
		})
	}

	// Check if we already have stored permissions for this origin
	pm := GetPermissionManager()
	var existingPerms map[string]bool
	if pm != nil {
		if app := pm.GetApp(origin); app != nil {
			existingPerms = make(map[string]bool)
			for p, granted := range app.Permissions {
				existingPerms[string(p)] = granted
			}
		}
	}

	// Check if this is a read-only request (no wallet permissions needed)
	isReadOnly := !HasAnyWalletPermission(requestedPerms)
	
	// Emit toast warning if no wallet is open AND app needs wallet access
	if !walletManager.isOpen && !isReadOnly {
		runtime.EventsEmit(s.app.ctx, "toast:show", map[string]interface{}{
			"type":    "warning",
			"message": "Connect a wallet to interact with " + appName,
		})
	}

	runtime.EventsEmit(s.app.ctx, "xswd:request", map[string]interface{}{
		"id":                   reqID,
		"type":                 "connect",
		"appName":              appName,
		"origin":               origin,
		"description":          description,
		"requestedPermissions": permInfos,
		"existingPermissions":  existingPerms,
		"isReadOnly":           isReadOnly,
	})

	resp := <-resChan

	s.pendingLock.Lock()
	delete(s.pendingRequests, reqID)
	s.pendingLock.Unlock()

	if err, ok := resp.(error); ok {
		s.sendRawJSON(conn, map[string]interface{}{
			"accepted": false,
			"error":    err.Error(),
		})
		return
	}

	message := "Wallet connection approved"
	var grantedPerms []XSWDPermission
	
	if respMap, ok := resp.(map[string]interface{}); ok {
		if msg, ok2 := respMap["message"].(string); ok2 {
			message = msg
		}
		// Extract granted permissions from response
		if perms, ok2 := respMap["permissions"].([]interface{}); ok2 {
			for _, p := range perms {
				if pStr, ok3 := p.(string); ok3 {
					grantedPerms = append(grantedPerms, XSWDPermission(pStr))
				}
			}
		}
	}
	
	// If no permissions explicitly granted, use requested permissions (backward compat)
	if len(grantedPerms) == 0 {
		grantedPerms = requestedPerms
	}
	
	// Store granted permissions
	if pm != nil && origin != "" {
		pm.GrantPermissions(origin, appName, description, grantedPerms)
		pm.SetActiveClient(origin, true)
	}
	
	// Store origin for this connection
	s.lock.Lock()
	s.clientOrigins[conn] = origin
	s.lock.Unlock()

	s.sendRawJSON(conn, map[string]interface{}{
		"accepted": true,
		"message":  message,
	})
}

func (s *XSWDServer) handleSigningRequest(conn *websocket.Conn, req JSONRPCRequest) {
	// Create channel for response
	resChan := make(chan interface{})
	reqID := fmt.Sprintf("%v", req.ID) // Use ID from request as key (simplification)
	if req.ID == nil {
		// Notification? Skip
		return 
	}
	
	// Parse params
	var paramsMap map[string]interface{}
	if err := json.Unmarshal(req.Params, &paramsMap); err != nil {
		s.sendResponse(conn, req.ID, nil, &JSONRPCError{Code: -32700, Message: "Parse error"})
		return
	}

	// Store request
	s.pendingLock.Lock()
	s.pendingRequests[reqID] = &XSWDPendingRequest{
		Method:   req.Method,
		Params:   paramsMap,
		RespChan: resChan,
	}
	s.pendingLock.Unlock()

	// Notify frontend
	log.Printf("[XSWD] Emitting xswd:request for %s", req.Method)
	runtime.EventsEmit(s.app.ctx, "xswd:request", map[string]interface{}{
		"id":      reqID,
		"method":  req.Method,
		"params":  paramsMap,
		"appName": "External dApp",
		"origin":  "Websocket",
	})

	// Wait for response (blocking this goroutine)
	resp := <-resChan

	// Clean up
	s.pendingLock.Lock()
	delete(s.pendingRequests, reqID)
	s.pendingLock.Unlock()

	// Send response
	if err, ok := resp.(error); ok {
		s.sendResponse(conn, req.ID, nil, &JSONRPCError{Code: -32000, Message: err.Error()})
	} else {
		// If result is map with error
		if rMap, ok := resp.(map[string]interface{}); ok && rMap["error"] != nil {
			s.sendResponse(conn, req.ID, nil, &JSONRPCError{Code: -32000, Message: fmt.Sprint(rMap["error"])})
		} else {
			s.sendResponse(conn, req.ID, resp, nil)
		}
	}
}

func (s *XSWDServer) sendResponse(conn *websocket.Conn, id interface{}, result interface{}, err *JSONRPCError) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}
	respBytes, _ := json.Marshal(resp)
	s.lock.Lock()
	conn.WriteMessage(websocket.TextMessage, respBytes)
	s.lock.Unlock()
}

func (s *XSWDServer) sendRawJSON(conn *websocket.Conn, payload interface{}) {
	respBytes, _ := json.Marshal(payload)
	s.lock.Lock()
	conn.WriteMessage(websocket.TextMessage, respBytes)
	s.lock.Unlock()
}

// ProcessApproval is called from App (legacy, no permissions)
func (s *XSWDServer) ProcessApproval(reqID string, approved bool, password string) {
	s.ProcessApprovalWithPermissions(reqID, approved, password, nil)
}

// ProcessApprovalWithPermissions is called from App with explicit permissions
func (s *XSWDServer) ProcessApprovalWithPermissions(reqID string, approved bool, password string, permissions []string) {
	s.pendingLock.Lock()
	req, ok := s.pendingRequests[reqID]
	s.pendingLock.Unlock()

	if !ok {
		log.Printf("[WARN] Unknown request ID approved: %s", reqID)
		return
	}

	if req.Method == "handshake" {
		if !approved {
			req.RespChan <- fmt.Errorf("User denied wallet connection")
		} else {
			// Include permissions in the response
			resp := map[string]interface{}{
				"message": "Wallet connection approved",
			}
			if permissions != nil {
				// Convert string slice to interface slice for JSON
				permInterface := make([]interface{}, len(permissions))
				for i, p := range permissions {
					permInterface[i] = p
				}
				resp["permissions"] = permInterface
			}
			req.RespChan <- resp
		}
		return
	}

	if !approved {
		req.RespChan <- fmt.Errorf("User denied request")
		return
	}

	// Execute wallet call
	// Use the App's InternalWalletCall
	res := s.app.InternalWalletCall(req.Method, req.Params, password)
	req.RespChan <- res
}

// IsWalletOpen returns whether a wallet is currently open (proxy to App)
func (s *XSWDServer) IsWalletOpen() bool {
	if s.app == nil {
		return false
	}
	return s.app.IsWalletOpen()
}

// GetWalletAddress returns the current wallet address (proxy to walletManager)
func (s *XSWDServer) GetWalletAddress() string {
	walletManager.RLock()
	defer walletManager.RUnlock()
	if !walletManager.isOpen || walletManager.wallet == nil {
		return ""
	}
	return walletManager.wallet.GetAddress().String()
}

// GetWalletBalance returns the current wallet balance (proxy to walletManager)
func (s *XSWDServer) GetWalletBalance() (uint64, error) {
	walletManager.RLock()
	defer walletManager.RUnlock()
	if !walletManager.isOpen || walletManager.wallet == nil {
		return 0, fmt.Errorf("wallet not open")
	}
	matureBalance, _ := walletManager.wallet.Get_Balance()
	return matureBalance, nil
}

// ==================== Subscription System ====================

// startSubscriptionPusher runs a background loop to push events to subscribed clients
func (s *XSWDServer) startSubscriptionPusher() {
	s.pusherRunning = true
	ticker := time.NewTicker(2 * time.Second) // Check every 2 seconds
	defer ticker.Stop()
	
	log.Println("[NET] Starting XSWD subscription pusher")
	
	for {
		select {
		case <-s.stopPusher:
			log.Println("[NET] Stopping XSWD subscription pusher")
			return
		case <-ticker.C:
			s.checkAndPushEvents()
		}
	}
}

// checkAndPushEvents checks for state changes and pushes events to subscribers
func (s *XSWDServer) checkAndPushEvents() {
	s.lock.RLock()
	clientCount := len(s.clients)
	s.lock.RUnlock()
	
	if clientCount == 0 {
		return // No clients connected
	}
	
	// Check for new_topoheight (block height change)
	if s.app != nil {
		currentHeight := s.getCurrentTopoheight()
		if currentHeight > 0 && currentHeight != s.lastTopoheight {
			s.lastTopoheight = currentHeight
			s.pushEvent(SubNewTopoheight, map[string]interface{}{
				"topoheight": currentHeight,
			})
		}
	}
	
	// Check for new_balance (wallet balance change)
	if walletManager.isOpen && walletManager.wallet != nil {
		currentBalance, _ := walletManager.wallet.Get_Balance()
		if currentBalance != s.lastBalance {
			s.lastBalance = currentBalance
			s.pushEvent(SubNewBalance, map[string]interface{}{
				"balance": currentBalance,
			})
		}
	}
	
	// Note: new_entry would require tracking transaction history
	// This is more complex and would need wallet tx monitoring
}

// getCurrentTopoheight gets the current blockchain height
func (s *XSWDServer) getCurrentTopoheight() int64 {
	if s.app == nil {
		return 0
	}
	
	// Try to get from cached stats first
	stats := s.app.GetLiveStats()
	if stats == nil {
		return 0
	}
	
	if height, ok := stats["topoheight"].(int64); ok {
		return height
	}
	if height, ok := stats["topoheight"].(float64); ok {
		return int64(height)
	}
	return 0
}

// pushEvent sends an event to all clients subscribed to that event type
func (s *XSWDServer) pushEvent(eventType SubscriptionType, data map[string]interface{}) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	
	for conn, subs := range s.clientSubscriptions {
		if subs == nil {
			continue
		}
		
		shouldPush := false
		switch eventType {
		case SubNewTopoheight:
			shouldPush = subs.NewTopoheight
		case SubNewBalance:
			shouldPush = subs.NewBalance
		case SubNewEntry:
			shouldPush = subs.NewEntry
		}
		
		if shouldPush {
			// Send event as JSON-RPC notification (no id)
			notification := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  string(eventType),
				"params":  data,
			}
			
			notifBytes, err := json.Marshal(notification)
			if err != nil {
				continue
			}
			
			// Don't hold the read lock while writing
			go func(c *websocket.Conn, msg []byte) {
				s.lock.Lock()
				c.WriteMessage(websocket.TextMessage, msg)
				s.lock.Unlock()
			}(conn, notifBytes)
		}
	}
}

// GetActiveConnections returns info about active XSWD connections (for UI)
func (s *XSWDServer) GetActiveConnections() []map[string]interface{} {
	s.lock.RLock()
	defer s.lock.RUnlock()
	
	connections := []map[string]interface{}{}
	pm := GetPermissionManager()
	
	for conn, origin := range s.clientOrigins {
		connInfo := map[string]interface{}{
			"origin": origin,
			"active": true,
		}
		
		// Add subscription info
		if subs := s.clientSubscriptions[conn]; subs != nil {
			connInfo["subscriptions"] = map[string]bool{
				"new_topoheight": subs.NewTopoheight,
				"new_balance":    subs.NewBalance,
				"new_entry":      subs.NewEntry,
			}
		}
		
		// Add permission info from permission manager
		if pm != nil && origin != "" {
			if app := pm.GetApp(origin); app != nil {
				connInfo["appName"] = app.Name
				connInfo["permissions"] = app.Permissions
			}
		}
		
		connections = append(connections, connInfo)
	}
	
	return connections
}
