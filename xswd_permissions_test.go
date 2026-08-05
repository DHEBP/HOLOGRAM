// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Unit tests for XSWD Permission System

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/deroproject/graviton"
)

// ============== Test Setup/Teardown ==============

func setupTestPermissionManager(t *testing.T) (*PermissionManager, func()) {
	t.Helper()

	// Create temp directory for test data
	tempDir, err := os.MkdirTemp("", "hologram_permissions_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create a Graviton store
	store, err := graviton.NewDiskStore(tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create Graviton store: %v", err)
	}

	pm := &PermissionManager{
		store:         store,
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tempDir)
	}

	return pm, cleanup
}

// ============== Permission Type Tests ==============

func TestAllPermissions(t *testing.T) {
	perms := AllPermissions()

	if len(perms) != 5 {
		t.Errorf("Expected 5 permissions, got %d", len(perms))
	}

	// Verify all expected permissions are present
	expected := map[XSWDPermission]bool{
		PermissionReadPublicData:  false,
		PermissionViewAddress:     false,
		PermissionViewBalance:     false,
		PermissionSignTransaction: false,
		PermissionSCInvoke:        false,
	}

	for _, p := range perms {
		if _, ok := expected[p]; !ok {
			t.Errorf("Unexpected permission: %s", p)
		}
		expected[p] = true
	}

	for p, found := range expected {
		if !found {
			t.Errorf("Missing permission: %s", p)
		}
	}
}

func TestGetPermissionInfo_KnownPermissions(t *testing.T) {
	tests := []struct {
		permission XSWDPermission
		expectName string
		expectAsk  bool
	}{
		{PermissionViewAddress, "View Wallet Address", false},
		{PermissionViewBalance, "View Balance & History", false},
		{PermissionSignTransaction, "Sign Transactions", true},
		{PermissionSCInvoke, "Smart Contract Calls", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.permission), func(t *testing.T) {
			info := GetPermissionInfo(tt.permission)

			if info.ID != tt.permission {
				t.Errorf("ID = %s, expected %s", info.ID, tt.permission)
			}
			if info.Name != tt.expectName {
				t.Errorf("Name = %s, expected %s", info.Name, tt.expectName)
			}
			if info.AlwaysAsk != tt.expectAsk {
				t.Errorf("AlwaysAsk = %v, expected %v", info.AlwaysAsk, tt.expectAsk)
			}
			if info.Description == "" {
				t.Error("Description should not be empty")
			}
		})
	}
}

func TestGetPermissionInfo_Unknown(t *testing.T) {
	info := GetPermissionInfo(XSWDPermission("unknown_permission"))

	if info.Name != "unknown_permission" {
		t.Errorf("Name = %s, expected unknown_permission", info.Name)
	}
	if info.Description != "Unknown permission" {
		t.Errorf("Description = %s, expected 'Unknown permission'", info.Description)
	}
	if !info.AlwaysAsk {
		t.Error("Unknown permissions should have AlwaysAsk = true")
	}
}

func TestAlwaysAskPermissions(t *testing.T) {
	// Verify that signing operations always require per-action approval
	signInfo := GetPermissionInfo(PermissionSignTransaction)
	if !signInfo.AlwaysAsk {
		t.Error("PermissionSignTransaction should have AlwaysAsk = true")
	}

	scInfo := GetPermissionInfo(PermissionSCInvoke)
	if !scInfo.AlwaysAsk {
		t.Error("PermissionSCInvoke should have AlwaysAsk = true")
	}

	// Verify read-only permissions don't require per-action approval
	addrInfo := GetPermissionInfo(PermissionViewAddress)
	if addrInfo.AlwaysAsk {
		t.Error("PermissionViewAddress should have AlwaysAsk = false")
	}

	balInfo := GetPermissionInfo(PermissionViewBalance)
	if balInfo.AlwaysAsk {
		t.Error("PermissionViewBalance should have AlwaysAsk = false")
	}
}

// ============== Method → Permission Mapping Tests ==============

