package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/deroproject/derohe/config"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
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

// "Too large" leaves the user guessing. The overage in gas is exact and must be carried; the
// byte figure is HALF it, because a byte the contract stores is charged on both spans — once
// in the marshalled arguments (dvm/sc.go) and once in the marshalled stored value
// (dvm/dvm_store.go). Reporting the gas overage as a byte count was a live 2x error.
func TestStorageGasError_SaysHowMuchToCut(t *testing.T) {
	ceiling := uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)
	err := storageGasError("setting this variable", ceiling+78)
	if err == nil {
		t.Fatal("expected an error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "setting this variable") {
		t.Fatalf("must name what was refused: %q", msg)
	}
	if !strings.Contains(msg, "78") {
		t.Fatalf("must say how far over the limit it is in gas, got %q", msg)
	}
	// NEGATIVE CONTROL: 39 bytes, not 78. The old message said 78 bytes and was twice the
	// real cut; reverting the /2 makes this line fail.
	if !strings.Contains(msg, "39") {
		t.Fatalf("byte figure must be half the gas overage, got %q", msg)
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

// ===== guard behaviour against a stubbed daemon =====

func gasResult(n uint64) interface{} {
	return map[string]interface{}{"gasstorage": float64(n)}
}

// The guard must never invent a way to lose a write. When the size cannot be measured — no
// daemon configured, an unreachable node, a contract without the entrypoint — the call
// proceeds exactly as it did before the guard existed.
//
// An earlier version failed CLOSED here and refused a write that would have succeeded: it
// asked tela, which dials walletapi.Daemon_Endpoint_Active, and simulator mode deliberately
// blanks that global, so the estimate fell back to the testnet port and could not connect.
func TestGuardStorageGas_FailsOpenWhenItCannotMeasure(t *testing.T) {
	args := rpc.Arguments{{Name: "entrypoint", DataType: rpc.DataString, Value: "SetVar"}}

	noDaemon := &App{}
	if err := noDaemon.guardStorageGas(args, "", "setting this variable"); err != nil {
		t.Fatalf("no daemon means no measurement, so the write must proceed; got %v", err)
	}
	if _, ok := noDaemon.storageGasFor(args, 2, ""); ok {
		t.Fatal("storageGasFor must report ok=false with no daemon to ask")
	}

	broken := &App{daemonClient: &fakeDaemon{err: errors.New("connection refused")}}
	if err := broken.guardStorageGas(args, "", "setting this variable"); err != nil {
		t.Fatalf("an unreachable daemon must not block the write; got %v", err)
	}

	garbled := &App{daemonClient: &fakeDaemon{result: map[string]interface{}{"nope": 1}}}
	if err := garbled.guardStorageGas(args, "", "setting this variable"); err != nil {
		t.Fatalf("an answer without gasstorage is not a measurement; got %v", err)
	}
}

// The one thing it MUST block: a size the daemon actually reported as over the ceiling.
func TestGuardStorageGas_RefusesAMeasuredOversize(t *testing.T) {
	args := rpc.Arguments{{Name: "entrypoint", DataType: rpc.DataString, Value: "SetVar"}}
	ceiling := uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)

	over := &App{daemonClient: &fakeDaemon{result: gasResult(ceiling + 77)}}
	err := over.guardStorageGas(args, "", "setting this variable")
	if err == nil {
		t.Fatal("a measured oversize must be refused — NEGATIVE CONTROL, this is the whole point")
	}
	if !strings.Contains(err.Error(), "77") {
		t.Fatalf("the refusal must say how far over it is, got %q", err)
	}

	at := &App{daemonClient: &fakeDaemon{result: gasResult(ceiling)}}
	if err := at.guardStorageGas(args, "", "setting this variable"); err != nil {
		t.Fatalf("exactly the ceiling is spendable and must pass; got %v", err)
	}
}

// A hash argument is a crypto.Hash ([32]byte), which JSON-encodes as an array of numbers.
// The daemon reads hashes as hex, so the SCID has to be converted or the estimate silently
// describes a different call than the one about to be sent.
func TestStorageGasFor_SendsAHashAsHex(t *testing.T) {
	scid := "fcf1270f7b98b9b517c9b4d7951fed01dc74dc66313b0941186c55f317b5b6c3"
	fake := &fakeDaemon{result: gasResult(10)}
	app := &App{daemonClient: fake}

	app.storageGasFor(rpc.Arguments{
		{Name: rpc.SCID, DataType: rpc.DataHash, Value: crypto.HashHexToHash(scid)},
	}, 2, "dero1qsigner")

	params, ok := fake.lastParams.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected params shape %#v", fake.lastParams)
	}
	if fake.lastMethod != "DERO.GetGasEstimate" {
		t.Fatalf("wrong method %q", fake.lastMethod)
	}
	if params["signer"] != "dero1qsigner" {
		t.Fatalf("signer must be forwarded, got %#v", params["signer"])
	}

	rows, ok := params["sc_rpc"].([]map[string]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("unexpected sc_rpc %#v", params["sc_rpc"])
	}
	if got := rows[0]["value"]; got != scid {
		t.Fatalf("hash must be sent as hex, got %#v", got)
	}
}

