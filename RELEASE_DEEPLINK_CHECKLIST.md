# Deep-Link Verification Checklist (`dero://`)

Use this checklist for release sign-off after building installers/packages from `dev`.

## Test Data

- Valid dURL: `dero://vault.tela`
- Valid SCID: `dero://0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`
- Invalid URL: `dero://` (empty target)
- Mixed-case scheme: `DeRo://vault.tela`

## Global Pass Criteria

- OS launches HOLOGRAM when a `dero://` link is opened externally.
- If HOLOGRAM is closed, it opens and routes to Browser with the target prefilled/navigated.
- If HOLOGRAM is already running, opening a `dero://` link does not crash/freeze.
- Invalid deep links fail gracefully (error/empty state, no crash).

## macOS

- Install `.app` (or packaged installer build) and move to `/Applications`.
- Run from Terminal once:
  - `open "dero://vault.tela"`
- Confirm HOLOGRAM launches and navigates in Browser.
- Run:
  - `open "DeRo://vault.tela"`
  - `open "dero://"`
- Confirm mixed-case works and invalid deep link is handled safely.

## Windows

- Install HOLOGRAM via installer.
- Open `Win + R` and run:
  - `dero://vault.tela`
- Confirm HOLOGRAM launches and navigates in Browser.
- Also test from browser address bar/link click if available.
- Repeat with:
  - `DeRo://vault.tela`
  - `dero://`
- Confirm no crash and graceful handling on invalid link.

## Linux

- Install package for target distro and confirm desktop integration.
- From terminal:
  - `xdg-open "dero://vault.tela"`
- Confirm HOLOGRAM launches and navigates in Browser.
- Repeat with:
  - `xdg-open "DeRo://vault.tela"`
  - `xdg-open "dero://"`
- Confirm invalid link does not crash.

## Regression Checks (All OS)

- After deep-link launch, favorites heart and Browser navigation still work.
- TELA app load path still works in both HTTP server mode and srcdoc fallback.
- Wallet/Explorer tabs remain functional after opening deep links.

## Sign-Off Template

- macOS: PASS / FAIL — Notes:
- Windows: PASS / FAIL — Notes:
- Linux: PASS / FAIL — Notes:
- Overall release deep-link status: PASS / FAIL
