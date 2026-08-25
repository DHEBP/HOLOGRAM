package main

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// commitTXID builds a distinct 64-char lowercase-hex TXID for commit n.
func commitTXID(n int) string { return fmt.Sprintf("%064x", n) }

// storedTXID encodes a TXID the way the contract stores it: the ASCII hex TXID
// hex-encoded again, so decodeHexString round-trips back to the TXID.
func storedTXID(n int) string { return hex.EncodeToString([]byte(commitTXID(n))) }

// numberedKeys returns a map of "0".."count-1" -> stored TXID.
func numberedKeys(count int) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 0; i < count; i++ {
		m[fmt.Sprintf("%d", i)] = storedTXID(i)
	}
	return m
}

func assertTenCommits(t *testing.T, commits []Commit) {
	t.Helper()
	if len(commits) != 10 {
		t.Fatalf("got %d commits, want 10", len(commits))
	}
	for i, c := range commits {
		if c.Number != i+1 {
			t.Fatalf("commit[%d].Number = %d, want %d", i, c.Number, i+1)
		}
		if want := commitTXID(i); c.TXID != want {
			t.Fatalf("commit[%d].TXID = %q, want %q", i, c.TXID, want)
		}
		wantCurrent := i == 9
		if c.IsCurrent != wantCurrent {
			t.Fatalf("commit[%d].IsCurrent = %v, want %v", i, c.IsCurrent, wantCurrent)
		}
	}
	if commits[0].Label != "Initial deployment" {
		t.Fatalf("commit[0].Label = %q, want %q", commits[0].Label, "Initial deployment")
	}
	if commits[1].Label != "Update #1" {
		t.Fatalf("commit[1].Label = %q, want %q", commits[1].Label, "Update #1")
	}
}

// The live contract stores its numbered keys in uint64keys. A reader that only
// scanned stringkeys would report nothing here.
func TestCommitsFromContract_TenVersionsInUint64Keys(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{vars: map[string]interface{}{
		"uint64keys": numberedKeys(10),
		"stringkeys": map[string]interface{}{"commit": float64(9)},
	}}}

	assertTenCommits(t, app.commitsFromContract(zeroSCID()))
}

// Twelve versions force two-digit keys ("10","11"), where lexical string order
// diverges from numeric order. This pins the numeric sort at tela_service.go: a
// lexical sort would place "10"/"11" before "2", scrambling both the per-index
// TXID mapping and IsCurrent. Single-digit fixtures cannot catch that regression.
func TestCommitsFromContract_TwelveVersionsNumericOrder(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{vars: map[string]interface{}{
		"uint64keys": numberedKeys(12),
	}}}

	commits := app.commitsFromContract(zeroSCID())
	if len(commits) != 12 {
		t.Fatalf("got %d commits, want 12", len(commits))
	}
	for i, c := range commits {
		if c.Number != i+1 {
			t.Fatalf("commit[%d].Number = %d, want %d", i, c.Number, i+1)
		}
		if want := commitTXID(i); c.TXID != want {
			t.Fatalf("commit[%d].TXID = %q, want %q (numeric key %d)", i, c.TXID, want, i)
		}
		if wantCurrent := i == 11; c.IsCurrent != wantCurrent {
			t.Fatalf("commit[%d].IsCurrent = %v, want %v", i, c.IsCurrent, wantCurrent)
		}
	}
	if commits[11].TXID != commitTXID(11) {
		t.Fatalf("last commit TXID = %q, want %q (numeric key 11)", commits[11].TXID, commitTXID(11))
	}
}

// The TELA spec template stores the numbered keys in stringkeys; same result.
func TestCommitsFromContract_TenVersionsInStringKeys(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{vars: map[string]interface{}{
		"stringkeys": numberedKeys(10),
	}}}

	assertTenCommits(t, app.commitsFromContract(zeroSCID()))
}

// A numeric key present in both maps is one commit, and the first map scanned
// (stringkeys) wins.
func TestCommitsFromContract_NumberInBothMapsCountedOnce(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{vars: map[string]interface{}{
		"stringkeys": map[string]interface{}{"0": storedTXID(0)},
		"uint64keys": map[string]interface{}{"0": storedTXID(99)},
	}}}

	commits := app.commitsFromContract(zeroSCID())
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1 (a number in both maps is one commit)", len(commits))
	}
	if commits[0].TXID != commitTXID(0) {
		t.Fatalf("TXID = %q, want the first-map (stringkeys) value %q", commits[0].TXID, commitTXID(0))
	}
}

// A numbered key whose value does not decode to a 64-hex TXID is skipped; the
// rest still come through and reindex around the gap.
func TestCommitsFromContract_NonTXIDNumberedKeySkipped(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{vars: map[string]interface{}{
		"uint64keys": map[string]interface{}{
			"0": storedTXID(0),
			"1": hex.EncodeToString([]byte("not-a-txid")), // decodes, but not 64 hex
			"2": storedTXID(2),
		},
	}}}

	commits := app.commitsFromContract(zeroSCID())
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2 (the non-TXID key is skipped)", len(commits))
	}
	if commits[0].TXID != commitTXID(0) || commits[1].TXID != commitTXID(2) {
		t.Fatalf("TXIDs = [%q, %q], want [%q, %q]", commits[0].TXID, commits[1].TXID, commitTXID(0), commitTXID(2))
	}
	if !commits[1].IsCurrent {
		t.Fatal("last surviving commit must be current")
	}
}

// No numbered keys -> nil, so GetCommitHistory falls through to the Gnomon/daemon
// paths instead of reporting an empty authoritative history.
func TestCommitsFromContract_NoNumberedKeysReturnsNil(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{vars: map[string]interface{}{
		"stringkeys": map[string]interface{}{"C": "deadbeef", "commit": float64(0)},
		"uint64keys": map[string]interface{}{},
	}}}

	if commits := app.commitsFromContract(zeroSCID()); commits != nil {
		t.Fatalf("got %d commits, want nil", len(commits))
	}
}

// GetCommitHistory must PREFER the contract path: with a fake daemon carrying
// numbered keys and NO gnomon client, the returned commits still carry real
// TXIDs rather than the empty gnomon/daemon-fallback shape.
func TestGetCommitHistory_PrefersContractOverGnomon(t *testing.T) {
	app := &App{daemonClient: &repoFakeDaemon{vars: map[string]interface{}{
		"uint64keys": numberedKeys(10),
	}}}
	// app.gnomonClient is nil: without the contract path this would fall to
	// getCommitHistoryFromDaemon, whose "C"-counter heuristic yields nothing.

	result := app.GetCommitHistory(zeroSCID())
	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("success = %v, want true", result["success"])
	}
	if result["count"].(int) != 10 {
		t.Fatalf("count = %v, want 10", result["count"])
	}
	commits, ok := result["commits"].([]Commit)
	if !ok {
		t.Fatalf("commits type = %T, want []Commit", result["commits"])
	}
	assertTenCommits(t, commits)
}

func zeroSCID() string { return commitTXID(0) }
