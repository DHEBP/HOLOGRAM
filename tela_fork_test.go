package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/rpc"
)

// These test the ARGUMENT CONSTRUCTION, which is the whole of a fork that can be
// checked without a chain: a fork is one SC_INSTALL, and everything that makes
// it a fork rather than a new app is in the contract body those arguments carry.
//
// The DOC list is read back out of the generated contract with a regexp rather
// than compared against the struct it came from. Comparing the struct would only
// prove the struct was copied; reading the artifact proves the SCIDs a miner
// would store are the source's, in the source's order.

const (
	srcDoc1 = "1111111111111111111111111111111111111111111111111111111111111111"
	srcDoc2 = "2222222222222222222222222222222222222222222222222222222222222222"
	srcDoc3 = "3333333333333333333333333333333333333333333333333333333333333333"
	srcSCID = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	srcAddr = "dero1qyfgz4gj8pt7dyjabcdefghijklmnopqrstuvwxyz0123456789abcdefghij"
)

func testSourceINDEX() tela.INDEX {
	return tela.INDEX{
		SCID:   srcSCID,
		Author: srcAddr,
		DURL:   "villager.tela",
		Mods:   "vsoo",
		DOCs:   []string{srcDoc1, srcDoc2, srcDoc3},
		Headers: tela.Headers{
			NameHdr:  "Villager",
			DescrHdr: "the original description",
			IconHdr:  "https://example.invalid/icon.svg",
		},
	}
}

func testForkRequest() ForkRequest {
	return ForkRequest{
		SourceSCID:  srcSCID,
		DURL:        "villager-fork.tela",
		Name:        "Villager Fork",
		Description: "my fork",
		IconURL:     "",
	}
}

// installCode pulls the SC_CODE argument out, and fails if the arguments are not
// the two an install is made of.
func installCode(t *testing.T, args rpc.Arguments) string {
	t.Helper()

	if len(args) != 2 {
		t.Fatalf("an install is SC_ACTION + SC_CODE, got %d arguments", len(args))
	}

	var action, code *rpc.Argument
	for i := range args {
		switch args[i].Name {
		case rpc.SCACTION:
			action = &args[i]
		case rpc.SCCODE:
			code = &args[i]
		}
	}
	if action == nil || code == nil {
		t.Fatalf("install arguments must carry %s and %s, got %+v", rpc.SCACTION, rpc.SCCODE, args)
	}
	if got, want := action.Value, uint64(rpc.SC_INSTALL); got != want {
		t.Fatalf("SC_ACTION = %v, want SC_INSTALL (%v) — a fork installs a new contract, it does not call one", got, want)
	}

	s, ok := code.Value.(string)
	if !ok {
		t.Fatalf("SC_CODE is %T, want string", code.Value)
	}
	return s
}

var docLineRe = regexp.MustCompile(`STORE\("DOC(\d+)", "([0-9a-fA-F]{64})"\)`)

// docsFromCode reads the DOC SCIDs back out of a generated contract, in the
// order their numbered keys put them in. DOC1 is the entrypoint, so order is not
// cosmetic.
func docsFromCode(t *testing.T, code string) []string {
	t.Helper()

	matches := docLineRe.FindAllStringSubmatch(code, -1)
	out := make([]string, 0, len(matches))
	for i, m := range matches {
		if want := fmt.Sprintf("%d", i+1); m[1] != want {
			t.Fatalf("DOC keys are out of sequence: entry %d is DOC%s, want DOC%s", i, m[1], want)
		}
		out = append(out, m[2])
	}
	return out
}

func buildForkArgs(t *testing.T, source tela.INDEX, req ForkRequest) (tela.INDEX, rpc.Arguments) {
	t.Helper()

	fork, err := forkINDEX(source, req)
	if err != nil {
		t.Fatalf("forkINDEX: %v", err)
	}
	args, err := tela.NewInstallArgs(&fork)
	if err != nil {
		t.Fatalf("NewInstallArgs: %v", err)
	}
	return fork, args
}

// The headline property: the fork lists the SAME documents, unchanged and in the
// same order. Anything else is a new app wearing a fork's name.
func Test_Fork_PreservesDOCListExactly(t *testing.T) {
	source := testSourceINDEX()
	_, args := buildForkArgs(t, source, testForkRequest())

	got := docsFromCode(t, installCode(t, args))
	if len(got) != len(source.DOCs) {
		t.Fatalf("fork lists %d documents, source lists %d", len(got), len(source.DOCs))
	}
	for i := range source.DOCs {
		if got[i] != source.DOCs[i] {
			t.Fatalf("document %d is %s, want %s — a fork must share the source's DOC contracts, not substitute them",
				i+1, got[i], source.DOCs[i])
		}
	}
}

