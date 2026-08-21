// Copyright 2025 HOLOGRAM Project. All rights reserved.
//
// Standing grants are scoped to the wallet that approved them. Before this, "Always allow"
// was filed against the app alone: approving an app under one wallet and then opening a
// different one handed it the new identity with no prompt — observed live against a mainnet
// wallet and villager.tela.
//
// Every test here fails if the corresponding guard is reverted.

package main

import (
	"encoding/json"
	"testing"

	"github.com/deroproject/graviton"
)

// pinWallet fakes an open wallet for the duration of a test. walletFingerprint is a variable
// precisely so this does not need a real wallet file, a password, or a daemon.
func pinWallet(t *testing.T, fingerprint string) {
	t.Helper()
	prev := walletFingerprint
	walletFingerprint = func() string { return fingerprint }
	t.Cleanup(func() { walletFingerprint = prev })
}

const (
	walletA = "aaaaaaaaaaaaaaaa"
	walletB = "bbbbbbbbbbbbbbbb"
)

// The leak this whole change exists to close.
func TestHasPermission_WalletDoorScopedToGrantingWallet(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	pinWallet(t, walletA)
	if err := pm.AddPermission("villager.tela", "Villager", "", PermissionViewAddress); err != nil {
		t.Fatalf("granting under wallet A: %v", err)
	}
	if !pm.HasPermission("villager.tela", PermissionViewAddress) {
		t.Fatal("wallet A granted view_address and should hold it")
	}

	pinWallet(t, walletB)
	if pm.HasPermission("villager.tela", PermissionViewAddress) {
		t.Fatal("wallet B never approved this app; a grant made under wallet A must not carry over")
	}

	pinWallet(t, "")
	if pm.HasPermission("villager.tela", PermissionViewAddress) {
		t.Fatal("no wallet open means no wallet door — the idle auto-lock relies on this")
	}
}

// Connecting is not a question about the wallet, so it must not be re-asked per wallet.
func TestHasPermission_PublicDataIsNotWalletScoped(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	pinWallet(t, walletA)
	if err := pm.AddPermission("villager.tela", "Villager", "", PermissionReadPublicData); err != nil {
		t.Fatalf("granting read_public_data: %v", err)
	}

	for _, fp := range []string{walletA, walletB, ""} {
		pinWallet(t, fp)
		if !pm.HasPermission("villager.tela", PermissionReadPublicData) {
			t.Fatalf("read_public_data is chain data, not wallet data; it must survive wallet %q", fp)
		}
	}
}

// "Always allow" has to record WHICH wallet allowed it, or it is the old unscoped grant again.
func TestAddPermission_RefusesWalletDoorWithNoWalletOpen(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	pinWallet(t, "")
	if err := pm.AddPermission("villager.tela", "Villager", "", PermissionViewAddress); err == nil {
		t.Fatal("a wallet door remembered with no wallet open cannot be attributed and must be refused")
	}

	// The wallet-independent door is still storable with nothing open.
	if err := pm.AddPermission("villager.tela", "Villager", "", PermissionReadPublicData); err != nil {
		t.Fatalf("read_public_data needs no wallet: %v", err)
	}
}

// GrantPermissions REPLACES the set. Scoping means it replaces this wallet's doors, not
// everyone's — reconnecting under wallet B must not revoke what wallet A chose to remember.
func TestGrantPermissions_LeavesOtherWalletsDoorsAlone(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	pinWallet(t, walletA)
	if err := pm.AddPermission("villager.tela", "Villager", "", PermissionViewAddress); err != nil {
		t.Fatalf("granting under wallet A: %v", err)
	}

	pinWallet(t, walletB)
	if err := pm.GrantPermissions("villager.tela", "Villager", "", []XSWDPermission{PermissionReadPublicData}); err != nil {
		t.Fatalf("reconnect under wallet B: %v", err)
	}

	pinWallet(t, walletA)
	if !pm.HasPermission("villager.tela", PermissionViewAddress) {
		t.Fatal("wallet B reconnecting silently revoked a door wallet A had remembered")
	}
}

