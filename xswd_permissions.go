package main

import (
	"crypto/sha256"
	"encoding/hex"
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

// CanStorePermission reports whether a permission may be persisted as a standing grant.
//
// Spending is an action, not a door you can leave open: transfer, scinvoke and SignData are
// approved per action, showing the amount, destination and entrypoint every time. Storing
// them would let one click buy permanent spend rights, which is the whole failure the
// consent sheet was replaced to remove. Engram permits it; HOLOGRAM deliberately does not.
//
// This is the floor rather than a UI convention: it is enforced where grants are written, so
// a compromised renderer cannot persist what the modal declines to offer.
func CanStorePermission(p XSWDPermission) bool {
	switch p {
	case PermissionSignTransaction, PermissionSCInvoke:
		return false
	default:
		return true
	}
}

// walletFingerprint identifies the wallet a standing grant belongs to. Indirected through a
// variable so tests can pin an identity without opening a wallet on disk.
var walletFingerprint = currentWalletFingerprint

// currentWalletFingerprint returns a stable opaque id for the open wallet, or "" when none is.
//
// The address is HASHED rather than recorded. The permission store lives in the app data
// directory beside the caches, and recent-wallets already limits itself to a 16-character
// address PREFIX (addToRecentWalletsWithInfo), so writing a full address here would make this
// the one place HOLOGRAM keeps one in the clear. Hashing costs nothing and keeps the store
// unreadable to anyone who does not already know the address.
//
// Same wallet on a different network yields a different address (dero1… vs deto1…) and so a
// different fingerprint — a grant made against the simulator does not carry to mainnet.
//
// LOCK ORDER: this takes walletManager's read lock, so it must be called BEFORE the
// PermissionManager lock, never under it. A caller already holding walletManager.Lock() —
// InternalWalletCall holds it for its whole body — must resolve permissions before locking,
// or Go's non-reentrant RWMutex will deadlock the goroutine against itself.
func currentWalletFingerprint() string {
	walletManager.RLock()
	defer walletManager.RUnlock()

	if !walletManager.isOpen || walletManager.wallet == nil {
		return ""
	}
	return fingerprintForAddress(walletManager.wallet.GetAddress().String())
}

// fingerprintForAddress hashes a wallet address into the id used as a grant key.
func fingerprintForAddress(address string) string {
	if address == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(address))
	return hex.EncodeToString(sum[:])[:16]
}

