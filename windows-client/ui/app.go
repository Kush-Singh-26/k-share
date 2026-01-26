package ui

import (
	"fmt"
	"k-share-client/api"
	"k-share-client/config"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type App struct {
	fyneApp   fyne.App
	window    fyne.Window
	apiClient *api.Client
	wsClient  *api.WSClient

	// UI bindings
	serverIP       binding.String
	pairingCode    binding.String
	downloadFolder binding.String
	statusText     binding.String
	clipboardText  binding.String

	// UI components
	filesList   *widget.List
	historyList *widget.List
	connectBtn  *widget.Button

	// Data
	files        []api.FileInfo
	historyItems []api.HistoryItem

	// Theme tracking
	currentTheme string
}

func NewApp() *App {
	fyneApp := app.NewWithID("com.kshare.client")
	fyneApp.Settings().SetTheme(&forcedLightTheme{}) // Start with light theme
	fyneApp.SetIcon(resourceIconPng)                 // Set application icon

	a := &App{
		fyneApp:        fyneApp,
		serverIP:       binding.NewString(),
		pairingCode:    binding.NewString(),
		downloadFolder: binding.NewString(),
		statusText:     binding.NewString(),
		clipboardText:  binding.NewString(),
		files:          []api.FileInfo{},
		historyItems:   []api.HistoryItem{},
		currentTheme:   "light",
	}

	// Load config
	config.Load()
	a.serverIP.Set(config.Current.ServerIP)
	a.pairingCode.Set(config.Current.PairingCode)
	a.downloadFolder.Set(config.Current.DownloadFolder)
	a.statusText.Set("🔴 Disconnected")

	return a
}

func (a *App) Run() {
	a.window = a.fyneApp.NewWindow("K-Share Client")
	a.window.Resize(fyne.NewSize(800, 600))

	// Initialize API client
	a.apiClient = api.NewClient(config.Current.ServerIP, config.Current.PairingCode)
	a.wsClient = api.NewWSClient(config.Current.ServerIP)

	// Setup WebSocket callbacks
	a.wsClient.OnClipUpdate = func() {
		go func() {
			a.fetchClipboard()
		}()
	}
	a.wsClient.OnHistoryUpdate = func() {
		go func() {
			a.loadHistory()
		}()
	}
	a.wsClient.OnFilesUpdate = func() {
		go func() {
			a.loadFiles()
		}()
	}

	// Build UI
	content := a.buildUI()
	a.window.SetContent(content)

	a.window.ShowAndRun()
}

func (a *App) buildUI() fyne.CanvasObject {
	// Header with connection controls
	header := a.buildHeader()

	// Left side: Clipboard
	clipboardPanel := a.buildClipboardPanel()

	// Right side: Files
	filesPanel := a.buildFilesPanel()

	// Main content: clipboard (40%) | files (60%)
	mainContent := container.NewHSplit(clipboardPanel, filesPanel)
	mainContent.SetOffset(0.35) // 35% clipboard, 65% files

	return container.NewBorder(header, nil, nil, nil, mainContent)
}

func (a *App) buildHeader() fyne.CanvasObject {
	// Server IP input
	serverEntry := widget.NewEntryWithData(a.serverIP)
	serverEntry.SetPlaceHolder("192.168.1.100:9823")
	serverEntry.OnChanged = func(s string) {
		config.Current.ServerIP = s
		config.Save()
		a.apiClient.SetServerIP(s)
	}

	// Pairing code input
	codeEntry := widget.NewPasswordEntry()
	codeEntry.Bind(a.pairingCode)
	codeEntry.SetPlaceHolder("Pairing Code")
	codeEntry.OnChanged = func(s string) {
		config.Current.PairingCode = s
		config.Save()
		a.apiClient.SetPairingCode(s)
	}

	// Connect button
	a.connectBtn = widget.NewButton("Connect", func() {
		// If IP is empty, try auto-discover
		ip, _ := a.serverIP.Get()
		if ip == "" {
			a.autoDiscover()
		} else {
			a.connect()
		}
	})

	// Status label
	statusLabel := widget.NewLabelWithData(a.statusText)

	// Download folder selector
	folderLabel := widget.NewLabelWithData(a.downloadFolder)
	folderLabel.Wrapping = fyne.TextTruncate

	selectFolderBtn := widget.NewButton("📂 Select Download Folder", func() {
		a.selectDownloadFolder()
	})

	row1 := container.NewBorder(nil, nil,
		widget.NewLabel("Server:"),
		container.NewHBox(a.connectBtn, statusLabel),
		serverEntry,
	)

	row2 := container.NewBorder(nil, nil,
		widget.NewLabel("Code:"),
		selectFolderBtn,
		codeEntry,
	)

	row3 := container.NewBorder(nil, nil,
		widget.NewLabel("Downloads:"),
		nil,
		folderLabel,
	)

	// Theme toggle button
	themeToggle := widget.NewButton("🌓 Toggle Theme", func() {
		if a.currentTheme == "light" {
			a.fyneApp.Settings().SetTheme(&forcedDarkTheme{})
			a.currentTheme = "dark"
		} else {
			a.fyneApp.Settings().SetTheme(&forcedLightTheme{})
			a.currentTheme = "light"
		}
	})

	return container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(themeToggle, layout.NewSpacer()),
		row1,
		row2,
		row3,
		widget.NewSeparator(),
	)
}

