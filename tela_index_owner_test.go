// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Regression test for the INDEX ownership check in UpdateINDEX.

package main

import (
	"testing"

	"github.com/deroproject/derohe/rpc"
)

// renderForSimulator re-renders a mainnet address vector (dero1...) for the
// simulator network (deto1...) without touching the key it wraps. A wallet opened
// on a simulator renders its own address this way, while the author the INDEX
// stored came from the DVM and is always in the dero1 form.
func renderForSimulator(t *testing.T, mainnet string) string {
	t.Helper()
	addr, err := rpc.NewAddress(mainnet)
	if err != nil {
		t.Fatalf("parse vector %s: %v", mainnet, err)
	}
	addr.Mainnet = false
	return addr.String()
}

// TestINDEXAuthorMatchesAcrossNetworkRendering locks in the fix for the defect that
// made update-index impossible on a simulator: the stored author and the wallet
// address are the same key rendered on two networks, and a string compare reads them
// as two different wallets. Both prefix and checksum differ, so the mismatch is total
// and the refusal was systematic.
func TestINDEXAuthorMatchesAcrossNetworkRendering(t *testing.T) {
	const owner = "dero1qyw4fl3dupcg5qlrcsvcedze507q9u67lxfpu8kgnzp04aq73yheqqg2ctjn4"
	const stranger = "dero1qy976ssakhfynpd4lnh39u7gw9spfzr9z55ckfd0yhrhsdr235glgqq28xlvm"

	ownerSim := renderForSimulator(t, owner)
	strangerSim := renderForSimulator(t, stranger)

	if ownerSim == owner {
		t.Fatal("simulator rendering is identical to the mainnet one; the vector says nothing")
	}

	cases := []struct {
		name          string
		stored        string
		walletAddress string
		want          bool
	}{
		{"same rendering", owner, owner, true},
		{"stored mainnet, wallet simulator", owner, ownerSim, true},
		{"stored simulator, wallet mainnet", ownerSim, owner, true},
		{"same rendering, simulator", ownerSim, ownerSim, true},
		{"different wallet, same network", owner, stranger, false},
		{"different wallet, across networks", owner, strangerSim, false},
		{"immutable INDEX", "anon", owner, false},
		{"empty stored author", "", owner, false},
		{"empty wallet address", owner, "", false},
		{"unparseable stored author", "not-an-address", owner, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameINDEXAuthor(tc.stored, tc.walletAddress); got != tc.want {
				t.Fatalf("sameINDEXAuthor(%q, %q) = %v, want %v", tc.stored, tc.walletAddress, got, tc.want)
			}
		})
	}
}

// TestINDEXAuthorIgnoresIntegratedPayload covers the case where one side carries an
// integrated payload: it is the same wallet, and an update must not be refused for a
// payment ID that has nothing to do with ownership.
func TestINDEXAuthorIgnoresIntegratedPayload(t *testing.T) {
	const owner = "dero1qyw4fl3dupcg5qlrcsvcedze507q9u67lxfpu8kgnzp04aq73yheqqg2ctjn4"

	addr, err := rpc.NewAddress(owner)
	if err != nil {
		t.Fatalf("parse owner vector: %v", err)
	}
	addr.Arguments = rpc.Arguments{
		{Name: rpc.RPC_COMMENT, DataType: rpc.DataString, Value: "hologram"},
	}
	integrated := addr.String()
	if integrated == owner {
		t.Skip("integrated rendering unavailable in this build")
	}

	if !sameINDEXAuthor(integrated, owner) {
		t.Fatalf("integrated address %q not recognised as owner of %q", integrated, owner)
	}
}
