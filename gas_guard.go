package main

import (
	"fmt"

	"github.com/deroproject/derohe/config"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
)

// Storage-gas pre-flight for smart contract writes.
//
// A DVM call is metered on how much it stores, and the chain clamps that budget to
// MAX_STORAGE_GAS_ATOMIC_UNITS whatever fee is attached (dvm/sc.go: whatever gas may be
// provided, upper limit of gas is this). A write over the ceiling cannot execute at any price:
// the interpreter panics with "Insufficient Storage Gas", the block connector discards every
// change, and the transaction is still mined and still costs the fee. Reproduced on a
// simulator chain — the transaction landed in a block and the contract stored nothing, while
// the interface reported success.
//
// Nothing upstream stops it: NewSetVarArgs bounds the key at 256 characters and leaves the
// value unbounded, NewUpdateArgs bounds nothing, and the generic invoke path bounds nothing.
//
// ⚠️ The estimate lies by omission. DERO.GetGasEstimate runs the contract with fees=0, and
// dvm/sc.go only arms the limiter when the incoming gas is above zero — so it never errors,
// however oversized the payload; it "passes" at 250 KB. Its NUMBER is still right, because
// ConsumeStorageGas accumulates unconditionally and the flag only decides whether to panic.
// So never read a successful estimate as "this will work". Read the number.
//
// Storage gas is charged TWICE, over two different spans: dvm/sc.go consumes the marshalled
// call arguments, dvm/dvm_store.go consumes the marshalled stored value, and the key is
// marshalled but never charged. A byte that arrives as an argument and is then stored is
// therefore charged 2; a byte that only arrives — a longer key, or an argument the contract
// does not store — is charged 1.
//
// Measured on a setter that stores its argument verbatim: 2 per byte plus fixed overhead,
// putting the edge at 9,949 bytes for the TELA vsoo INDEX with a five-character key (19,999
// fits, 9,950 gives 20,001). A longer key leaves LESS room against the ceiling, since it rides
// in the charged argument blob — which is why this asks the chain rather than counting locally.
//
// ⚠️ A wrong SIGNER looks exactly like a working write. vsoo's SetVar returns 1 unless the
// signer owns the contract, and the daemon reports any non-zero return as "Discarded
// knowingly" — an error, so nothing is measured and the guard fails open at every size. That
// is the right call (such a write was already doomed, for an unrelated reason) but on screen
// it is indistinguishable from having no guard: check the console for "Could not measure"
// before concluding the guard is broken.

// storageGasExceeded reports whether a call needs more storage gas than one call may ever use.
// Exactly the ceiling is fine — the chain refuses only what is strictly above it.
func storageGasExceeded(gas uint64) bool {
	return gas > config.MAX_STORAGE_GAS_ATOMIC_UNITS
}

// storageGasError says how much too big the write is, because "too large" leaves the user
// guessing at what to cut. The overage is reported in gas, which is exact; the byte figure is
// half that, because it assumes the contract stores what it is passed and such a byte is
// charged on both spans. A contract that does not store its argument is charged once and needs
// twice the reported cut, so the message names the assumption rather than hiding it.
func storageGasError(what string, gas uint64) error {
	over := gas - uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)
	return fmt.Errorf("%s needs %d storage gas but a single contract call may use at most %d — %d over, roughly %d bytes if the contract stores what you pass it",
		what, gas, uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS), over, over/2)
}

// storageGasFor asks the daemon what a contract call would store, returning ok=false when no
// answer was available.
//
// It goes through a.daemonClient rather than tela's estimator on purpose. tela dials
// walletapi.Daemon_Endpoint_Active, a global HOLOGRAM deliberately blanks in simulator mode to
// free the daemon's single WebSocket slot (sc_function_parser.go). An earlier version of this
// guard trusted that global, fell back to the testnet default port, and refused a write that
// would have succeeded — a false refusal is worse than no guard. a.daemonClient is HOLOGRAM's
// own configured connection and is unaffected by that blanking.
func (a *App) storageGasFor(args rpc.Arguments, ringsize uint64, signer string) (uint64, bool) {
	if a.daemonClient == nil {
		return 0, false
	}

	// rpc.Argument already carries the {name,datatype,value} JSON shape the daemon reads.
	// The exception is a hash: crypto.Hash is a [32]byte and would serialise as an array of
	// numbers, so it goes as the hex string the daemon expects.
	scRPC := make([]map[string]interface{}, 0, len(args))
	code := ""
	for _, arg := range args {
		value := arg.Value
		if arg.DataType == rpc.DataHash {
			if h, ok := value.(crypto.Hash); ok {
				value = h.String()
			}
		}
		if arg.Name == rpc.SCCODE {
			if s, ok := value.(string); ok {
				code = s
			}
		}
		scRPC = append(scRPC, map[string]interface{}{
			"name":     arg.Name,
			"datatype": string(arg.DataType),
			"value":    value,
		})
	}

	params := map[string]interface{}{"sc_rpc": scRPC, "ringsize": ringsize}
	if signer != "" {
		params["signer"] = signer
	}

	// An INSTALL has to be estimated through the daemon's install branch, and that
	// branch is chosen by the presence of the top-level "sc" parameter — with sc_rpc
	// alone the daemon takes the call branch and answers "cannot install code using
	// this api" (dvm/simulator.go RunSC), which fails open and measures nothing.
	// Measured against a live simulator: without this the estimate errored at every
	// size. A call never carries SC_CODE, so this is inert on the invoke paths, and
	// leaving the argument in sc_rpc as well is the shape tela itself sends.
	if code != "" {
		params["sc"] = code
	}

	result, err := a.daemonClient.Call("DERO.GetGasEstimate", params)
	if err != nil {
		return 0, false
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return 0, false
	}
	gasStorage, ok := resultMap["gasstorage"].(float64)
	if !ok {
		return 0, false
	}
	return uint64(gasStorage), true
}

