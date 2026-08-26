package main

import (
	"fmt"
	"strings"

	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/proof"
)

// Offline proof receipts.
//
// ValidateProofFull (explorer_service.go) can only check a proof by calling
// DERO.GetTransaction on a daemon first, because proof.Prove needs the raw
// transaction and the completed ring and neither is recoverable from the proof
// string alone. That puts a third party in the verification path: whoever
// serves that RPC learns which transaction the user cares about, and that they
// are a party to it.
//
// A sender holds every input Prove needs at the moment they send. So the sender
// can package them once and hand them over, and the receiver can verify with no
// daemon, no explorer and no network at all.
//
// This does NOT make anything on chain more private -- the transaction is still
// public and still has a visible txid. It removes the lookup, not the record.

// ExportProofBundle builds a self-contained, shareable receipt for a
// transaction the caller already knows about.
//
// This half still needs the daemon, because it is assembling the receipt from
// chain data. That is fine: it runs on the SENDER's machine, about the sender's
// own transaction. The privacy win is on the receiving side.
func (a *App) ExportProofBundle(proofString string, txid string) map[string]interface{} {
	if strings.TrimSpace(proofString) == "" {
		return map[string]interface{}{"success": false, "error": "No proof string provided"}
	}
	if strings.TrimSpace(txid) == "" {
		return map[string]interface{}{"success": false, "error": "Transaction ID required to build a receipt"}
	}

	a.logToConsole(fmt.Sprintf("[WALLET] Building offline proof receipt for %s", txid))

	txHex, ringData, err := a.fetchTxAndRing(txid)
	if err != nil {
		return ErrorResponse(err)
	}

	bundle := proof.NewBundle(a.proofNetworkName(), txHex, ringData, proofString)

	// Verify before handing it out. A receipt that does not verify on the
	// sender's own machine will not verify on the receiver's, and finding that
	// out now is much cheaper than after it has been sent.
	if _, err := bundle.Verify(); err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Refusing to export a receipt that does not verify: %v", err))
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Receipt does not verify locally, refusing to export: %v", err),
		}
	}

	data, err := bundle.Marshal()
	if err != nil {
		return ErrorResponse(err)
	}

	a.logToConsole("[OK] Offline proof receipt built and self-verified")
	return map[string]interface{}{
		"success": true,
		"bundle":  string(data),
		"txid":    bundle.TXID,
		"network": bundle.Network,
	}
}

// VerifyProofBundle checks a receipt with NO network access whatsoever.
//
// This is the point of the feature. No daemon, no explorer, no RPC -- it works
// on an air-gapped machine, and nobody learns which transaction was checked.
func (a *App) VerifyProofBundle(bundleJSON string) map[string]interface{} {
	if strings.TrimSpace(bundleJSON) == "" {
		return map[string]interface{}{"success": false, "error": "No receipt provided"}
	}

	a.logToConsole("[WALLET] Verifying proof receipt offline")

	bundle, err := proof.UnmarshalBundle([]byte(bundleJSON))
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}

	// Bundle.Verify recovers internally -- a receipt is untrusted input by
	// construction, and derohe's transaction parser panics on some malformed
	// bytes rather than returning an error.
	result, err := bundle.Verify()
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] Receipt did not verify: %v", err))
		return map[string]interface{}{
			"success": true,
			"valid":   false,
			"error":   err.Error(),
		}
	}

	// Same hard-cap screen ValidateProofFull applies, so a fabricated amount
	// cannot be displayed as real through this path either.
	for i, amount := range result.Amounts {
		if verr := proof.ValidatePayloadProofAmount(amount); verr != nil {
			a.logToConsole(fmt.Sprintf("[SECURITY] Fake receipt rejected for receiver %d: %s", i+1, verr))
			return map[string]interface{}{
				"success":      true,
				"valid":        false,
				"error":        verr.Error(),
				"securityNote": "This receipt claims an amount that exceeds the DERO hard cap (21M) and is therefore fabricated.",
			}
		}
	}

	receivers := make([]map[string]interface{}, 0, len(result.Receivers))
	for i := range result.Receivers {
		entry := map[string]interface{}{"address": result.Receivers[i]}
		if i < len(result.Amounts) {
			entry["amount"] = result.Amounts[i]
			entry["amountFormatted"] = globals.FormatMoney(result.Amounts[i])
		}
		if i < len(result.Payloads) {
			entry["payload"] = result.Payloads[i]
		}
		receivers = append(receivers, entry)
	}

	a.logToConsole(fmt.Sprintf("[OK] Receipt verified offline, %d receiver(s)", len(receivers)))
	return map[string]interface{}{
		"success":   true,
		"valid":     true,
		"txid":      result.TXID,
		"network":   bundle.Network,
		"receivers": receivers,
		"offline":   true,
	}
}

// proofNetworkName reports the network name the proof package expects.
func (a *App) proofNetworkName() string {
	if network, ok := a.settings["network"].(string); ok && network == "simulator" {
		return "testnet"
	}
	return "mainnet"
}

// fetchTxAndRing pulls the raw transaction hex and completed ring for a txid.
// Extracted from ValidateProofFull so both paths share one implementation.
func (a *App) fetchTxAndRing(txid string) (txHex string, ringData [][]string, err error) {
	params := map[string]interface{}{
		"txs_hashes":     []string{txid},
		"decode_as_json": 1,
	}

	result, err := a.daemonClient.Call("DERO.GetTransaction", params)
	if err != nil {
		return "", nil, err
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("unexpected daemon response for %s", txid)
	}

	if txsHex, ok := resultMap["txs_as_hex"].([]interface{}); ok && len(txsHex) > 0 {
		if hexStr, ok := txsHex[0].(string); ok {
			txHex = hexStr
		}
	}

	if txs, ok := resultMap["txs"].([]interface{}); ok && len(txs) > 0 {
		if tx, ok := txs[0].(map[string]interface{}); ok {
			if ring, ok := tx["ring"].([]interface{}); ok {
				for _, payload := range ring {
					var payloadRing []string
					if addresses, ok := payload.([]interface{}); ok {
						for _, addr := range addresses {
							if addrStr, ok := addr.(string); ok {
								payloadRing = append(payloadRing, addrStr)
							}
						}
					}
					ringData = append(ringData, payloadRing)
				}
			}
		}
	}

	if txHex == "" {
		return "", nil, fmt.Errorf("could not extract transaction hex from daemon response")
	}
	return txHex, ringData, nil
}
