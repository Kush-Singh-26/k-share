package main

import (
	"k-share-client/config"
	"k-share-client/ui"
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

	// Create and run app
	app := ui.NewApp()
	app.Run()
}
