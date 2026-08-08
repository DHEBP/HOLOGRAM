package main

// An end-to-end fork against a real chain: install DOCs, install an INDEX from
// one wallet, fork it from a DIFFERENT wallet, and read both back.
//
// This is the only thing the offline tests cannot show — that the contract
// forkINDEX generates is accepted by the DVM, and that after mining, the fork
// reads back listing the source's documents under a new owner.
//
// SKIPPED unless HOLOGRAM_SIM_CHAIN_TEST=1, so `go test ./...` never talks to a
// daemon. It also refuses to run against anything that is not a simulator, and
// prints the height it saw: HOLOGRAM attaches to whatever already holds :20000,
// and a chain someone else started (or wiped) has been mistaken for a failing
// fix before.
//
//	./build/bin/simulator-linux-amd64 --data-dir=/tmp/telafork-sim --http-address=127.0.0.1:8091
//	HOLOGRAM_SIM_CHAIN_TEST=1 go test -tags webkit2_41 -run Test_ForkOnChain -v .

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/rpc"
)

const (
	forkSimDaemon  = "127.0.0.1:20000"
	forkSimWalletA = "http://127.0.0.1:30000/json_rpc"
	forkSimWalletB = "http://127.0.0.1:30001/json_rpc"
)

func forkSimRPC(t *testing.T, url, method string, params interface{}) map[string]interface{} {
	t.Helper()

	body := map[string]interface{}{"jsonrpc": "2.0", "id": "1", "method": method}
	if params != nil {
		body["params"] = params
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal %s: %v", method, err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Result map[string]interface{} `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatalf("%s returned unparseable JSON: %s", method, out)
	}
	if envelope.Error != nil {
		t.Fatalf("%s: %s", method, envelope.Error.Message)
	}
	return envelope.Result
}

func forkSimAddress(t *testing.T, wallet string) string {
	t.Helper()
	addr, _ := forkSimRPC(t, wallet, "GetAddress", nil)["address"].(string)
	if addr == "" {
		t.Fatalf("wallet %s returned no address", wallet)
	}
	return addr
}

// forkSimApp is an App with nothing but a daemon connection, so the on-chain run
// exercises HOLOGRAM's own estimator rather than a second one written for the
// test. That is what caught the install branch: storageGasFor sent sc_rpc only,
// and the daemon answered "cannot install code using this api" at every size.
func forkSimApp() *App {
	return &App{daemonClient: NewDaemonClient("http://" + forkSimDaemon)}
}

// forkSimInstall broadcasts an install and returns its txid, which is the SCID.
//
// The fee is HOLOGRAM's own storage-gas figure for these exact arguments. A
// contract install is charged for what it stores and the attached fee IS that
// budget, so an under-funded install mines, is charged, and stores nothing.
func forkSimInstall(t *testing.T, wallet string, args rpc.Arguments) string {
	t.Helper()

	addr := forkSimAddress(t, wallet)

	// The install rides on a 0 transfer and the wallet refuses to send to itself,
	// so it goes to the other pre-seeded wallet. Any registered address does; the
	// amount is zero and the contract is what is being paid for.
	dest := forkSimAddress(t, forkSimWalletB)
	if dest == addr {
		dest = forkSimAddress(t, forkSimWalletA)
	}

	fees, ok := forkSimApp().storageGasFor(args, 2, addr)
	if !ok {
		t.Fatalf("storageGasFor could not measure this install — the panel would quote no cost and the guard would not fire")
	}
	t.Logf("install storage gas: %d (about %.5f DERO)", fees, float64(fees)/100000)

	result := forkSimRPC(t, wallet, "transfer", map[string]interface{}{
		"transfers": []map[string]interface{}{{"destination": dest, "amount": 0}},
		"sc_rpc":    args,
		"ringsize":  2,
		"fees":      fees,
	})
	txid, _ := result["txid"].(string)
	if txid == "" {
		t.Fatalf("install returned no txid: %+v", result)
	}
	return txid
}

