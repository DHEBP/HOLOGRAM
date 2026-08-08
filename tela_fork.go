package main

// Forking a TELA INDEX.
//
// A fork is ONE new INDEX contract listing the SAME DOC SCIDs as the INDEX it
// forks. No DOC is copied and no DOC is redeployed, which is exactly why the
// author signatures survive: a signature is verified against the DOC contract's
// own owner (tela_signature.go verifyDOCAt), and the INDEX is never consulted.
// Files inside a fork therefore still verify to the person who wrote them, not
// to the forker.
//
// The chain assigns, and the forker cannot set: the new SCID (the install txid),
// the owner (TELA-INDEX-1 does STORE("owner", address()) where address() is
// SIGNER()), commit 0, and likes 0 / dislikes 0. RATINGS DO NOT CARRY OVER — a
// fork of a highly rated app starts at zero, and the fork panel says so.
//
// ⚠️ NO PARENT POINTER IS WRITTEN, deliberately. TELA-INDEX-1 has no field for
// "this INDEX forks that one". Every place one could be put is either a private
// convention this repository would be minting on the standard's behalf, or a
// second transaction that can fail on its own and leave a fork silently
// unattributed. That is a decision about TELA, not about HOLOGRAM, so the fork
// is honest about carrying no machine-readable link back to its source.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/civilware/tela"
	"github.com/deroproject/derohe/rpc"
)

// forkRingsize is the ring size every fork installs at.
//
// It is a constant rather than a literal at three call sites because it is not
// a tuning knob: the estimate must be measured at the ring size the install
// actually uses, and the number is quoted to the user as the reason their
// address ends up in the contract. See ForkTELA for why it is 2 and not 16.
const forkRingsize = 2

// ForkRequest is what the user confirmed in the fork panel.
//
// It carries NO DOC list. The whole point of a fork is that the file set is the
// source's, so it is read from the source contract at install time rather than
// accepted from the caller — a frontend that could hand over a DOC list could
// hand over a different one, and the result would still be presented as a fork.
type ForkRequest struct {
	SourceSCID  string `json:"sourceScid"`
	DURL        string `json:"durl"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"iconUrl"`
}

// forkBuild is one fork worked out in full: what it came from, what it installs,
// and the arguments that install it. Preview and the real thing both go through
// it so the cost quoted is measured over the arguments actually sent.
type forkBuild struct {
	Source tela.INDEX
	Fork   tela.INDEX
	Args   rpc.Arguments
}

// suggestForkDURL proposes a dURL for a fork that is not the source's.
//
// It inserts before the LAST dot so a TELA tag survives: TELA reads .lib,
// .shards and .bootstrap off the end of a dURL as structural meaning
// (tela.go TAG_LIBRARY and friends), so appending would change what the INDEX
// claims to be. "villager.tela" becomes "villager-fork.tela"; "a.b.bootstrap"
// becomes "a.b-fork.bootstrap".
func suggestForkDURL(durl string) string {
	durl = strings.TrimSpace(durl)
	if durl == "" {
		return ""
	}
	if i := strings.LastIndex(durl, "."); i > 0 {
		return durl[:i] + "-fork" + durl[i:]
	}
	return durl + "-fork"
}

// validateForkDURL refuses a dURL a fork must not be installed under.
//
// A fork MUST NOT reuse its source's dURL, and nothing on chain stops it.
// Two reasons, both real:
//
//   - dURL is not unique and is not enforced. Resolution ranks every INDEX
//     sharing a dURL by most recent interaction (gnomon.go ResolveDURLAll), so a
//     fork on the same dURL takes over dero://<durl> the moment it is updated or
//     rated. HOLOGRAM already carries a live collision of exactly this shape in
//     app_navigation.go.
//   - dURL is the on-disk key. Clones land in <path>/<dURL> and cloning a second
//     INDEX there fails outright with "file already exists", so two INDEXes on
//     one dURL cannot both be cloned or served — and HOLOGRAM's own cache clear
//     removes datashards/tela/<durl>, which would wipe both.
//
// The character rules exist because that same dURL becomes a path element on
// clone and a string literal in the generated contract.
func validateForkDURL(durl, sourceDURL string) error {
	if durl == "" {
		return fmt.Errorf("a fork needs its own dURL")
	}
	if strings.EqualFold(durl, strings.TrimSpace(sourceDURL)) {
		return fmt.Errorf("a fork cannot use %q, the dURL it is forking — both would answer to the same address and only one of them could be cloned", sourceDURL)
	}
	if strings.ContainsAny(durl, `/\"`) || strings.Contains(durl, "..") {
		return fmt.Errorf("a dURL cannot contain a quote, a slash or '..' — it becomes a folder name when this fork is cloned")
	}
	for _, r := range durl {
		if r <= ' ' || r == 0x7f {
			return fmt.Errorf("a dURL cannot contain spaces or control characters")
		}
	}
	return nil
}

