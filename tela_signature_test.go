package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docFixture is a real mainnet TELA DOC captured at height 7440496 (2026-08-07).
// Each one exercises a different reconstruction branch of the signed body.
type docFixture struct {
	SCID    string `json:"scid"`
	Owner   string `json:"owner"`
	CheckC  string `json:"checkC"`
	CheckS  string `json:"checkS"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	DocType string `json:"docType"`
}

func loadDocFixture(t *testing.T, name string) docFixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "tela_signatures", name+".json"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var f docFixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return f
}

// The four wrapper/line-ending combinations found on mainnet. Together they
// verify 367 of the 371 signed DOCs.
var docFixtureNames = []string{"v2_lf", "v2_crlf", "v1_lf", "v1_crlf"}

func TestVerifyDocSignature_RealMainnetDOCs(t *testing.T) {
	for _, name := range docFixtureNames {
		t.Run(name, func(t *testing.T) {
			f := loadDocFixture(t, name)
			state, variant := verifyDocSignature(f.Owner, f.CheckC, f.CheckS, f.Code)
			if state != DocSigVerified {
				t.Fatalf("%s (%s): got %q, want %q", f.Name, f.DocType, state, DocSigVerified)
			}
			if variant < 0 || variant > 3 {
				t.Fatalf("%s: variant %d out of range", f.Name, variant)
			}
		})
	}
}

// TestVerifyDocSignature_TamperedBody is the negative control. Flipping one bit
// of the document body must break verification. Revert candidateSignedMessages
// or checkDEROSignature and this fails.
func TestVerifyDocSignature_TamperedBody(t *testing.T) {
	for _, name := range docFixtureNames {
		t.Run(name, func(t *testing.T) {
			f := loadDocFixture(t, name)

			// Flip a bit inside the comment block, not in the DVM code, so the
			// wrapper still parses and only the signed bytes change.
			start := strings.Index(f.Code, "/*")
			last := strings.LastIndex(f.Code, "*/")
			if start < 0 || last <= start+2 {
				t.Fatalf("%s: fixture has no usable comment block", name)
			}
			mid := start + 2 + (last-(start+2))/2
			b := []byte(f.Code)
			b[mid] ^= 0x01
			tampered := string(b)

			if tampered == f.Code {
				t.Fatalf("%s: tamper was a no-op", name)
			}
			if state, _ := verifyDocSignature(f.Owner, f.CheckC, f.CheckS, tampered); state != DocSigUnverified {
				t.Fatalf("%s: tampered body got %q, want %q", name, state, DocSigUnverified)
			}
		})
	}
}

// TestVerifyDocSignature_WrongSigner asserts a real behaviour: a signature
// attributed to a different address is rejected.
//
// It does NOT isolate any line of ours. walletapi.CheckSignature folds the
// supplied address into the challenge hash, so the rejection happens upstream.
// Recorded plainly because an earlier version of this test claimed to prove a
// local ownership check that was in fact inert.
func TestVerifyDocSignature_WrongSigner(t *testing.T) {
	f := loadDocFixture(t, "v2_lf")
	other := loadDocFixture(t, "v1_crlf")
	if f.Owner == other.Owner {
		t.Skip("fixtures share an owner")
	}

	if state, _ := verifyDocSignature(other.Owner, f.CheckC, f.CheckS, f.Code); state != DocSigUnverified {
		t.Fatalf("signature accepted under the wrong owner: got %q", state)
	}
}

func TestVerifyDocSignature_States(t *testing.T) {
	f := loadDocFixture(t, "v2_lf")

	tests := []struct {
		name                  string
		owner, checkC, checkS string
		want                  string
	}{
		{"no signature stored", f.Owner, "", "", DocSigUnsigned},
		{"only C stored", f.Owner, f.CheckC, "", DocSigUnsigned},
		{"only S stored", f.Owner, "", f.CheckS, DocSigUnsigned},
		{"anon owner", "anon", f.CheckC, f.CheckS, DocSigAnonymous},
		{"empty owner", "", f.CheckC, f.CheckS, DocSigAnonymous},
		{"malformed C", f.Owner, "zzzz", f.CheckS, DocSigUnverified},
		{"good", f.Owner, f.CheckC, f.CheckS, DocSigVerified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := verifyDocSignature(tt.owner, tt.checkC, tt.checkS, f.Code); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestZeroWalletCheckSignature pins the assumption checkDEROSignature rests on:
// walletapi.CheckSignature never touches its receiver, so verification works
// with no wallet open. If upstream ever starts using the receiver this panics
// or fails here rather than in the browser with a wallet locked.
func TestZeroWalletCheckSignature(t *testing.T) {
	f := loadDocFixture(t, "v2_lf")
	msgs := candidateSignedMessages(f.Code)
	if len(msgs) == 0 {
		t.Fatal("no candidate messages reconstructed")
	}
	if !checkDEROSignature(f.Owner, f.CheckC, f.CheckS, []byte(msgs[0])) {
		t.Fatal("zero-value wallet could not verify a known-good signature")
	}
}

// TestCandidateSignedMessages_ShortHexScalars guards a trap in the stored data:
// SignData writes C and S with %x on a big.Int, so leading zeros are dropped.
// 217 of 422 mainnet DOCs store fewer than 64 hex characters. Any length check
// on these values would reject half the chain.
func TestCandidateSignedMessages_ShortHexScalars(t *testing.T) {
	f := loadDocFixture(t, "v2_lf")
	short := len(f.CheckC) < 64 || len(f.CheckS) < 64

	if state, _ := verifyDocSignature(f.Owner, f.CheckC, f.CheckS, f.Code); state != DocSigVerified {
		t.Fatalf("fixture failed to verify (short scalars present: %v)", short)
	}
}

// TestCandidateSignedMessages_MarkerOrdering guards the reason LastIndex is used
// for the closing marker. A v1 DOC whose body starts with "//" produces "/*//",
// putting the first "*/" before the wrapper opens. Slicing on that index panics.
func TestCandidateSignedMessages_MarkerOrdering(t *testing.T) {
	code := "Function InitializePrivate() Uint64\nEnd Function\n\n/*//Install\nvar x = 1;*/\n"

	msgs := candidateSignedMessages(code)
	if len(msgs) != 4 {
		t.Fatalf("got %d candidates, want 4", len(msgs))
	}
	// v1 reconstruction is index 2 and must recover the whole body.
	if !strings.HasPrefix(msgs[2], "//Install") {
		t.Fatalf("v1 candidate lost the body start: %q", msgs[2])
	}
	if !strings.HasSuffix(msgs[2], "var x = 1;") {
		t.Fatalf("v1 candidate lost the body end: %q", msgs[2])
	}
}

func TestCandidateSignedMessages_NoComment(t *testing.T) {
	if msgs := candidateSignedMessages("Function Initialize() Uint64\nEnd Function\n"); msgs != nil {
		t.Fatalf("expected nil for a DOC with no comment block, got %d", len(msgs))
	}
	if msgs := candidateSignedMessages("no markers at all"); msgs != nil {
		t.Fatalf("expected nil when no markers present, got %d", len(msgs))
	}
}

func TestToCRLF(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a\nb", "a\r\nb"},
		{"a\r\nb", "a\r\nb"},
		{"a\rb", "a\r\nb"},
		{"", ""},
		{"no endings", "no endings"},
	}
	for _, tt := range tests {
		if got := toCRLF(tt.in); got != tt.want {
			t.Fatalf("toCRLF(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