// storageGasWhat names the contract function in a refusal, so the message reads "calling
// SetVar needs …" rather than something the user has to map back to what they clicked. Falls
// back to a generic phrase; a refusal that cannot name the entrypoint is still worth sending.
func storageGasWhat(args rpc.Arguments) string {
	for _, arg := range args {
		if arg.Name == "entrypoint" {
			if name, ok := arg.Value.(string); ok && name != "" {
				return "calling " + name
			}
		}
	}
	return "this contract call"
}

// storageGasRefusal returns the error a contract call should fail with when the chain could not
// apply it at any fee, or nil to proceed.
//
// The ceiling is not a price. dvm/sc.go clamps whatever fee is attached down to
// MAX_STORAGE_GAS_ATOMIC_UNITS, so a call needing more cannot execute however much is offered:
// the interpreter panics, the connector discards every change, and the transaction still mines
// and still costs the fee. Funding such a call to the ceiling — which is what happens without
// this — just pays for the funeral.
//
// FAILS OPEN on measured=false. Every reason a measurement fails is unrelated to size.
func storageGasRefusal(args rpc.Arguments, need uint64, measured bool) error {
	if !measured || !storageGasExceeded(need) {
		return nil
	}
	return storageGasError(storageGasWhat(args), need)
}

// storageTopUp decides whether a built transaction carries enough fee to pay for the storage
// it is about to perform, and what to rebuild it with if not.
//
// carried is tx.Fees(), not the number handed to the wallet: Fees() sums the zero-SCID
// statements, and that sum is exactly what the block connector passes the DVM as the storage
// budget. Comparing anything else would be comparing against a number the chain never reads.
//
// Only ever raises. A transaction that already pays enough is left alone, so the common small
// call never pays for a second proof. Above the chain ceiling it asks for the ceiling and no
// more — the DVM clamps to it regardless (dvm/sc.go), so a larger fee buys nothing and is
// simply spent. Refusing such a write is a separate job, done before broadcast by
// guardStorageGas.
//
// Measured on a simulator chain against a contract that stores its argument verbatim: a
// 500-byte write carried 960 gas and needed 1,088, and was mined, charged and discarded. At
// exactly the measured cost it stored. NO HEADROOM IS ADDED, and the measurement is what makes
// that safe — an estimate taken over different arguments than the ones sent underpays by the
// difference, because the key rides in the charged argument blob (a six-character longer key
// measured six gas higher). The caller must therefore measure the arguments it actually sends.
func storageTopUp(carried, need uint64) (uint64, bool) {
	if need > uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS) {
		need = uint64(config.MAX_STORAGE_GAS_ATOMIC_UNITS)
	}
	if carried >= need {
		return 0, false
	}
	return need, true
}

// guardStorageGas refuses a write the chain could not apply, and only that.
//
// FAILS OPEN. When the size cannot be measured — no daemon, an unreachable node, a wrong
// signer, a contract without the entrypoint — the write proceeds exactly as it did before this
// guard existed. Blocking instead would refuse writes that would have succeeded, and every one
// of those failure modes is unrelated to size.
//
// Nothing downstream catches what this misses. tela's estimate never compares against the
// ceiling — grep MAX_STORAGE_GAS over the module for zero hits — so an unmeasured oversize is
// broadcast and silently discarded exactly as it was before. That is the accepted cost of not
// inventing a new way to lose a write.
func (a *App) guardStorageGas(args rpc.Arguments, signer, what string) error {
	gas, ok := a.storageGasFor(args, 2, signer)
	if !ok {
		a.logToConsole(fmt.Sprintf("[WARN] Could not measure the cost of %s — proceeding unchecked", what))
		return nil
	}
	if storageGasExceeded(gas) {
		return storageGasError(what, gas)
	}
	return nil
}
