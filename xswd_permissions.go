package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/deroproject/graviton"
)

// Permission types for XSWD
type XSWDPermission string

const (
	PermissionReadPublicData  XSWDPermission = "read_public_data" // Read-only daemon data (GetInfo, GetBlock, etc.)
	PermissionViewAddress     XSWDPermission = "view_address"
	PermissionViewBalance     XSWDPermission = "view_balance"
	PermissionSignTransaction XSWDPermission = "sign_transaction"
	PermissionSCInvoke        XSWDPermission = "sc_invoke"
)

// AllPermissions returns all defined permission types
func AllPermissions() []XSWDPermission {
	return []XSWDPermission{
		PermissionReadPublicData,
		PermissionViewAddress,
		PermissionViewBalance,
		PermissionSignTransaction,
		PermissionSCInvoke,
	}
}

// PermissionInfo provides human-readable info about a permission
type PermissionInfo struct {
	ID          XSWDPermission `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	AlwaysAsk   bool           `json:"alwaysAsk"` // If true, requires per-action approval even when granted
}

// GetPermissionInfo returns metadata about a permission
func GetPermissionInfo(p XSWDPermission) PermissionInfo {
	switch p {
	case PermissionReadPublicData:
		return PermissionInfo{
			ID:          p,
			Name:        "Read Public Blockchain Data",
			Description: "Can read public blockchain info (blocks, transactions, network stats)",
			AlwaysAsk:   false,
		}
	case PermissionViewAddress:
		return PermissionInfo{
			ID:          p,
			Name:        "View Wallet Address",
			Description: "Can see your public wallet address",
			AlwaysAsk:   false,
		}
	case PermissionViewBalance:
		return PermissionInfo{
			ID:          p,
			Name:        "View Balance & History",
			Description: "Can see your balance and full transfer history — senders, recipients, amounts, payment proofs, comments and your private labels",
			AlwaysAsk:   false,
		}
	case PermissionSignTransaction:
		return PermissionInfo{
			ID:          p,
			Name:        "Sign Transactions",
			Description: "Can request to send DERO (requires approval each time)",
			AlwaysAsk:   true,
		}
	case PermissionSCInvoke:
		return PermissionInfo{
			ID:          p,
			Name:        "Smart Contract Calls",
			Description: "Can request smart contract interactions (requires approval each time)",
			AlwaysAsk:   true,
		}
	default:
		return PermissionInfo{
			ID:          p,
			Name:        string(p),
			Description: "Unknown permission",
			AlwaysAsk:   true,
		}
	}
}

// ConnectedApp represents a dApp that has connected via XSWD
type ConnectedApp struct {
	Origin       string                  `json:"origin"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description,omitempty"`
	Permissions  map[XSWDPermission]bool `json:"permissions"`
	GrantedAt    int64                   `json:"grantedAt"`
	LastAccessed int64                   `json:"lastAccessed"`
}

// PermissionManager handles XSWD permission storage and checking
type PermissionManager struct {
	sync.RWMutex
	store         *graviton.Store
	apps          map[string]*ConnectedApp // origin -> app
	activeClients map[string]bool          // origin -> is currently connected
}

var permissionManager *PermissionManager

// InitPermissionManager initializes the global permission manager
func InitPermissionManager(cache *GravitonCache) {
	pm := &PermissionManager{
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}

	// Use the same Graviton store as the cache
	if cache != nil {
		pm.store = cache.store
	}

	// Load persisted permissions
	pm.loadFromStorage()

	permissionManager = pm
}

// GetPermissionManager returns the global permission manager
func GetPermissionManager() *PermissionManager {
	return permissionManager
}