func TestGetRequiredPermission_AllMethods(t *testing.T) {
	tests := []struct {
		method   string
		expected XSWDPermission
	}{
		// Address methods
		{"GetAddress", PermissionViewAddress},
		{"DERO.GetAddress", PermissionViewAddress},

		// Balance methods.
		// GetHeight (wallet-side) returns wallet's last-seen sync height — wallet state.
		// DERO.GetHeight (daemon-side) returns chain-tip block height — public data,
		// classified under PermissionReadPublicData below.
		{"GetBalance", PermissionViewBalance},
		{"DERO.GetBalance", PermissionViewBalance},
		{"GetHeight", PermissionViewBalance},

		// Read-only daemon methods (public chain data, no wallet needed)
		{"DERO.GetInfo", PermissionReadPublicData},
		{"DERO.GetHeight", PermissionReadPublicData},
		{"DERO.GetBlock", PermissionReadPublicData},
		{"DERO.GetTxPool", PermissionReadPublicData},
		{"DERO.GetSC", PermissionReadPublicData},

		// Transfer methods
		{"transfer", PermissionSignTransaction},
		{"Transfer", PermissionSignTransaction},
		{"DERO.Transfer", PermissionSignTransaction},

		// SC invoke methods
		{"scinvoke", PermissionSCInvoke},
		{"SC_Invoke", PermissionSCInvoke},
		{"DERO.SC_Invoke", PermissionSCInvoke},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := GetRequiredPermission(tt.method)
			if result != tt.expected {
				t.Errorf("GetRequiredPermission(%s) = %s, expected %s", tt.method, result, tt.expected)
			}
		})
	}
}

func TestGetRequiredPermission_Unknown(t *testing.T) {
	// Unknown methods should return empty string
	unknownMethods := []string{
		"Ping",
		"Echo",
		"Login",
		"SomeRandomMethod",
		"",
	}

	for _, method := range unknownMethods {
		result := GetRequiredPermission(method)
		if result != "" {
			t.Errorf("GetRequiredPermission(%s) = %s, expected empty string", method, result)
		}
	}
}

func TestDefaultRequestedPermissions(t *testing.T) {
	defaults := DefaultRequestedPermissions()

	// Default is now read-only only - apps must explicitly request wallet permissions
	if len(defaults) != 1 {
		t.Errorf("Expected 1 default permission (read-only), got %d", len(defaults))
	}

	// Should only include read_public_data by default
	permSet := make(map[XSWDPermission]bool)
	for _, p := range defaults {
		permSet[p] = true
	}

	if !permSet[PermissionReadPublicData] {
		t.Error("Default permissions should include read_public_data")
	}

	// Wallet permissions should NOT be included by default
	if permSet[PermissionViewAddress] {
		t.Error("view_address should NOT be a default permission")
	}
	if permSet[PermissionViewBalance] {
		t.Error("view_balance should NOT be a default permission")
	}
	if permSet[PermissionSignTransaction] {
		t.Error("sign_transaction should NOT be a default permission")
	}
	if permSet[PermissionSCInvoke] {
		t.Error("sc_invoke should NOT be a default permission")
	}
}

// ============== Permission Manager Creation Tests ==============

func TestPermissionManager_NilStore(t *testing.T) {
	pm := &PermissionManager{
		store:         nil,
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}

	// Should not panic with nil store
	pm.loadFromStorage()

	// Grant should return error but not panic
	err := pm.GrantPermissions("origin", "name", "desc", []XSWDPermission{PermissionViewAddress})
	if err == nil {
		t.Error("Expected error when granting with nil store")
	}
}

// ============== Grant/Revoke Permission Tests ==============

