package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/walletapi"
)

// A dApp connected over XSWD talks to one wallet: GetAddress, GetBalance, scinvoke and
// transfer all answer for the wallet open in the app. A `transfer` carrying `sc` deploys a
// contract, and the contract records SIGNER() forever. These tests hold the line that the
// deployment is signed by that same wallet, that the install core takes no wallet decision
// of its own, and that the approval prompt says who signs before it signs.

// funcBody returns the source of the named top-level func in file, from its signature to
// the next top-level declaration. Enough to assert what a single function does or does not
// reach — the repo already uses source-level sentinels for contracts like this one.
func funcBody(t *testing.T, file, signature string) string {
	t.Helper()

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	src := string(data)

	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("%s: %q not found — the function was renamed or removed", file, signature)
	}

	rest := src[start+len(signature):]
	if end := strings.Index(rest, "\nfunc "); end >= 0 {
		return rest[:end]
	}
	return rest
}

// TestXSWDDeploySignsWithTheConnectedWallet pins the fix. InternalWalletCall resolves the
// open wallet and holds walletManager's write lock for its whole body. Handing the deploy
// to InstallSmartContract made that wallet irrelevant: in simulator mode the install took
// wallet #0, so the contract recorded a deployer the dApp had never connected to and then
// refused its own creator; off simulator it called GetWallet(), whose RLock can never be
// taken by the goroutine already holding Lock() — sync.RWMutex is not reentrant, so the
// call never returns and the wallet stays locked for the rest of the process.
func TestXSWDDeploySignsWithTheConnectedWallet(t *testing.T) {
	body := funcBody(t, "wallet.go", "func (a *App) InternalWalletCall(")

	if !strings.Contains(body, "a.installSmartContractWith(wallet,") {
		t.Fatal("the XSWD transfer-with-sc branch no longer passes its own resolved wallet to " +
			"installSmartContractWith — the deployment would be signed by whichever wallet the " +
			"install path picks, not by the wallet the dApp is connected to")
	}

	if strings.Contains(body, "a.InstallSmartContract(") {
		t.Fatal("InternalWalletCall calls InstallSmartContract again: that re-decides the signer " +
			"(wallet #0 in simulator mode) and, off simulator, takes walletManager.RLock() while " +
			"this function holds Lock() — a deadlock that never releases the wallet")
	}
}

// TestInstallCoreTakesNoWalletDecision keeps the core reachable from under the lock. Any
// walletManager access inside it reintroduces both defects at once.
func TestInstallCoreTakesNoWalletDecision(t *testing.T) {
	body := funcBody(t, "sc_function_parser.go", "func (a *App) installSmartContractWith(")

	for _, forbidden := range []string{"walletManager", "GetWallet()", "GetPrimaryWallet("} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("installSmartContractWith reaches %s: it must sign with the wallet it is "+
				"given. Deciding here overrides callers that already resolved a wallet, and any "+
				"walletManager lock deadlocks the XSWD path, which calls it under the write lock",
				forbidden)
		}
	}
}

// TestDeploymentPathsShareOneWalletPolicy: every deployment resolves its signer through
// getWalletForDeployment — open wallet first, primary as the fallback when nothing is open.
// A path that reaches for wallet #0 on its own signs as a wallet the user did not choose.
func TestDeploymentPathsShareOneWalletPolicy(t *testing.T) {
	for _, file := range []string{"sc_function_parser.go", "simulator_developer.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(data)

		if strings.Contains(src, "GetPrimaryWallet(") {
			t.Fatalf("%s selects the primary wallet directly; use getWalletForDeployment so the "+
				"wallet open in the interface signs, with the primary only as a fallback", file)
		}
		if !strings.Contains(src, "getWalletForDeployment(") {
			t.Fatalf("%s no longer resolves its signer through getWalletForDeployment", file)
		}
	}
}

// TestDeploymentSignerIsTheOpenWallet is the behavioural half: with a wallet open, the
// deployment policy returns that wallet, in simulator mode included.
func TestDeploymentSignerIsTheOpenWallet(t *testing.T) {
	wallet, err := walletapi.Create_Encrypted_Wallet(
		filepath.Join(t.TempDir(), "deploy_signer.db"), "", crypto.RandomScalarBNRed())
	if err != nil {
		t.Fatalf("create test wallet: %v", err)
	}
	defer wallet.Close_Encrypted_Wallet()

	walletManager.Lock()
	originalWallet, originalIsOpen := walletManager.wallet, walletManager.isOpen
	walletManager.wallet, walletManager.isOpen = wallet, true
	walletManager.Unlock()

	defer func() {
		walletManager.Lock()
		walletManager.wallet, walletManager.isOpen = originalWallet, originalIsOpen
		walletManager.Unlock()
	}()

	app := &App{settings: make(map[string]interface{}), consoleLogs: make([]ConsoleLog, 0)}

	if got := app.getWalletForDeployment(true); got != wallet {
		t.Fatal("simulator deployment did not resolve to the open wallet — a contract would " +
			"record a deployer other than the one the user opened")
	}
	if got := app.getWalletForDeployment(false); got != wallet {
		t.Fatal("mainnet deployment did not resolve to the open wallet")
	}
}

// TestSigningRequestNamesItsSigner: the app holds the wallet, so the app must say which one
// signs. Approving a prompt that does not name the signer is approving blind.
func TestSigningRequestNamesItsSigner(t *testing.T) {
	event := signingRequestEvent("sign-1", "scinvoke",
		map[string]interface{}{"scid": "deadbeef"}, "An App", "https://example.invalid", "deto1qy...")

	if event["walletAddress"] != "deto1qy..." {
		t.Fatalf("approval event does not carry the signing wallet: %v", event["walletAddress"])
	}
	if _, marked := event["scDeploy"]; marked {
		t.Fatal("a plain scinvoke must not be announced as a contract deployment")
	}
}

// TestDeployRequestIsAnnouncedAsADeployment: a transfer carrying `sc` has no destination, no
// SCID and no entrypoint. Unannounced, it reaches the modal as an empty request while a
// contract is being deployed.
func TestDeployRequestIsAnnouncedAsADeployment(t *testing.T) {
	code := "Function Initialize() Uint64\n10 RETURN 0\nEnd Function"
	event := signingRequestEvent("sign-2", "transfer",
		map[string]interface{}{"sc": code, "ringsize": float64(2)}, "An App", "Websocket", "deto1qy...")

	if event["scDeploy"] != true {
		t.Fatal("a transfer carrying sc is a contract deployment and must be announced as one")
	}
	if event["scCodeBytes"] != len(code) {
		t.Fatalf("deployment size not disclosed: %v", event["scCodeBytes"])
	}
}