// loadFromStorage loads persisted permissions from Graviton
func (pm *PermissionManager) loadFromStorage() {
	if pm.store == nil {
		return
	}

	ss, err := pm.store.LoadSnapshot(0)
	if err != nil {
		return
	}

	tree, _ := ss.GetTree("xswd_permissions")
	if tree == nil {
		return
	}

	// Iterate all keys to load apps
	cursor := tree.Cursor()
	for k, v, err := cursor.First(); err == nil; k, v, err = cursor.Next() {
		if k == nil {
			break
		}

		var app ConnectedApp
		if err := json.Unmarshal(v, &app); err == nil {
			pm.apps[string(k)] = &app
		}
	}
}

// saveToStorage persists a single app's permissions to Graviton
func (pm *PermissionManager) saveToStorage(app *ConnectedApp) error {
	if pm.store == nil {
		return fmt.Errorf("storage not initialized")
	}

	ss, err := pm.store.LoadSnapshot(0)
	if err != nil {
		return err
	}

	tree, _ := ss.GetTree("xswd_permissions")

	data, err := json.Marshal(app)
	if err != nil {
		return err
	}

	if err := tree.Put([]byte(app.Origin), data); err != nil {
		return err
	}

	_, err = graviton.Commit(tree)
	return err
}

// deleteFromStorage removes an app's permissions from Graviton
func (pm *PermissionManager) deleteFromStorage(origin string) error {
	if pm.store == nil {
		return fmt.Errorf("storage not initialized")
	}

	ss, err := pm.store.LoadSnapshot(0)
	if err != nil {
		return err
	}

	tree, _ := ss.GetTree("xswd_permissions")

	if err := tree.Delete([]byte(origin)); err != nil {
		return err
	}

	_, err = graviton.Commit(tree)
	return err
}

// GrantPermissions stores permissions for an app
func (pm *PermissionManager) GrantPermissions(origin, name, description string, permissions []XSWDPermission) error {
	pm.Lock()
	defer pm.Unlock()

	now := time.Now().Unix()

	app, exists := pm.apps[origin]
	if !exists {
		app = &ConnectedApp{
			Origin:      origin,
			Name:        name,
			Description: description,
			Permissions: make(map[XSWDPermission]bool),
			GrantedAt:   now,
		}
		pm.apps[origin] = app
	} else {
		// Update name/description if provided
		if name != "" {
			app.Name = name
		}
		if description != "" {
			app.Description = description
		}
	}

	// Replace the permission set — connect approval is authoritative.
	// Additive grants made unchecked boxes in the connect modal theater.
	app.Permissions = make(map[XSWDPermission]bool, len(permissions))
	for _, p := range permissions {
		app.Permissions[p] = true
	}
	app.LastAccessed = now

	return pm.saveToStorage(app)
}

// DenyUnlessPermission returns a JSON-RPC error when the connection has not
// completed handshake (empty origin) or lacks the required permission.
// Empty origin must deny — the old "origin != \"\" &&" guard skipped checks
// entirely for unauthenticated sockets (R2-B1).
func DenyUnlessPermission(origin string, perm XSWDPermission) *JSONRPCError {
	if origin == "" {
		return &JSONRPCError{Code: -32003, Message: "Permission denied: XSWD handshake required"}
	}
	pm := GetPermissionManager()
	if pm == nil {
		return &JSONRPCError{Code: -32003, Message: "Permission denied: permission manager unavailable"}
	}
	if !pm.HasPermission(origin, perm) {
		permInfo := GetPermissionInfo(perm)
		return &JSONRPCError{Code: -32003, Message: fmt.Sprintf("Permission denied: %s permission not granted", permInfo.Name)}
	}
	return nil
}

// RevokePermission removes a specific permission from an app
func (pm *PermissionManager) RevokePermission(origin string, permission XSWDPermission) error {
	pm.Lock()
	defer pm.Unlock()

	app, exists := pm.apps[origin]
	if !exists {
		return nil
	}

	delete(app.Permissions, permission)
	return pm.saveToStorage(app)
}

