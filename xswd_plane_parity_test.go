package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The Browser plane keeps its own method -> permission table, because it gates calls in the
// renderer and cannot make a round trip per call. Two tables for one fact have already
// drifted once (DERO.GetHeight was routed to the wallet and demanded balance access), so
// this pins them together: every wallet door Go knows about must be spelled the same way in
// Browser.svelte's walletMethodPermission.
//
// It parses the Svelte switch rather than asserting the function merely exists — a test that
// only checks for a symbol passes against code that never calls it.
func TestBrowserPlaneKnowsEveryGatedMethod(t *testing.T) {
	const sveltePath = "frontend/src/routes/Browser.svelte"

	src, err := os.ReadFile(sveltePath)
	if err != nil {
		t.Fatalf("read %s: %v", sveltePath, err)
	}

	svelteTable := parseWalletMethodPermission(t, string(src))
	if len(svelteTable) == 0 {
		t.Fatal("parsed no cases from walletMethodPermission — the parser or the function moved")
	}

	// Only the doors matter. Public chain data is free after connect, and spending is
	// approved per action, so neither is gated by the renderer's table.
	for _, method := range []string{
		"GetAddress", "GetPublicKey", "MakeIntegratedAddress", "SplitIntegratedAddress",
		"GetBalance", "GetHeight", "GetTransfers", "GetTransferbyTXID",
	} {
		want := GetRequiredPermission(method)
		if want != PermissionViewAddress && want != PermissionViewBalance {
			t.Fatalf("%s is no longer a wallet door in Go (got %q) — update this list", method, want)
		}

		key := strings.ToLower(strings.TrimPrefix(method, "DERO."))
		got, ok := svelteTable[key]
		if !ok {
			t.Errorf("%s: Go requires %q but Browser.svelte has no case for %q — the renderer would let it through ungated", method, want, key)
			continue
		}
		if got != string(want) {
			t.Errorf("%s: Go requires %q, Browser.svelte returns %q", method, want, got)
		}
	}
}

// parseWalletMethodPermission pulls the case -> permission pairs out of the Svelte switch.
func parseWalletMethodPermission(t *testing.T, src string) map[string]string {
	t.Helper()

	start := strings.Index(src, "function walletMethodPermission")
	if start < 0 {
		t.Fatal("walletMethodPermission not found in Browser.svelte — was it renamed?")
	}
	body := src[start:]
	if end := strings.Index(body, "\n  }"); end > 0 {
		body = body[:end]
	}

	caseRe := regexp.MustCompile(`case '([^']+)':`)
	returnRe := regexp.MustCompile(`return '([^']+)';`)

	table := map[string]string{}
	var pending []string
	for _, line := range strings.Split(body, "\n") {
		if m := caseRe.FindStringSubmatch(line); m != nil {
			pending = append(pending, m[1])
			continue
		}
		if m := returnRe.FindStringSubmatch(line); m != nil {
			for _, c := range pending {
				table[c] = m[1]
			}
			pending = nil
		}
	}
	return table
}
