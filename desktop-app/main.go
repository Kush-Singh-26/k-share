//go:build !tui

package main

import (
	"desktop-app/config"
	"desktop-app/ui"
	"log"

	"golang.design/x/clipboard"
)

func main() {
	// Initialize clipboard for image support
	if err := clipboard.Init(); err != nil {
		log.Printf("Warning: Clipboard init failed: %v", err)
	}

	// Load configuration
	if err := config.Load(); err != nil {
		log.Printf("Failed to load config: %v", err)
	}

	// Launch Graphical UI (Default)
	app := ui.NewApp()
	app.Run()
}
