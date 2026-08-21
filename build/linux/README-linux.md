# HOLOGRAM for Linux

## Install (recommended)
Open a terminal in this folder and run:

    ./install.sh

This registers HOLOGRAM in your application menu with an icon and a working
double-click launcher — no root needed. Then open it from your menu.

Remove it any time with `./uninstall.sh` (your wallet data in `~/.dero` is left alone).

## Or run it directly

    ./Hologram

## Requirements
HOLOGRAM needs **GTK 3** and **WebKitGTK 4.1**, which ship on all current distros.
If yours doesn't have them:

    # Fedora
    sudo dnf install gtk3 webkit2gtk4.1
    # Debian / Ubuntu
    sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0

## Blank or frozen window?
The binary already disables WebKitGTK's DMA-BUF renderer, which fixes the blank-window
hang seen on many NVIDIA / Wayland setups. If a blank window still appears, try:

    WEBKIT_DISABLE_COMPOSITING_MODE=1 ./Hologram

and let us know your GPU and desktop environment.
