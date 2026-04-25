package ui

import (
	"context"
	"fmt"
	"time"
	"desktop-app/api"
	"desktop-app/clipboardops"
	"desktop-app/config"
	"desktop-app/fileops"
	"desktop-app/historyops"
	"desktop-app/platform"
	"desktop-app/presentation"
	"desktop-app/session"
	"desktop-app/thumbnails"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.design/x/clipboard"
)

type App struct {
	fyneApp    fyne.App
	window     fyne.Window
	apiClient  *api.Client
	wsClient   *api.WSClient
	session    *session.Manager
	fileOps    *fileops.Service
	clipOps    *clipboardops.Service
	historyOps *historyops.Service
	thumbs     *thumbnails.Service

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
	backBtn     *widget.Button

	// Data
	filesBinding    binding.UntypedList
	historyItems    []api.HistoryItem
	currentPath     string
	breadcrumbPath  binding.String

	// Theme tracking
	currentTheme string

	// Clipboard Channel
	clipboardChannel string

	// Guest Mode UI
	isGuest           bool
	clipChannelSelect *widget.Select
	clipGuestLabel    *widget.Label

	// Connection state
	isConnecting int32
	isConnected  int32

	// History Popup state
	historyWindow fyne.Window
	historyVBox   *fyne.Container

	// Image clipboard sync
	lastImageHash string
}

func NewApp() *App {
	fyneApp := app.NewWithID("com.kshare.desktop")
	fyneApp.Settings().SetTheme(&forcedLightTheme{}) // Start with light theme
	fyneApp.SetIcon(resourceIconPng)                 // Set application icon

	a := &App{
		fyneApp:        fyneApp,
		serverIP:       binding.NewString(),
		pairingCode:    binding.NewString(),
		downloadFolder: binding.NewString(),
		statusText:     binding.NewString(),
		clipboardText:  binding.NewString(),
		breadcrumbPath: binding.NewString(),
		filesBinding:   binding.NewUntypedList(),
		historyItems:   []api.HistoryItem{},
		currentPath:    "",
		currentTheme:   "light",
	}

	// Load config - Removed redundant config.Load() as it's done in main.go
	cfg := config.Get()
	a.serverIP.Set(cfg.ServerIP)
	a.pairingCode.Set(cfg.PairingCode)
	a.downloadFolder.Set(cfg.DownloadFolder)
	a.statusText.Set("🔴 Disconnected")
	a.breadcrumbPath.Set("📁 Root")
	a.clipboardChannel = "" // Default to main
	a.isGuest = false
	a.session = session.New(cfg.ServerIP, cfg.PairingCode)
	a.apiClient = a.session.APIClient()
	a.wsClient = a.session.WSClient()
	a.fileOps = fileops.New(a.apiClient, cfg.DownloadFolder)
	a.clipOps = clipboardops.New(a.apiClient)
	a.historyOps = historyops.New(a.apiClient)
	a.thumbs = thumbnails.New(a.apiClient)

	return a
}

func (a *App) setStatus(msg string, duration time.Duration) {
	a.statusText.Set(msg)
	if duration > 0 {
		time.AfterFunc(duration, func() {
			if current, _ := a.statusText.Get(); current == msg {
				if atomic.LoadInt32(&a.isConnected) == 1 {
					a.statusText.Set("🟢 Connected")
				} else {
					a.statusText.Set("🔴 Disconnected")
				}
			}
		})
	}
}

