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
