//go:build tui

package main

import (
	"desktop-app/config"
	"desktop-app/tui"
	"os"

	"golang.design/x/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			// Ensure TUI is exited if possible, though panic usually means it's gone
			print("Critical panic in TUI: ", r, "\n")
		}
	}()
	// Initialize clipboard for image support
	if err := clipboard.Init(); err != nil {
		// Silent fail - don't print to terminal during TUI
	}

	// Load configuration
	if err := config.Load(); err != nil {
		// Silent fail - don't print to terminal during TUI
	}

	// Setup file logging BEFORE starting TUI to prevent terminal corruption
	// Bubble Tea owns the terminal; stray stdout/stderr causes display corruption
	logFile, err := tea.LogToFile("k-share.log", "tui")
	if err == nil {
		defer logFile.Close()
	}

	// Launch Terminal UI
	app := tui.NewApp()
	if err := app.Run(); err != nil {
		// Output only on actual fatal exit, after TUI has released terminal
		if file := os.Stderr; file != nil {
			print("Error running TUI: " + err.Error() + "\n")
		}
		os.Exit(1)
	}
}