// Records written before scoping hold wallet doors with nothing to say who approved them.
func TestLoadFromStorage_DropsUnattributedWalletGrants(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	legacy := map[string]interface{}{
		"origin": "villager.tela",
		"name":   "Villager",
		"permissions": map[string]bool{
			string(PermissionReadPublicData): true,
			string(PermissionViewAddress):    true,
			string(PermissionViewBalance):    true,
		},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshalling legacy record: %v", err)
	}

	ss, err := pm.store.LoadSnapshot(0)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	tree, _ := ss.GetTree("xswd_permissions")
	if err := tree.Put([]byte("villager.tela"), raw); err != nil {
		t.Fatalf("writing legacy record: %v", err)
	}
	if _, err := graviton.Commit(tree); err != nil {
		t.Fatalf("commit: %v", err)
	}

	pm.apps = make(map[string]*ConnectedApp)
	pm.loadFromStorage()

	pinWallet(t, walletA)
	if pm.HasPermission("villager.tela", PermissionViewAddress) {
		t.Fatal("a pre-scoping wallet grant cannot be attributed to any wallet and must not be honoured")
	}
	if pm.HasPermission("villager.tela", PermissionViewBalance) {
		t.Fatal("same for view_balance")
	}
	if !pm.HasPermission("villager.tela", PermissionReadPublicData) {
		t.Fatal("the connect grant is wallet-independent and must survive the migration")
	}

	// Settings reads this list; a door that opens nothing must not be displayed as granted.
	app := pm.GetApp("villager.tela")
	if app == nil {
		t.Fatal("record disappeared")
	}
	if app.Permissions[PermissionViewAddress] {
		t.Fatal("dropped grant is still listed, which is the permission theatre the rebuild removed")
	}
}

// The storage reset loops GetAllApps and promises to clear XSWD permissions. Filtering that
// by wallet would leave other wallets' grants on disk while reporting them gone.
func TestGetAllApps_StaysUnfilteredSoResetClearsEverything(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	pinWallet(t, walletA)
	if err := pm.AddPermission("a.tela", "A", "", PermissionViewAddress); err != nil {
		t.Fatalf("grant under A: %v", err)
	}
	pinWallet(t, walletB)
	if err := pm.AddPermission("b.tela", "B", "", PermissionViewAddress); err != nil {
		t.Fatalf("grant under B: %v", err)
	}

	// Standing in wallet B, the reset must still see the app only wallet A ever touched.
	seen := map[string]bool{}
	for _, app := range pm.GetAllApps() {
		seen[app.Origin] = true
	}
	if !seen["a.tela"] || !seen["b.tela"] {
		t.Fatalf("GetAllApps must report every wallet's records, got %v", seen)
	}

	for _, app := range pm.GetAllApps() {
		if err := pm.RevokeAllPermissions(app.Origin); err != nil {
			t.Fatalf("revoke %s: %v", app.Origin, err)
		}
	}
	pinWallet(t, walletA)
	if pm.HasPermission("a.tela", PermissionViewAddress) {
		t.Fatal("reset left wallet A's grant behind")
	}
}

// The display view is the opposite: only what the open wallet holds.
func TestGetAppsForCurrentWallet_ShowsOnlyTheOpenWalletsDoors(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	pinWallet(t, walletA)
	if err := pm.AddPermission("villager.tela", "Villager", "", PermissionViewAddress); err != nil {
		t.Fatalf("grant under A: %v", err)
	}
	if err := pm.AddPermission("villager.tela", "Villager", "", PermissionReadPublicData); err != nil {
		t.Fatalf("grant public data: %v", err)
	}

	find := func() *AppView {
		for _, v := range pm.GetAppsForCurrentWallet() {
			if v.Origin == "villager.tela" {
				return v
			}
		}
		return nil
	}

	view := find()
	if view == nil || !view.Permissions[PermissionViewAddress] {
		t.Fatal("wallet A should see the door it granted")
	}

	pinWallet(t, walletB)
	view = find()
	if view == nil {
		t.Fatal("the app is still connected under wallet B and must remain listed")
	}
	if view.Permissions[PermissionViewAddress] {
		t.Fatal("wallet B is shown a door it never granted")
	}
	if !view.Permissions[PermissionReadPublicData] {
		t.Fatal("the connect grant is wallet-independent and belongs in every wallet's view")
	}
}

