package main

import "testing"

// The on-chain TELA contract declares `Function Rate(r Uint64)`. The DVM binds
// arguments by the contract's declared parameter name, so the sc_rpc arg MUST be
// named "r". Passing any other name (e.g. "rating") fails with
// `Argument "r" is missing while invoking "Rate"` and the rating never records.
func TestBuildRateArgs_UsesContractParamName(t *testing.T) {
	args := buildRateArgs(88)

	if len(args) != 1 {
		t.Fatalf("expected exactly 1 arg, got %d", len(args))
	}

	if name := args[0]["name"]; name != "r" {
		t.Fatalf(`Rate arg name must be "r" to match the on-chain contract, got %q`, name)
	}
	if dt := args[0]["datatype"]; dt != "U" {
		t.Fatalf(`Rate arg datatype must be "U" (Uint64), got %q`, dt)
	}
	if v := args[0]["value"]; v != uint64(88) {
		t.Fatalf("Rate arg value must be uint64(88), got %#v", v)
	}
}

// Discover Apps rates with the integrated wallet open ("Wallet Ready"). The
// sidebar XSWD light can be green from the *server* alone — Rate must not
// require an Engram xswdClient connection before trying the local wallet.
func TestRateTELAApp_NoEngramStillAttemptsLocalPath(t *testing.T) {
	a := &App{consoleLogs: make([]ConsoleLog, 0)}
	// No local wallet, no Engram client — should fail with the dual-path error,
	// not the old Engram-only "Wallet not connected via XSWD".
	result := a.RateTELAApp("f794891d817ce8837ce35e3a0eb1def6d14bfe5aae9483f106826c598d189fe2", 99)
	if ok, _ := result["success"].(bool); ok {
		t.Fatal("expected failure with no wallet")
	}
	errMsg, _ := result["error"].(string)
	if errMsg == "Wallet not connected via XSWD" {
		t.Fatal("Rate still hard-requires Engram XSWD; integrated-wallet ratings stay broken")
	}
	if errMsg != "No wallet available. Open a wallet or connect via XSWD." {
		t.Fatalf("unexpected error: %q", errMsg)
	}
}
