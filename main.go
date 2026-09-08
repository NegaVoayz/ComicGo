package main

import (
	"embed"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Covers/pages are served from a loopback HTTP server while the app origin
	// is wails://. WebView2 treats http from an https-ish origin as mixed
	// content; --allow-insecure-localhost exempts loopback. This must be set
	// before the webview is created.
	existing := os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS")
	if !strings.Contains(existing, "allow-insecure-localhost") {
		merged := strings.TrimSpace(existing + " --allow-insecure-localhost")
		_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", merged)
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "ComicGo",
		Width:     1200,
		Height:    820,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Enables Wails' native file-drop handling so dragged files/folders
		// arrive as absolute paths instead of navigating the webview; the
		// frontend registers OnFileDrop to import them.
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		BackgroundColour: &options.RGBA{R: 24, G: 26, B: 32, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
