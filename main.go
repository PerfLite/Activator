package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[string]("cmd-output")
}

func main() {
	activatorService := NewActivatorService()

	app := application.New(application.Options{
		Name:        "Windows Activator",
		Description: "GUI Activator for Windows based on slmgr and KMS",
		Services: []application.Service{
			application.NewService(activatorService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Windows: application.WindowsOptions{
			WndClass: "WindowsActivatorWindow",
		},
	})

	activatorService.SetApp(app)

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Windows Activator (Win 10/11)",
		Width:            860,
		Height:           530,
		MinWidth:         760,
		MinHeight:        460,
		BackgroundColour: application.NewRGB(15, 17, 26),
		URL:              "/",
		Windows: application.WindowsWindow{
			Theme: application.Dark,
		},
	})

	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