// Revoking in Settings acts on what Settings shows: this wallet.
func TestRevokePermission_ScopedToOpenWallet(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	for _, fp := range []string{walletA, walletB} {
		pinWallet(t, fp)
		if err := pm.AddPermission("villager.tela", "Villager", "", PermissionViewAddress); err != nil {
			t.Fatalf("grant under %s: %v", fp, err)
		}
	}

	pinWallet(t, walletB)
	if err := pm.RevokePermission("villager.tela", PermissionViewAddress); err != nil {
		t.Fatalf("revoke under B: %v", err)
	}
	if pm.HasPermission("villager.tela", PermissionViewAddress) {
		t.Fatal("wallet B revoked its own door and must no longer hold it")
	}

	pinWallet(t, walletA)
	if !pm.HasPermission("villager.tela", PermissionViewAddress) {
		t.Fatal("revoking under wallet B silently took away wallet A's answer")
	}
}

// The fingerprint must never be a wallet address in the clear: the permission store sits in
// the app data directory, and recent-wallets already limits itself to a 16-char prefix.
func TestCurrentWalletFingerprint_IsOpaqueAndEmptyWhenClosed(t *testing.T) {
	if got := currentWalletFingerprint(); got != "" {
		t.Fatalf("no wallet is open in tests; want empty fingerprint, got %q", got)
	}

	const addr = "dero1qykzc9hnwv2nzeqwuqmg22c4d4gv5c9dwqk3xsjdgcahdxxx9wnl7qgngdd0h"
	fp := fingerprintForAddress(addr)
	if fp == "" || len(fp) != 16 {
		t.Fatalf("want a 16-char fingerprint, got %q", fp)
	}
	if fingerprintForAddress(addr) != fp {
		t.Fatal("fingerprint must be stable for the same address")
	}
	if len(addr) >= 16 && fp == addr[:16] {
		t.Fatal("fingerprint is an address prefix; it must be hashed, not recorded")
	}
}

// Connecting must survive a wallet door it cannot file. The connection buys public chain data
// either way, and the door is asked for at first use — failing the whole connect instead
// would leave the app unable to load at all.
func TestApproveWalletConnection_SurvivesUnattributableDoor(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()
	prev := permissionManager
	permissionManager = pm
	defer func() { permissionManager = prev }()

	pinWallet(t, "")
	app := &App{}
	res := app.ApproveWalletConnection("scid:deadbeef", "TELA App", "", []string{"read_public_data", "view_address"})
	if success, _ := res["success"].(bool); !success {
		t.Fatalf("connect must still succeed, got %#v", res)
	}
	if !pm.HasPermission("scid:deadbeef", PermissionReadPublicData) {
		t.Fatal("the connect grant itself must be stored")
	}

	pinWallet(t, walletA)
	if pm.HasPermission("scid:deadbeef", PermissionViewAddress) {
		t.Fatal("an unattributable door must not land on the next wallet to open")
	}
}

// pushEvent re-checks the permission on every push rather than only at Subscribe. This tests
// the condition it evaluates, not the websocket write: with grants scoped per wallet, an
// established balance feed stops the moment a different wallet is opened, and resumes only if
// that wallet granted the door itself.
func TestSubscriptionGate_BalanceFeedStopsOnWalletSwitch(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	const origin = "xswd:https://dapp.example"
	required := SubscriptionPermission(SubNewBalance)

	pinWallet(t, walletA)
	if err := pm.AddPermission(origin, "Feed", "", PermissionViewBalance); err != nil {
		t.Fatalf("grant under wallet A: %v", err)
	}
	if !pm.HasPermission(origin, required) {
		t.Fatal("wallet A granted the balance door; its feed should push")
	}

	pinWallet(t, walletB)
	if pm.HasPermission(origin, required) {
		t.Fatal("the feed kept pushing after a wallet switch — wallet B never approved it")
	}

	pinWallet(t, "")
	if pm.HasPermission(origin, required) {
		t.Fatal("a closed wallet must not keep feeding balance events")
	}
}
