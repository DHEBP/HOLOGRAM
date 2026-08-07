# Changelog

All notable changes to HOLOGRAM are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.8] - 2026-08-06

A rebuilt consent model for dApps — connecting no longer asks for wallet access, and access that is remembered is remembered per wallet. Plus sender-attribution privacy, cold-wallet genesis, XSWD/Browser trust-boundary hardening, fund-safety guards on the hot send path, and Linux launch fixes for WebKitGTK.

### Added
- Wallet: cold-wallet offline genesis — mine registration air-gapped, then broadcast a saved DCSP blob from inside HOLOGRAM (paste → fingerprint check → send).
- Wallet: sender-attribution controls on Send (anonymize / preferred decoys) with an approval modal that shows the effective ring and only promises a decoy when it holds.
- Send: Ring Members inline editor — build and name a decoy set without leaving the form.
- Linux: desktop installer in the release tarball (`install.sh` / `uninstall.sh` / icon / `.desktop`) so double-click and the app menu work without a bare binary.
- Builds against the consensus-identical `DHEBP/derohe` fork (walletapi privacy patches only).

### Changed
- **dApp consent is now three doors.** Connecting grants public blockchain data and nothing else — no checkboxes. Anything touching the wallet is asked for at the moment the app reaches for it, and that answer can be remembered. Spending is asked every time and can never be stored, enforced where grants are written rather than in the interface.
- **Remembered access is scoped to the wallet that granted it.** Approving an app under one wallet no longer covers the next wallet you open — that one is asked again. Grants made before this update cannot be attributed to a wallet and are dropped, so each app asks once more per wallet.
- Permission prompts list their three answers in a column, least commitment first, so a long label cannot squeeze them into unreadable chips.
- Smart contract writes report **submitted**, not saved. The call returns when the transaction is broadcast; the contract applies it when the transaction is mined and can still refuse.
- App state consolidated under `~/.dero/hologram` (legacy CWD litter migrated best-effort).
- Idle auto-lock: UI drops to the unlock screen on `wallet:autoLocked`; docs claim an app-layer lock (spend refused), not in-memory secret scrub.
- Browser session auth is parent-owned.
- Linux release folder ships binary + installer assets instead of a bare executable.

### Fixed
- Smart contract writes are paid for. A contract call is charged for what it stores, and the fee attached to the transaction *is* that budget — but the fee the wallet works out on its own covers less than the chain charges, so a write past a few hundred bytes was mined, charged, and stored nothing while the interface reported success. HOLOGRAM now measures what a call will store and, when the transaction it built does not carry enough, rebuilds it carrying the measured amount. It only ever raises the fee, and never above the chain's per-call ceiling, beyond which the extra would be spent without buying anything. On a test chain, writes of 500, 2,000 and 5,000 bytes were all lost before this change and all stored after it. This covers calls made from the Explorer and Studio and those arriving from dApps, since both now share the funded path.
- Smart contract writes that the chain cannot apply at any price are refused before broadcast, on the Explorer variable editor, the Explorer/Studio function caller, and INDEX updates. Above the storage ceiling a write is still mined and still charged while the contract stores nothing — HOLOGRAM checks the cost first and says how much to cut. When the cost cannot be measured the write proceeds as before rather than being blocked on a failed measurement; dApp calls arriving over XSWD are funded but not yet refused this way.
- dApps can read the chain tip again: `DERO.GetHeight` and the other no-argument daemon calls were rejected outright when a dApp sent an empty parameter object, which is ordinary JSON-RPC client behaviour.
- A refused prompt reaches the dApp as the spec's own “permission denied” code instead of a generic failure, on every bridge — so an app can tell a refusal from a broken call.
- Permission prompts name the method that triggered them, show a full-length contract origin without clipping it mid-string, and no longer fragment their own explanatory text into columns.
- Browser: standing grants are read back from storage, so “remember this” survives a tab switch; grants key on the chain-resolved contract id after a session is restored, and a requested navigation wins over a restored one.
- TELA Rate from Discover Apps: submit no longer requires an Engram XSWD *client* connection — with the integrated wallet open (“Wallet Ready”) it invokes `Rate` locally. The sidebar XSWD light only meant HOLOGRAM’s server was up, so ratings failed with “Wallet not connected via XSWD” and no console trail.
- About / version: `AppVersion` is embedded from the `VERSION` file (kept in sync with CHANGELOG by CI) so About can no longer stick on a stale hardcoded release like 1.0.5.
- XSWD: empty-origin sockets no longer skip permission checks; handshake requires a non-empty `url`.
- Browser: connect permissions are enforced on integrated-wallet reads; client `authState` is ignored (no forgeable `'ok'`).
- Browser: srcdoc loads lock sandbox **without** `allow-same-origin` before content is assigned (closes a same-origin/`window.go` pivot race).
- Approval modal shows every destination, token lines, and `sc_dero_deposit` / `sc_token_deposit` — what you approve is what executes.
- Native-DERO burn guard: junk `sc_rpc` without a real entrypoint/`SCACTION` no longer bypasses the block or labels destruction as a deposit.
- Integrated-address invoice checks (amount + expiry) run on the hot `InternalWalletCall` path the Send UI actually uses.
- XSWD anonymize clamps ring size to ≥16 so the decoy promise cannot ship at ring 2.
- TELA ratings: Rate arg is `"r"` (was `"rating"` — ratings never recorded); non-functional Like/Dislike removed for Engram parity.
- Browser: Discover Apps keeps polling until Gnomon’s first index finishes (~5 min), with a one-shot TOP RATED → ALL fallback on empty cold start.
- Wallet: empty-destination sends rejected; wrong-network destinations blocked on XSWD/token paths; unresolved destinations no longer misdiagnosed as “not registered.”
- Linux: WebKitGTK DMA-BUF hang on NVIDIA/Wayland — set `WEBKIT_DISABLE_DMABUF_RENDERER=1` before the WebView starts.

