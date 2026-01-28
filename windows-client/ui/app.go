package ui

import (
	"fmt"
	"k-share-client/api"
	"k-share-client/config"
	"os/exec"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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
	historyBtn  *widget.Button

	// Data
	filesBinding binding.UntypedList
	historyItems []api.HistoryItem

	// Theme tracking
	currentTheme string

	// Clipboard Channel
	clipboardChannel string

	// Guest Mode UI
	isGuest           bool
	clipChannelSelect *widget.Select
	clipGuestLabel    *widget.Label

	// Connection state
	isConnecting bool
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
		filesBinding:   binding.NewUntypedList(),
		historyItems:   []api.HistoryItem{},
		currentTheme:   "light",
	}

	// Load config
	config.Load()
	a.serverIP.Set(config.Current.ServerIP)
	a.pairingCode.Set(config.Current.PairingCode)
	a.downloadFolder.Set(config.Current.DownloadFolder)
	a.statusText.Set("🔴 Disconnected")
	a.clipboardChannel = "" // Default to main
	a.isGuest = false

	return a
}

func (a *App) Run() {
	a.window = a.fyneApp.NewWindow("K-Share Client")
	a.window.Resize(fyne.NewSize(800, 600))

	// Initialize API client
	a.apiClient = api.NewClient(config.Current.ServerIP, config.Current.PairingCode)
	a.wsClient = api.NewWSClient(config.Current.ServerIP, config.Current.PairingCode)

	// Setup WebSocket callbacks
	a.wsClient.OnClipUpdate = func() {
		if a.clipboardChannel == "" {
			go a.fetchClipboard()
		}
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

	// Dedicated stable Status Bar at the bottom
	statusLabel := widget.NewLabelWithData(a.statusText)
	statusLabel.Alignment = fyne.TextAlignCenter
	statusLabel.TextStyle = fyne.TextStyle{Italic: true}

	statusBar := container.NewVBox(
		widget.NewSeparator(),
		statusLabel,
	)

	return container.NewBorder(header, statusBar, nil, nil, mainContent)
}

func (a *App) buildHeader() fyne.CanvasObject {
	// Server IP input
	serverEntry := widget.NewEntryWithData(a.serverIP)
	serverEntry.SetPlaceHolder("IP:Port (e.g. 192.168.1.10:26260)")
	serverEntry.OnChanged = func(s string) {
		config.Current.ServerIP = s
		config.Save()
		a.apiClient.SetServerIP(s)
	}

	// Pairing code input
	codeEntry := widget.NewPasswordEntry()
	codeEntry.Bind(a.pairingCode)
	codeEntry.SetPlaceHolder("Auth Code (Admin/Guest)")
	codeEntry.OnChanged = func(s string) {
		config.Current.PairingCode = s
		config.Save()
		a.apiClient.SetAuthCode(s)
	}

	// Connect button
	a.connectBtn = widget.NewButtonWithIcon("Connect", resourceIconPng, func() {
		a.connect()
	})
	a.connectBtn.Importance = widget.HighImportance

	// Discover button
	discoverBtn := widget.NewButton("🔍 Discover", func() {
		a.autoDiscover()
	})

	// Download folder selector
	folderLabel := widget.NewLabelWithData(a.downloadFolder)
	folderLabel.Wrapping = fyne.TextTruncate

	selectFolderBtn := widget.NewButton("📂 Folder", func() {
		a.selectDownloadFolder()
	})

	openFolderBtn := widget.NewButton("Open", func() {
		folder, _ := a.downloadFolder.Get()
		if folder != "" {
			exec.Command("explorer", folder).Start()
		}
	})

	// Theme toggle button
	themeToggle := widget.NewButton("🌓", func() {
		if a.currentTheme == "light" {
			a.fyneApp.Settings().SetTheme(&forcedDarkTheme{})
			a.currentTheme = "dark"
		} else {
			a.fyneApp.Settings().SetTheme(&forcedLightTheme{})
			a.currentTheme = "light"
		}
	})

	// Settings/Saved Networks button
	settingsBtn := widget.NewButton("⚙️", func() {
		a.showSettingsPopup()
	})

	// STABLE LAYOUT CONSTRUCTION
	// Theme toggle and Settings on the far right
	topRow := container.NewHBox(layout.NewSpacer(), settingsBtn, themeToggle)

	row1 := container.New(layout.NewFormLayout(),
		widget.NewLabel("Server:"),
		container.NewBorder(nil, nil, nil, container.NewHBox(discoverBtn, a.connectBtn), serverEntry),
	)

	row2 := container.New(layout.NewFormLayout(),
		widget.NewLabel("Code:"),
		codeEntry,
	)

	row3 := container.New(layout.NewFormLayout(),
		widget.NewLabel("Save to:"),
		container.NewBorder(nil, nil, nil, container.NewHBox(selectFolderBtn, openFolderBtn), folderLabel),
	)

	return container.NewVBox(
		widget.NewSeparator(),
		topRow,
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

	// Channel Toggle (Admin)
	a.clipChannelSelect = widget.NewSelect([]string{"My Clipboard", "Guest Clipboard"}, func(s string) {
		if s == "Guest Clipboard" {
			a.clipboardChannel = "guest"
		} else {
			a.clipboardChannel = ""
		}
		go a.fetchClipboard()
	})
	a.clipChannelSelect.SetSelected("My Clipboard")

	// Guest Label (Guest)
	a.clipGuestLabel = widget.NewLabel("Guest Mode (Shared)")
	a.clipGuestLabel.TextStyle = fyne.TextStyle{Italic: true}
	a.clipGuestLabel.Hide() // Hidden by default (assume admin until connect)

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

	a.historyBtn = widget.NewButton("History", func() {
		a.showHistoryPopup()
	})

	buttons := container.NewHBox(pushBtn, fetchBtn, copyBtn, a.historyBtn)

	header := container.NewVBox(
		container.NewBorder(nil, nil, titleLabel, container.NewStack(a.clipChannelSelect, a.clipGuestLabel)),
		buttons,
	)

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

	pasteBtn := widget.NewButton("📋 Paste & Send", func() {
		a.pasteAndSend()
	})
	pasteBtn.Importance = widget.HighImportance

	uploadBtns := container.NewHBox(uploadFileBtn, uploadFolderBtn, pasteBtn)

	// Files list from server's from_phone folder
	a.filesList = widget.NewListWithData(
		a.filesBinding,
		func() fyne.CanvasObject {
			thumb := canvas.NewImageFromImage(nil)
			thumb.SetMinSize(fyne.NewSize(48, 48))
			thumb.FillMode = canvas.ImageFillContain
			return container.NewBorder(
				nil, nil,
				thumb,
				widget.NewButton("🗑️", func() {}), // Delete button (Right)
				container.NewVBox(
					widget.NewLabel(""), // File name
					widget.NewLabel(""), // Info
				),
			)
		},
		func(i binding.DataItem, obj fyne.CanvasObject) {
			itemVal, _ := i.(binding.Untyped).Get()
			file := itemVal.(api.FileInfo)

			box := obj.(*fyne.Container) // Border container

			var thumbImg *canvas.Image
			var delBtn *widget.Button
			var infoBox *fyne.Container

			// Robustly find objects by type (NewBorder order can vary)
			for _, o := range box.Objects {
				switch obj := o.(type) {
				case *canvas.Image:
					thumbImg = obj
				case *widget.Button:
					delBtn = obj
				case *fyne.Container:
					infoBox = obj
				}
			}

			if thumbImg == nil || delBtn == nil || infoBox == nil {
				return // Should not happen
			}

			nameLabel := infoBox.Objects[0].(*widget.Label)
			infoLabel := infoBox.Objects[1].(*widget.Label)

			// Configure delete button
			delBtn.OnTapped = func() {
				a.deleteFile(file.Name)
			}
			if a.isGuest {
				delBtn.Hide()
			} else {
				delBtn.Show()
			}

			// Handle "Guest" prefix
			displayName := file.Name
			guestLabel := ""
			if strings.HasPrefix(displayName, "Public/") {
				displayName = strings.TrimPrefix(displayName, "Public/")
				guestLabel = " [Guest]"
			}

			// Load thumbnail for images, show icon for others
			// We must use the REAL file name (with Public/) for thumbnails
			if isImageFile(displayName) && !file.IsDirectory {
				a.setThumbnailTarget(thumbImg, file.Name) // Use full name key
				thumbImg.Image = nil
				thumbImg.Resource = theme.FileImageIcon() // Placeholder until loaded
				thumbImg.Refresh()
				go a.loadThumbnail(file.Name, thumbImg) // Use full name for fetch
			} else {
				a.setThumbnailTarget(thumbImg, "")
				thumbImg.Image = nil
				if file.IsDirectory {
					thumbImg.Resource = theme.FolderIcon()
				} else {
					thumbImg.Resource = getFileResource(displayName)
				}
				thumbImg.Refresh()
			}

			// Get appropriate icon based on file type
			icon := getFileIcon(displayName, file.IsDirectory)

			// First line: icon + name + tag
			nameLabel.SetText(icon + " " + displayName + guestLabel)
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
		itemVal, err := a.filesBinding.GetValue(id)
		if err == nil {
			file := itemVal.(api.FileInfo)
			a.downloadFile(file)
		}
		a.filesList.UnselectAll()
	}

	refreshBtn := widget.NewButton("🔄 Refresh", func() {
		a.loadFiles()
	})

	header := container.NewVBox(
		widget.NewLabel("📤 Upload to server"),
		uploadBtns,
		widget.NewSeparator(),
		widget.NewLabel("📥 Download from server (click to download)"),
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

func (a *App) showSettingsPopup() {
	settingsWindow := a.fyneApp.NewWindow("Saved Networks")
	settingsWindow.Resize(fyne.NewSize(400, 300))

	content := container.NewVBox()
	content.Add(widget.NewLabelWithStyle("Saved Server IPs per Subnet", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	content.Add(widget.NewSeparator())

	if len(config.Current.SavedNetworks) == 0 {
		content.Add(widget.NewLabel("No saved networks yet."))
	}

	for subnet, ip := range config.Current.SavedNetworks {
		s, i := subnet, ip // capture
		row := container.NewBorder(nil, nil, nil,
			widget.NewButton("Delete", func() {
				delete(config.Current.SavedNetworks, s)
				config.Save()
				settingsWindow.Close()
				a.showSettingsPopup()
			}),
			container.NewVBox(
				widget.NewLabel("Subnet: "+s),
				widget.NewLabel("Last IP: "+i),
			),
		)
		content.Add(row)
		content.Add(widget.NewSeparator())
	}

	scroll := container.NewVScroll(content)
	closeBtn := widget.NewButton("Close", func() { settingsWindow.Close() })

	settingsWindow.SetContent(container.NewBorder(nil, closeBtn, nil, nil, scroll))
	settingsWindow.Show()
}

func getFileResource(filename string) fyne.Resource {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".pdf") {
		return theme.FileTextIcon()
	}
	if strings.HasSuffix(lower, ".mp3") || strings.HasSuffix(lower, ".wav") || strings.HasSuffix(lower, ".flac") {
		return theme.FileAudioIcon()
	}
	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".avi") || strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".mov") {
		return theme.FileVideoIcon()
	}
	if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".rar") || strings.HasSuffix(lower, ".7z") || strings.HasSuffix(lower, ".tar") {
		return theme.StorageIcon()
	}
	return theme.FileIcon()
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
