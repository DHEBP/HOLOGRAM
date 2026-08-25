package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/rpc"
	"github.com/gorilla/websocket"
)

// repoFakeDaemon is a BlockchainClient that answers GetSC from a fixed table.
// Named apart from the package's existing fakeDaemon, whose GetSC is a nil stub.
type repoFakeDaemon struct {
	code    map[string]interface{}
	err     error
	last    string
	vars    map[string]interface{}
	varsErr error
}

func (f *repoFakeDaemon) GetSC(scid string, _, _ bool) (map[string]interface{}, error) {
	f.last = scid
	return f.code, f.err
}

func (f *repoFakeDaemon) Call(string, interface{}) (interface{}, error) { return nil, nil }
func (f *repoFakeDaemon) GetInfo() (map[string]interface{}, error)      { return nil, nil }
func (f *repoFakeDaemon) GetSCVariables(string, bool, bool) (map[string]interface{}, error) {
	return f.vars, f.varsErr
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

// telaDaemonStub is the smallest daemon a TELA read needs: a websocket at /ws
// answering DERO.GetSC from a fixed table.
//
// tela dials ws://<endpoint>/ws itself (getContractVars), so an INDEX cannot be
// handed to GetRepositoryFiles any other way. A SCID the table does not carry
// answers with an RPC error, which is exactly what an unreadable entry looks
// like from this side.
type telaDaemonStub struct {
	server *httptest.Server
	vars   map[string]map[string]interface{}
}

func newTELADaemonStub(t *testing.T) *telaDaemonStub {
	t.Helper()

	stub := &telaDaemonStub{vars: map[string]map[string]interface{}{}}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var req struct {
				ID     interface{} `json:"id"`
				Method string      `json:"method"`
				Params struct {
					SCID string `json:"scid"`
				} `json:"params"`
			}
			if json.Unmarshal(message, &req) != nil {
				continue
			}

			reply := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID}
			if vars, ok := stub.vars[req.Params.SCID]; ok && req.Method == "DERO.GetSC" {
				reply["result"] = map[string]interface{}{"status": "OK", "stringkeys": vars}
			} else {
				reply["error"] = map[string]interface{}{"code": -32098, "message": "SCID not found"}
			}

			out, _ := json.Marshal(reply)
			if conn.WriteMessage(websocket.TextMessage, out) != nil {
				return
			}
		}
	}))

	t.Cleanup(stub.server.Close)
	return stub
}

// serveINDEX registers a real TELA-INDEX-1 contract listing docSCID verbatim.
//
// The DOC SCID is spliced into the contract AFTER tela rendered it, which is the
// whole point: tela applies no length rule to a listed SCID (parseINDEXForDOCs
// returns whatever the contract stored), so a published INDEX can carry any
// string there. parseDocShards has its own len(scid) != 64 guard, so this is a
// gap tela knows about elsewhere.
func (s *telaDaemonStub) serveINDEX(t *testing.T, scid string, docSCIDs ...string) {
	t.Helper()

	placeholders := make([]string, len(docSCIDs))
	for i := range docSCIDs {
		placeholders[i] = strings.Repeat(string(rune('a'+i)), 64)
	}

	index := tela.INDEX{DURL: "hostile.tela", DOCs: placeholders}
	index.NameHdr = "hostile"
	index.DescrHdr = "an INDEX built for a test"

	args, err := tela.NewInstallArgs(&index)
	if err != nil {
		t.Fatalf("NewInstallArgs: %v", err)
	}

	code := ""
	for _, arg := range args {
		if arg.Name == rpc.SCCODE {
			code, _ = arg.Value.(string)
		}
	}
	if code == "" {
		t.Fatal("NewInstallArgs produced no SC_CODE")
	}
	for i, placeholder := range placeholders {
		if !strings.Contains(code, placeholder) {
			t.Fatal("the rendered INDEX does not carry its DOC SCIDs verbatim; this stub needs updating")
		}
		code = strings.Replace(code, placeholder, docSCIDs[i], 1)
	}

	s.vars[scid] = map[string]interface{}{
		"C":     hex.EncodeToString([]byte(code)),
		"dURL":  hex.EncodeToString([]byte(index.DURL)),
		"owner": hex.EncodeToString([]byte("anon")),
	}
}

