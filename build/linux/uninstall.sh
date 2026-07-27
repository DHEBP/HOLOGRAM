#!/usr/bin/env bash
# HOLOGRAM — remove the user-level install created by install.sh.
# Wallet data in ~/.dero is NOT touched.
set -euo pipefail

DATA="${XDG_DATA_HOME:-$HOME/.local/share}"
rm -f "$HOME/.local/bin/hologram" \
      "$DATA/applications/hologram.desktop" \
      "$DATA/icons/hologram.png"
update-desktop-database "$DATA/applications" 2>/dev/null || true
echo "HOLOGRAM uninstalled. Your wallet data in ~/.dero was left untouched."