// Teeth for the test above: if the order were not carried, this would still pass
// on a set comparison. It must not.
func Test_Fork_PreservesDOCOrder(t *testing.T) {
	source := testSourceINDEX()
	source.DOCs = []string{srcDoc3, srcDoc1, srcDoc2}

	_, args := buildForkArgs(t, source, testForkRequest())

	got := docsFromCode(t, installCode(t, args))
	want := []string{srcDoc3, srcDoc1, srcDoc2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DOC%d is %s, want %s — DOC1 is the entrypoint, so order is load bearing", i+1, got[i], want[i])
		}
	}
}

// The source's own slice must not be reachable from the fork, or a later caller
// could reorder the file list of the INDEX it was reading.
func Test_Fork_CopiesDOCSliceRatherThanAliasingIt(t *testing.T) {
	source := testSourceINDEX()

	fork, err := forkINDEX(source, testForkRequest())
	if err != nil {
		t.Fatalf("forkINDEX: %v", err)
	}

	fork.DOCs[0] = srcDoc3
	if source.DOCs[0] != srcDoc1 {
		t.Fatalf("writing to the fork's DOC list changed the source's: source DOC1 is now %s", source.DOCs[0])
	}
}

// Ownership is not an install argument at all. The contract stores SIGNER(), so
// the owner is whoever broadcasts — which is what makes it a fork rather than a
// copy of somebody else's INDEX.
func Test_Fork_OwnerIsTheSignerNotTheSourceAuthor(t *testing.T) {
	source := testSourceINDEX()
	fork, args := buildForkArgs(t, source, testForkRequest())

	if fork.Author != "" {
		t.Fatalf("fork carries Author %q before install — ownership is assigned on chain, not requested", fork.Author)
	}
	if fork.SCID != "" {
		t.Fatalf("fork carries SCID %q before install — the SCID is the install txid", fork.SCID)
	}

	code := installCode(t, args)
	if strings.Contains(code, srcAddr) {
		t.Fatalf("the source author's address appears in the fork's contract:\n%s", code)
	}
	if !strings.Contains(code, `STORE("owner", address())`) {
		t.Fatalf("the fork does not store its owner from address()/SIGNER():\n%s", code)
	}
	if !strings.Contains(code, "LET s = SIGNER()") {
		t.Fatalf("address() no longer resolves to SIGNER() — the owner may not be the forker:\n%s", code)
	}
}

// Ratings are chain state on the contract, not headers, so a fork starts at
// zero. Stated as a test because the UI promises it.
func Test_Fork_StartsWithNoRatingsAndNoHistory(t *testing.T) {
	_, args := buildForkArgs(t, testSourceINDEX(), testForkRequest())
	code := installCode(t, args)

	for _, want := range []string{`STORE("likes", 0)`, `STORE("dislikes", 0)`, `STORE("commit", 0)`} {
		if !strings.Contains(code, want) {
			t.Fatalf("fork does not start clean: %s missing from:\n%s", want, code)
		}
	}
}

func Test_Fork_CarriesModsVerbatim(t *testing.T) {
	source := testSourceINDEX()
	source.Mods = "vsoo,txdwd"

	fork, args := buildForkArgs(t, source, testForkRequest())
	if fork.Mods != "vsoo,txdwd" {
		t.Fatalf("fork mods = %q, want the source's", fork.Mods)
	}
	if !strings.Contains(installCode(t, args), `STORE("mods", "vsoo,txdwd")`) {
		t.Fatalf("mods were not written into the fork's contract")
	}
}

// The generated body has to be a real TELA-INDEX-1, or nothing will serve it.
func Test_Fork_GeneratesAValidINDEXContract(t *testing.T) {
	source := testSourceINDEX()
	fork, args := buildForkArgs(t, source, testForkRequest())

	_, version, err := tela.ValidINDEXVersion(installCode(t, args), fork.Mods)
	if err != nil {
		t.Fatalf("fork does not parse as TELA-INDEX-1: %v", err)
	}
	if version.String() == "" {
		t.Fatalf("fork carries no TELA contract version")
	}
}

// A fork on the source's dURL competes for dero://<durl> and cannot be cloned
// alongside it — the clone path is keyed on dURL, not SCID.
func Test_Fork_RefusesTheSourceDURL(t *testing.T) {
	source := testSourceINDEX()

	for _, durl := range []string{"villager.tela", "  villager.tela  ", "VILLAGER.TELA"} {
		req := testForkRequest()
		req.DURL = durl
		if _, err := forkINDEX(source, req); err == nil {
			t.Fatalf("dURL %q was accepted — it collides with the INDEX being forked", durl)
		}
	}
}

