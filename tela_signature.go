package main

import (
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/walletapi"
)

// TELA DOC signature states.
//
// A DOC stores its author's address in "owner" and a Schnorr signature over the
// document body in "fileCheckC"/"fileCheckS". Nothing in TELA or HOLOGRAM has
// ever verified them. These are the only honest answers:
//
//	DocSigUnsigned   - no signature stored, so there is nothing to check
//	DocSigVerified   - the stored owner signed exactly this document body
//	DocSigAnonymous  - a signature is stored, but no author address was recorded
//	DocSigUnverified - a signature is stored and does not verify
//
// DocSigAnonymous is a separate state on purpose, and it is not an edge case:
// 51 of the 422 DOCs on mainnet are in it. Publishing with a ring size above 2
// hides the sender, so the contract records "anon" and there is no key to check
// the signature against. Those publishers chose MORE privacy, and folding them
// into DocSigUnverified would render a privacy feature as a defect on a privacy
// chain. It is also not a way to dodge the label: an attacker wants to appear to
// be a trusted author, and publishing anonymously gives them no name at all.
//
// DocSigUnverified deliberately does NOT claim forgery. It means "we could not
// reproduce the signed bytes", which covers both a bad signature and a document
// whose original bytes cannot be recovered from what the chain stores.
// Two further states exist for entries that are not signable documents at all.
// They are NOT signature outcomes and must never render as warnings:
//
//	DocSigIndex      - the entry is a nested INDEX, not a DOC
//	DocSigUnreadable - the entry parses as neither, so nothing can be said
//
// DocSigIndex is not rare and not an error. TELA shards a large file across a
// child INDEX, so an INDEX routinely lists other INDEXes: villager.tela holds 3
// of them among 11 entries. Reporting those as "did not verify" would put a
// warning on the single most normal thing a large app does.
const (
	DocSigUnsigned   = "unsigned"
	DocSigVerified   = "verified"
	DocSigAnonymous  = "anonymous"
	DocSigUnverified = "unverified"
	DocSigIndex      = "index"
	DocSigUnreadable = "unreadable"
)

// docSigPEMType is the block type walletapi.SignData writes.
const docSigPEMType = "DERO SIGNED MESSAGE"

// toCRLF normalizes to LF then rewrites every line ending as CRLF.
func toCRLF(s string) string {
	lf := strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.ReplaceAll(lf, "\n", "\r\n")
}

// candidateSignedMessages reconstructs the document body that was signed, from
// the SC code the chain stores.
//
// The body is wrapped in a trailing multiline comment, but the wrapper differs
// between DOC template generations and the DVM does not preserve CR bytes, so
// there is no single reconstruction that works. Measured against all 371 signed
// DOCs on mainnet (2026-08-07, height 7440496):
//
//	v2 wrapper, LF     293   v2 wrapper, CRLF   251
//	v1 wrapper, LF      51   v1 wrapper, CRLF    64
//	union              367 / 371  (98.9%)
//
// For comparison, TELA's own serving-path extractor (parseDocCode, which ends in
// strings.TrimSpace) reproduces only 237 of 371. Verifying against that path
// would brand 134 correctly-signed documents as bad, so it is not used here.
//
// Order matters only for reporting which variant matched; all are tried.
func candidateSignedMessages(scCode string) []string {
	start := strings.Index(scCode, "/*")
	if start < 0 {
		return nil
	}
	body := scCode[start+2:]

	// Not strings.Index: a document body legitimately contains "*/", and a v1
	// DOC whose body starts with "//" produces "/*//", where the first "*/"
	// sits before the wrapper even opens.
	last := strings.LastIndex(body, "*/")
	if last < 0 {
		return nil
	}
	core := body[:last]

	// v2 template wraps as "\n\n/*\n" + body + "\n*/"
	v2 := strings.TrimSuffix(strings.TrimPrefix(core, "\n"), "\n")
	// v1 template wraps as "\n\n/*" + body + "*/\n"
	v1 := core

	return []string{v2, toCRLF(v2), v1, toCRLF(v1)}
}

// verifyDocSignature checks a stored TELA DOC signature against the SC code.
//
// It returns the state and, when verified, the reconstruction index that
// matched (for diagnostics only).
func verifyDocSignature(owner, checkC, checkS, scCode string) (state string, variant int) {
	if checkC == "" || checkS == "" {
		return DocSigUnsigned, -1
	}
	// Ring size above 2 hides the installer, so the contract records "anon".
	// There is no address to check the signature against.
	if owner == "" || owner == "anon" {
		return DocSigAnonymous, -1
	}

	for i, msg := range candidateSignedMessages(scCode) {
		if checkDEROSignature(owner, checkC, checkS, []byte(msg)) {
			return DocSigVerified, i
		}
	}
	return DocSigUnverified, -1
}

// checkDEROSignature rebuilds the PEM block walletapi.SignData produces and
// hands it back to walletapi's own verifier.
//
// The Schnorr math is deliberately NOT reimplemented here. Two consequences of
// how CheckSignature works, both verified by reverting them and watching tests:
//
//   - It never touches its receiver, so a zero-value wallet is enough and
//     verification works with no wallet open. TestZeroWalletCheckSignature pins
//     this, because it is an assumption about upstream code, not about ours.
//   - It folds the supplied address AND the message into the challenge hash, so
//     a wrong signer or a wrong body already fails inside it. Re-comparing
//     either afterwards is inert; those checks were written, measured to change
//     nothing, and removed.
func checkDEROSignature(owner, checkC, checkS string, message []byte) bool {
	block := &pem.Block{
		Type: docSigPEMType,
		Headers: map[string]string{
			"Address": owner,
			"C":       checkC,
			"S":       checkS,
		},
		Bytes: message,
	}

	var w walletapi.Wallet_Memory
	_, _, err := w.CheckSignature(pem.EncodeToMemory(block))
	return err == nil
}