// ConnectedApp represents a dApp that has connected via XSWD.
//
// Permissions holds only the doors that are not about the wallet — connecting grants public
// chain data, and that answer is the same whichever wallet happens to be open (or none).
// WalletPermissions holds the rest, filed under the fingerprint of the wallet that granted
// them: "remember this" has to mean "for THIS wallet", or approving an app under one identity
// silently hands it the next one you open.
type ConnectedApp struct {
	Origin      string                  `json:"origin"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Permissions map[XSWDPermission]bool `json:"permissions"`
	// fingerprint -> permissions granted while that wallet was open
	WalletPermissions map[string]map[XSWDPermission]bool `json:"walletPermissions,omitempty"`
	GrantedAt         int64                              `json:"grantedAt"`
	LastAccessed      int64                              `json:"lastAccessed"`
}

// permissionsForWallet flattens the doors this app holds for one wallet: the
// wallet-independent set plus whatever that fingerprint granted. An empty fingerprint (no
// wallet open) yields the wallet-independent set alone.
func (app *ConnectedApp) permissionsForWallet(fingerprint string) map[XSWDPermission]bool {
	out := make(map[XSWDPermission]bool, len(app.Permissions)+2)
	for p, granted := range app.Permissions {
		if granted {
			out[p] = true
		}
	}
	if fingerprint == "" {
		return out
	}
	for p, granted := range app.WalletPermissions[fingerprint] {
		if granted {
			out[p] = true
		}
	}
	return out
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
			// Records written before grants were scoped hold wallet doors in the
			// wallet-independent set, where there is nothing to say WHICH wallet approved
			// them. Drop them: a grant that cannot be attributed must not be honoured, and
			// leaving it visible in Settings would advertise a door that opens nothing —
			// the permission theatre the consent rebuild exists to remove. Cost is one
			// re-prompt per app per wallet, asked at the moment of use.
			for p := range app.Permissions {
				if RequiresWallet(p) {
					delete(app.Permissions, p)
				}
			}
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
	// Resolved before the lock — see the LOCK ORDER note on currentWalletFingerprint.
	fingerprint := walletFingerprint()

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
	// Non-storable permissions are dropped here rather than at the caller, so every
	// write path (browser connect, WebSocket connect) inherits the rule.
	//
	// The replace is scoped to the granting wallet: it clears this wallet's doors and the
	// wallet-independent ones, and leaves every OTHER wallet's grants alone. Wiping those
	// would let connecting under one identity silently revoke another's.
	app.Permissions = make(map[XSWDPermission]bool, len(permissions))
	if fingerprint != "" {
		if app.WalletPermissions == nil {
			app.WalletPermissions = make(map[string]map[XSWDPermission]bool, 1)
		}
		app.WalletPermissions[fingerprint] = make(map[XSWDPermission]bool, len(permissions))
	}
	for _, p := range permissions {
		if !CanStorePermission(p) {
			continue
		}
		if !RequiresWallet(p) {
			app.Permissions[p] = true
			continue
		}
		// A wallet door with no wallet open has nothing to attach to; silently storing it
		// globally is exactly the leak this scoping closes.
		if fingerprint == "" {
			continue
		}
		app.WalletPermissions[fingerprint][p] = true
	}
	app.LastAccessed = now

	return pm.saveToStorage(app)
}

// AddPermission grants a single permission WITHOUT disturbing the ones already held.
//
// This is what "Always allow" calls. GrantPermissions cannot be reused for it: that replaces
// the whole set, so answering "always" to a balance prompt would silently revoke an address
// grant the user had already given.
func (pm *PermissionManager) AddPermission(origin, name, description string, permission XSWDPermission) error {
	if origin == "" {
		return fmt.Errorf("origin required")
	}
	if !CanStorePermission(permission) {
		return fmt.Errorf("permission %q is approved per action and cannot be stored", permission)
	}

	// Resolved before the lock — see the LOCK ORDER note on currentWalletFingerprint.
	fingerprint := walletFingerprint()
	// "Always allow" on a wallet door has to record WHICH wallet allowed it. With none open
	// there is no answer, so refuse rather than file it somewhere every wallet can read.
	if RequiresWallet(permission) && fingerprint == "" {
		return fmt.Errorf("permission %q needs an open wallet to be remembered", permission)
	}

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
		if name != "" {
			app.Name = name
		}
		if description != "" {
			app.Description = description
		}
		if app.Permissions == nil {
			app.Permissions = make(map[XSWDPermission]bool)
		}
	}

	if RequiresWallet(permission) {
		if app.WalletPermissions == nil {
			app.WalletPermissions = make(map[string]map[XSWDPermission]bool, 1)
		}
		if app.WalletPermissions[fingerprint] == nil {
			app.WalletPermissions[fingerprint] = make(map[XSWDPermission]bool, 1)
		}
		app.WalletPermissions[fingerprint][permission] = true
	} else {
		app.Permissions[permission] = true
	}
	app.LastAccessed = now

	return pm.saveToStorage(app)
}

// XSWDPermissionDenied is the canonical XSWD code for a refused permission
// (walletapi/xswd/xswd.go). Denials previously went out as -32003, which dApps that branch
// on the spec code — Villager does — could not tell apart from any other failure.
const XSWDPermissionDenied = -32043

// DenyUnlessHandshake returns a JSON-RPC error when the connection has not completed the
// handshake. Used by the signing methods, which are gated by per-action approval rather than
// by a standing grant, but must still refuse an unauthenticated socket (R2-B1).
func DenyUnlessHandshake(origin string) *JSONRPCError {
	if origin == "" {
		return &JSONRPCError{Code: -32003, Message: "Permission denied: XSWD handshake required"}
	}
	return nil
}

// DenyUnlessPermission returns a JSON-RPC error when the connection has not
// completed handshake (empty origin) or lacks the required permission.
// Empty origin must deny — the old "origin != \"\" &&" guard skipped checks
// entirely for unauthenticated sockets (R2-B1).
func DenyUnlessPermission(origin string, perm XSWDPermission) *JSONRPCError {
	// Handshake-required keeps -32003: it is a different condition from a permission the
	// user declined, and the caller has nothing to prompt for yet.
	if err := DenyUnlessHandshake(origin); err != nil {
		return err
	}
	pm := GetPermissionManager()
	if pm == nil {
		return &JSONRPCError{Code: XSWDPermissionDenied, Message: "Permission denied: permission manager unavailable"}
	}
	if !pm.HasPermission(origin, perm) {
		permInfo := GetPermissionInfo(perm)
		return &JSONRPCError{Code: XSWDPermissionDenied, Message: fmt.Sprintf("Permission denied: %s permission not granted", permInfo.Name)}
	}
	return nil
}

// RevokePermission removes a specific permission from an app.
//
// A wallet door is revoked for the wallet that is open, matching what Settings displays.
// Other wallets keep their own answer; "Revoke All & Disconnect" is the control that forgets
// the app everywhere.
func (pm *PermissionManager) RevokePermission(origin string, permission XSWDPermission) error {
	// Resolved before the lock — see the LOCK ORDER note on currentWalletFingerprint.
	fingerprint := ""
	if RequiresWallet(permission) {
		fingerprint = walletFingerprint()
	}

	pm.Lock()
	defer pm.Unlock()

	app, exists := pm.apps[origin]
	if !exists {
		return nil
	}

	if fingerprint != "" {
		delete(app.WalletPermissions[fingerprint], permission)
	}
	// Also clear the wallet-independent slot: pre-scoping records could hold a wallet door
	// there, and a revoke the user asked for must not leave a copy behind.
	delete(app.Permissions, permission)
	return pm.saveToStorage(app)
}

// RevokeAllPermissions removes an app's record entirely — every wallet, not just the open
// one. This is what "Revoke All & Disconnect" calls, and the button says so: a control that
// forgets less than the user expects is worse than one that forgets more.
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
	// Resolved before the lock — see the LOCK ORDER note on currentWalletFingerprint.
	fingerprint := ""
	if RequiresWallet(permission) {
		fingerprint = walletFingerprint()
		// No wallet open, no wallet door. The idle auto-lock closes the handle, so a
		// standing grant stops being honoured until the user unlocks again.
		if fingerprint == "" {
			return false
		}
	}

	pm.Lock()
	defer pm.Unlock()

	app, exists := pm.apps[origin]
	if !exists {
		return false
	}

	app.LastAccessed = time.Now().Unix()

	if fingerprint != "" {
		return app.WalletPermissions[fingerprint][permission]
	}
	return app.Permissions[permission]
}

// cloneApp deep-copies a record so callers cannot race the live maps. Every nested map has
// to be copied, not just the top-level struct — a shallow copy hands out the same inner map
// the write paths mutate under lock.
func cloneApp(app *ConnectedApp) *ConnectedApp {
	appCopy := *app

	permCopy := make(map[XSWDPermission]bool, len(app.Permissions))
	for k, v := range app.Permissions {
		permCopy[k] = v
	}
	appCopy.Permissions = permCopy

	if app.WalletPermissions != nil {
		walletCopy := make(map[string]map[XSWDPermission]bool, len(app.WalletPermissions))
		for fp, perms := range app.WalletPermissions {
			inner := make(map[XSWDPermission]bool, len(perms))
			for k, v := range perms {
				inner[k] = v
			}
			walletCopy[fp] = inner
		}
		appCopy.WalletPermissions = walletCopy
	}
	return &appCopy
}

// GetApp returns a connected app by origin, with every wallet's grants intact.
func (pm *PermissionManager) GetApp(origin string) *ConnectedApp {
	pm.RLock()
	defer pm.RUnlock()

	if app, exists := pm.apps[origin]; exists {
		return cloneApp(app)
	}
	return nil
}

// GetAllApps returns all connected apps, UNFILTERED — every wallet's grants included.
//
// Deliberately not scoped to the open wallet: this feeds the storage reset (app_storage.go),
// which promises to clear XSWD permissions, and the storage usage count. Filtering here would
// leave other wallets' grants on disk while reporting them gone. Display paths want
// GetAppsForCurrentWallet instead.
func (pm *PermissionManager) GetAllApps() []*ConnectedApp {
	pm.RLock()
	defer pm.RUnlock()

	apps := make([]*ConnectedApp, 0, len(pm.apps))
	for _, app := range pm.apps {
		apps = append(apps, cloneApp(app))
	}
	return apps
}

// AppView is a connected app as it applies to one wallet: the doors actually in force,
// flattened, with the other wallets' answers left out of the picture.
type AppView struct {
	Origin       string
	Name         string
	Description  string
	Permissions  map[XSWDPermission]bool
	GrantedAt    int64
	LastAccessed int64
}

// GetAppsForCurrentWallet returns every connected app with its permissions collapsed to what
// the currently open wallet holds. This is the view Settings and the browser consent gate
// read: a door granted under another identity must not read as granted here.
//
// Apps are still listed even when they hold nothing for this wallet — the connection is real,
// it just has no wallet doors open yet, and hiding it would make a returning app look new.
func (pm *PermissionManager) GetAppsForCurrentWallet() []*AppView {
	// Resolved before the lock — see the LOCK ORDER note on currentWalletFingerprint.
	fingerprint := walletFingerprint()

	pm.RLock()
	defer pm.RUnlock()

	views := make([]*AppView, 0, len(pm.apps))
	for _, app := range pm.apps {
		views = append(views, &AppView{
			Origin:       app.Origin,
			Name:         app.Name,
			Description:  app.Description,
			Permissions:  app.permissionsForWallet(fingerprint),
			GrantedAt:    app.GrantedAt,
			LastAccessed: app.LastAccessed,
		})
	}
	return views
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