// forkSimWaitForINDEX blocks until the contract is mined and readable as a TELA
// INDEX. A read that succeeds is the proof the DVM accepted the code.
func forkSimWaitForINDEX(t *testing.T, scid string) tela.INDEX {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		index, err := tela.GetINDEXInfo(scid, forkSimDaemon)
		if err == nil {
			return index
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("contract %s never became a readable TELA INDEX: %v", scid, lastErr)
	return tela.INDEX{}
}

func forkSimWaitForDOC(t *testing.T, scid string) tela.DOC {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		doc, err := tela.GetDOCInfo(scid, forkSimDaemon)
		if err == nil {
			return doc
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("contract %s never became a readable TELA DOC: %v", scid, lastErr)
	return tela.DOC{}
}

// A placeholder signature. Well formed but not a real one, because the wallet
// RPC exposes no way to sign arbitrary data — TELA requires the fields to be
// present but does not check them at install. It is enough for what this test
// asks: whether a fork changes what a document verifies to, not whether these
// particular documents verify.
const forkSimCheck = "1111111111111111111111111111111111111111111111111111111111111111"

func Test_ForkOnChain(t *testing.T) {
	if os.Getenv("HOLOGRAM_SIM_CHAIN_TEST") == "" {
		t.Skip("set HOLOGRAM_SIM_CHAIN_TEST=1 with a simulator on :20000 to run this")
	}

	info := forkSimRPC(t, "http://"+forkSimDaemon+"/json_rpc", "DERO.GetInfo", nil)
	network, _ := info["network"].(string)
	height, _ := info["height"].(float64)
	if network != "Simulator" {
		t.Fatalf("WRONG CHAIN: :20000 reports network %q at height %.0f — refusing to install anything", network, height)
	}
	t.Logf("chain: %s at height %.0f", network, height)

	authorA := forkSimAddress(t, forkSimWalletA)
	forkerB := forkSimAddress(t, forkSimWalletB)
	if authorA == forkerB {
		t.Fatalf("both wallets are the same address, the ownership check would prove nothing")
	}

	stamp := time.Now().UnixNano()
	sourceDURL := fmt.Sprintf("forktest-%d.tela", stamp)

	// --- two real DOC contracts, installed by wallet A ---
	docSCIDs := make([]string, 0, 2)
	for _, doc := range []tela.DOC{
		{
			DocType:   "TELA-HTML-1",
			Code:      "<html><body>original</body></html>",
			DURL:      sourceDURL,
			Headers:   tela.Headers{NameHdr: "index.html"},
			Signature: tela.Signature{CheckC: forkSimCheck, CheckS: forkSimCheck},
		},
		{
			DocType:   "TELA-CSS-1",
			Code:      "body { color: #22d3ee; }",
			DURL:      sourceDURL,
			Headers:   tela.Headers{NameHdr: "style.css"},
			Signature: tela.Signature{CheckC: forkSimCheck, CheckS: forkSimCheck},
		},
	} {
		d := doc
		args, err := tela.NewInstallArgs(&d)
		if err != nil {
			t.Fatalf("DOC install args: %v", err)
		}
		// Confirmed one at a time, not broadcast back to back. A second transaction
		// from the same wallet before the first is mined carries the old committed
		// nonce and the daemon rejects it outright ("Invalid Nonce, not usable"),
		// which looks exactly like a contract that never appeared.
		scid := forkSimInstall(t, forkSimWalletA, args)
		forkSimWaitForDOC(t, scid)
		docSCIDs = append(docSCIDs, scid)
	}
	t.Logf("documents installed: %v", docSCIDs)

	// --- the INDEX being forked, installed by wallet A ---
	sourceIndex := tela.INDEX{
		DURL:    sourceDURL,
		DOCs:    docSCIDs,
		Headers: tela.Headers{NameHdr: "Fork Test", DescrHdr: "the original"},
	}
	sourceArgs, err := tela.NewInstallArgs(&sourceIndex)
	if err != nil {
		t.Fatalf("source install args: %v", err)
	}
	sourceSCID := forkSimInstall(t, forkSimWalletA, sourceArgs)
	source := forkSimWaitForINDEX(t, sourceSCID)
	t.Logf("source INDEX %s owned by %s", sourceSCID, source.Author)

	if source.Author != authorA {
		t.Fatalf("source owner is %s, expected the installing wallet %s", source.Author, authorA)
	}

	// --- the fork, built by the code under test and installed by wallet B ---
	build, err := forkINDEX(source, ForkRequest{
		SourceSCID:  sourceSCID,
		Name:        "Fork Test (forked)",
		Description: "forked by wallet B",
	})
	if err != nil {
		t.Fatalf("forkINDEX: %v", err)
	}
	if build.DURL != suggestForkDURL(sourceDURL) {
		t.Fatalf("fork dURL is %q, expected the suggestion %q", build.DURL, suggestForkDURL(sourceDURL))
	}

	forkArgs, err := tela.NewInstallArgs(&build)
	if err != nil {
		t.Fatalf("fork install args: %v", err)
	}
	forkSCID := forkSimInstall(t, forkSimWalletB, forkArgs)
	fork := forkSimWaitForINDEX(t, forkSCID)
	t.Logf("fork INDEX %s owned by %s", forkSCID, fork.Author)

	// --- what a fork must be, read back off the chain ---
	if len(fork.DOCs) != len(source.DOCs) {
		t.Fatalf("fork lists %d documents, source lists %d", len(fork.DOCs), len(source.DOCs))
	}
	for i := range source.DOCs {
		if fork.DOCs[i] != source.DOCs[i] {
			t.Fatalf("fork document %d is %s, source has %s — the fork must share the source's contracts",
				i+1, fork.DOCs[i], source.DOCs[i])
		}
	}
	if fork.Author != forkerB {
		t.Fatalf("fork owner is %s, expected the forking wallet %s", fork.Author, forkerB)
	}
	if fork.Author == source.Author {
		t.Fatalf("fork and source share an owner (%s) — nothing changed hands", fork.Author)
	}
	if fork.SCID == source.SCID {
		t.Fatalf("fork and source are the same contract")
	}
	if strings.EqualFold(fork.DURL, source.DURL) {
		t.Fatalf("fork and source share dURL %q — they would compete for one address", fork.DURL)
	}
	if fork.NameHdr != "Fork Test (forked)" {
		t.Fatalf("fork name is %q, expected the requested one", fork.NameHdr)
	}

	// The point of sharing DOC contracts: every file still resolves to the person
	// who published it, whoever owns the INDEX pointing at it.
	for _, scid := range fork.DOCs {
		doc := forkSimWaitForDOC(t, scid)
		if doc.Author != authorA {
			t.Fatalf("document %s in the fork resolves to %s, but was published by %s", scid, doc.Author, authorA)
		}
	}
}