func TestGrantPermissions_NewApp(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"
	name := "Test App"
	description := "A test application"
	perms := []XSWDPermission{PermissionViewAddress, PermissionViewBalance}

	err := pm.GrantPermissions(origin, name, description, perms)
	if err != nil {
		t.Fatalf("GrantPermissions failed: %v", err)
	}

	// Verify app was created
	app := pm.GetApp(origin)
	if app == nil {
		t.Fatal("App should exist after granting permissions")
	}

	if app.Origin != origin {
		t.Errorf("Origin = %s, expected %s", app.Origin, origin)
	}
	if app.Name != name {
		t.Errorf("Name = %s, expected %s", app.Name, name)
	}
	if app.Description != description {
		t.Errorf("Description = %s, expected %s", app.Description, description)
	}

	// Verify permissions
	if !app.Permissions[PermissionViewAddress] {
		t.Error("PermissionViewAddress should be granted")
	}
	if !app.Permissions[PermissionViewBalance] {
		t.Error("PermissionViewBalance should be granted")
	}
	if app.Permissions[PermissionSignTransaction] {
		t.Error("PermissionSignTransaction should NOT be granted")
	}

	// Verify timestamps
	if app.GrantedAt == 0 {
		t.Error("GrantedAt should be set")
	}
	if app.LastAccessed == 0 {
		t.Error("LastAccessed should be set")
	}
}

func TestGrantPermissions_ExistingApp(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"

	// First grant
	err := pm.GrantPermissions(origin, "Original Name", "Original Desc", []XSWDPermission{PermissionViewAddress})
	if err != nil {
		t.Fatalf("First grant failed: %v", err)
	}

	firstApp := pm.GetApp(origin)
	originalGrantedAt := firstApp.GrantedAt

	// Second grant replaces the permission set (connect approval is authoritative)
	err = pm.GrantPermissions(origin, "Updated Name", "Updated Desc", []XSWDPermission{PermissionViewBalance})
	if err != nil {
		t.Fatalf("Second grant failed: %v", err)
	}

	updatedApp := pm.GetApp(origin)

	// Name and description should be updated
	if updatedApp.Name != "Updated Name" {
		t.Errorf("Name not updated: %s", updatedApp.Name)
	}
	if updatedApp.Description != "Updated Desc" {
		t.Errorf("Description not updated: %s", updatedApp.Description)
	}

	// Replace semantics: only the latest grant set remains
	if updatedApp.Permissions[PermissionViewAddress] {
		t.Error("ViewAddress should have been replaced away by the second grant")
	}
	if !updatedApp.Permissions[PermissionViewBalance] {
		t.Error("ViewBalance should be granted after second grant")
	}

	// GrantedAt should remain original
	if updatedApp.GrantedAt != originalGrantedAt {
		t.Error("GrantedAt should not change on update")
	}

	// LastAccessed should be set (>= GrantedAt since it's updated each time)
	if updatedApp.LastAccessed < originalGrantedAt {
		t.Error("LastAccessed should be >= GrantedAt")
	}
}

func TestRevokePermission_Single(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"

	// Grant multiple permissions
	err := pm.GrantPermissions(origin, "Test", "", []XSWDPermission{
		PermissionViewAddress,
		PermissionViewBalance,
		PermissionSignTransaction,
	})
	if err != nil {
		t.Fatalf("GrantPermissions failed: %v", err)
	}

	// Revoke one permission
	err = pm.RevokePermission(origin, PermissionViewBalance)
	if err != nil {
		t.Fatalf("RevokePermission failed: %v", err)
	}

	app := pm.GetApp(origin)

	// Verify only the revoked permission is gone
	if !app.Permissions[PermissionViewAddress] {
		t.Error("PermissionViewAddress should still be granted")
	}
	if app.Permissions[PermissionViewBalance] {
		t.Error("PermissionViewBalance should be revoked")
	}
	if !app.Permissions[PermissionSignTransaction] {
		t.Error("PermissionSignTransaction should still be granted")
	}
}

func TestRevokePermission_Nonexistent(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	// Revoking from nonexistent app should not error
	err := pm.RevokePermission("nonexistent-origin", PermissionViewAddress)
	if err != nil {
		t.Errorf("RevokePermission for nonexistent app should not error: %v", err)
	}
}