---

## [1.0.7] - 2026-06-18

Privacy Mode, automatic token discovery, clearer asset handling, hardened transfer validation, and Linux release binaries that run on current distros out of the box.

### Added
- **Privacy Mode** — seals HOLOGRAM's network connections behind your approval. Switch it on and the app blocks every outbound connection until you approve the destination, so nothing reaches the network without your say-so. Its armed state shows on the wallet anchor.
- **Signal Dark** — display masking for your address, balances, tokens, and avatar, now an independent control separate from Privacy Mode.
- Wallet: automatic token discovery via Gnomon — held tokens and NFAs are detected and added to the portfolio without manual SCID entry.

### Changed
- Wallet: native DERO is managed as the base coin (Dashboard / Send), separate from the contract-token portfolio — which now lists contract assets only.
- Wallet: refreshed token portfolio rows with per-token actions (send, refresh metadata, remove) and an improved empty state.

### Fixed
- Wallet: hardened transfer validation so a native-DERO burn is consistently rejected across all send paths.
- Wallet: token transfers are credited via the amount field on the token's SCID, and per-token encrypted balances and metadata resolve correctly (including unindexed SCIDs).
- XSWD: canonical response shapes, correct SC deposit semantics, scid-aware balance reads for the TELA bridge, and a permission-tracking data-race fix.
- Linux: release binaries are built against `webkit2gtk-4.1` (libsoup3) instead of the discontinued `4.0`, so they launch on Ubuntu 24.04+, Debian 13+, Fedora 40+, and Arch without a manual library symlink. CI now fails the release if a Linux binary links the old `4.0` runtime.

---

## [1.0.6] - 2026-06-08

Payment URI workflow, unified storage controls, and XSWD bridge fixes.

### Added
- Wallet: smart-paste payment URI field with a 7-state input model
- Settings: Data & Storage section — a unified clear/reset surface

### Changed
- Tightened the XSWD RPC surface and hardened CI checks

### Fixed
- XSWD: route `DERO.GetHeight` through the daemon proxy and reclassify it as read-public-data
- XSWD bridge: dispatch message events to `addEventListener` handlers
- Wallet: `CreatePaymentRequest` uses the local wallet path; failures are surfaced instead of swallowed
- Payment URI: dual-path integrated-address decode + address-aware OmniSearch
- Studio: accept `InitializePrivate` as a valid SC entrypoint
- Dev server: hot reload actually reloads, and real errors are surfaced

---

## [1.0.2] - 2026-04-20

Wallet registration and expanded platform support.

### Added
- Manual PoW-based wallet registration — new wallets can register on-chain without waiting for incoming DERO
- Registration progress UI with hash count, elapsed time, and cancel option
- Blockchain confirmation polling after registration TX broadcast
- Linux ARM64 (aarch64) binary for Raspberry Pi and ARM servers

### Changed
- PoW registration uses all available CPU cores (GOMAXPROCS-1) for faster completion
- Release artifacts renamed from `Hologram-*` to `HOLOGRAM-*` for brand consistency

### Fixed
- Duplicate toast notifications when starting wallet registration

---

## [1.0.1] - 2026-04-20

Cross-platform binaries and release automation.

### Added
- Pre-built binaries for Linux (amd64) and Windows (amd64) — closes the gap from v1.0.0 release notes
- GitHub Actions release workflow (`.github/workflows/release.yml`) — tag-triggered cross-platform builds
- Universal macOS binary (Intel + Apple Silicon in one file)
- SHA256 checksums for all release artifacts