// A blank dURL is defaulted, not rejected, so the panel gets the suggestion back
// from the same code that enforces the rule.
func Test_Fork_BlankDURLTakesTheSuggestion(t *testing.T) {
	source := testSourceINDEX()

	req := testForkRequest()
	req.DURL = "   "

	fork, err := forkINDEX(source, req)
	if err != nil {
		t.Fatalf("forkINDEX: %v", err)
	}
	if fork.DURL != suggestForkDURL(source.DURL) {
		t.Fatalf("dURL = %q, want the suggestion %q", fork.DURL, suggestForkDURL(source.DURL))
	}
}

func Test_Fork_RefusesUnusableDURLs(t *testing.T) {
	source := testSourceINDEX()

	cases := map[string]string{
		"slash":           "villager/fork.tela",
		"backslash":       `villager\fork.tela`,
		"parent":          "../fork.tela",
		"quote":           `villager".tela`,
		"space":           "villager fork.tela",
		"newline":         "villager\nfork.tela",
		"control charact": "villager\x00fork.tela",
	}

	for name, durl := range cases {
		req := testForkRequest()
		req.DURL = durl
		if _, err := forkINDEX(source, req); err == nil {
			t.Errorf("%s: dURL %q was accepted", name, durl)
		}
	}
}

func Test_Fork_RefusesWhenNoDURLIsAvailable(t *testing.T) {
	source := testSourceINDEX()
	source.DURL = ""

	req := testForkRequest()
	req.DURL = ""

	if _, err := forkINDEX(source, req); err == nil {
		t.Fatalf("a fork with no dURL was built — TELA rejects an empty dURL")
	}
}

func Test_Fork_RefusesAnINDEXWithNoDocuments(t *testing.T) {
	source := testSourceINDEX()
	source.DOCs = nil

	if _, err := forkINDEX(source, testForkRequest()); err == nil {
		t.Fatalf("an INDEX listing no documents was forked")
	}
}

// The name header is required by the standard, so a blank one is filled from the
// source. Nothing else is: clearing a description is a choice, and resurrecting
// the source's would put words in the forker's mouth.
func Test_Fork_NameFallsBackButProseDoesNot(t *testing.T) {
	source := testSourceINDEX()

	req := testForkRequest()
	req.Name = "  "
	req.Description = ""
	req.IconURL = ""

	fork, err := forkINDEX(source, req)
	if err != nil {
		t.Fatalf("forkINDEX: %v", err)
	}
	if fork.NameHdr != "Villager" {
		t.Fatalf("name = %q, want the source's name as a fallback", fork.NameHdr)
	}
	if fork.DescrHdr != "" {
		t.Fatalf("description = %q, want the empty one that was asked for", fork.DescrHdr)
	}
	if fork.IconHdr != "" {
		t.Fatalf("icon = %q, want the empty one that was asked for", fork.IconHdr)
	}
}

func Test_Fork_RefusesWhenNoNameIsAvailable(t *testing.T) {
	source := testSourceINDEX()
	source.NameHdr = ""

	req := testForkRequest()
	req.Name = ""

	if _, err := forkINDEX(source, req); err == nil {
		t.Fatalf("a fork with no name was built — TELA rejects an empty name header")
	}
}

func Test_Fork_UsesTheRequestedHeadersNotTheSources(t *testing.T) {
	source := testSourceINDEX()

	req := testForkRequest()
	req.Name = "Something Else"
	req.Description = "rewritten"
	req.IconURL = "https://example.invalid/other.svg"

	fork, args := buildForkArgs(t, source, req)
	if fork.NameHdr != "Something Else" || fork.DescrHdr != "rewritten" {
		t.Fatalf("fork headers = %q / %q, want the requested ones", fork.NameHdr, fork.DescrHdr)
	}

	code := installCode(t, args)
	if strings.Contains(code, "the original description") {
		t.Fatalf("the source's description survived into the fork:\n%s", code)
	}
	if !strings.Contains(code, `STORE("var_header_name", "Something Else")`) {
		t.Fatalf("the requested name was not written:\n%s", code)
	}
}

// suggestForkDURL goes before the last dot because TELA reads .lib, .shards and
// .bootstrap off the end of a dURL as structural meaning.
func Test_SuggestForkDURL(t *testing.T) {
	cases := map[string]string{
		"villager.tela":     "villager-fork.tela",
		"zero.lib":          "zero-fork.lib",
		"a.index.bootstrap": "a.index-fork.bootstrap",
		"plain":             "plain-fork",
		"  spaced.tela  ":   "spaced-fork.tela",
		"":                  "",
	}

	for in, want := range cases {
		if got := suggestForkDURL(in); got != want {
			t.Errorf("suggestForkDURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// A suggestion that the validator would then reject is worse than none.
func Test_SuggestedDURLIsAcceptedByTheValidator(t *testing.T) {
	for _, durl := range []string{"villager.tela", "zero.lib", "a.index.bootstrap", "plain"} {
		if err := validateForkDURL(suggestForkDURL(durl), durl); err != nil {
			t.Errorf("the suggestion for %q is refused: %v", durl, err)
		}
	}
}