// serveDOC registers a real TELA-DOC-1 contract. Its stored body is irrelevant
// here: repositoryFileFromDOC reads the body through a.daemonClient, so the test
// controls the size from there.
func (s *telaDaemonStub) serveDOC(t *testing.T, scid, name string) {
	t.Helper()

	doc := tela.DOC{DocType: "TELA-JS-1", DURL: "hostile.tela", Code: "//"}
	doc.NameHdr = name
	// A DOC contract stores its author signature; NewInstallArgs refuses an empty
	// one. Nothing here verifies it, so any non-empty value renders a valid
	// TELA-DOC-1 for GetDOCInfo to parse.
	doc.CheckC = "0"
	doc.CheckS = "0"

	args, err := tela.NewInstallArgs(&doc)
	if err != nil {
		t.Fatalf("NewInstallArgs(DOC): %v", err)
	}

	code := ""
	for _, arg := range args {
		if arg.Name == rpc.SCCODE {
			code, _ = arg.Value.(string)
		}
	}
	if code == "" {
		t.Fatal("NewInstallArgs produced no SC_CODE for the DOC")
	}

	s.vars[scid] = map[string]interface{}{
		"C":       hex.EncodeToString([]byte(code)),
		"dURL":    hex.EncodeToString([]byte(doc.DURL)),
		"docType": hex.EncodeToString([]byte(doc.DocType)),
		"nameHdr": hex.EncodeToString([]byte(name)),
	}
}

// TestGetRepositoryFiles_ShortDOCSCIDIsListedNotFatal pins that an INDEX listing
// a SCID shorter than 16 characters is reported rather than fatal.
//
// Wails recovers a panic in its dispatcher and then never sends the callback, so
// the frontend await never settles: the repository pane would sit on "Reading
// contracts…" forever, armed against every viewer by whoever published the INDEX.
func TestGetRepositoryFiles_ShortDOCSCIDIsListedNotFatal(t *testing.T) {
	stub := newTELADaemonStub(t)
	indexSCID := strings.Repeat("b", 64)
	stub.serveINDEX(t, indexSCID, "abc")

	app := &App{
		daemonClient: &repoFakeDaemon{},
		settings:     map[string]interface{}{"daemon_endpoint": stub.server.URL},
	}

	result := app.GetRepositoryFiles(indexSCID)
	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("expected the INDEX to be read, got error %v", result["error"])
	}

	files, _ := result["files"].([]RepositoryFile)
	if len(files) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(files))
	}
	if files[0].SCID != "abc" {
		t.Fatalf("entry SCID = %q, want %q", files[0].SCID, "abc")
	}
	if files[0].Kind != "unreadable" {
		t.Fatalf("entry Kind = %q, want unreadable", files[0].Kind)
	}
	if files[0].Name == "" {
		t.Fatal("entry has no name")
	}
}