// forkINDEX builds the INDEX a fork installs from the INDEX it forks.
//
// DOCs and Mods are carried over exactly; everything else the user owns. The DOC
// slice is copied rather than aliased so nothing downstream can reorder the
// source's file list — order is load bearing, DOC1 is the entrypoint.
//
// Author is deliberately not set. It is not an install argument at all: the
// contract stores SIGNER() at install, so the owner is whoever broadcasts.
func forkINDEX(source tela.INDEX, req ForkRequest) (tela.INDEX, error) {
	// A blank dURL takes the suggestion rather than failing, so the panel can ask
	// for a preview before the user has typed anything and get the default back
	// from here. Keeping the rule in one place is the point: a copy of it in the
	// frontend would be a second definition of the collision the rule prevents.
	durl := strings.TrimSpace(req.DURL)
	if durl == "" {
		durl = suggestForkDURL(source.DURL)
	}
	if err := validateForkDURL(durl, source.DURL); err != nil {
		return tela.INDEX{}, err
	}

	if len(source.DOCs) == 0 {
		return tela.INDEX{}, fmt.Errorf("this INDEX lists no documents, so there is nothing to fork")
	}

	// Only the name falls back to the source's. TELA rejects an INDEX with an
	// empty name header, so a blank one has to be filled from somewhere; a blank
	// description or icon is a legitimate choice and is left blank.
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(source.NameHdr)
	}
	if name == "" {
		return tela.INDEX{}, fmt.Errorf("a fork needs a name")
	}

	docs := make([]string, len(source.DOCs))
	copy(docs, source.DOCs)

	return tela.INDEX{
		DURL: durl,
		DOCs: docs,
		Mods: source.Mods,
		Headers: tela.Headers{
			NameHdr:  name,
			DescrHdr: req.Description,
			IconHdr:  req.IconURL,
		},
	}, nil
}

// buildFork reads the source INDEX and works the fork out end to end.
func (a *App) buildFork(requestJSON string) (*forkBuild, error) {
	var req ForkRequest
	if err := json.Unmarshal([]byte(requestJSON), &req); err != nil {
		return nil, fmt.Errorf("invalid fork request: %s", err)
	}

	if len(req.SourceSCID) != 64 {
		return nil, fmt.Errorf("invalid SCID, must be exactly 64 characters")
	}

	source, err := tela.GetINDEXInfo(req.SourceSCID, a.telaEndpoint())
	if err != nil {
		return nil, fmt.Errorf("could not read the INDEX being forked: %s", err)
	}

	fork, err := forkINDEX(source, req)
	if err != nil {
		return nil, err
	}

	// NewInstallArgs is also the size gate: it renders the contract and refuses
	// anything over MAX_INDEX_INSTALL_SIZE, which a large DOC list can reach.
	args, err := tela.NewInstallArgs(&fork)
	if err != nil {
		return nil, err
	}

	return &forkBuild{Source: source, Fork: fork, Args: args}, nil
}

// forkSigner is the address the fork would be installed from, or "" when no
// wallet is open. Used only to make the cost estimate match the real call.
func (a *App) forkSigner() string {
	wallet := a.getWalletForDeployment(a.IsInSimulatorMode())
	if wallet == nil {
		return ""
	}
	return wallet.GetAddress().String()
}