func TestRevokeAllPermissions(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"

	// Grant permissions and set as active
	pm.GrantPermissions(origin, "Test", "", AllPermissions())
	pm.SetActiveClient(origin, true)

	// Verify setup
	if pm.GetApp(origin) == nil {
		t.Fatal("App should exist before revoke")
	}
	if !pm.IsClientActive(origin) {
		t.Fatal("Client should be active before revoke")
	}

	// Revoke all
	err := pm.RevokeAllPermissions(origin)
	if err != nil {
		t.Fatalf("RevokeAllPermissions failed: %v", err)
	}

	// Verify app is gone
	if pm.GetApp(origin) != nil {
		t.Error("App should be removed after RevokeAllPermissions")
	}

	// Verify client is no longer active
	if pm.IsClientActive(origin) {
		t.Error("Client should not be active after RevokeAllPermissions")
	}
}

func TestRevokeAllPermissions_Nonexistent(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	// Should not error for nonexistent app
	err := pm.RevokeAllPermissions("nonexistent-origin")
	if err != nil {
		t.Errorf("RevokeAllPermissions for nonexistent app should not error: %v", err)
	}
}

// ============== Permission Checking Tests ==============

func TestHasPermission_Granted(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"
	pm.GrantPermissions(origin, "Test", "", []XSWDPermission{PermissionViewAddress})

	if !pm.HasPermission(origin, PermissionViewAddress) {
		t.Error("HasPermission should return true for granted permission")
	}
}

func TestHasPermission_NotGranted(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"
	pm.GrantPermissions(origin, "Test", "", []XSWDPermission{PermissionViewAddress})

	if pm.HasPermission(origin, PermissionViewBalance) {
		t.Error("HasPermission should return false for non-granted permission")
	}
}

func TestHasPermission_NoApp(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	if pm.HasPermission("nonexistent-origin", PermissionViewAddress) {
		t.Error("HasPermission should return false for unknown origin")
	}
}

// ============== App Management Tests ==============

func TestGetApp_Exists(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"
	pm.GrantPermissions(origin, "Test App", "Description", []XSWDPermission{PermissionViewAddress})

	app := pm.GetApp(origin)
	if app == nil {
		t.Fatal("GetApp should return app for existing origin")
	}

	// Verify it's a copy (modifying shouldn't affect original)
	app.Name = "Modified"
	app.Permissions[PermissionViewBalance] = true

	original := pm.GetApp(origin)
	if original.Name == "Modified" {
		t.Error("GetApp should return a copy, not the original")
	}
	if original.Permissions[PermissionViewBalance] {
		t.Error("Modifying returned app should not affect stored app")
	}
}

func TestGetApp_NotExists(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	app := pm.GetApp("nonexistent-origin")
	if app != nil {
		t.Error("GetApp should return nil for nonexistent origin")
	}
}

func TestGetAllApps(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	// Add multiple apps
	pm.GrantPermissions("origin1", "App 1", "", []XSWDPermission{PermissionViewAddress})
	pm.GrantPermissions("origin2", "App 2", "", []XSWDPermission{PermissionViewBalance})
	pm.GrantPermissions("origin3", "App 3", "", []XSWDPermission{PermissionSignTransaction})

	apps := pm.GetAllApps()

	if len(apps) != 3 {
		t.Errorf("Expected 3 apps, got %d", len(apps))
	}

	// Verify apps are copies
	for _, app := range apps {
		app.Name = "Modified"
	}

	// Original should be unchanged
	original := pm.GetApp("origin1")
	if original.Name == "Modified" {
		t.Error("GetAllApps should return copies")
	}
}

func TestGetAllApps_Empty(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	apps := pm.GetAllApps()
	if len(apps) != 0 {
		t.Errorf("Expected 0 apps, got %d", len(apps))
	}
}

// ============== Active Client Tests ==============

func TestSetActiveClient_True(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"

	pm.SetActiveClient(origin, true)

	if !pm.IsClientActive(origin) {
		t.Error("Client should be active after SetActiveClient(true)")
	}
}

func TestSetActiveClient_False(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	origin := "https://testapp.dero"

	pm.SetActiveClient(origin, true)
	pm.SetActiveClient(origin, false)

	if pm.IsClientActive(origin) {
		t.Error("Client should not be active after SetActiveClient(false)")
	}
}

func TestIsClientActive_NotSet(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	if pm.IsClientActive("unknown-origin") {
		t.Error("Unknown client should not be active")
	}
}

