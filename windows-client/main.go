package main

import (
	"k-share-client/config"
	"k-share-client/ui"
	"log"
)

func main() {
	// Load configuration
	if err := config.Load(); err != nil {
		log.Printf("Failed to load config: %v", err)
	}

	// Create and run app
	app := ui.NewApp()
	app.Run()
}
