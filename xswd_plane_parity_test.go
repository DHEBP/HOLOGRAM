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

// getXSWDBridgeScript exists twice — once in Go for HTTP-served content, once in Svelte for
// srcdoc — and both are string literals, so nothing in the toolchain notices when they
// diverge. They had: the file input interceptor was added to the Go copy only, so a dApp on
// the srcdoc path could never open a file picker.
//
// This pins the capability set rather than the text. The two scripts are allowed to differ
// (the Svelte one carries console forwarding the Go one has no use for); they are not
// allowed to offer different features to the app running inside them.
func TestBridgeScriptsInstallTheSameInterceptors(t *testing.T) {
	goSrc, err := os.ReadFile("server_manager.go")
	if err != nil {
		t.Fatalf("read server_manager.go: %v", err)
	}
	svelteSrc, err := os.ReadFile("frontend/src/routes/Browser.svelte")
	if err != nil {
		t.Fatalf("read Browser.svelte: %v", err)
	}

	installed := regexp.MustCompile(`\[Bridge\] ([A-Za-z ]+ (?:interceptor|guard|receiver)) installed`)
	collect := func(src string) map[string]bool {
		out := map[string]bool{}
		for _, m := range installed.FindAllStringSubmatch(src, -1) {
			out[m[1]] = true
		}
		return out
	}

	goHas, svelteHas := collect(string(goSrc)), collect(string(svelteSrc))
	if len(goHas) == 0 {
		t.Fatal("found no interceptors in server_manager.go — the marker string moved")
	}
	for name := range goHas {
		if !svelteHas[name] {
			t.Errorf("Go bridge installs the %q interceptor and the Svelte bridge does not", name)
		}
	}
	for name := range svelteHas {
		if !goHas[name] {
			t.Errorf("Svelte bridge installs the %q interceptor and the Go bridge does not", name)
		}
	}
}

// A file dragged from the desktop does not advertise 'Files'. On WebKitGTK it arrives as
// text/uri-list,text/html, so a guard testing only for 'Files' never cancels dragover — and
// an uncancelled dragover means the engine never dispatches drop at all: it navigates the
// webview to the dropped file instead, destroying the session with no way back.
//
// That is what shipped, in four places at once, because every site had written the check by
// hand. This pins the predicate wherever it is spelled out: both bridge copies, which cannot
// import the shared helper because they are string literals injected into another document.
func TestBridgeGuardsAcceptURIListDrags(t *testing.T) {
	for _, path := range []string{"server_manager.go", "frontend/src/routes/Browser.svelte"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		// Matches the EXPRESSION, not the phrase: the phrase appears in the comments that
		// explain the bug, so a prose match would pass even with the check deleted.
		if !strings.Contains(string(src), `indexOf.call(t, 'text/uri-list')`) {
			t.Errorf("%s: bridge file-drop guard does not accept text/uri-list, so a desktop "+
				"drag will not be cancelled and WebKit will navigate to the file", path)
		}
	}
}

// The same predicate, in the two documents HOLOGRAM itself owns. These CAN import the shared
// helper, and must — a hand-written copy here is how the bug reached four sites.
func TestParentDocumentsUseTheSharedFileDragPredicate(t *testing.T) {
	for _, path := range []string{"frontend/src/App.svelte", "frontend/src/routes/Browser.svelte"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		// Browser.svelte also CONTAINS the srcdoc bridge as a string literal, and that copy
		// is injected into another document where the import does not exist — so only the
		// component's own code is held to this.
		own := string(src)
		if i := strings.Index(own, "function getXSWDBridgeScript()"); i != -1 {
			own = own[:i]
		}
		if strings.Contains(own, "indexOf.call(t, 'Files')") {
			t.Errorf("%s: hand-written 'Files' check — use isFileDrag() from lib/utils/dragdrop.js", path)
		}
	}
	src, err := os.ReadFile("frontend/src/lib/utils/dragdrop.js")
	if err != nil {
		t.Fatalf("read dragdrop.js: %v", err)
	}
	if !strings.Contains(string(src), `indexOf.call(types, 'text/uri-list')`) {
		t.Fatal("isFileDrag() does not accept text/uri-list — a desktop file drag will not be recognised")
	}
}

// Every log() call in a bridge must have a definition in that same bridge. The Svelte copy
// called log() from its first statement and never declared it, so under 'use strict' the
// whole script threw ReferenceError at "[Bridge] Initializing..." — and the console
// forwarding installed just above it kept working, which hid the failure completely.
func TestBridgeScriptsDefineTheirOwnLogHelper(t *testing.T) {
	for _, f := range []struct{ path, open string }{
		{"server_manager.go", "func getXSWDBridgeScript() string {"},
		{"frontend/src/routes/Browser.svelte", "function getXSWDBridgeScript() {"},
	} {
		src, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		start := strings.Index(string(src), f.open)
		if start < 0 {
			t.Fatalf("%s: getXSWDBridgeScript not found — it was renamed or moved", f.path)
		}
		// Bound the body by the template literal itself. Closing on the first "\n}" or
		// "\n  }" instead matches a brace INSIDE the script, truncating before any log()
		// call and passing vacuously — this test did exactly that until a revert exposed it.
		open := strings.Index(string(src)[start:], "return `")
		if open < 0 {
			t.Fatalf("%s: no template literal in getXSWDBridgeScript", f.path)
		}
		open += start + len("return `")
		close := strings.Index(string(src)[open:], "`")
		if close < 0 {
			t.Fatalf("%s: unterminated bridge template literal", f.path)
		}
		body := string(src)[open : open+close]

		if !strings.Contains(body, "log(") {
			t.Errorf("%s: bridge body has no log() calls at all — the extractor is wrong, "+
				"not the code", f.path)
			continue
		}
		if !strings.Contains(body, "function log(") {
			t.Errorf("%s: getXSWDBridgeScript calls log() but never defines it — "+
				"every statement after the first call is dead under 'use strict'", f.path)
		}
	}
}
