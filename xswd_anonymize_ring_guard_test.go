package main

import (
	"os"
	"strings"
	"testing"
)

// The XSWD/dApp transfer path MUST clamp the ring size up to at least
// minAnonymizeRingSize whenever a request sets anonymize. anonymize is a
// structural no-op at ring size 2 (the fork frames a decoy only when the ring has
// >2 members, so the receiver decodes the REAL sender), while the approval modal
// promises "a decoy ring member, not your address". Wiring params["anonymize"] ->
// AttributionAnonymous WITHOUT the clamp is therefore a false-privacy deanonymization
// reachable by any permission-granted dApp.
//
// This is the sentinel the attribution design ledger (O3) required: it fails if the
// anonymize->attribution mapping is present without the ring clamp, so the fix cannot
// silently regress.
func TestXSWDAnonymizeClampsRingSize(t *testing.T) {
	src, err := os.ReadFile("wallet.go")
	if err != nil {
		t.Fatalf("read wallet.go: %v", err)
	}
	s := string(src)

	if !strings.Contains(s, "minAnonymizeRingSize = 16") {
		t.Fatal("minAnonymizeRingSize const missing or no longer 16 -- this is the anonymize ring floor for the dApp path")
	}

	// If the code maps a request to anonymous attribution, the clamp must be present.
	if strings.Contains(s, "walletapi.AttributionAnonymous") &&
		!strings.Contains(s, "ringsize = minAnonymizeRingSize") {
		t.Fatal("wallet.go sets AttributionAnonymous but the `ringsize = minAnonymizeRingSize` clamp is gone -- " +
			"anonymize would be a no-op at ring 2 and the approval modal would promise a decoy while the receiver " +
			"decodes the real sender (false-privacy deanonymization). Re-add the ring clamp on the XSWD transfer path.")
	}
}
