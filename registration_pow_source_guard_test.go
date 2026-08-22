package main

// Regression guard for the hot-wallet registration PoW target (a1637ee,
// "fix(wallet): raise registration PoW target 24->28 bits, both paths").
//
// RegisterWallet's mining goroutine (wallet.go) has no unit-test coverage of
// its own -- it needs a live/simulated wallet+daemon to actually mine a
// registration, which is not something a fast unit test should do (the cold
// path already pays that cost once via TestMineRegistrationFromSeed_Live,
// gated behind -short). What CAN be caught cheaply is the specific
// regression this commit is defending against: wallet.go's PoW-acceptance
// check silently drifting away from the shared genesis.MeetsRegistrationTarget
// helper (e.g. a merge conflict resolving back to an inline 3-byte check).
//
// This test reads wallet.go's own source and asserts:
//   - it still calls the shared, tested helper (genesis.MeetsRegistrationTarget)
//   - it does not contain a reintroduced inline hash[0]/hash[1]/hash[2] check,
//     which is exactly the old 24-bit-only acceptance test this commit removed.
//
// It is a source guard, not a behavioural test -- it cannot prove wallet.go's
// goroutine is wired up correctly, only that it hasn't silently reverted to
// bypassing the shared, tested target function.

import (
	"os"
	"regexp"
	"testing"
)

func TestRegisterWalletUsesSharedRegistrationTarget(t *testing.T) {
	src, err := os.ReadFile("wallet.go")
	if err != nil {
		t.Fatalf("read wallet.go: %v", err)
	}
	s := string(src)

	if !regexp.MustCompile(`genesis\.MeetsRegistrationTarget\s*\(\s*hash\s*\)`).MatchString(s) {
		t.Fatal("wallet.go no longer calls genesis.MeetsRegistrationTarget(hash) -- " +
			"the hot-wallet registration miner must accept a PoW winner through the " +
			"same shared, tested function as the cold-wallet miner (wallet/genesis/registration.go), " +
			"not a reimplemented or drifted inline check")
	}

	if regexp.MustCompile(`hash\[0\]\s*==\s*0\s*&&\s*hash\[1\]\s*==\s*0\s*&&\s*hash\[2\]\s*==\s*0`).MatchString(s) {
		t.Fatal("wallet.go contains a reintroduced inline 24-bit (3-zero-byte) PoW acceptance " +
			"check -- this is exactly the pre-a1637ee regression this test exists to catch; " +
			"the acceptance check must live only in genesis.MeetsRegistrationTarget")
	}
}