// ===== funding the write, not just refusing the oversized one =====

// The guard above only ever refused writes ABOVE the chain ceiling. The far more common loss
// is a write BELOW it that is simply not paid for: the fee IS the storage budget, the wallet's
// own formula covers 1.5 atomic units per SCDATA byte, and the chain charges 2 for a byte that
// is both passed and stored. Measured on a simulator chain against a contract storing its
// argument verbatim — 100 bytes stored, and 500, 2,000 and 5,000 bytes were each mined,
// charged and silently discarded.
func TestStorageTopUp_PaysForWritesTheDefaultFeeWouldLose(t *testing.T) {
	// The measured curve. carried is what the wallet's own fee formula supplies.
	cases := []struct {
		bytes, carried, need uint64
		wantRebuild          bool
	}{
		{100, 360, 287, false},    // already covered — must NOT pay for a second proof
		{500, 960, 1088, true},    // LOST today
		{2000, 3060, 4089, true},  // LOST today
		{5000, 7560, 10089, true}, // LOST today
	}
	for _, c := range cases {
		fee, rebuild := storageTopUp(c.carried, c.need)
		if rebuild != c.wantRebuild {
			t.Fatalf("%d bytes: carried %d against need %d — rebuild=%v, want %v",
				c.bytes, c.carried, c.need, rebuild, c.wantRebuild)
		}
		if rebuild && fee < c.need {
			t.Fatalf("%d bytes: rebuilding with %d still underpays a need of %d",
				c.bytes, fee, c.need)
		}
	}
}

// NEGATIVE CONTROL, and the whole point of the change: reverting it means never topping up,
// which is exactly the shipped behaviour that loses the write.
func TestStorageTopUp_ShortMeansShort(t *testing.T) {
	if _, rebuild := storageTopUp(960, 1088); !rebuild {
		t.Fatal("a transaction carrying less than the write costs MUST be rebuilt — " +
			"this failing means writes are being mined, charged and discarded")
	}
	// Exactly enough is enough: the DVM panics on used > limit, not >=.
	if _, rebuild := storageTopUp(1088, 1088); rebuild {
		t.Fatal("exactly the measured cost is sufficient and must not force a second proof")
	}
}

// Never lowers a fee. An explicit, generous fee must survive contact with a small estimate,
// or this would quietly under-fund a caller who got it right.
func TestStorageTopUp_OnlyEverRaises(t *testing.T) {
	if _, rebuild := storageTopUp(20000, 300); rebuild {
		t.Fatal("a fee already above the need must be left alone")
	}
}

// Above the ceiling the DVM clamps to it whatever is attached, so asking for more than the
// ceiling spends the user's money on gas the chain will not use. Such a write cannot succeed
// at any price; refusing it is guardStorageGas's job, before broadcast.
func TestStorageTopUp_NeverAsksAboveTheChainCeiling(t *testing.T) {
	ceiling := uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)
	fee, rebuild := storageTopUp(100, ceiling*3)
	if !rebuild {
		t.Fatal("an underfunded oversize write should still be topped to the ceiling")
	}
	if fee != ceiling {
		t.Fatalf("must ask for the ceiling and no more, got %d", fee)
	}
	if _, rebuild := storageTopUp(ceiling, ceiling*3); rebuild {
		t.Fatal("already at the ceiling — a rebuild would buy nothing")
	}
}
