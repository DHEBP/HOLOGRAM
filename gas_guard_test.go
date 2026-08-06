package main

import (
	"strings"
	"testing"

	"github.com/deroproject/derohe/config"
)

// These test the DECISION, not the round trip: guardStorageGas dials a daemon, so the part
// that can be pinned here is what it does with the number it gets back. The estimate itself is
// covered by the hands-on check in the commit message.

// The ceiling is a hard chain limit, not a fee the user can outbid — dvm/sc.go clamps whatever
// gas is provided down to it. A write above it can never execute.
func TestStorageGasGuard_RefusesOnlyAboveTheChainCeiling(t *testing.T) {
	ceiling := uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)

	if storageGasExceeded(ceiling - 1) {
		t.Fatal("just under the ceiling must be allowed")
	}
	// Exactly the ceiling is spendable: the panic fires on used > limit, not >=.
	if storageGasExceeded(ceiling) {
		t.Fatal("exactly the ceiling must be allowed — the chain refuses only what is above it")
	}
	if !storageGasExceeded(ceiling + 1) {
		t.Fatal("one over the ceiling must be refused — NEGATIVE CONTROL, this is the whole point")
	}
}

// "Too large" leaves the user guessing. Storage gas is charged per byte stored, so the overage
// is close enough to a byte count to act on, and the message has to carry it.
func TestStorageGasError_SaysHowMuchToCut(t *testing.T) {
	ceiling := uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)
	err := storageGasError("setting this variable", ceiling+77)
	if err == nil {
		t.Fatal("expected an error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "setting this variable") {
		t.Fatalf("must name what was refused: %q", msg)
	}
	if !strings.Contains(msg, "77") {
		t.Fatalf("must say how far over the limit it is, got %q", msg)
	}
}

// The ceiling is read from derohe rather than copied into HOLOGRAM, so a consensus change
// cannot leave a stale number here. This pins that it stays imported.
func TestStorageGasCeiling_ComesFromTheChainNotACopy(t *testing.T) {
	if config.MAX_STORAGE_GAS_ATOMIC_UNITS != 20000 {
		t.Logf("chain ceiling moved to %d — expected, the guard follows it automatically",
			config.MAX_STORAGE_GAS_ATOMIC_UNITS)
	}
	if storageGasExceeded(uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)) {
		t.Fatal("guard must track the chain constant, whatever its value")
	}
}