func TestGetActiveClients(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	pm.SetActiveClient("origin1", true)
	pm.SetActiveClient("origin2", true)
	pm.SetActiveClient("origin3", true)
	pm.SetActiveClient("origin2", false) // Deactivate one

	clients := pm.GetActiveClients()

	if len(clients) != 2 {
		t.Errorf("Expected 2 active clients, got %d", len(clients))
	}

	// Verify correct clients
	clientSet := make(map[string]bool)
	for _, c := range clients {
		clientSet[c] = true
	}

	if !clientSet["origin1"] {
		t.Error("origin1 should be active")
	}
	if clientSet["origin2"] {
		t.Error("origin2 should not be active")
	}
	if !clientSet["origin3"] {
		t.Error("origin3 should be active")
	}
}

// ============== Persistence Tests ==============

func TestPermissions_PersistAcrossReload(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "hologram_permissions_persist_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origin := "https://testapp.dero"

	// Create first manager and grant permissions
	store1, _ := graviton.NewDiskStore(tempDir)
	pm1 := &PermissionManager{
		store:         store1,
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}

	pm1.GrantPermissions(origin, "Test App", "Persisted app", []XSWDPermission{
		PermissionViewAddress,
		PermissionViewBalance,
	})

	store1.Close()

	// Create second manager and load from same directory
	store2, _ := graviton.NewDiskStore(tempDir)
	pm2 := &PermissionManager{
		store:         store2,
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}
	pm2.loadFromStorage()
	defer store2.Close()

	// Verify permissions persisted
	app := pm2.GetApp(origin)
	if app == nil {
		t.Fatal("App should persist across reload")
	}

	if app.Name != "Test App" {
		t.Errorf("Name = %s, expected 'Test App'", app.Name)
	}
	if !app.Permissions[PermissionViewAddress] {
		t.Error("PermissionViewAddress should persist")
	}
	if !app.Permissions[PermissionViewBalance] {
		t.Error("PermissionViewBalance should persist")
	}
}

// ============== Concurrent Access Tests ==============

func TestPermissionManager_ConcurrentGrantRevoke(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	var wg sync.WaitGroup
	iterations := 50

	// Concurrent grants
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			origin := "origin" + string(rune('A'+id%26))
			pm.GrantPermissions(origin, "App", "", []XSWDPermission{PermissionViewAddress})
		}(i)
	}

	// Concurrent revokes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			origin := "origin" + string(rune('A'+id%26))
			pm.RevokePermission(origin, PermissionViewBalance)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			origin := "origin" + string(rune('A'+id%26))
			_ = pm.HasPermission(origin, PermissionViewAddress)
			_ = pm.GetApp(origin)
		}(i)
	}

	wg.Wait()
	// No race conditions or panics = success
}

func TestPermissionManager_ConcurrentActiveClients(t *testing.T) {
	pm, cleanup := setupTestPermissionManager(t)
	defer cleanup()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent set active
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			origin := "origin" + string(rune('A'+id%26))
			pm.SetActiveClient(origin, id%2 == 0)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			origin := "origin" + string(rune('A'+id%26))
			_ = pm.IsClientActive(origin)
			_ = pm.GetActiveClients()
		}(i)
	}

	wg.Wait()
	// No race conditions or panics = success
}

// ============== Benchmark Tests ==============

func BenchmarkHasPermission(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "hologram_bench_*")
	defer os.RemoveAll(tempDir)

	store, _ := graviton.NewDiskStore(tempDir)
	defer store.Close()

	pm := &PermissionManager{
		store:         store,
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}

	// Pre-populate with some apps
	for i := 0; i < 100; i++ {
		origin := "origin" + string(rune('A'+i%26)) + string(rune('0'+i%10))
		pm.GrantPermissions(origin, "App", "", AllPermissions())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		origin := "origin" + string(rune('A'+i%26)) + string(rune('0'+i%10))
		pm.HasPermission(origin, PermissionViewAddress)
	}
}

