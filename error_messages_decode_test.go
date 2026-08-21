// Copyright 2025 HOLOGRAM Project. All rights reserved.
// Regression test: an unresolvable-destination error must not be misdiagnosed.

package main

import (
	"strings"
	"testing"
)

// TestFriendlyErrorNameDecodeNotMisdiagnosed locks in that a "could not decode name or address"
// failure -- a mistyped/truncated address or an unresolvable on-chain name -- is reported as a
// clear "check the address" message, deterministically. The inner text carries "-32098" and
// "leaf not found", which the friendly-error map (iterated in Go's random order) previously
// matched as either "recipient not registered, click Register Now" or a generic "not found" --
// both wrong, and flipping run to run. If a future edit removes the precedence branch, the
// determinism check below will start failing.
func TestFriendlyErrorNameDecodeNotMisdiagnosed(t *testing.T) {
	// The real wrapped shape from derohe (walletapi/wallet_transfer.go): a decode wrapper around
	// a daemon -32098 leaf-not-found, carrying the user's destination in the trailing name '...'.
	msg := "could not decode name or address err '[-32098] leaf not found: collision, keyhash abc not found' name 'alicee'"

	first := FriendlyErrorString(msg)
	if strings.Contains(strings.ToLower(first), "register now") {
		t.Fatalf("misdiagnosed an unresolvable destination as unregistered-recipient: %q", first)
	}
	if first == "The requested item was not found." {
		t.Fatalf("unresolvable destination fell through to the generic not-found message: %q", first)
	}

	// Determinism: the same input must not flip messages across runs (the map-order bug).
	for i := 0; i < 200; i++ {
		if got := FriendlyErrorString(msg); got != first {
			t.Fatalf("non-deterministic message: run %d returned %q, first returned %q", i, got, first)
		}
	}

	// A genuinely unregistered recipient (decodable address, no decode wrapper) must STILL get
	// the registration guidance -- the fix must not swallow that path.
	unreg := FriendlyErrorString("Account Unregistered")
	if !strings.Contains(strings.ToLower(unreg), "register now") {
		t.Fatalf("unregistered-recipient message regressed: %q", unreg)
	}
}
