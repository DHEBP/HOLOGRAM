package main

import (
	"strings"
	"testing"
)

func TestDenyUnlessPermission_EmptyOrigin(t *testing.T) {
	err := DenyUnlessPermission("", PermissionViewAddress)
	if err == nil {
		t.Fatal("empty origin must be denied (handshake required)")
	}
	if err.Code != -32003 {
		t.Fatalf("expected -32003, got %d", err.Code)
	}
	if !strings.Contains(strings.ToLower(err.Message), "handshake") {
		t.Fatalf("expected handshake message, got %q", err.Message)
	}
}

func TestDenyUnlessPermission_MissingGrant(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()
	prev := permissionManager
	permissionManager = pm
	defer func() { permissionManager = prev }()

	origin := "https://dapp.example"
	if err := pm.GrantPermissions(origin, "App", "", []XSWDPermission{PermissionReadPublicData}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	if err := DenyUnlessPermission(origin, PermissionViewAddress); err == nil {
		t.Fatal("view_address must be denied when not granted")
	}
}

func TestDenyUnlessPermission_Granted(t *testing.T) {
	// Wallet doors are filed against the wallet that granted them, so these need an
	// identity to be granted under. Pinned rather than opened: no wallet file, no daemon.
	pinWallet(t, walletA)
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()
	prev := permissionManager
	permissionManager = pm
	defer func() { permissionManager = prev }()

	origin := "https://dapp.example"
	if err := pm.GrantPermissions(origin, "App", "", []XSWDPermission{PermissionViewAddress}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	if err := DenyUnlessPermission(origin, PermissionViewAddress); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
}

func TestApproveWalletConnection_RequiresOrigin(t *testing.T) {
	app := &App{}
	res := app.ApproveWalletConnection("", "App", "", []string{"view_address"})
	if success, _ := res["success"].(bool); success {
		t.Fatal("empty origin must fail")
	}
}

func TestApproveWalletConnection_PersistsGrants(t *testing.T) {
	// Wallet doors are filed against the wallet that granted them, so these need an
	// identity to be granted under. Pinned rather than opened: no wallet file, no daemon.
	pinWallet(t, walletA)
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()
	prev := permissionManager
	permissionManager = pm
	defer func() { permissionManager = prev }()

	app := &App{}
	origin := "scid:deadbeef"
	res := app.ApproveWalletConnection(origin, "TELA App", "desc", []string{"view_address", "view_balance"})
	if success, _ := res["success"].(bool); !success {
		t.Fatalf("expected success, got %#v", res)
	}

	if !pm.HasPermission(origin, PermissionViewAddress) {
		t.Fatal("view_address should be persisted")
	}
	if !pm.HasPermission(origin, PermissionViewBalance) {
		t.Fatal("view_balance should be persisted")
	}
	if pm.HasPermission(origin, PermissionSignTransaction) {
		t.Fatal("sign_transaction should not be granted")
	}
}

// A door the user chose to remember must survive the app reconnecting. Connect grants public
// chain data only, so if it REPLACED the set it would quietly revoke that memory and the app
// would prompt again for something already answered.
func TestApproveWalletConnection_DoesNotClobberRememberedGrants(t *testing.T) {
	// Wallet doors are filed against the wallet that granted them, so these need an
	// identity to be granted under. Pinned rather than opened: no wallet file, no daemon.
	pinWallet(t, walletA)
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()
	prev := permissionManager
	permissionManager = pm
	defer func() { permissionManager = prev }()

	app := &App{}
	origin := "scid:deadbeef"

	// First visit: connect, then "Always allow" on the balance door.
	app.ApproveWalletConnection(origin, "TELA App", "desc", []string{"read_public_data"})
	if err := pm.AddPermission(origin, "TELA App", "", PermissionViewBalance); err != nil {
		t.Fatalf("AddPermission: %v", err)
	}

	// Second visit: the app reconnects.
	res := app.ApproveWalletConnection(origin, "TELA App", "desc", []string{"read_public_data"})
	if success, _ := res["success"].(bool); !success {
		t.Fatalf("expected success, got %#v", res)
	}

	if !pm.HasPermission(origin, PermissionViewBalance) {
		t.Fatal("remembered view_balance must survive a reconnect")
	}
	if !pm.HasPermission(origin, PermissionReadPublicData) {
		t.Fatal("read_public_data should still be granted")
	}
}
