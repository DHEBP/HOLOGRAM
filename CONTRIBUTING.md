# Contributing to HOLOGRAM

Thank you for your interest in contributing to HOLOGRAM — a native desktop DERO browser built with Go and Svelte.

---

## Before You Start

- Search [existing issues](https://github.com/DHEBP/HOLOGRAM/issues) before opening a new one.
- For larger changes, open an issue first to discuss the approach before writing code.
- All contributions are subject to the project [LICENSE](LICENSE).

---

## What We Welcome

- Bug fixes and reliability improvements
- Performance improvements
- Documentation fixes and improvements
- UI/UX polish and accessibility improvements
- TELA browser compatibility improvements
- dApp developer experience improvements (`telaHost` API, Studio, Serve)

## What Is Out of Scope (for now)

- **Messenger / MTP backend** — backend prototype is complete but product integration is intentionally deferred. Please do not open PRs that touch `messenger/` or `cmd/mtp-anchor/` without prior discussion.
- **Protocol-level changes to DERO/TELA** — those belong upstream in `deroproject/derohe` or `tela-developer/tela`.
- **Dependency version bumps** without an accompanying fix or feature rationale.

---

## Development Setup

### Prerequisites

- **Go** 1.24.0+
- **Wails v2 CLI:** `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Node.js** 18+

### Linux users

```bash
# Ubuntu/Debian
sudo apt install libgtk-3-dev libglib2.0-dev libwebkit2gtk-4.0-dev

# Fedora
sudo dnf install gtk3-devel glib2-devel webkit2gtk4.1-devel

# Arch Linux
sudo pacman -S gtk3 glib2 webkit2gtk
```

### Run in development mode

```bash
git clone https://github.com/DHEBP/HOLOGRAM.git
cd HOLOGRAM
cd frontend && npm install && cd ..
wails dev
```

### Build

```bash
# Full build (HOLOGRAM + derod + simulator)
make all

# HOLOGRAM only
wails build
```

### Run Go tests

```bash
go test ./...
```

---

## Workflow

1. **Fork** the repository and clone your fork.
2. **Branch from `dev`** — not `main`.
   ```bash
   git checkout dev
   git pull origin dev
   git checkout -b fix/your-description
   ```
3. **Make your changes** — keep commits focused and atomic.
4. **Test locally** — `wails dev` for runtime, `go test ./...` for unit tests, `wails build` for a clean compile check.
5. **Push and open a PR** targeting the `dev` branch.
6. **Respond to review feedback** — maintainers will review and may request changes.

### Branch naming

| Prefix | Use for |
|--------|---------|
| `feature/` | New functionality |
| `fix/` | Bug fixes |
| `docs/` | Documentation only |
| `refactor/` | Code restructuring without behaviour change |
| `chore/` | Build, CI, dependency hygiene |

### Commit messages

Use clear, imperative-mood subject lines:

```
fix: prevent gnomon sync from blocking UI on startup
feat: add telaHost.getTransaction() method
docs: correct wails dev prerequisites for Arch Linux
```

---

## Code Guidelines

### Go

- Run `go fmt ./...` before committing.
- Run `go vet ./...` — fix any warnings before opening a PR.
- Add doc comments to all exported functions.
- Handle errors explicitly — do not silently swallow them.

### Svelte / Frontend

- Keep components focused and reusable.
- Match existing naming and file structure conventions.
- Test UI changes across multiple states (loading, error, empty, populated).
- Run `npm run build` to confirm the frontend compiles clean before pushing.

---

## Reporting Issues

Include the following when filing a bug:

- HOLOGRAM version (shown in app or `make --version`)
- Operating system and version
- Steps to reproduce
- Expected vs actual behaviour
- Relevant logs (found in the app or console output)

---

## Security Issues

**Do not report security vulnerabilities as public GitHub issues.**  
See [SECURITY.md](SECURITY.md) for the responsible disclosure process.

---

## Questions

- Open a [GitHub Discussion](https://github.com/DHEBP/HOLOGRAM/discussions) for general questions.
- Join the [DERO Discord](https://discord.gg/H95TJDp) community.

---

By contributing, you agree that your contributions will be licensed under the project's [MIT License](LICENSE).