// TestShortSCID pins the naming helper every chain-supplied SCID goes through.
func TestShortSCID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{strings.Repeat("a", 16), strings.Repeat("a", 16)},
		{strings.Repeat("a", 17), strings.Repeat("a", 16) + "…"},
		{strings.Repeat("a", 64), strings.Repeat("a", 16) + "…"},
	}
	for _, tc := range cases {
		if got := shortSCID(tc.in); got != tc.want {
			t.Errorf("shortSCID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestRepositoryFileFromDOC_NoDaemonIsStatedNotFatal pins that a file entry read
// with no daemon connection is a stated reason rather than a nil dereference.
//
// connectToExternalNode restores a nil oldClient when a connection test fails,
// and every other GetSC caller in this package already guards for it.
func TestRepositoryFileFromDOC_NoDaemonIsStatedNotFatal(t *testing.T) {
	app := &App{}

	got := app.repositoryFileFromDOC(strings.Repeat("a", 64), tela.DOC{})

	if got.Reason == "" {
		t.Fatal("Reason is empty; no daemon connection must say so rather than crash")
	}
	if got.Content != "" {
		t.Fatalf("Content length = %d, want 0", len(got.Content))
	}
}

// TestGetDOCSignatures_ShortDOCSCIDIsListedNotFatal covers the other read the
// repository view fires against the same INDEX in the same Promise.all. Fixing
// only the file list would still leave that INDEX able to hang the signature
// panel.
func TestGetDOCSignatures_ShortDOCSCIDIsListedNotFatal(t *testing.T) {
	stub := newTELADaemonStub(t)
	indexSCID := strings.Repeat("c", 64)
	stub.serveINDEX(t, indexSCID, "abc")

	app := &App{settings: map[string]interface{}{"daemon_endpoint": stub.server.URL}}

	result := app.GetDOCSignatures(indexSCID)
	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("expected the INDEX to be read, got error %v", result["error"])
	}

	signatures, _ := result["signatures"].([]docSignature)
	if len(signatures) != 1 {
		t.Fatalf("expected 1 signature row, got %d", len(signatures))
	}
	if signatures[0].SCID != "abc" {
		t.Fatalf("row SCID = %q, want %q", signatures[0].SCID, "abc")
	}
	if signatures[0].Path == "" {
		t.Fatal("row has no path")
	}
}

// TestGetRepositoryFiles_TotalSizeIsCapped pins the budget across the whole
// reply, not just per file.
//
// maxPlaintextBytes caps ONE file at 2MiB. An INDEX may list many, and the
// measured practical ceiling before tela's own contract-size limit refuses the
// install is 119 DOCs — so without a running total a single call can marshal
// close to 240MB into one Wails message. Entries past the budget stay listed
// with their real size; only the body is withheld.
func TestGetRepositoryFiles_TotalSizeIsCapped(t *testing.T) {
	const perFile = maxPlaintextBytes // 2MiB each
	const count = 6                   // 12MiB total against an 8MiB budget

	stub := newTELADaemonStub(t)
	indexSCID := strings.Repeat("f", 64)

	docSCIDs := make([]string, count)
	for i := range docSCIDs {
		docSCIDs[i] = strings.Repeat(fmt.Sprintf("%x", i), 64)
		stub.serveDOC(t, docSCIDs[i], fmt.Sprintf("file%d.js", i))
	}
	stub.serveINDEX(t, indexSCID, docSCIDs...)

	app := &App{
		daemonClient: &repoFakeDaemon{code: map[string]interface{}{"code": wrapDoc(strings.Repeat("x", perFile))}},
		settings:     map[string]interface{}{"daemon_endpoint": stub.server.URL},
	}

	result := app.GetRepositoryFiles(indexSCID)
	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("expected the INDEX to be read, got error %v", result["error"])
	}

	files, _ := result["files"].([]RepositoryFile)
	if len(files) != count {
		t.Fatalf("got %d entries, want all %d listed even when their bodies are not", len(files), count)
	}

	total, withheld := 0, 0
	for _, f := range files {
		total += len(f.Content)
		if f.Content == "" {
			withheld++
			if f.Reason == "" {
				t.Errorf("%s carries no body and no reason", f.Name)
			}
			if f.Bytes != perFile {
				t.Errorf("%s reports %d bytes, want its real size %d", f.Name, f.Bytes, perFile)
			}
		}
	}

	if total > maxRepositoryBytes {
		t.Fatalf("reply carries %d bytes of file bodies, past the %d-byte budget", total, maxRepositoryBytes)
	}
	if withheld == 0 {
		t.Fatalf("nothing was withheld from %d x %d bytes against a %d-byte budget", count, perFile, maxRepositoryBytes)
	}
}