// forkResponse describes a worked-out fork for the confirmation panel: what will
// be installed, what it costs, and whether the chain would refuse it.
//
// Every key here is read by RepoFork.svelte. The source's own SCID, dURL, owner
// and name were on this map and were not — the panel already holds the
// GetINDEXInfo result it was opened from — and a shape nothing reads is a shape
// a future frontend has to keep working.
func (a *App) forkResponse(build *forkBuild) map[string]interface{} {
	// The same lookup ForkTELA does, so the panel cannot say "ready" for a wallet
	// the install would then refuse — or refuse in simulator mode, where a
	// deployable wallet exists that the frontend's own walletState does not know
	// about.
	signer := a.forkSigner()

	out := map[string]interface{}{
		"success":  true,
		"durl":     build.Fork.DURL,
		"mods":     build.Fork.Mods,
		"docCount": len(build.Fork.DOCs),
		// Rendered, not decoration: a ring size of 2 is what writes the forker's
		// address into the contract as its public owner. See ForkTELA.
		"ringsize":    forkRingsize,
		"walletReady": signer != "",
	}

	// Measured over build.Args, the arguments this fork actually installs with.
	// An estimate taken over anything else underpays by the difference, because
	// the whole contract body rides in the charged argument blob (gas_guard.go).
	if gas, ok := a.storageGasFor(build.Args, forkRingsize, signer); ok {
		out["storageGas"] = gas
		out["storageGasDero"] = float64(gas) / 100000
		out["costMeasured"] = true
		// storageGasError rather than storageGasRefusal: the refusal helper names the
		// entrypoint, and an install has none, so it would say "this contract call".
		if storageGasExceeded(gas) {
			out["tooLarge"] = true
			out["costWarning"] = storageGasError("this fork", gas).Error()
		}
	} else {
		out["costMeasured"] = false
	}

	return out
}

// PreviewTELAFork works out what forking an INDEX would install and what it
// would cost, and writes nothing. The panel that asks for confirmation is built
// from this.
func (a *App) PreviewTELAFork(requestJSON string) map[string]interface{} {
	build, err := a.buildFork(requestJSON)
	if err != nil {
		return ErrorResponse(err)
	}
	return a.forkResponse(build)
}

// ForkTELA installs a fork of an INDEX: one new contract, the source's DOC
// SCIDs, owned by whoever signs it.
//
// Ringsize is 2 rather than the source's. At ring 16 or above the contract
// stores "anon" as its owner and can never be updated, so an immutable fork
// would be a dead end for the very person creating it. The source's own ring
// size is irrelevant here — nothing about the fork inherits from it.
//
// The install self-funds: tela.Transfer estimates over these same arguments and
// attaches the result as the fee, so the storage-gas top-up the invoke paths
// need does not apply. The guard below is a different job — refusing a fork the
// chain could not apply at any fee.
//
// ⚠️ That guard is UNREACHABLE for a plain fork today, and it is kept anyway.
// Measured against a simulator: 119 DOCs costs 19,699 storage gas, 301 short of
// the chain's 20,000 ceiling, and at 120 tela refuses first on its own 11.64KB
// contract-size limit. So the refusal a user actually meets comes from
// NewInstallArgs in buildFork, not from here. This stays because that 11.64KB is
// a constant in a third-party module rather than a chain rule: if it is ever
// raised, the ceiling becomes the real limit and this is the only thing between
// a fork and a transaction that mines, charges, and stores nothing.
func (a *App) ForkTELA(requestJSON string) map[string]interface{} {
	build, err := a.buildFork(requestJSON)
	if err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] Fork rejected: %v", err))
		return ErrorResponse(err)
	}

	wallet := a.getWalletForDeployment(a.IsInSimulatorMode())
	if wallet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "No wallet is currently open",
		}
	}

	if err := a.guardStorageGas(build.Args, wallet.GetAddress().String(), "this fork"); err != nil {
		a.logToConsole(fmt.Sprintf("[ERR] [TELA] Fork refused: %v", err))
		return ErrorResponse(err)
	}

	a.logToConsole(fmt.Sprintf("[TELA] Forking %s → dURL %s (%d documents, shared with the original)",
		build.Source.SCID[:16]+"...", build.Fork.DURL, len(build.Fork.DOCs)))

	// Installed through InstallINDEX rather than tela.Installer directly, so the
	// fork uses the same wallet, endpoint and simulator handling as every other
	// INDEX install instead of a second copy of it that can drift.
	payload, err := json.Marshal(INDEXInfo{
		Name:        build.Fork.NameHdr,
		Description: build.Fork.DescrHdr,
		DURL:        build.Fork.DURL,
		IconURL:     build.Fork.IconHdr,
		DOCSCIDs:    build.Fork.DOCs,
		Ringsize:    forkRingsize,
		Mods:        build.Fork.Mods,
	})
	if err != nil {
		return ErrorResponse(err)
	}

	result := a.InstallINDEX(string(payload))
	if ok, _ := result["success"].(bool); !ok {
		return result
	}

	// txid and durl come from InstallINDEX. Only what the panel prints is added
	// here; the source SCID and the DOC list were on this map and read nowhere.
	result["docCount"] = len(build.Fork.DOCs)
	result["ringsize"] = forkRingsize
	result["message"] = "Fork submitted — it becomes a contract once the transaction is mined"
	return result
}
