# Hologram

**A Native Desktop Browser for the DERO Decentralized Web**

---

## Overview

**Hologram** is a native desktop application for browsing **TELA applications** stored entirely on the DERO blockchain. Built with Wails v2 (Go + Svelte), it provides direct blockchain access without web browser security restrictions.

> **Philosophy:** *"Browser-First, Not Wallet-First"* — Hologram is a TELA browser that happens to have wallet features.

### Key Features

| Category | Features |
|----------|----------|
| **Browser** | TELA app rendering, dURL navigation, content caching, offline mode |
| **Wallet** | Create, restore, manage DERO wallets with full transaction history |
| **Discovery** | Gnomon-powered search, ratings, and content filtering |
| **Explorer** | Block/TX explorer with DeroProof validation |
| **Simulator** | One-click dev environment with instant blocks |
| **Studio** | Local dev server with hot reload for TELA development |
| **Privacy** | No tracking, no ads, censorship-resistant |

---

## Quick Start

### Prerequisites

- **Go** 1.21+
- **Wails** v2 CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Node.js** 18+
- **DERO Daemon** (derod) — auto-downloads if not found

### Development

```bash
git clone https://github.com/DHEBP/HOLOGRAM.git
cd HOLOGRAM
cd frontend && npm install && cd ..
wails dev
```

### Production Build

```bash
wails build
# Output: build/bin/Hologram.app (macOS)
```

---

## Architecture

```
Hologram (Wails v2)
├── Direct HTTP → derod:10102 (blockchain reads)
├── XSWD Server → 127.0.0.1:44326 (integrated wallet + dApp bridge)
├── XSWD Client → Engram (optional external wallet)
├── Gnomon Indexer (content discovery)
├── Graviton Cache (persistent storage with versioning)
└── Iframe → TELA content (sandboxed + telaHost injection)
```

### Network Modes

| Mode | Description |
|------|-------------|
| **Mainnet** | Full production network |
| **Testnet** | Development and testing |
| **Simulator** | Local environment with instant blocks (no real DERO) |

---

## For dApp Developers

### telaHost Bridge API

Hologram injects a `telaHost` JavaScript API into every TELA application, enabling blockchain and wallet interactions:

```javascript
// Check if running in Hologram
if (window.telaHost) {
    // Read-only operations (instant)
    const info = await telaHost.call('DERO.GetInfo');
    const sc = await telaHost.getSmartContract(scid, true, true);
    
    // Wallet operations (requires user approval)
    await telaHost.connect();
    const address = await telaHost.getAddress();
    const balance = await telaHost.getBalance();
    
    // Transactions (triggers approval modal)
    await telaHost.transfer({ transfers: [...], ringsize: 16 });
    await telaHost.scInvoke({ scid, entrypoint: 'Vote', params: [...] });
}
```

| Method | Description |
|--------|-------------|
| `call(method, params)` | Generic RPC call |
| `connect()` | Request wallet connection |
| `isConnected()` | Check wallet status |
| `getAddress()` | Get wallet address |
| `getBalance()` | Get balance (total + unlocked) |
| `getNetworkInfo()` | Chain height, difficulty, peers |
| `getBlock(height)` | Block data |
| `getTransaction(txid)` | Transaction details |
| `getSmartContract(scid)` | SC code and variables |
| `transfer(params)` | Send DERO (requires approval) |
| `scInvoke(params)` | Invoke SC function (requires approval) |

### Local Development

1. Open **Studio → Serve** tab
2. Select your TELA app directory
3. Server starts with hot reload
4. Full `telaHost` API available during development

---

## Project Structure

```
Hologram/
├── main.go                 # Wails entry point
├── app.go                  # Core app logic (~2000+ lines)
├── wallet.go               # Wallet operations
├── xswd_server.go          # Integrated XSWD proxy
├── xswd_client.go          # External wallet connection
├── gnomon.go               # Content discovery
├── epoch_handler.go        # Developer support
├── simulator_*.go          # Simulator mode (4 files)
├── local_dev_server.go     # Hot reload dev server
├── explorer_service.go     # Block explorer
├── time_travel_explorer.go # SC state history
├── offline_cache.go        # Offline browsing
│
├── frontend/
│   ├── src/
│   │   ├── routes/         # Page components
│   │   │   ├── Search.svelte
│   │   │   ├── Browser.svelte
│   │   │   ├── Explorer.svelte
│   │   │   ├── Network.svelte
│   │   │   ├── Studio.svelte
│   │   │   ├── Wallet.svelte
│   │   │   └── Settings.svelte
│   │   ├── lib/components/ # Reusable components
│   │   └── styles/
│   │       └── hologram.css # v6.1 Design System
│   └── wailsjs/            # Auto-generated Go bindings
│
├── README.md               # This file
├── wails.json              # Wails configuration
└── go.mod                  # Go dependencies
```

---

## License

See [LICENSE](LICENSE) file for details.

---

**Version:** 1.0.0  
**Status:** Production Ready  
**Last Updated:** December 27, 2025