### Changed
- Added plain-language disclaimer section to README reinforcing MIT "AS IS" terms for wallet-adjacent software

---

## [1.0.0] - 2026-04-18

First public release

### Added (post-RC)
- `hologram-explorer-search` message handler — TELA apps can invoke Explorer searches
- Privacy Mode enforcement for external link opens (https intercept + user prompt)
- `dero://` deep link protocol registration and launch URL handling

### Fixed (post-RC)
- Network classification now uses daemon-reported field (fixes simulator edge cases)
- Recent search history scoped per-network
- SC deployment TXIDs auto-pivot to contract view in Explorer
- Gnomon corrupted cache recovery on startup
- Batch deploy budget gate and mainnet precheck hardening
- Simulator network switching and wallet state alignment
- EPOCH attribution and uptime overflow guards
- Favorite toggling and offline cache metadata display
- Active wallet filename kept in sync after operations

---

## [1.0.0-rc] - 2026-04-01

Release candidate — full feature set for testing.

### Added

#### TELA Browser
- Full TELA decentralized web browser — resolves INDEX and DOC contracts, reconstructs multi-shard apps, and renders them in an isolated webview
- Per-tab browser history with back/forward navigation and iframe caching
- Auto-start Gnomon on Browser page mount for immediate app discovery
- TELA icon rendering with V2 header support and SCID icon resolution
- TELA-STATIC text file rendering support
- Content filtering for Browser app list
- Download interceptor for dApp blob/local file downloads via JS bridge

#### Studio
- **Batch Upload** — scan a local folder, auto-infer app name/description/dURL from `package.json`, `index.html`, and `README.md`, preview a preflight summary (file count, sizes, oversized warnings), and deploy as a TELA INDEX + DOC set
- **DocShard Manager** — shard any file >18 KB into `.shard` fragments and reconstruct from a shard folder; drag-and-drop file intake for both modes
- **Install DOC / Install INDEX** — deploy individual contracts with DocType selector
- **Version Control** — file-based diff viewer with TELA version history and semantic labels
- **Clone** — clone an existing TELA app from chain
- **Deploy SC** — raw smart contract deployment with DVM validation, gas estimation, and safety guardrails
- **Deploy SC Function Interactor** — dynamic SC function call UI
- dURL auto-slug populated on folder scan; reactive warning when a shard batch is missing the `.tela.shards` dURL suffix
- Preflight summary panel: file count, total size, oversized file detection with shard-manager hints

#### Wallet
- Connect via XSWD protocol (Engram and compatible wallets)
- Send DERO and tokens with ringsize selection, fee display, and integrated address validation
- Receive with integrated address generation and payment URI support
- Transaction history with export, TXID caching, and semantic labels
- Hide balance / hide address privacy toggles (per-field eye icon, persisted across restarts)
- Privacy masking propagated to Sidebar (expanded, collapsed, and menu states), WalletModal, Recent Activity, and History
- Change wallet password
- Wallet recovery from backup
- Internal wallet polling with asset support
- `GetPublicKey` and `DecryptPayload` for Dead Drop encryption

#### Gnomon Integration
- Gnomon-powered app discovery with fastsync and Time Machine (historical snapshot browser)
- OmniSearch — unified search across apps, smart contracts, transactions, and block numbers, with autocomplete and cross-tab support
- Tagging and content-class metadata on discovered apps
- Simple-Gnomon: historical queries and on-chain data allocation
- Resync UI with fastsync option; DB reset on height mismatch

#### XSWD API Server
- Built-in XSWD WebSocket server for dApp ↔ wallet communication
- Handles `DERO.*`, `Gnomon.*`, `EPOCH.*`, and `DeroAuth.*` method namespaces
- OAuth-style redirect flow for DeroAuth
- Per-app permission scoping with read-only app detection
- `telaHost` API injected into XSWD bridge for cross-origin and local dev compatibility
- XSWD bridge injection for local dev server HTML responses

#### Simulator
- Full local simulator mode — runs an embedded DERO daemon and test wallets for offline development
- Pre-seeded test wallets UI on Wallet page
- Complete DVM deploy + invoke flow in simulator
- Simulator crash detection and notification
- `ReconnectSimulatorMode` for app restart with an existing daemon
- Automatic fallback to mainnet when simulator daemon is unreachable