func BenchmarkGetAllApps(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "hologram_bench_*")
	defer os.RemoveAll(tempDir)

	store, _ := graviton.NewDiskStore(tempDir)
	defer store.Close()

	pm := &PermissionManager{
		store:         store,
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}

	// Pre-populate with apps
	for i := 0; i < 50; i++ {
		pm.GrantPermissions("origin"+string(rune(i)), "App", "", AllPermissions())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.GetAllApps()
	}
}

func BenchmarkGrantPermissions(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "hologram_bench_*")
	defer os.RemoveAll(tempDir)

	store, _ := graviton.NewDiskStore(tempDir)
	defer store.Close()

	pm := &PermissionManager{
		store:         store,
		apps:          make(map[string]*ConnectedApp),
		activeClients: make(map[string]bool),
	}

	perms := []XSWDPermission{PermissionViewAddress, PermissionViewBalance}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		origin := "origin" + string(rune(i%1000))
		pm.GrantPermissions(origin, "App", "Desc", perms)
	}
}

func BenchmarkGetRequiredPermission(b *testing.B) {
	methods := []string{"GetAddress", "DERO.GetBalance", "transfer", "scinvoke", "Ping"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetRequiredPermission(methods[i%len(methods)])
	}
}

// The bug this pins: the handshake parsed only the array form, so a canonical XSWD dApp
// sending the map form was handed the one-entry default and every wallet call it made was
// then denied forever. Revert ParseRequestedPermissions to array-only and the map case here
// fails — this test is coupled to the fix, not merely to the helper's existence.
func TestParseRequestedPermissions(t *testing.T) {
	cases := []struct {
		name string
		raw  interface{}
		want []XSWDPermission
	}{
		{"canonical map form", map[string]interface{}{
			"view_address": true, "view_balance": true, "sign_transaction": false,
		}, []XSWDPermission{PermissionViewAddress, PermissionViewBalance}},
		{"legacy array form", []interface{}{"view_address", "sc_invoke"},
			[]XSWDPermission{PermissionViewAddress, PermissionSCInvoke}},
		{"absent falls back to public data", nil, DefaultRequestedPermissions()},
		{"empty array does not widen", []interface{}{}, DefaultRequestedPermissions()},
		{"all-false map does not widen", map[string]interface{}{"view_balance": false}, DefaultRequestedPermissions()},
		{"unknown ids dropped", []interface{}{"view_address", "be_admin", 42},
			[]XSWDPermission{PermissionViewAddress}},
		{"garbage type falls back", "view_balance", DefaultRequestedPermissions()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseRequestedPermissions(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestSubscriptionPermissionMapping(t *testing.T) {
	if got := SubscriptionPermission(SubNewTopoheight); got != PermissionReadPublicData {
		t.Errorf("new_topoheight: got %v, want read_public_data", got)
	}
	for _, ev := range []SubscriptionType{SubNewBalance, SubNewEntry} {
		if got := SubscriptionPermission(ev); got != PermissionViewBalance {
			t.Errorf("%v: got %v, want view_balance", ev, got)
		}
	}
}

// A colliding JSON-RPC id used to route an approval — and the password typed into it — to a
// different pending request. Keys must not be derived from anything the caller controls.
func TestNextRequestIDIsUniqueAndNotCallerDerived(t *testing.T) {
	s := &XSWDServer{}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := s.nextRequestID("handshake")
		if seen[id] {
			t.Fatalf("duplicate request id: %s", id)
		}
		seen[id] = true
	}
}

func TestXSWDOriginKeyIsNamespaced(t *testing.T) {
	// A dApp naming a browser TELA app's SCID must not land on that app's grant record.
	scid := "deadbeef"
	if XSWDOriginKey(scid) == scid {
		t.Fatal("dApp-supplied origin was not namespaced")
	}
	if XSWDOriginKey(" "+scid) != XSWDOriginKey(scid) {
		t.Fatal("whitespace produced a second key for the same origin")
	}
}

// The signing oracle: handleAuthPage took callback and domain as independent params, so the
// site named in the approval prompt need not be where the signature is delivered. Revert
// either gate and the reject cases here still pass (they test the matcher, not its adoption)
// -- but revert the MATCHER and every reject case fails.
func TestAuthURLMatchesDomain(t *testing.T) {
	reject := []struct{ name, raw, domain string }{
		{"plain cross-host", "https://evil.tld/cb", "good.com"},
		{"backslash userinfo", "https://good.com\\@evil.tld/cb", "good.com"},
		{"userinfo", "https://good.com@evil.tld/cb", "good.com"},
		{"encoded slash userinfo", "https://good.com%2f@evil.tld/cb", "good.com"},
		{"scheme-relative", "//evil.tld/cb", "good.com"},
		{"javascript scheme", "javascript:alert(1)", "good.com"},
		{"embedded tab", "https://good\t.com/cb", "good.com"},
		{"suffix not on dot boundary", "https://notgood.com/cb", "good.com"},
		{"domain is prose", "https://good.com/cb", "the good site"},
		{"empty domain", "https://good.com/cb", ""},
		{"not a url", "good.com/cb", "good.com"},
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if err := authURLMatchesDomain(tc.raw, tc.domain); err == nil {
				t.Fatalf("accepted %q for domain %q", tc.raw, tc.domain)
			}
		})
	}

	accept := []struct{ name, raw, domain string }{
		{"exact host", "https://good.com/cb", "good.com"},
		{"subdomain", "https://login.good.com/cb", "good.com"},
		{"case insensitive", "https://GOOD.com/cb", "Good.COM"},
		{"http allowed", "http://good.com/cb", "good.com"},
		{"trailing dot both sides", "https://good.com./cb", "good.com."},
		{"domain carries a port", "https://good.com/cb", "good.com:8443"},
		// "#" opens a fragment, so the host really is good.com. The ledger listed this as a
		// vector that must fail closed; it must not -- rejecting it would break real callbacks.
		{"fragment only looks like userinfo", "https://good.com#@evil.tld/cb", "good.com"},
	}
	for _, tc := range accept {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			if err := authURLMatchesDomain(tc.raw, tc.domain); err != nil {
				t.Fatalf("rejected %q for domain %q: %v", tc.raw, tc.domain, err)
			}
		})
	}
}