// RevokeAllPermissions removes all permissions for an app
func (pm *PermissionManager) RevokeAllPermissions(origin string) error {
	pm.Lock()
	defer pm.Unlock()

	if _, exists := pm.apps[origin]; !exists {
		return nil
	}

	delete(pm.apps, origin)
	delete(pm.activeClients, origin)
	return pm.deleteFromStorage(origin)
}

// HasPermission checks if an app has a specific permission.
//
// Takes the full write lock, not RLock: this also refreshes LastAccessed, and a
// field write under RLock races the struct-copy reads in GetApp/GetAllApps
// (which run concurrently under RLock). The critical section is two map lookups
// plus a timestamp assignment, and this is a per-request call (not a hot loop),
// so serializing it is effectively free.
func (pm *PermissionManager) HasPermission(origin string, permission XSWDPermission) bool {
	pm.Lock()
	defer pm.Unlock()

	app, exists := pm.apps[origin]
	if !exists {
		return false
	}

	app.LastAccessed = time.Now().Unix()

	return app.Permissions[permission]
}

// GetApp returns a connected app by origin
func (pm *PermissionManager) GetApp(origin string) *ConnectedApp {
	pm.RLock()
	defer pm.RUnlock()

	if app, exists := pm.apps[origin]; exists {
		// Return a copy to avoid race conditions
		appCopy := *app
		permCopy := make(map[XSWDPermission]bool)
		for k, v := range app.Permissions {
			permCopy[k] = v
		}
		appCopy.Permissions = permCopy
		return &appCopy
	}
	return nil
}

// GetAllApps returns all connected apps
func (pm *PermissionManager) GetAllApps() []*ConnectedApp {
	pm.RLock()
	defer pm.RUnlock()

	apps := make([]*ConnectedApp, 0, len(pm.apps))
	for _, app := range pm.apps {
		// Return copies
		appCopy := *app
		permCopy := make(map[XSWDPermission]bool)
		for k, v := range app.Permissions {
			permCopy[k] = v
		}
		appCopy.Permissions = permCopy
		apps = append(apps, &appCopy)
	}
	return apps
}

// SetActiveClient marks a client as actively connected
func (pm *PermissionManager) SetActiveClient(origin string, active bool) {
	pm.Lock()
	defer pm.Unlock()

	if active {
		pm.activeClients[origin] = true
	} else {
		delete(pm.activeClients, origin)
	}
}

// IsClientActive checks if a client is currently connected
func (pm *PermissionManager) IsClientActive(origin string) bool {
	pm.RLock()
	defer pm.RUnlock()

	return pm.activeClients[origin]
}

// GetActiveClients returns all currently connected app origins
func (pm *PermissionManager) GetActiveClients() []string {
	pm.RLock()
	defer pm.RUnlock()

	clients := make([]string, 0, len(pm.activeClients))
	for origin := range pm.activeClients {
		clients = append(clients, origin)
	}
	return clients
}

