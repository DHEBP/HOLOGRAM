// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Regression test for the empty-destination transfer guard.

package main

import (
	"testing"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
)

// TestHasEmptyValueDestination locks in the rule that a value transfer (Amount > 0) with an
// empty destination is rejected before it reaches the wallet library. This is the incident
// where the Send screen displayed a valid recipient but dispatched an empty destination: the
// wallet library then failed with "Main Destination cannot be empty" and the daemon reported
// a misleading "-32098 leaf not found" from resolving the empty string. If a future edit lets
// an empty-destination value transfer through, the cases below start returning false and fail
// here rather than at the user's wallet.
func TestHasEmptyValueDestination(t *testing.T) {
	tokenSCID := crypto.HashHexToHash("a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90")
	const recipient = "dero1qy38hp6recipientplaceholderqqtlladg"

	cases := []struct {
		name      string
		transfers []rpc.Transfer
		want      bool
	}{
		{
			name:      "native value transfer, empty destination -> rejected (the incident)",
			transfers: []rpc.Transfer{{Amount: 10000, SCID: crypto.ZEROHASH}},
			want:      true,
		},
		{
			name:      "native value transfer, whitespace-only destination -> rejected",
			transfers: []rpc.Transfer{{Amount: 10000, Destination: "   ", SCID: crypto.ZEROHASH}},
			want:      true,
		},
		{
			name:      "token value transfer, empty destination -> rejected",
			transfers: []rpc.Transfer{{Amount: 5, SCID: tokenSCID}},
			want:      true,
		},
		{
			name:      "native value transfer, real destination -> allowed",
			transfers: []rpc.Transfer{{Amount: 10000, Destination: recipient, SCID: crypto.ZEROHASH}},
			want:      false,
		},
		{
			name:      "zero-amount placeholder, empty destination -> not a value transfer, allowed",
			transfers: []rpc.Transfer{{Amount: 0, SCID: crypto.ZEROHASH}},
			want:      false,
		},
		{
			name:      "pure burn, empty destination -> not a value transfer here (burn guard owns this)",
			transfers: []rpc.Transfer{{Burn: 1500000000, SCID: crypto.ZEROHASH}},
			want:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasEmptyValueDestination(tc.transfers); got != tc.want {
				t.Fatalf("hasEmptyValueDestination() = %v, want %v", got, tc.want)
			}
		})
	}
}