#### Settings & Infrastructure
- Settings persistence across restarts (daemon endpoint, network, privacy toggles, and more)
- Remote daemon endpoints persisted across restarts
- First-run wizard with node detection, LAN/external node option, and developer support screen
- LAN node connection support for power users
- SHA256 checksum verification for downloaded binaries
- EPOCH fair developer support address switching
- Battery detection for developer support (Windows via PowerShell/WMI)
- Villager identicon avatar system with 12 background patterns
- `derod` and `simulator` built from derohe source via Makefile (`make all`)
- Build metadata (`version`, `commit`, `buildDate`) injected at build time via `-ldflags`

### Fixed

#### XSWD / Wallet Parity
- `SIGNER()` returned empty string — was caused by hardcoded ringsize 16; now uses ringsize from params (default 2)
- `transfers[].scid` not parsed — token transfers were broken
- `fees` always hardcoded to 0
- `sc_rpc` only handled U/S/H types — `I` (int64) was silently dropped
- `sc_dero_deposit` / `sc_token_deposit` not parsed
- SC deployment via XSWD (`sc` param) not routed correctly
- `DERO.*` and `Gnomon.*` methods not forwarded to WebSocket dApps
- `AttemptEPOCHWithAddr` not handled
- `GetMaxHashesEPOCH` response key mismatch
- Null bytes in SCIDs causing key lookup failures — stripped in `sanitizeSCID()` and `decodeHexString()`
- Hex-encoded string values in `GetAllSCIDVariableDetails` responses not decoded
- `GetDaemon` endpoint format corrected to match Engram; simulator endpoint returned correctly
- `GetHeight` now populates `stableheight` and `topoheight`
- Double approval modal on TELA app reconnect
- Missing XSWD methods, lowercase aliases, and permission mappings

#### Simulator
- Test wallet showed 0 DERO balance
- Test wallets not loading on app restart
- Simulator daemon crashed on SC deploy
- Gnomon stale height after simulator restart
- Unreachable simulator now falls back to mainnet correctly
- SC deploy crash via disconnect-before-pause and post-tx settle delay
- WebSocket sequencing conflicts in batch upload
- Fund Wallet network mismatch and balance sync
- Fastsync disabled in simulator mode (was causing incorrect progress display)

#### Studio / Shards
- Shard `outputDir` reported a phantom relative path (`./datashards/shards/`) instead of the actual output directory (`filepath.Dir(filePath)`)
- GZIP compression toggle was not actually compressing before sharding
- Double extension when discovering compressed shard files (`.gz.gz`)
- Shard discovery and ordering in `ConstructFromShards`
- dURL tag detection aligned with backend (`.tela.shards`, `.tela.lib` suffixes)
- Drop-hijack bug: dropping a file into the app caused the webview to navigate to the file, rendering the app non-functional — fixed with `DisableWebViewDrop: true` in Wails config and a global JS `preventDefault` guard

#### Browser
- Blob downloads not intercepted for local dev server — added blob cache interceptor
- TELA entry point ordering: `index.html` now deployed as DOC1
- `telaHost` not enabled for local files — fixed
- New XSWD methods not matched due to case-sensitive string comparison
- Browser console auto-scroll fighting user scroll-up
- Double OmniSearch dropdown on load
- Normal transactions misclassified as smart contracts in Explorer

#### Wallet
- Broadcast transactions to daemon after building (were not being sent)
- Amount rounding, integrated address validation, payment URI parsing
- `walletPath` lost on wallet close, breaking re-open fallback
- Reserved insufficient fees when sending max balance
- Ringsize selection missing from Transfer and TokenSend modals
- Non-blocking daemon connect, address prefix restore, file picker fixes
- TELA app reconnect shown double approval modal

#### Network / Settings
- Daemon endpoint display and effective network label on startup
- Settings not persisted across restarts (daemon endpoint)
- Gnomon DB not reset when stored height exceeded daemon chain height
- Network mismatch detection on startup

#### UI / General
- Gas estimation formula was ~100x too high
- OmniSearch dropdown opened automatically on load
- Scroll freezing in Wallet History and SyncManager
- Block number color, sidebar indicator navigation, search autocomplete
- `window.confirm` replaced with modal for destructive actions (Full Resync)
- Design System v7.0 compliance pass (emojis removed from log/UI, colors, layout)
- Infinite log loop in production builds
- Console clear button not working

### Changed
- Go module path is `github.com/DHEBP/HOLOGRAM`
- Testnet support removed — mainnet and simulator only
- Historical Timeline feature removed (superseded by Time Machine)
- Dead code, phantom mining, bookmark, and text-index bindings pruned
- README updated: Go requirement corrected to 1.24.0+, build instructions clarified
- Copyright year updated to 2026

[1.0.0]: https://github.com/DHEBP/HOLOGRAM/releases/tag/v1.0.0
[1.0.0-rc]: https://github.com/DHEBP/HOLOGRAM/releases/tag/v1.0.0-rc
