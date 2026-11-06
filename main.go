package main

import (
	"comicgo/internal/application"
	"comicgo/internal/ui"
	_ "comicgo/internal/ui/screens"
)

func main() {
	ui.Run(application.NewApp())
}
