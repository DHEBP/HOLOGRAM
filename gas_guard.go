package main

import (
	"fmt"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/config"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/walletapi"
)

// Storage-gas pre-flight for smart contract writes.
//
// A DVM call is metered on how much it stores, and the chain clamps that budget to
// MAX_STORAGE_GAS_ATOMIC_UNITS no matter what fee is attached (dvm/sc.go: whatever gas may be
// provided, upper limit of gas is this). A write that needs more than the ceiling cannot
// execute at any price: the interpreter panics with "Insufficient Storage Gas", the block
// connector discards every change, and the transaction is still mined and still costs the fee.
//
// Nothing upstream refuses it first. NewSetVarArgs bounds the key at 256 characters and leaves
// the value unbounded; NewUpdateArgs bounds nothing. So the write is built, broadcast, mined,
// paid for, and silently dropped.
//
// ⚠️ The estimate lies by omission, and this is the part worth remembering. DERO.GetGasEstimate
// runs the contract with fees=0, and dvm/sc.go only arms the limiter when the incoming gas is
// above zero — so the estimate never errors, however oversized the payload. It "passes" at
// 250 KB. But ConsumeStorageGas adds to GasStoreUsed unconditionally and the flag only decides
// whether to panic, so the NUMBER it reports is accurate even while the limiter is off.
//
// Therefore: never read a successful estimate as "this will work". Read the number.

// storageGasExceeded reports whether a call needs more storage gas than one call may ever use.
// Exactly the ceiling is fine — the chain refuses only what is strictly above it.
func storageGasExceeded(gas uint64) bool {
	return gas > config.MAX_STORAGE_GAS_ATOMIC_UNITS
}

// storageGasError says how much too big the write is, because "too large" leaves the user
// guessing at what to cut. Storage gas is charged per byte stored, so the overage doubles as a
// byte count close enough to act on.
func storageGasError(what string, gas uint64) error {
	return fmt.Errorf("%s needs %d storage gas but a single contract call may use at most %d — shorten it by roughly %d bytes",
		what, gas, uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS), gas-uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS))
}

// guardStorageGas asks the daemon what a call would cost and refuses to broadcast one that
// cannot execute. Ring size 2 matches what tela's own SetVar and Updater use (transfer0).
//
// The daemon is asked rather than the arithmetic reproduced here: the chain already computes
// this exact number, and a second copy of its rules in HOLOGRAM would drift from it silently.
// The cost is one extra round trip on a user-initiated write, which tela then repeats inside
// its own Transfer; that is cheap next to broadcasting a transaction that cannot succeed.
func guardStorageGas(wallet *walletapi.Wallet_Disk, args rpc.Arguments, what string) error {
	if wallet == nil {
		return fmt.Errorf("no wallet open")
	}

	gas, err := tela.GetGasEstimate(wallet, 2, nil, args)
	if err != nil {
		return fmt.Errorf("could not estimate the cost of %s: %w", what, err)
	}

	if storageGasExceeded(gas) {
		return storageGasError(what, gas)
	}

	return nil
}