// telaEndpoint resolves the daemon address in the form the tela library wants.
// Mirrors the resolution used throughout tela_service.go.
func (a *App) telaEndpoint() string {
	if a.IsInSimulatorMode() {
		return "127.0.0.1:20000"
	}
	if ep, ok := a.settings["daemon_endpoint"].(string); ok && ep != "" {
		ep = strings.TrimPrefix(ep, "http://")
		return strings.TrimPrefix(ep, "https://")
	}
	return "127.0.0.1:10102"
}

// docSignature is one row of the signature panel.
type docSignature struct {
	SCID    string `json:"scid"`
	Path    string `json:"path"`
	DocType string `json:"docType"`
	State   string `json:"state"`
	Signer  string `json:"signer"`
}

// verifyDOCAt checks a single DOC contract by SCID.
func verifyDOCAt(scid, endpoint string) (docSignature, error) {
	doc, err := tela.GetDOCInfo(scid, endpoint)
	if err != nil {
		return docSignature{}, err
	}

	state, _ := verifyDocSignature(doc.Author, doc.CheckC, doc.CheckS, doc.Code)

	// Same helper the version and diff viewers use, so a file is named
	// identically wherever it appears.
	path := docFileName(doc)

	signer := doc.Author
	if state == DocSigAnonymous {
		signer = ""
	}

	return docSignature{SCID: scid, Path: path, DocType: doc.DocType, State: state, Signer: signer}, nil
}

// VerifyDOCSignature reports whether a TELA DOC's stored signature was made by
// its recorded owner over the document the chain is serving.
func (a *App) VerifyDOCSignature(scid string) map[string]interface{} {
	if len(scid) != 64 {
		return map[string]interface{}{"success": false, "error": "Invalid SCID. Must be exactly 64 characters"}
	}

	sig, err := verifyDOCAt(scid, a.telaEndpoint())
	if err != nil {
		return ErrorResponse(err)
	}

	a.logToConsole(fmt.Sprintf("[TELA] Signature %s: %s (%s)", sig.State, scid[:16]+"...", sig.Path))
	return map[string]interface{}{"success": true, "scid": scid, "signature": sig}
}

// GetDOCSignatures reports the author signature on every DOC of a TELA INDEX,
// or on a single DOC when given one directly.
//
// Scope is deliberate and load-bearing: a DOC's fileCheckC/fileCheckS are
// written once at install and never change, while its code can be replaced with
// UpdateCode. So the answer is always about the CURRENT state of each DOC, which
// is why this is not commit-scoped. Attaching it to a historical commit would
// read as a claim about that commit's bytes, which it is not - and the DOC list
// the daemon returns is the current one regardless of which commit is selected.
//
// Deliberately returns no roll-up. Per-file state is what the evidence supports;
// a single app-level verdict would have to invent a rule for mixed results and
// would read as noise the moment one file is anonymous.
func (a *App) GetDOCSignatures(scid string) map[string]interface{} {
	if len(scid) != 64 {
		return map[string]interface{}{"success": false, "error": "Invalid SCID. Must be exactly 64 characters"}
	}

	endpoint := a.telaEndpoint()
	signatures := []docSignature{}
	kind := "INDEX"

	index, err := tela.GetINDEXInfo(scid, endpoint)
	if err != nil {
		// Not an INDEX. A bare DOC SCID is a valid thing to ask about.
		sig, docErr := verifyDOCAt(scid, endpoint)
		if docErr != nil {
			// Some published INDEXes parse as neither - feed.tela at
			// 98bfbc34 is one, and the browser already shows its own banner
			// for it. Log so this is not a silent blank panel.
			a.logToConsole(fmt.Sprintf("[TELA] No signatures for %s: not a TELA INDEX or DOC (%v)", scid[:16]+"...", err))
			return ErrorResponse(err)
		}
		kind = "DOC"
		signatures = append(signatures, sig)
	} else {
		for _, docSCID := range index.DOCs {
			sig, docErr := verifyDOCAt(docSCID, endpoint)
			if docErr != nil {
				// Not a DOC. Distinguish a nested INDEX - the normal way TELA
				// shards a large file - from something genuinely unreadable.
				// Both are neutral: neither is a signature failure, and one
				// entry we cannot read must not blank the whole panel.
				state := DocSigUnreadable
				// shortSCID, not a direct slice: a listed DOC SCID is whatever
				// string the INDEX contract stored and can be shorter than 16.
				path := shortSCID(docSCID)
				if child, idxErr := tela.GetINDEXInfo(docSCID, endpoint); idxErr == nil {
					state = DocSigIndex
					if child.DURL != "" {
						path = child.DURL
					}
				} else {
					a.logToConsole(fmt.Sprintf("[TELA] Signature read failed for %s: %v", path, docErr))
				}
				signatures = append(signatures, docSignature{SCID: docSCID, Path: path, State: state})
				continue
			}
			signatures = append(signatures, sig)
		}
	}

	a.logToConsole(fmt.Sprintf("[TELA] Checked %d signature(s) for %s", len(signatures), scid[:16]+"..."))
	return map[string]interface{}{
		"success":    true,
		"scid":       scid,
		"kind":       kind,
		"signatures": signatures,
	}
}
