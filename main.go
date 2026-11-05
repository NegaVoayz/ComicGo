package main

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"
)

func main() {
	go func() {
		ui := newAppUI(NewApp())
		err := ui.run()
		if err != nil {
			log.Printf("ComicGo stopped: %v", err)
		}
		os.Exit(0)
	}()

	app.Main()
}

func windowOptions(window *app.Window) {
	window.Option(
		app.Title("ComicGo"),
		app.Size(unit.Dp(1200), unit.Dp(820)),
		app.MinSize(unit.Dp(900), unit.Dp(600)),
		app.Decorated(false),
	)
}