// Adoption test. The matcher test above passes even if nobody CALLS the matcher -- that is
// how the original false claim survived review. This drives the real handler, so deleting
// the gate in handleAuthPage fails here.
func TestHandleAuthPageRejectsCrossHostCallback(t *testing.T) {
	s := &XSWDServer{}

	bad := httptest.NewRequest(http.MethodGet,
		"/auth?domain=good.com&callback=https://evil.tld/steal&nonce=abc123", nil)
	rec := httptest.NewRecorder()
	s.handleAuthPage(rec, bad)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-host callback: status %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if strings.Contains(rec.Body.String(), "evil.tld") {
		t.Fatal("rejection echoed the attacker callback into the served page")
	}

	good := httptest.NewRequest(http.MethodGet,
		"/auth?domain=good.com&callback=https://good.com/cb&nonce=abc123", nil)
	recGood := httptest.NewRecorder()
	s.handleAuthPage(recGood, good)

	if recGood.Code != http.StatusOK {
		t.Fatalf("legitimate sign-in was rejected: status %d", recGood.Code)
	}
}

// GetDaemon was gated three inconsistent ways: view_address on the WebSocket path,
// nothing at all on the Browser path, and a Svelte mapping that was never enforced.
// The endpoint is public-chain access, so read_public_data is the honest requirement.
func TestGetDaemonRequiresPublicDataNotAddress(t *testing.T) {
	for _, m := range []string{"GetDaemon", "DERO.GetDaemon"} {
		got := GetRequiredPermission(m)
		if got == PermissionViewAddress {
			t.Errorf("%s requires view_address — the daemon endpoint says nothing about the wallet", m)
		}
		if got != PermissionReadPublicData {
			t.Errorf("%s: got %q, want %q", m, got, PermissionReadPublicData)
		}
	}
	// Address methods must NOT have been swept along with it.
	if GetRequiredPermission("GetAddress") != PermissionViewAddress {
		t.Error("GetAddress no longer requires view_address")
	}
}
