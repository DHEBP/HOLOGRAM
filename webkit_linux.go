//go:build linux

package main

import "os"

// init disables WebKitGTK's DMA-BUF renderer on Linux before the WebView starts.
//
// WebKitGTK's DMA-BUF GPU renderer silently fails to initialize on many Linux setups
// -- notably NVIDIA proprietary drivers and some Wayland compositors -- and draws a
// blank, frozen window instead of the app. This is the "HOLOGRAM doesn't respond on
// Fedora" report: the process is running, the window just renders nothing. Disabling
// the DMA-BUF path makes it fall back to a renderer that works, with no loss of WebKit
// functionality (hardware acceleration is not reported as disabled).
//
// Runs from an init() in a linux-only file so it takes effect before main() calls
// wails.Run() and before the WebView is created. An explicitly set value is respected
// so a user can override. Ref: wailsapp/wails#4985.
func init() {
	if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}
}
