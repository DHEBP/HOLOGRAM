package main

// Reading a TELA INDEX as a repository: the file list at its current state.
//
// This exists rather than reusing GetCommitContent because that call is
// destructive by construction. It clones into tela.GetClonePath() and then
// os.RemoveAll's that same directory (tela_service.go), which is the SHARED
// clone root CloneTELA writes user clones into. Opening a read-only view must
// not delete the user's clones, and the repository view opens on every visit.
//
// It also reads no files off disk, so it cannot pick up another app's clone the
// way readClonedFiles can, and it names files with docFileName - the same helper
// GetDOCSignatures uses - so a file and its signature always agree.

import (
	"fmt"

	"github.com/civilware/tela"
)

// RepositoryFile is one entry of a TELA INDEX as it stands now.
//
// SCID is carried alongside Name so the signature panel can join on the SCID
// rather than on a path string. The two lists are built from the same DOC order
// with the same naming helper, but joining on an attacker-supplied name would
// mis-attribute a signature the moment two DOCs claim the same NameHdr.
type RepositoryFile struct {
	Name    string `json:"name"`
	SCID    string `json:"scid"`
	DocType string `json:"docType"`
	SubDir  string `json:"subDir"`
	Kind    string `json:"kind"` // "doc" | "index" | "unreadable"
	Content string `json:"content"`
	Bytes   int    `json:"bytes"`
	Reason  string `json:"reason,omitempty"`
}

// maxRepositoryBytes caps the total document body one GetRepositoryFiles reply
// carries, across every file in it.
//
// ⚠️ It does NOT bound the transient cost of getting there. A DOC body is stored
// as base64(gzip(...)) and tela.Decompress takes a []byte and returns a []byte
// with no streaming entry point, so a ~13KB stored blob still expands in full
// before its size can be judged (tela's own Compress reaches 771:1). Bounding
// that would mean reimplementing tela's base64+gzip path here, which would
// silently stop bounding anything the day tela adds a second format - the DOC
// type already carries a Compression field for exactly that. The expansion is
// bounded by the chain's contract size limit and is discarded immediately; what
// is RETAINED and marshalled across the bridge is what this cap governs.
const maxRepositoryBytes = 8 << 20

// shortSCID abbreviates a SCID for display.
//
// It exists because a SCID read off chain is NOT guaranteed to be 64 characters.
// tela.ParseINDEXForDOCs returns whatever string the INDEX contract stored and
// applies no length rule to it (tela's own parseDocShards carries a len != 64
// guard, so the gap is known), which makes every listed DOC SCID an
// attacker-controlled string. Slicing one directly panics on a short one.
func shortSCID(scid string) string {
	if len(scid) <= 16 {
		return scid
	}
	return scid[:16] + "…"
}

// GetRepositoryFiles lists every file a TELA INDEX carries, with its contents.
//
// Entries that are not DOCs are listed rather than dropped. A nested INDEX is
// how TELA shards a large file and is completely normal; something that parses
// as neither is rare but real. Both used to vanish silently from the file list,
// which reads as "this repository has 8 files" when it has 11.
func (a *App) GetRepositoryFiles(scid string) map[string]interface{} {
	if len(scid) != 64 {
		return map[string]interface{}{"success": false, "error": "Invalid SCID. Must be exactly 64 characters"}
	}

	endpoint := a.telaEndpoint()
	files := []RepositoryFile{}

	index, err := tela.GetINDEXInfo(scid, endpoint)
	if err != nil {
		// Not an INDEX. A bare DOC SCID is a valid thing to open.
		doc, docErr := tela.GetDOCInfo(scid, endpoint)
		if docErr != nil {
			a.logToConsole(fmt.Sprintf("[TELA] No repository for %s: not a TELA INDEX or DOC (%v)", scid[:16]+"...", err))
			return ErrorResponse(err)
		}

		files = append(files, a.repositoryFileFromDOC(scid, doc))
		return map[string]interface{}{
			"success": true,
			"scid":    scid,
			"kind":    "DOC",
			"durl":    doc.DURL,
			"files":   files,
		}
	}

	// maxPlaintextBytes caps ONE file; this caps the response. Without it an
	// INDEX listing many large DOCs marshals the sum of them into a single Wails
	// message - at the measured practical ceiling of 119 DOCs that is nearly
	// 240MB in one piece. Past the budget an entry is still listed with its real
	// size, only its body is withheld.
	remaining := maxRepositoryBytes

	for _, docSCID := range index.DOCs {
		doc, docErr := tela.GetDOCInfo(docSCID, endpoint)
		if docErr != nil {
			entry := RepositoryFile{SCID: docSCID, Kind: "unreadable", Name: shortSCID(docSCID)}
			if child, idxErr := tela.GetINDEXInfo(docSCID, endpoint); idxErr == nil {
				entry.Kind = "index"
				entry.Reason = "nested INDEX"
				if child.DURL != "" {
					entry.Name = child.DURL
				}
			} else {
				a.logToConsole(fmt.Sprintf("[TELA] Repository entry unreadable %s: %v", shortSCID(docSCID), docErr))
			}
			files = append(files, entry)
			continue
		}

		entry := a.repositoryFileFromDOC(docSCID, doc)
		if len(entry.Content) > remaining {
			entry.Content = ""
			entry.Reason = "not shown: this repository's files add up to more than can be displayed at once"
		} else {
			remaining -= len(entry.Content)
		}
		files = append(files, entry)
	}

	a.logToConsole(fmt.Sprintf("[TELA] Repository %s: %d entries", scid[:16]+"...", len(files)))
	return map[string]interface{}{
		"success": true,
		"scid":    scid,
		"kind":    "INDEX",
		"durl":    index.DURL,
		"files":   files,
	}
}

// repositoryFileFromDOC reads one DOC's stored document.
//
// The size guard is on the DECOMPRESSED body, not the contract: a DOC is capped
// at 18KB on chain, but a compressed one holds gzip, so a hostile publisher can
// store a small contract that expands to something that freezes the UI thread
// when it crosses the Wails bridge and enters the DOM. Past the cap the size is
// reported and the body is not.
func (a *App) repositoryFileFromDOC(docSCID string, doc tela.DOC) RepositoryFile {
	entry := RepositoryFile{
		Name:    docFileName(doc),
		SCID:    docSCID,
		DocType: doc.DocType,
		SubDir:  doc.SubDir,
		Kind:    "doc",
	}

	// connectToExternalNode restores a nil client when a connection test fails, so
	// this is a real state and not a theoretical one. Without the guard a nil
	// client is a panic Wails recovers and never reports, which leaves the pane
	// waiting forever rather than showing this reason on one file.
	if a.daemonClient == nil {
		entry.Reason = "no daemon connection"
		return entry
	}

	scData, err := a.daemonClient.GetSC(docSCID, true, false)
	if err != nil {
		entry.Reason = "contract code could not be read"
		return entry
	}

	code, ok := scData["code"].(string)
	if !ok {
		entry.Reason = "contract carries no code"
		return entry
	}

	content := docContentFromSC(doc, code)
	entry.Bytes = len(content)
	if len(content) > maxPlaintextBytes {
		entry.Reason = "too large to display"
		return entry
	}

	entry.Content = content
	return entry
}
