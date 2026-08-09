package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The file-drop guard and receiver are injected into an app document by three separate
// bridges — the srcdoc path in Browser.svelte, the TELA proxy in server_manager.go, and the
// local dev server. Three copies of one script is not a design choice; there is no shared
// asset pattern here (go:embed carries only VERSION and the built frontend), and inventing a
// cross-language one for ~35 lines of JS costs more than it saves. So they are pinned
// instead, the same way TestBrowserPlaneKnowsEveryGatedMethod pins the permission tables.
//
// This matters more than ordinary duplication: local_dev_server.go shipped WITHOUT the drop
// scripts at all, which meant a locally-served app silently received nothing — identical on
// screen to the original bug, and the reason drag-and-drop could only be tested against
// something already published on chain.
//
// It compares executable code, not comments. Each copy documents its own context and those
// are allowed to differ; the behaviour is not.
const (
	guardPath    = "guard"
	receiverPath = "receiver"
)

var dropBridgeSources = map[string]string{
	"Browser.svelte (srcdoc)":         "frontend/src/routes/Browser.svelte",
	"server_manager.go (TELA proxy)":  "server_manager.go",
	"local_dev_server.go (local dev)": "local_dev_server.go",
}

func TestFileDropBridgeParity(t *testing.T) {
	guards := map[string]string{}
	receivers := map[string]string{}

	for label, path := range dropBridgeSources {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)

		guard := sliceBetween(t, label, text,
			"function isFileDrag(t) {",
			"log('[Bridge] File drop guard installed');")
		receiver := sliceBetween(t, label, text,
			"if (!d || d.type !== 'hologram-file-drop') return;",
			"log('[Bridge] File drop receiver installed');")

		guards[label] = normalizeJS(guard)
		receivers[label] = normalizeJS(receiver)
	}

	assertAllEqual(t, guardPath, guards)
	assertAllEqual(t, receiverPath, receivers)
}

// The guard runs in capture on document and cancels anything that looks like a file drag.
// The receiver's own synthetic drop carries a file, so without the isTrusted check the guard
// cancels it before the app's handler runs and defaultPrevented — the signal the parent reads
// back as "the app took the file" — is true for every app. Measured 2026-08-09 against
// calculator.tela, which has no drop handler and reported "app accepted it".
//
// Reverting `e.isTrusted &&` in any one copy fails this test.
func TestFileDropGuardIgnoresSyntheticEvents(t *testing.T) {
	for label, path := range dropBridgeSources {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		guard := sliceBetween(t, label, string(src),
			"function isFileDrag(t) {",
			"log('[Bridge] File drop guard installed');")

		if !strings.Contains(normalizeJS(guard), "if (e.isTrusted && isFileDrag(") {
			t.Errorf("%s: the drop guard cancels untrusted events.\n"+
				"The receiver dispatches a synthetic drop carrying a file; cancelling it in "+
				"capture makes every app report that it accepted the file, including apps with "+
				"no drop handler at all.", label)
		}
	}
}

// sliceBetween returns the text from the line containing start through the line containing
// end. It fails rather than returning empty, because a silently-empty slice would make every
// comparison below trivially pass.
func sliceBetween(t *testing.T, label, text, start, end string) string {
	t.Helper()

	i := strings.Index(text, start)
	if i < 0 {
		t.Fatalf("%s: could not find %q — the bridge script moved or was renamed", label, start)
	}
	rest := text[i:]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("%s: found %q but not its closing %q", label, start, end)
	}
	return rest[:j+len(end)]
}

var (
	lineComment = regexp.MustCompile(`(?m)^\s*//.*$`)
	whitespace  = regexp.MustCompile(`\s+`)
)

// normalizeJS drops comment lines and collapses whitespace, so the copies may be indented
// differently and document their own context while still being compared on behaviour.
func normalizeJS(s string) string {
	s = lineComment.ReplaceAllString(s, "")
	return strings.TrimSpace(whitespace.ReplaceAllString(s, " "))
}

func assertAllEqual(t *testing.T, what string, byLabel map[string]string) {
	t.Helper()

	var refLabel, ref string
	for label, body := range byLabel {
		if body == "" {
			t.Fatalf("%s: extracted an empty %s block — the parser lost its anchors", label, what)
		}
		if ref == "" {
			refLabel, ref = label, body
			continue
		}
		if body != ref {
			t.Errorf("the %s script has drifted between copies.\n\n%s:\n%s\n\n%s:\n%s",
				what, refLabel, ref, label, body)
		}
	}
}