func (a *App) Run() {
	a.window = a.fyneApp.NewWindow("K-Share Client")
	a.window.Resize(fyne.NewSize(800, 600))

	// Redundant service creation removed to preserve initialization from NewApp()

	// Setup WebSocket callbacks
	a.wsClient.SetOnClipUpdate(func() {
		if a.clipboardChannel == "" {
			go a.fetchClipboard()
		}
	})
	a.wsClient.SetOnClipImageUpdate(func() {
		go a.fetchClipboardImage()
	})

	a.wsClient.SetOnHistoryUpdate(func() {
		go func() {
			a.loadHistory()
		}()
	})
	a.wsClient.SetOnFilesUpdate(func() {
		go func() {
			a.loadFiles()
		}()
	})

	// Start clipboard image watcher
	go func() {
		ch := clipboard.Watch(context.Background(), clipboard.FmtImage)
		for data := range ch {
			if len(data) == 0 || a.clipOps == nil {
				continue
			}

			hash := a.clipOps.SystemImageHash(data)
			if hash == a.lastImageHash {
				continue
			}

			a.lastImageHash = hash
			if !a.isGuest && atomic.LoadInt32(&a.isConnected) == 1 {
				a.setStatus("⏏ Pushing image to server...", 0)
				err := a.clipOps.PushImage(context.Background(), data)
				if err != nil {
					a.setStatus("🔴 Push image failed: "+err.Error(), 5*time.Second)
				} else {
					a.setStatus("✅ Image pushed to server", 3*time.Second)
				}
			}
		}
	}()

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
		_ = config.SetServerIP(s)
		a.apiClient.SetServerIP(s)
		a.session.SetServerIP(s)
	}

	// Pairing code input
	codeEntry := widget.NewPasswordEntry()
	codeEntry.Bind(a.pairingCode)
	codeEntry.SetPlaceHolder("Auth Code (Admin/Guest)")
	codeEntry.OnChanged = func(s string) {
		_ = config.SetPairingCode(s)
		a.apiClient.SetAuthCode(s)
		a.session.SetPairingCode(s)
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

	openFolderBtn := widget.NewButtonWithIcon("Open", theme.FolderOpenIcon(), func() {
		folder, _ := a.downloadFolder.Get()
		if folder != "" {
			_ = platform.OpenFolder(folder)
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
			row := &fileListRow{}
			row.thumb = canvas.NewImageFromImage(nil)
			row.thumb.SetMinSize(fyne.NewSize(48, 48))
			row.thumb.FillMode = canvas.ImageFillContain

			row.nameLabel = widget.NewLabel("")
			row.nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			row.infoLabel = widget.NewLabel("")

			row.delBtn = widget.NewButton("🗑️", func() {})
			row.dlBtn = widget.NewButton("📥", func() {})

			labels := container.NewVBox(row.nameLabel, row.infoLabel)
			buttons := container.NewHBox(row.dlBtn, row.delBtn)
			row.container = container.NewBorder(nil, nil, row.thumb, buttons, labels)

			return row.container
		},
		func(i binding.DataItem, obj fyne.CanvasObject) {
			itemVal, _ := i.(binding.Untyped).Get()
			file := itemVal.(api.FileInfo)

			border, ok := obj.(*fyne.Container)
			if !ok {
				return
			}

			// Manual child lookup (reliable across Fyne versions)
			var delBtn, dlBtn *widget.Button
			var thumbImg *canvas.Image
			var infoBox *fyne.Container

			for _, child := range border.Objects {
				switch v := child.(type) {
				case *fyne.Container:
					// Check if this is the buttons container or the infoBox
					if len(v.Objects) > 0 {
						if _, ok := v.Objects[0].(*widget.Button); ok {
							// Buttons container
							for _, btnObj := range v.Objects {
								if b, ok := btnObj.(*widget.Button); ok {
									if b.Text == "📥" {
										dlBtn = b
									} else if b.Text == "🗑️" {
										delBtn = b
									}
								}
							}
						} else {
							infoBox = v
						}
					}
				case *canvas.Image:
					thumbImg = v
				}
			}

			if delBtn == nil || thumbImg == nil || infoBox == nil || dlBtn == nil {
				return
			}

			var nameLabel, infoLabel *widget.Label
			for _, child := range infoBox.Objects {
				if l, ok := child.(*widget.Label); ok {
					if nameLabel == nil {
						nameLabel = l
					} else {
						infoLabel = l
						break
					}
				}
			}

			if nameLabel == nil || infoLabel == nil {
				return
			}

			// Configure buttons
			delBtn.OnTapped = func() {
				a.deleteFile(file.Name)
			}
			dlBtn.OnTapped = func() {
				a.forceDownload(file)
			}

			if a.isGuest {
				delBtn.Hide()
			} else {
				delBtn.Show()
			}

			uiRow := presentation.BuildFileRow(file, a.isGuest)

			// Load thumbnail for images, show icon for others.
			if uiRow.IsImage {
				a.thumbs.SetTarget(thumbImg, uiRow.ThumbnailKey)
				thumbImg.Image = nil
				thumbImg.Resource = theme.FileImageIcon() // Placeholder until loaded
				thumbImg.Refresh()
				go a.thumbs.Request(uiRow.ThumbnailKey, thumbImg)
			} else {
				a.thumbs.SetTarget(thumbImg, "")
				thumbImg.Image = nil
				if uiRow.IsDirectory {
					thumbImg.Resource = theme.FolderIcon()
				} else {
					thumbImg.Resource = getFileResource(uiRow.DisplayName)
				}
				thumbImg.Refresh()
			}

			// First line: icon + name + tag
			nameLabel.SetText(uiRow.Icon + " " + uiRow.DisplayName + uiRow.GuestLabel)
			// Second line: size • timestamp
			infoLabel.SetText(uiRow.Info)
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

	downloadDirBtn := widget.NewButton("📥 Download All", func() {
		a.forceDownload(api.FileInfo{
			Name:        a.currentPath,
			IsDirectory: true,
		})
	})
	
	// Search input
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("🔍 Search files...")
	searchEntry.OnChanged = func(query string) {
		go a.searchFiles(query)
	}

	breadcrumb := widget.NewLabelWithData(a.breadcrumbPath)
	breadcrumb.TextStyle = fyne.TextStyle{Bold: true, Italic: true}
	
	a.backBtn = widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		a.navigateUp()
	})
	a.backBtn.Disable() // Start disabled

	header := container.NewVBox(
		widget.NewLabel("📤 Upload to server"),
		uploadBtns,
		widget.NewSeparator(),
		widget.NewLabel("📥 Download from server (click to download)"),
		searchEntry,
		container.NewHBox(a.backBtn, breadcrumb, layout.NewSpacer(), downloadDirBtn, refreshBtn),
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
	if a.historyWindow != nil {
		a.historyWindow.RequestFocus()
		return
	}

	// Create a new popup window for history
	a.historyWindow = a.fyneApp.NewWindow("Clipboard History")
	a.historyWindow.Resize(fyne.NewSize(500, 400))
	a.historyWindow.CenterOnScreen()
	a.historyWindow.SetOnClosed(func() {
		a.historyWindow = nil
		a.historyVBox = nil
	})

	a.historyVBox = container.NewVBox()
	a.rebuildHistoryUI()

	scroll := container.NewVScroll(a.historyVBox)

	refreshBtn := widget.NewButton("🔄 Refresh", func() {
		go a.loadHistory()
	})

	closeBtn := widget.NewButton("Close", func() {
		a.historyWindow.Close()
	})

	header := container.NewHBox(refreshBtn, layout.NewSpacer(), closeBtn)
	a.historyWindow.SetContent(container.NewBorder(header, nil, nil, nil, scroll))
	a.historyWindow.Show()
}

func (a *App) rebuildHistoryUI() {
	if a.historyVBox == nil {
		return
	}

	a.historyVBox.Objects = nil
	for _, item := range a.historyItems {
		currentItem := item
		textLabel := widget.NewLabel(currentItem.Text)
		textLabel.Wrapping = fyne.TextWrapWord

		timeLabel := widget.NewLabel(presentation.HistoryTimestamp(currentItem.Timestamp))
		timeLabel.TextStyle = fyne.TextStyle{Italic: true}

		deleteBtn := widget.NewButton("🗑️", func() {
			a.deleteHistoryItem(currentItem.ID)
		})

		restoreBtn := widget.NewButton("📋", func() {
			a.clipboardText.Set(currentItem.Text)
			a.setStatus("📋 Restored from history", 3*time.Second)
			if a.historyWindow != nil {
				a.historyWindow.Close()
			}
		})

		rightButtons := container.NewHBox(restoreBtn, deleteBtn)
		row := container.NewBorder(nil, timeLabel, nil, rightButtons, textLabel)

		a.historyVBox.Add(row)
		a.historyVBox.Add(widget.NewSeparator())
	}
	a.historyVBox.Refresh()
}

type fileListRow struct {
	thumb     *canvas.Image
	delBtn    *widget.Button
	dlBtn     *widget.Button
	nameLabel *widget.Label
	infoLabel *widget.Label
	container fyne.CanvasObject
}

func (a *App) showSettingsPopup() {
	settingsWindow := a.fyneApp.NewWindow("Saved Networks")
	settingsWindow.Resize(fyne.NewSize(400, 300))

	content := container.NewVBox()
	content.Add(widget.NewLabelWithStyle("Saved Server IPs per Subnet", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	content.Add(widget.NewSeparator())

	cfg := config.Get()
	if len(cfg.SavedNetworks) == 0 {
		content.Add(widget.NewLabel("No saved networks yet."))
	}

	for subnet, ip := range cfg.SavedNetworks {
		s, i := subnet, ip // capture
		row := container.NewBorder(nil, nil, nil,
			widget.NewButton("Delete", func() {
				_ = config.RemoveSavedNetwork(s)
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

func (a *App) searchFiles(query string) {
	if atomic.LoadInt32(&a.isConnected) != 1 {
		return
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	results, err := a.apiClient.Search(ctx, query)
	if err != nil {
		a.setStatus("🔴 Search failed: "+err.Error(), 5*time.Second)
		return
	}
	
	// Clear and update binding
	a.filesBinding.Set(nil)
	for i := range results {
		// Convert SearchResult to FileInfo to avoid UI panic
		file := api.FileInfo{
			Name:        results[i].Path,
			IsDirectory: results[i].IsDirectory,
			Size:        results[i].Size,
			ModTime:     results[i].ModTime,
		}
		a.filesBinding.Append(file)
	}
	
	a.setStatus(fmt.Sprintf("✅ Found %d matching files", len(results)), 3*time.Second)
}

func getFileResource(filename string) fyne.Resource {
	category := presentation.GetFileCategory(filename)
	switch category {
	case "pdf":
		return theme.FileTextIcon()
	case "audio":
		return theme.FileAudioIcon()
	case "video":
		return theme.FileVideoIcon()
	case "archive":
		return theme.StorageIcon()
	default:
		return theme.FileIcon()
	}
}
