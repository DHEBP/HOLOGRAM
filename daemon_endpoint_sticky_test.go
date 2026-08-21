package main

import (
	"testing"
)

// A custom daemon endpoint used to be overwritten by whichever network was selected,
// because five places recompute the endpoint from the network mode and could not tell an
// endpoint HOLOGRAM derived from one the user typed. Reported by secretnamebasis: a node
// on :40402 with Gnomon dialling :20000 while the settings field still read :40402.
//
// The rule these tests pin: HOLOGRAM only overwrites endpoints it generated itself.
// Prove they have teeth by reverting any isDerivedEndpoint guard back to
// isLocalhostEndpoint (or removing it) — the matching case must fail.

func TestIsDerivedEndpoint(t *testing.T) {
	cases := []struct {
		endpoint string
		want     bool
		why      string
	}{
		{"", true, "unset is ours to fill in"},
		{"http://127.0.0.1:10102", true, "mainnet default"},
		{"http://127.0.0.1:20000", true, "simulator default"},
		{"http://localhost:10102", true, "same box and port, spelled differently"},
		{"http://127.0.0.1:40402", false, "localhost, custom port — the reported case"},
		{"http://127.0.0.1:40401", false, "localhost, custom port"},
		{"http://node.example.com:10102", false, "remote host, even on a default port"},
		{"http://192.168.1.50:10102", false, "LAN node"},
	}

	for _, c := range cases {
		if got := isDerivedEndpoint(c.endpoint); got != c.want {
			t.Errorf("isDerivedEndpoint(%q) = %v, want %v — %s", c.endpoint, got, c.want, c.why)
		}
	}
}

// The site that bit the reporter: choosing a network in Settings.
func TestSetNetworkModeKeepsCustomEndpoint(t *testing.T) {
	tmp := t.TempDir()
	originalOverride := testDataDirOverride
	testDataDirOverride = tmp
	defer func() { testDataDirOverride = originalOverride }()

	const custom = "http://127.0.0.1:40402"

	app := &App{
		settings: map[string]interface{}{
			"daemon_endpoint": custom,
			"network":         "mainnet",
		},
		daemonClient: NewDaemonClient(custom),
	}

	resetNodeManagerForTest(t)
	res := app.SetNetworkMode("simulator")

	if ok, _ := res["success"].(bool); !ok {
		t.Fatalf("SetNetworkMode failed: %#v", res)
	}
	if got := app.settings["daemon_endpoint"]; got != custom {
		t.Errorf("stored endpoint = %v, want %q — the network switch overwrote a user-set endpoint", got, custom)
	}
	if got := res["endpoint"]; got != custom {
		t.Errorf("returned endpoint = %v, want %q — the frontend would show the wrong node", got, custom)
	}
	// The network label and the ports used to START a node still follow the selection;
	// only the endpoint is the user's.
	if got := app.settings["network"]; got != "simulator" {
		t.Errorf("network = %v, want simulator", got)
	}
	if got := res["rpcPort"]; got != GetNetworkConfig(NetworkSimulator).RPCPort {
		t.Errorf("rpcPort = %v, want the simulator default", got)
	}
}

// The correction the guard must NOT disable: a stale default left over from the other
// network still gets repointed, which is why these sites exist at all.
func TestSetNetworkModeStillCorrectsDerivedEndpoint(t *testing.T) {
	tmp := t.TempDir()
	originalOverride := testDataDirOverride
	testDataDirOverride = tmp
	defer func() { testDataDirOverride = originalOverride }()

	mainnetDefault := "http://127.0.0.1:10102"

	app := &App{
		settings: map[string]interface{}{
			"daemon_endpoint": mainnetDefault,
			"network":         "mainnet",
		},
		daemonClient: NewDaemonClient(mainnetDefault),
	}

	resetNodeManagerForTest(t)
	app.SetNetworkMode("simulator")

	want := "http://127.0.0.1:20000"
	if got := app.settings["daemon_endpoint"]; got != want {
		t.Errorf("stored endpoint = %v, want %q — a derived endpoint must still be corrected", got, want)
	}
}

// Startup path: an unreachable user-set endpoint is preserved rather than replaced with
// a default. Port 1 is used because nothing listens there, so TestConnection really fails.
func TestReconcilePreservesUnreachableCustomEndpoint(t *testing.T) {
	tmp := t.TempDir()
	originalOverride := testDataDirOverride
	testDataDirOverride = tmp
	defer func() { testDataDirOverride = originalOverride }()

	const custom = "http://127.0.0.1:1"

	app := &App{
		settings: map[string]interface{}{
			"daemon_endpoint": custom,
			"network":         "simulator",
		},
		daemonClient: NewDaemonClient(custom),
	}

	resetNodeManagerForTest(t)
	app.reconcileDaemonEndpoint()

	if got := app.settings["daemon_endpoint"]; got != custom {
		t.Errorf("stored endpoint = %v, want %q — startup reconciliation overwrote a user-set endpoint", got, custom)
	}
}

// SetNetworkMode refuses to run while a node is up, and the reconciliation paths write
// nodeManager's ports, so every test here needs it in a known, stopped state.
func resetNodeManagerForTest(t *testing.T) {
	t.Helper()
	nodeManager.Lock()
	defer nodeManager.Unlock()
	nodeManager.isRunning = false
	nodeManager.networkMode = NetworkMainnet
	nodeManager.rpcPort = GetNetworkConfig(NetworkMainnet).RPCPort
}
