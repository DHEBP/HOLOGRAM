// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Regression test for the wrong-network destination guard.

package main

import (
	"strings"
	"testing"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
)

// deriveTestnetAddress renders a mainnet vector for the simulator/testnet network
// (deto1...) by flipping the network byte on the same public key, so the test needs
// no hardcoded deto1 vector. Both forms wrap the identical key -- which is exactly
// why the guard has to exist: the wallet library cannot tell them apart.
func deriveTestnetAddress(t *testing.T, mainnet string) string {
	t.Helper()
	addr, err := rpc.NewAddress(mainnet)
	if err != nil {
		t.Fatalf("parse mainnet vector: %v", err)
	}
	addr.Mainnet = false
	return addr.String()
}

// TestFirstWrongNetworkDestination locks in the rule that a value-send to an address
// rendered for a different network than the wallet is rejected before it reaches the
// wallet library. A mainnet (dero1) and a simulator/testnet (deto1) address share the
// same public key, so the library builds and broadcasts a wrong-network paste silently;
// this guard is the only thing that catches it on the XSWD and token send paths.
func TestFirstWrongNetworkDestination(t *testing.T) {
	const mainnetAddr = "dero1qyw4fl3dupcg5qlrcsvcedze507q9u67lxfpu8kgnzp04aq73yheqqg2ctjn4"
	testnetAddr := deriveTestnetAddress(t, mainnetAddr)

	// Sanity: the derived address really round-trips as a testnet address.
	if a, err := rpc.NewAddress(testnetAddr); err != nil || a.IsMainnet() {
		t.Fatalf("derived testnet address invalid: err=%v", err)
	}

	cases := []struct {
		name            string
		transfers       []rpc.Transfer
		walletIsMainnet bool
		wantDestNet     string
		wantMismatch    bool
	}{
		{
			name:            "mainnet address on simulator wallet -> rejected",
			transfers:       []rpc.Transfer{{Amount: 10000, Destination: mainnetAddr}},
			walletIsMainnet: false,
			wantDestNet:     "mainnet",
			wantMismatch:    true,
		},
		{
			name:            "testnet address on mainnet wallet -> rejected",
			transfers:       []rpc.Transfer{{Amount: 10000, Destination: testnetAddr}},
			walletIsMainnet: true,
			wantDestNet:     "simulator/testnet",
			wantMismatch:    true,
		},
		{
			name:            "mainnet address on mainnet wallet -> allowed",
			transfers:       []rpc.Transfer{{Amount: 10000, Destination: mainnetAddr}},
			walletIsMainnet: true,
			wantMismatch:    false,
		},
		{
			name:            "testnet address on simulator wallet -> allowed",
			transfers:       []rpc.Transfer{{Amount: 10000, Destination: testnetAddr}},
			walletIsMainnet: false,
			wantMismatch:    false,
		},
		{
			name:            "empty destination (SC deposit / burn) -> skipped, not a mismatch",
			transfers:       []rpc.Transfer{{Burn: 1500000000, SCID: crypto.ZEROHASH}},
			walletIsMainnet: true,
			wantMismatch:    false,
		},
		{
			name:            "unparseable name destination -> skipped (library resolves names)",
			transfers:       []rpc.Transfer{{Amount: 10000, Destination: "somename"}},
			walletIsMainnet: true,
			wantMismatch:    false,
		},
		{
			name:            "mismatch among later entries -> still caught",
			transfers:       []rpc.Transfer{{Burn: 1, SCID: crypto.ZEROHASH}, {Amount: 5, Destination: testnetAddr}},
			walletIsMainnet: true,
			wantDestNet:     "simulator/testnet",
			wantMismatch:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			destNet, mismatch := firstWrongNetworkDestination(tc.transfers, tc.walletIsMainnet)
			if mismatch != tc.wantMismatch {
				t.Fatalf("mismatch = %v, want %v", mismatch, tc.wantMismatch)
			}
			if tc.wantMismatch && destNet != tc.wantDestNet {
				t.Fatalf("destNet = %q, want %q", destNet, tc.wantDestNet)
			}
		})
	}
}

// TestNetworkMismatchError confirms the shared rejection is a failure that names both
// the destination network and the wallet network, so the user sees what mismatched.
func TestNetworkMismatchError(t *testing.T) {
	resp := networkMismatchError("mainnet", false)
	if ok, _ := resp["success"].(bool); ok {
		t.Fatal("expected success=false")
	}
	msg, _ := resp["error"].(string)
	if !strings.Contains(msg, "mainnet") || !strings.Contains(msg, "simulator/testnet") {
		t.Fatalf("error message should name both networks, got: %q", msg)
	}
}
