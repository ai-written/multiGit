package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed frontend
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "GitDesk",
		Width:            1100,
		Height:           680,
		MinWidth:         900,
		MinHeight:        520,
		Frameless:        true,
		CSSDragProperty:  "--wails-draggable",
		CSSDragValue:     "drag",
		Assets:           assets,
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		BackgroundColour: &options.RGBA{R: 13, G: 17, B: 23, A: 255},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
		os.Exit(1)
	}
}
