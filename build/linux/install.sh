#!/usr/bin/env bash
# HOLOGRAM — user-level installer (no root needed).
#
# A bare binary does not launch on double-click on most Linux desktops, and does not
# appear in the application menu. This registers a .desktop launcher + icon for the
# current user so HOLOGRAM behaves like a normal app. The WebKitGTK DMA-BUF render fix
# is compiled into the binary itself, so no launcher env tricks are needed.
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA="${XDG_DATA_HOME:-$HOME/.local/share}"
BIN="$HOME/.local/bin"
APPS="$DATA/applications"
ICONS="$DATA/icons"

mkdir -p "$BIN" "$APPS" "$ICONS"

install -m 0755 "$SRC/Hologram" "$BIN/hologram"
install -m 0644 "$SRC/appicon.png" "$ICONS/hologram.png"

cat > "$APPS/hologram.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=HOLOGRAM
GenericName=DERO Wallet & TELA Browser
Comment=DERO wallet, TELA dApp browser, and node manager
Exec=$BIN/hologram
Icon=$ICONS/hologram.png
Terminal=false
Categories=Network;Finance;
StartupWMClass=Hologram
Keywords=DERO;wallet;TELA;crypto;blockchain;
EOF
chmod 0644 "$APPS/hologram.desktop"

update-desktop-database "$APPS" 2>/dev/null || true

echo "HOLOGRAM installed for your user."
echo "  - Launch it from your application menu (search 'HOLOGRAM')."
echo "  - Or run:  $BIN/hologram"
echo "  - Uninstall:  ./uninstall.sh"
if ! printf '%s' ":$PATH:" | grep -q ":$BIN:"; then
  echo "  Note: $BIN is not on your PATH — use the app menu, or add it to PATH."
fi