// GetRequiredPermission returns the permission required for a given XSWD method
func GetRequiredPermission(method string) XSWDPermission {
	switch method {
	case "GetAddress", "DERO.GetAddress",
		"GetPublicKey",
		"MakeIntegratedAddress", "SplitIntegratedAddress":
		return PermissionViewAddress
	// The daemon endpoint is a gateway to PUBLIC chain data and reveals nothing about the
	// wallet, so demanding "View Wallet Address" for it mislabels what is being granted.
	// It still needs a gate: handing an app the endpoint lets it query the node directly,
	// outside HOLOGRAM, and discloses a custom remote daemon if one is configured.
	case "GetDaemon", "DERO.GetDaemon":
		return PermissionReadPublicData
	case "GetBalance", "DERO.GetBalance",
		"GetHeight",
		"GetTransfers", "GetTransferbyTXID":
		return PermissionViewBalance
	case "transfer", "Transfer", "DERO.Transfer":
		return PermissionSignTransaction
	case "scinvoke", "SC_Invoke", "DERO.SC_Invoke":
		return PermissionSCInvoke
	case "SignData", "DecryptPayload":
		return PermissionSignTransaction
	// Read-only daemon methods - no wallet needed.
	// DERO.GetHeight is the daemon-side chain-tip block height (public data),
	// distinct from the wallet-side "GetHeight" above which returns the
	// wallet's last-seen sync height (genuinely wallet state). Grouping
	// DERO.GetHeight with PermissionViewBalance over-restricted public
	// chain reads; the reference XSWD impl (walletapi/xswd/xswd.go:651-693)
	// treats all DERO.* daemon proxies as always-permitted post-handshake.
	case "DERO.GetInfo", "GetInfo",
		"DERO.GetBlock", "GetBlock",
		"DERO.GetBlockHeaderByHash", "GetBlockHeaderByHash",
		"DERO.GetBlockHeaderByTopoHeight", "GetBlockHeaderByTopoHeight",
		"DERO.GetHeight",
		"DERO.GetTxPool", "GetTxPool",
		"DERO.GetTransaction", "GetTransaction",
		"DERO.GetRandomAddress", "GetRandomAddress",
		"DERO.GetSC", "GetSC",
		"DERO.GetGasEstimate", "GetGasEstimate",
		"DERO.NameToAddress", "NameToAddress":
		return PermissionReadPublicData
	default:
		return ""
	}
}

// RequiresWallet returns true if the permission requires wallet access
func RequiresWallet(p XSWDPermission) bool {
	switch p {
	case PermissionViewAddress, PermissionViewBalance, PermissionSignTransaction, PermissionSCInvoke:
		return true
	default:
		return false
	}
}

// DefaultRequestedPermissions returns the default permissions a dApp requests if not specified
// Now defaults to read-only only - apps must explicitly request wallet permissions
func DefaultRequestedPermissions() []XSWDPermission {
	return []XSWDPermission{
		PermissionReadPublicData,
	}
}

// ParseRequestedPermissions reads the permission list a dApp sends in its handshake.
// Canonical XSWD sends a MAP (permission -> bool); older clients send an array. Handling
// only the array form left every spec-compliant dApp holding the one-entry default, which
// unlocks no wallet method at all. Unknown ids are dropped so a page cannot smuggle its own
// wording onto the consent sheet, and an absent or unusable list falls back to public data
// only — never to the full vocabulary, which would turn a request into an upgrade.
func ParseRequestedPermissions(raw interface{}) []XSWDPermission {
	if raw == nil {
		return DefaultRequestedPermissions()
	}
	known := make(map[XSWDPermission]bool, len(AllPermissions()))
	for _, p := range AllPermissions() {
		known[p] = true
	}
	parsed := []XSWDPermission{}
	switch v := raw.(type) {
	case []interface{}:
		for _, p := range v {
			if s, ok := p.(string); ok && known[XSWDPermission(s)] {
				parsed = append(parsed, XSWDPermission(s))
			}
		}
	case map[string]interface{}:
		for _, p := range AllPermissions() {
			if wanted, ok := v[string(p)].(bool); ok && wanted {
				parsed = append(parsed, p)
			}
		}
	}
	if len(parsed) == 0 {
		return DefaultRequestedPermissions()
	}
	return parsed
}

// OriginNamespaceXSWD prefixes grant keys for dApp-supplied WebSocket origins.
const OriginNamespaceXSWD = "xswd:"

// XSWDOriginKey namespaces a dApp-supplied origin. Without this a page can hand us
// url:"<a trusted SCID>" and REPLACE (GrantPermissions overwrites) the stored grant
// record of a browser/TELA app it does not control.
func XSWDOriginKey(raw string) string {
	return OriginNamespaceXSWD + strings.TrimSpace(raw)
}

// HasAnyWalletPermission returns true if the permission list includes any wallet-related permissions
func HasAnyWalletPermission(perms []XSWDPermission) bool {
	for _, p := range perms {
		if RequiresWallet(p) {
			return true
		}
	}
	return false
}
