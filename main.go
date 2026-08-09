package main

import (
	"embed"
	"os"

	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/walletapi"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	originalArgs := append([]string(nil), os.Args...)

	// Capture launch args (e.g., dero:// links) before clearing Wails/CLI flags.
	launchArgs := []string{}
	if len(originalArgs) > 1 {
		launchArgs = append(launchArgs, originalArgs[1:]...)
	}

	// Clear args to prevent DERO globals from picking up Wails flags
	os.Args = []string{os.Args[0]}

	// Initialize DERO globals for mainnet
	globals.Arguments = make(map[string]interface{})
	globals.Arguments["--testnet"] = false  // Required by DERO library, but testnet is not used in Hologram
	globals.Arguments["--simulator"] = false
	// A4: pin DERO's data directory to the canonical HOLOGRAM dir BEFORE Initialize()
	// (and before any goroutine). globals.Initialize()'s one-shot MkdirAll then creates
	// its network stub under ~/.dero/hologram instead of the working dir. walletapi opens
	// wallets by absolute path and never reads GetDataDirectory(), so this key is inert
	// in-process — it only stops the empty ~/mainnet | ~/testnet litter in $HOME.
	globals.Arguments["--data-dir"] = getHologramDataDir()
	globals.Initialize()
	globals.InitNetwork() // This sets up the correct address prefixes for mainnet

	// Initialize wallet lookup table (required for crypto operations)
	go walletapi.Initialize_LookupTable(1, 1<<21)

	// Create an instance of the app structure
	app := NewApp()
	app.captureLaunchURLFromArgs(launchArgs)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "HOLOGRAM - DERO Decentralized Web",
		Width:  1400,
		Height: 900,
		// A3: graviton has no file lock, so two instances sharing the now-canonical
		// datashards dir could corrupt it. This single-instance lock is the mandatory
		// backstop — a second launch is redirected to the running instance instead of
		// opening a second unlocked store handle.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.hologram.dero.messenger",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				if app.ctx != nil {
					wailsRuntime.WindowUnminimise(app.ctx)
					wailsRuntime.Show(app.ctx)
				}
			},
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 12, G: 12, B: 20, A: 1}, // --void-base: #0c0c14
		OnStartup:        app.startup,
		OnShutdown:        app.shutdown,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
			// ⚠️ On Linux this suppresses the NATIVE file-drop channel that EnableFileDrop
			// above is asking for. SetupWebview (Wails v2.11.0,
			// internal/frontend/desktop/linux/window.c:571) calls gtk_drag_dest_unset(webview)
			// when this is set, so the widget stops being a GTK drop destination — and then
			// connects "drag-drop" and "drag-data-received" to it anyway. Those signals only
			// fire on a widget that IS a drop destination, and Wails never calls
			// gtk_drag_dest_set() to restore it.
			//
			// ⚠️ IT MUST STAY FALSE, and it is HALF of a pair. The other half is the file-drop
			// guard in the XSWD bridge (server_manager.go), which cancels dragover inside the
			// TELA frame. Measured 2026-08-09, both halves are load-bearing:
			//
			//   true            → no drag events reach any document; a dropped file is ignored,
			//                     and OnFileDrop never fires, so nothing can deliver it.
			//   false, no guard → an uncancelled dragover means WebKit never dispatches drop at
			//                     all; it performs its default instead and NAVIGATES the whole
			//                     webview to the file, with no way back.
			//   false + guard   → drop is dispatched in the frame and cancelled, and OnFileDrop
			//                     delivers the real path to Browser.svelte.
			//
			// The DOM alone is not enough on Linux and cannot become enough: the drop event
			// fires but arrives EMPTY (types=text/uri-list,text/html, files.length 0, empty
			// text/uri-list). The native channel is the only source of the bytes.
			DisableWebViewDrop: false,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: false, // Standard macOS title bar for proper window dragging
			},
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
