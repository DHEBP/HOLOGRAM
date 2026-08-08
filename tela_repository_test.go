package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/civilware/tela"
)

// repoFakeDaemon is a BlockchainClient that answers GetSC from a fixed table.
// Named apart from the package's existing fakeDaemon, whose GetSC is a nil stub.
type repoFakeDaemon struct {
	code map[string]interface{}
	err  error
	last string
}

func (f *repoFakeDaemon) GetSC(scid string, _, _ bool) (map[string]interface{}, error) {
	f.last = scid
	return f.code, f.err
}

func (f *repoFakeDaemon) Call(string, interface{}) (interface{}, error) { return nil, nil }
func (f *repoFakeDaemon) GetInfo() (map[string]interface{}, error)      { return nil, nil }
func (f *repoFakeDaemon) GetSCVariables(string, bool, bool) (map[string]interface{}, error) {
	return nil, nil
}
func (f *repoFakeDaemon) TestConnection() error { return nil }
func (f *repoFakeDaemon) GetEndpoint() string   { return "" }
func (f *repoFakeDaemon) SetEndpoint(_ string)  {}

// wrapDoc builds SC code shaped the way TELA stores a document: contract source,
// then the document inside a trailing multiline comment.
func wrapDoc(body string) string {
	return "Function Initialize()\n10 RETURN 0\nEnd Function\n\n/*\n" + body + "\n*/"
}

func docWith(name, subDir, docType, body string) (tela.DOC, map[string]interface{}) {
	doc := tela.DOC{DocType: docType, SubDir: subDir}
	doc.NameHdr = name
	return doc, map[string]interface{}{"code": wrapDoc(body)}
}

func TestRepositoryFileFromDOC_ReadsDocumentBody(t *testing.T) {
	doc, sc := docWith("app.js", "", "TELA-JS-1", "console.log('hi')")
	app := &App{daemonClient: &repoFakeDaemon{code: sc}}

	got := app.repositoryFileFromDOC(strings.Repeat("a", 64), doc)

	if got.Kind != "doc" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "doc")
	}
	if got.Name != "app.js" {
		t.Fatalf("Name = %q, want %q", got.Name, "app.js")
	}
	if got.Content != "console.log('hi')" {
		t.Fatalf("Content = %q, want the document body only (not the contract)", got.Content)
	}
	if got.Bytes != len("console.log('hi')") {
		t.Fatalf("Bytes = %d, want %d", got.Bytes, len("console.log('hi')"))
	}
	if got.Reason != "" {
		t.Fatalf("Reason = %q, want empty on the happy path", got.Reason)
	}
	if got.SCID != strings.Repeat("a", 64) {
		t.Fatalf("SCID = %q, want the DOC SCID carried through for the signature join", got.SCID)
	}
}

// The signature panel joins on SCID, and the tree keys on Name. Both must come
// from docFileName so a file reads identically in either place.
func TestRepositoryFileFromDOC_NameIncludesSubDir(t *testing.T) {
	doc, sc := docWith("style.css", "assets/css", "TELA-CSS-1", "body{}")
	app := &App{daemonClient: &repoFakeDaemon{code: sc}}

	got := app.repositoryFileFromDOC(strings.Repeat("b", 64), doc)

	if got.Name != "assets/css/style.css" {
		t.Fatalf("Name = %q, want the subdirectory joined in", got.Name)
	}
	if got.SubDir != "assets/css" {
		t.Fatalf("SubDir = %q, want it reported separately too", got.SubDir)
	}
}

func TestRepositoryFileFromDOC_DaemonErrorIsStatedNotSwallowed(t *testing.T) {
	doc, _ := docWith("index.html", "", "TELA-HTML-1", "<p>hi</p>")
	app := &App{daemonClient: &repoFakeDaemon{err: errors.New("connection refused")}}

	got := app.repositoryFileFromDOC(strings.Repeat("c", 64), doc)

	if got.Content != "" {
		t.Fatalf("Content = %q, want empty when the contract could not be read", got.Content)
	}
	if got.Reason == "" {
		t.Fatal("Reason is empty; an unreadable contract must say so rather than render as an empty file")
	}
	if got.Name != "index.html" {
		t.Fatalf("Name = %q, want the entry still listed by name", got.Name)
	}
}

func TestRepositoryFileFromDOC_MissingCodeKey(t *testing.T) {
	doc := tela.DOC{DocType: "TELA-HTML-1"}
	doc.NameHdr = "index.html"
	app := &App{daemonClient: &repoFakeDaemon{code: map[string]interface{}{"variables": 1}}}

	got := app.repositoryFileFromDOC(strings.Repeat("d", 64), doc)

	if got.Reason == "" {
		t.Fatal("Reason is empty; a contract with no code must say so")
	}
	if got.Content != "" {
		t.Fatalf("Content = %q, want empty", got.Content)
	}
}

// A DOC is capped at 18KB on chain, but a compressed one expands. The guard is
// on the decompressed body, so a body past the cap is reported by size and not
// shipped across the bridge.
func TestRepositoryFileFromDOC_OversizeBodyIsReportedNotShipped(t *testing.T) {
	body := strings.Repeat("x", maxPlaintextBytes+1)
	doc, sc := docWith("huge.txt", "", "TELA-TXT-1", body)
	app := &App{daemonClient: &repoFakeDaemon{code: sc}}

	got := app.repositoryFileFromDOC(strings.Repeat("e", 64), doc)

	if got.Content != "" {
		t.Fatalf("Content length = %d, want 0 past the cap", len(got.Content))
	}
	if got.Bytes != len(body) {
		t.Fatalf("Bytes = %d, want the real size %d reported", got.Bytes, len(body))
	}
	if got.Reason == "" {
		t.Fatal("Reason is empty; an oversize file must explain why nothing is shown")
	}
}

func TestRepositoryFileFromDOC_AtCapIsStillShipped(t *testing.T) {
	body := strings.Repeat("x", maxPlaintextBytes)
	doc, sc := docWith("big.txt", "", "TELA-TXT-1", body)
	app := &App{daemonClient: &repoFakeDaemon{code: sc}}

	got := app.repositoryFileFromDOC(strings.Repeat("f", 64), doc)

	if len(got.Content) != maxPlaintextBytes {
		t.Fatalf("Content length = %d, want %d — the cap is exclusive", len(got.Content), maxPlaintextBytes)
	}
	if got.Reason != "" {
		t.Fatalf("Reason = %q, want empty exactly at the cap", got.Reason)
	}
}

func TestGetRepositoryFiles_RejectsBadSCID(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{}}

	for _, scid := range []string{"", "abc", strings.Repeat("a", 63), strings.Repeat("a", 65)} {
		got := app.GetRepositoryFiles(scid)
		if got["success"] != false {
			t.Fatalf("GetRepositoryFiles(%q) succeeded; a malformed SCID must be refused before any daemon call", scid)
		}
	}
}