func (a *App) buildClipboardPanel() fyne.CanvasObject {
	// Header
	titleLabel := widget.NewLabel("SHARED CLIPBOARD")
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Multiline text entry
	clipEntry := widget.NewMultiLineEntry()
	clipEntry.Bind(a.clipboardText)
	clipEntry.Wrapping = fyne.TextWrapWord

	// Buttons
	pushBtn := widget.NewButton("Push", func() {
		a.pushClipboard()
	})

	fetchBtn := widget.NewButton("Fetch", func() {
		a.fetchClipboard()
	})

	copyBtn := widget.NewButton("Copy", func() {
		a.copyToSystemClipboard()
	})

	historyBtn := widget.NewButton("History", func() {
		a.showHistoryPopup()
	})

	buttons := container.NewHBox(pushBtn, fetchBtn, copyBtn, historyBtn)

	header := container.NewBorder(nil, nil, titleLabel, buttons)

	return container.NewBorder(
		header,
		nil,
		nil,
		nil,
		container.NewScroll(clipEntry),
	)
}

func (a *App) buildFilesPanel() fyne.CanvasObject {
	// Upload buttons
	uploadFileBtn := widget.NewButton("📄 Upload File", func() {
		a.uploadFile()
	})

	uploadFolderBtn := widget.NewButton("📁 Upload Folder", func() {
		a.uploadFolder()
	})

	uploadBtns := container.NewHBox(uploadFileBtn, uploadFolderBtn)

	// Files list from server's from_phone folder
	a.filesList = widget.NewList(
		func() int {
			return len(a.files)
		},
		func() fyne.CanvasObject {
			thumb := canvas.NewImageFromImage(nil)
			thumb.SetMinSize(fyne.NewSize(48, 48))
			thumb.FillMode = canvas.ImageFillContain
			return container.NewHBox(
				thumb,
				container.NewVBox(
					widget.NewLabel(""), // File name with icon
					widget.NewLabel(""), // Size • Timestamp
				),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			file := a.files[id]
			box := obj.(*fyne.Container)

			thumbImg := box.Objects[0].(*canvas.Image)
			infoBox := box.Objects[1].(*fyne.Container)
			nameLabel := infoBox.Objects[0].(*widget.Label)
			infoLabel := infoBox.Objects[1].(*widget.Label)

			// Load thumbnail for images, show icon for others
			if isImageFile(file.Name) && !file.IsDirectory {
				// Register ownership of this widget to this file
				a.setThumbnailTarget(thumbImg, file.Name)
				// Clear old image immediately to prevent showing wrong image while loading
				thumbImg.Image = nil
				thumbImg.Refresh()

				// Start async load
				go a.loadThumbnail(file.Name, thumbImg)
			} else {
				// Not an image, register as empty target
				a.setThumbnailTarget(thumbImg, "")
				thumbImg.Image = nil
				thumbImg.Refresh()
			}

			// Get appropriate icon based on file type
			icon := getFileIcon(file.Name, file.IsDirectory)

			// First line: icon + name
			nameLabel.SetText(icon + " " + file.Name)
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}

			// Second line: size • timestamp (like Android)
			if file.IsDirectory {
				infoLabel.SetText("Folder • " + file.ModTime)
			} else {
				infoLabel.SetText(formatSize(file.Size) + " • " + file.ModTime)
			}
		},
	)

	a.filesList.OnSelected = func(id widget.ListItemID) {
		a.downloadFile(a.files[id])
		a.filesList.UnselectAll()
	}

	refreshBtn := widget.NewButton("🔄 Refresh", func() {
		a.loadFiles()
	})

	header := container.NewVBox(
		widget.NewLabel("📤 Upload to Phone"),
		uploadBtns,
		widget.NewSeparator(),
		widget.NewLabel("📥 Download from Phone (click to download)"),
		refreshBtn,
	)

	return container.NewBorder(
		header,
		nil,
		nil,
		nil,
		a.filesList,
	)
}

func (a *App) showHistoryPopup() {
	// Create a new popup window for history
	historyWindow := a.fyneApp.NewWindow("Clipboard History")
	historyWindow.Resize(fyne.NewSize(500, 400))

	contentVBox := container.NewVBox()

	for _, item := range a.historyItems {
		// Capture item for closure
		currentItem := item

		textLabel := widget.NewLabel(currentItem.Text)
		textLabel.Wrapping = fyne.TextWrapWord

		timeLabel := widget.NewLabel(currentItem.Timestamp.Format("2006-01-02 15:04:05"))
		timeLabel.TextStyle = fyne.TextStyle{Italic: true}

		deleteBtn := widget.NewButton("🗑️", func() {
			a.deleteHistoryItem(currentItem.ID)
			historyWindow.Close() // Close to refresh
			a.showHistoryPopup()  // Re-open to refresh list
		})

		restoreBtn := widget.NewButton("📋", func() {
			a.clipboardText.Set(currentItem.Text)
			a.statusText.Set("📋 Restored from history")
			historyWindow.Close()
		})

		rightButtons := container.NewHBox(restoreBtn, deleteBtn)

		row := container.NewBorder(
			nil,          // Top
			timeLabel,    // Bottom
			nil,          // Left
			rightButtons, // Right
			textLabel,    // Center
		)

		contentVBox.Add(row)
		contentVBox.Add(widget.NewSeparator())
	}

	scroll := container.NewVScroll(contentVBox)

	// Close button
	closeBtn := widget.NewButton("Close", func() {
		historyWindow.Close()
	})

	refreshBtn := widget.NewButton("🔄 Refresh", func() {
		historyWindow.Close()
		a.loadHistory()
		// Re-open to refresh (hacky but functional for now)
		a.showHistoryPopup()
	})

	header := container.NewHBox(refreshBtn, layout.NewSpacer(), closeBtn)

	content := container.NewBorder(header, nil, nil, nil, scroll)
	historyWindow.SetContent(content)
	historyWindow.Show()
}

func formatSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGT"[exp])
}
