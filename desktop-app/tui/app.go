package tui

import (
	"desktop-app/api"
	"desktop-app/clipboardops"
	"desktop-app/config"
	"desktop-app/fileops"
	"desktop-app/historyops"
	"desktop-app/platform"
	"desktop-app/session"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.design/x/clipboard"
	"golang.org/x/term"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)


type AppModel struct {
	apiClient  *api.Client
	wsClient   *api.WSClient
	session    *session.Manager
	fileOps    *fileops.Service
	clipOps    *clipboardops.Service
	historyOps *historyops.Service

	// UI State
	currentTab string // "history", "files", "clipboard", "settings"
	width      int
	height     int
	ready      bool // true after receiving valid WindowSizeMsg
	statusMsg  string
	toastMsg   string
	isLoading  bool

	// List dimensions (calculated from window size)
	listWidth  int
	listHeight int

	// File picker dimensions
	filePickerWidth  int
	filePickerHeight int

	// Components
	historyList    list.Model
	filesList      list.Model
	filePicker     filepicker.Model
	showFilePicker bool
	clipViewport   viewport.Model
	clipTextArea   textarea.Model
	spinner        spinner.Model
	help           help.Model
	uploadProgress progress.Model
	isUploading    bool
	downloadProgress progress.Model
	isDownloading   bool

	// Settings Form
	inputs      []textinput.Model
	focusIndex  int
	isGuestMode bool

	// Clipboard channel ("" = private/main, "guest" = guest)
	clipboardChannel string

	// Confirmation Dialogs
	showConfirm         bool
	pendingAction       func() tea.Cmd
	pendingActionLabel  string
	pendingActionTarget string

	// File Explorer Expansion
	currentPath   string
	selectedFiles map[string]bool

	clipLog []string

	program *tea.Program
}

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Tab      key.Binding
	Enter    key.Binding
	Help     key.Binding
	Quit     key.Binding
	Fetch    key.Binding
	Delete   key.Binding
	Upload   key.Binding
	Open     key.Binding
	Push     key.Binding
	Space    key.Binding
	SelectAll key.Binding
	Clear    key.Binding
	CopyLog  key.Binding
	ToggleChannel key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Tab, k.Enter, k.Fetch, k.Push, k.CopyLog},
		{k.SelectAll, k.Clear, k.Upload, k.Open, k.Delete, k.Help, k.Quit},
	}
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "move left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "move right"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next tab"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm/copy"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "q"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
	Fetch: key.NewBinding(
		key.WithKeys("r", "ctrl+r"),
		key.WithHelp("r", "refresh"),
	),
	Delete: key.NewBinding(
		key.WithKeys("delete", "d"),
		key.WithHelp("d/del", "delete"),
	),
	Upload: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "upload file"),
	),
	Open: key.NewBinding(
		key.WithKeys("o", "ctrl+o"),
		key.WithHelp("o/ctrl+o", "open folder"),
	),
	Push: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "push clipboard"),
	),
	Space: key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "Select/Mark"),
	),
	SelectAll: key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "Select All"),
	),
	Clear: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "Clear Selection"),
	),
	CopyLog: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "copy log"),
	),
	ToggleChannel: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithKeys("g"),
		key.WithHelp("g", "toggle clipboard"),
	),
}

func getTerminalSize() (int, int) {
	// Try multiple FDs - some terminals only report size on specific ones
	fds := []uintptr{os.Stdout.Fd(), os.Stdin.Fd(), os.Stderr.Fd()}
	for _, fd := range fds {
		if w, h, err := term.GetSize(int(fd)); err == nil && w > 0 && h > 0 {
			return w, h
		}
	}
	// Fallback to environment variables
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w := parseInt(cols); w > 0 {
			if lines := os.Getenv("LINES"); lines != "" {
				if h := parseInt(lines); h > 0 {
					return w, h
				}
			}
		}
	}
	// Sensible default
	return 80, 24
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func NewApp() *AppModel {
	sess := session.New(config.Current.ServerIP, config.Current.PairingCode)
	apiClient := sess.APIClient()
	wsClient := sess.WSClient()

	fileOps := fileops.New(apiClient, config.Current.DownloadFolder)
	clipOps := clipboardops.New(apiClient)
	historyOps := historyops.New(apiClient)

	// Setup history list with initial dimensions
	hList := list.New([]list.Item{}, list.NewDefaultDelegate(), 60, 20)
	hList.Title = "History (Enter/C to copy, R to refresh, Del to delete)"
	// Apply custom styles to list items
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = ListItemStyle
	delegate.Styles.SelectedTitle = SelectedListItemStyle
	delegate.Styles.NormalDesc = DimTextStyle
	delegate.Styles.SelectedDesc = SelectedListItemStyle.Copy().Foreground(DimTextColor)
	hList.SetDelegate(delegate)
	hList.SetShowTitle(false)
	hList.SetShowHelp(false)
	hList.SetFilteringEnabled(true)

	// Setup files list with initial dimensions
	fList := list.New([]list.Item{}, list.NewDefaultDelegate(), 60, 18)
	fList.Title = "Files (Space: Select, Ctrl+A: Select All, Esc: Clear, Enter: Nav/Dwnld, U: Upload, D: Delete)"
	delegate2 := list.NewDefaultDelegate()
	delegate2.Styles.NormalTitle = ListItemStyle
	delegate2.Styles.SelectedTitle = SelectedListItemStyle
	delegate2.Styles.NormalDesc = DimTextStyle
	delegate2.Styles.SelectedDesc = SelectedListItemStyle.Copy().Foreground(DimTextColor)
	fList.SetDelegate(delegate2)
	fList.SetShowTitle(false)
	fList.SetShowHelp(false)
	fList.SetFilteringEnabled(true)

	// Setup file picker
	fp := filepicker.New()
	fp.AllowedTypes = []string{} // allow all
	fp.CurrentDirectory, _ = os.UserHomeDir()

	// Setup clipboard viewport and textarea
	vp := viewport.New(0, 0)
	vp.SetContent("Waiting for clipboard updates...")
	
	ta := textarea.New()
	ta.Placeholder = "Type text to share and press Ctrl+S to push..."
	ta.Blur()
	ta.ShowLineNumbers = false

	// Setup Spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(SecondaryColor)

	// Setup Help
	h := help.New()

	// Setup Settings Inputs
	inputs := make([]textinput.Model, 3)
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "Server IP (e.g., localhost:26260)"
	inputs[0].SetValue(config.Current.ServerIP)
	inputs[0].Focus()

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "Pairing Code"
	inputs[1].SetValue(config.Current.PairingCode)

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "Download Folder"
	inputs[2].SetValue(config.Current.DownloadFolder)

	app := &AppModel{
		session:        sess,
		apiClient:      apiClient,
		wsClient:       wsClient,
		fileOps:        fileOps,
		clipOps:        clipOps,
		historyOps:     historyOps,
		currentTab:     "history",
		statusMsg:      "🔴 Disconnected",
		historyList:    hList,
		filesList:      fList,
		filePicker:     fp,
		showFilePicker: false,
		clipViewport:   vp,
		clipTextArea:   ta,
		spinner:        s,
		help:           h,
		inputs:         inputs,
		focusIndex:     0,
		currentPath:    "",
		selectedFiles:  make(map[string]bool),
		uploadProgress: progress.New(progress.WithGradient("#D97706", "#FBBF24")),
		downloadProgress: progress.New(progress.WithGradient("#D97706", "#FBBF24")),
	}

	return app
}

func (m *AppModel) Run() error {
	// Enable alternate screen and mouse support
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p

	// Register WebSocket Callbacks
	m.wsClient.OnHistoryUpdate = func() {
		m.program.Send(wsHistoryUpdateMsg{})
	}
	m.wsClient.OnFilesUpdate = func() {
		m.program.Send(wsFilesUpdateMsg{})
	}
	m.wsClient.OnClipUpdate = func() {
		m.program.Send(wsClipUpdateMsg{})
	}
	m.wsClient.OnStatusChange = func(status string) {
		m.program.Send(wsStatusMsg{status: status})
	}

	_, err := p.Run()
	return err
}

type clearToastMsg struct{}

func (m *AppModel) clearToastCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return clearToastMsg{}
	})
}

func (m *AppModel) getHeaderView() string {
	statusDot := "●"
	dotColor := ErrorColor
	if strings.Contains(m.statusMsg, "Connected") {
		dotColor = SuccessColor
	}
	statusIndicator := lipgloss.NewStyle().Foreground(dotColor).Render(statusDot)

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Center,
			TitleStyle.Render("K-Share Desktop"),
			" ",
			statusIndicator,
		),
		"", // Spacer
	)
}

func (m *AppModel) getFooterView() string {
	spinnerView := ""
	if m.isLoading {
		spinnerView = m.spinner.View() + " "
	}

	statusText := spinnerView + m.statusMsg
	if m.currentTab == "files" && len(m.selectedFiles) > 0 {
		statusText += fmt.Sprintf(" • %d selected", len(m.selectedFiles))
	}

	var helpView string
	if m.help.ShowAll {
		helpView = "" // Will be rendered at the bottom
	} else {
		helpView = m.help.ShortHelpView(keys.ShortHelp())
	}

	// Safety: ensure positive width even before terminal size is detected
	safeWidth := m.width
	if safeWidth < 10 {
		safeWidth = 80 // Default fallback
	}

	leftWidth := safeWidth / 2
	rightWidth := safeWidth - leftWidth - 4 // -4 accounts for StatusStyle padding (1+1 on each side)
	if rightWidth < 10 {
		rightWidth = 10
	}

	leftFooter := StatusStyle.Width(leftWidth).Render(statusText)
	rightFooter := StatusStyle.Width(rightWidth).Align(lipgloss.Right).Render(helpView)

	footer := lipgloss.JoinHorizontal(lipgloss.Top, leftFooter, rightFooter)

	if m.toastMsg != "" {
		// If toast is active, show it instead of help in the status bar
		toast := ToastStyle.Width(rightWidth).Render(m.toastMsg)
		footer = lipgloss.JoinHorizontal(lipgloss.Top, leftFooter, toast)
	}

	var bars []string
	if m.isUploading {
		bars = append(bars, m.uploadProgress.View())
	}
	if m.isDownloading {
		bars = append(bars, m.downloadProgress.View())
	}

	if len(bars) > 0 {
		footer = lipgloss.JoinVertical(lipgloss.Left, footer, strings.Join(bars, "\n"))
	}

	if m.help.ShowAll {
		footer = lipgloss.JoinVertical(lipgloss.Left, footer, HelpStyle.Render(m.help.View(keys)))
	}

	return footer
}

func (m *AppModel) refreshLayout() {
	_, w, h := m.calcLayout()
	m.syncComponentSizes(w, h)
}

func (m *AppModel) calcLayout() (int, int, int) {
	// Add a small safety buffer to the width to prevent wrapping issues on some terminals/zoom levels
	safeWidth := m.width
	if safeWidth > 0 {
		safeWidth -= 1
	}

	headerHeight := lipgloss.Height(m.getHeaderView())
	footerHeight := lipgloss.Height(m.getFooterView())
	
	// Safety: clamp header/footer heights to sensible ranges
	// prevents negative mainHeight if lipgloss.Height returns garbage
	if headerHeight <= 0 || headerHeight > 50 {
		headerHeight = 10 // Estimate for header
	}
	if footerHeight <= 0 || footerHeight > 50 {
		footerHeight = 5 // Minimum footer
	}

	mainHeight := m.height - headerHeight - footerHeight
	if mainHeight < 10 {
		mainHeight = 10 // Minimum usable height
	}

	sidebarView := SidebarStyle.Width(sidebarWidth).Height(mainHeight).Render("")
	sidebarActualWidth := lipgloss.Width(sidebarView)

	totalContentWidth := safeWidth - sidebarActualWidth
	if totalContentWidth < 0 {
		totalContentWidth = 0
	}

	return sidebarActualWidth, totalContentWidth, mainHeight
}

func (m *AppModel) syncComponentSizes(contentWidth, mainHeight int) {
	// Standard list dimensions (used by history and files)
	// Padding(0, 2) in View() means we lose 4 chars of width
	// Subtract 4 lines to account for Title, Instructions, and Spacing in View()
	m.listWidth = contentWidth - 4
	m.listHeight = mainHeight - 4
	if m.listWidth < 10 {
		m.listWidth = 10
	}
	if m.listHeight < 5 {
		m.listHeight = 5
	}

	if m.historyList.Width() != m.listWidth || m.historyList.Height() != m.listHeight {
		m.historyList.SetSize(m.listWidth, m.listHeight)
	}
	
	if m.filesList.Width() != m.listWidth || m.filesList.Height() != m.listHeight {
		m.filesList.SetSize(m.listWidth, m.listHeight)
	}

	// File picker dimensions
	m.filePickerWidth = m.listWidth
	m.filePickerHeight = m.listHeight - 2
	m.filePicker, _ = m.filePicker.Update(tea.WindowSizeMsg{Width: m.filePickerWidth, Height: m.filePickerHeight})

	// Clipboard panels
	// Account for tabHeader(1) + instructions(1) + panel borders(2x2)
	panelHeight := (mainHeight - 10) / 2
	if panelHeight < 3 {
		panelHeight = 3
	}
	// panelWidth (outer) is contentWidth - 4
	// panelInnerWidth (inner) is outer - border(2) - padding(4) = outer - 6
	panelInnerWidth := (contentWidth - 4) - 6

	if m.clipTextArea.Width() != panelInnerWidth || m.clipTextArea.Height() != panelHeight-2 {
		m.clipTextArea.SetWidth(panelInnerWidth)
		m.clipTextArea.SetHeight(panelHeight - 2)
	}
	if m.clipViewport.Width != panelInnerWidth || m.clipViewport.Height != panelHeight {
		m.clipViewport.Width = panelInnerWidth
		m.clipViewport.Height = panelHeight
	}
}

func (m *AppModel) Init() tea.Cmd {
	// Perform role detection via session
	go func() {
		role, err := m.session.Connect()
		if err != nil {
			// Send status to TUI instead of logging to stderr
			// Logging to stderr during TUI can corrupt the display
			m.program.Send(wsStatusMsg{status: "🔴 Connection Failed"})
			return
		}

		// Complete the connection with the detected role
		m.session.CompleteConnection(role)
		m.program.Send(wsStatusMsg{status: "🟢 Connected as " + role})

		// Set guest mode based on role
		m.isGuestMode = (role == "guest")
		m.clipboardChannel = m.session.ClipboardChannel()
	}()

	// Start background processes (like WS connection)
	go func() {
		m.wsClient.OnClipGuestUpdate = func() {
			m.program.Send(wsClipGuestUpdateMsg{})
		}

		if err := m.wsClient.Connect(); err != nil {
			// Send status to TUI instead of logging to stderr
			// Logging to stderr during TUI can corrupt the display
			m.program.Send(wsStatusMsg{status: "🔴 WS Connect Failed"})
		}
	}()

	return tea.Batch(
		tea.SetWindowTitle("K-Share TUI"),
		tea.WindowSize(),
		m.fetchHistoryCmd(),
		m.fetchFilesCmd(),
		m.spinner.Tick,
	)
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case wsHistoryUpdateMsg:
		cmds = append(cmds, m.fetchHistoryCmd())
	case wsFilesUpdateMsg:
		cmds = append(cmds, m.fetchFilesCmd())
	case wsClipUpdateMsg:
		if m.clipboardChannel == "" {
			cmds = append(cmds, m.fetchClipboardCmd())
		}
	case wsClipGuestUpdateMsg:
		if m.clipboardChannel == "guest" {
			cmds = append(cmds, m.fetchClipboardCmd())
		}
	case wsStatusMsg:
		m.statusMsg = msg.status
		m.refreshLayout() // Status changes footer height, need to refresh layout
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case clearToastMsg:
		m.toastMsg = ""
		return m, nil
	case progress.FrameMsg:
		progressModel, cmd := m.uploadProgress.Update(msg)
		m.uploadProgress = progressModel.(progress.Model)
		return m, cmd
case progressMsg:
		if msg.isDownload {
			cmd := m.downloadProgress.SetPercent(msg.percent)
			if msg.percent >= 1.0 {
				m.isDownloading = false
			} else {
				m.isDownloading = true
			}
			m.refreshLayout() // Progress bars change footer height
			return m, cmd
		} else {
		cmd := m.uploadProgress.SetPercent(msg.percent)
		if msg.percent >= 1.0 {
			m.isUploading = false
		} else {
			m.isUploading = true
		}
		m.refreshLayout() // Progress bars change footer height
		return m, cmd
	}

	case tea.KeyMsg:
		if m.showConfirm {
			switch msg.String() {
			case "y", "Y":
				cmd := m.pendingAction()
				m.showConfirm = false
				m.pendingAction = nil
				return m, cmd
			case "n", "N", "esc":
				m.showConfirm = false
				m.pendingAction = nil
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+o":
			m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Opening download folder..."
			return m, func() tea.Msg {
				_ = platform.OpenFolder(config.Current.DownloadFolder)
				return nil
			}
		case "ctrl+l":
			text := m.statusMsg
			// Basic cleanup: remove common icons if they are at the start
			icons := []string{IconSuccess, IconError, IconLoading, IconHistory, IconFiles, IconClipboard, IconSettings, IconServer, IconDelete}
			for _, icon := range icons {
				text = strings.TrimPrefix(text, icon)
			}
			text = strings.TrimSpace(text)

			clipboard.Write(clipboard.FmtText, []byte(text))
			m.toastMsg = "Log Copied!"
			return m, m.clearToastCmd()
		case "?":
			m.help.ShowAll = !m.help.ShowAll
			m.refreshLayout() // Help toggle changes footer height
			return m, nil
		case "tab":
			if m.currentTab == "history" {
				m.currentTab = "files"
			} else if m.currentTab == "files" {
				m.currentTab = "clipboard"
				m.clipTextArea.Focus()
			} else if m.currentTab == "clipboard" {
				m.clipTextArea.Blur()
				m.currentTab = "settings"
			} else {
				m.currentTab = "history"
			}
			_, w, h := m.calcLayout()
			m.syncComponentSizes(w, h)
			return m, nil
		case "shift+tab":
			if m.currentTab == "settings" {
				m.currentTab = "clipboard"
				m.clipTextArea.Focus()
			} else if m.currentTab == "clipboard" {
				m.clipTextArea.Blur()
				m.currentTab = "files"
			} else if m.currentTab == "files" {
				m.currentTab = "history"
			} else {
				m.currentTab = "settings"
			}
			_, w, h := m.calcLayout()
			m.syncComponentSizes(w, h)
			return m, nil
		}

	case tea.WindowSizeMsg:
		// Ignore zero sizes - common in IDE terminals at startup
		if msg.Width == 0 || msg.Height == 0 {
			return m, nil
		}
		m.width = msg.Width
		m.height = msg.Height
		_, w, h := m.calcLayout()
		m.syncComponentSizes(w, h)

		// Mark ready and clear screen on first valid size
		if !m.ready {
			m.ready = true
			return m, tea.ClearScreen
		}
		return m, nil
	}

	// Input Isolation: Route KeyMsg ONLY to the active tab
	// Non-KeyMsgs go to all to handle background updates
	if _, ok := msg.(tea.KeyMsg); ok {
		switch m.currentTab {
		case "history":
			updatedApp, cmd := m.updateHistoryList(msg)
			m = updatedApp.(*AppModel)
			cmds = append(cmds, cmd)
		case "files":
			updatedApp, cmd := m.updateFilesList(msg)
			m = updatedApp.(*AppModel)
			cmds = append(cmds, cmd)
		case "clipboard":
			updatedApp, cmd := m.updateClipboardView(msg)
			m = updatedApp.(*AppModel)
			cmds = append(cmds, cmd)
		case "settings":
			updatedApp, cmd := m.updateSettingsView(msg)
			m = updatedApp.(*AppModel)
			cmds = append(cmds, cmd)
		}
	} else {
		// Non-Key messages (like network updates) go to all components

		updatedApp, cmd1 := m.updateHistoryList(msg)
		m = updatedApp.(*AppModel)
		cmds = append(cmds, cmd1)

		updatedApp, cmd2 := m.updateFilesList(msg)
		m = updatedApp.(*AppModel)
		cmds = append(cmds, cmd2)

		updatedApp, cmd3 := m.updateClipboardView(msg)
		m = updatedApp.(*AppModel)
		cmds = append(cmds, cmd3)

		updatedApp, cmd4 := m.updateSettingsView(msg)
		m = updatedApp.(*AppModel)
		cmds = append(cmds, cmd4)
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) View() string {
	// If terminal size is not yet detected or too small for a usable UI
	if !m.ready || m.width < 50 || m.height < 15 {
		return "Initializing UI... (Expand terminal if it stays like this)\nMinimum size required: 80x24"
	}

	header := m.getHeaderView()
	footer := m.getFooterView()

	_, totalContentWidth, mainHeight := m.calcLayout()

	// Render tabs
	tabData := []struct{ id, label, icon string }{
		{"history", "HISTORY", IconHistory},
		{"files", "FILES", IconFiles},
		{"clipboard", "CLIPBOARD", IconClipboard},
		{"settings", "SETTINGS", IconSettings},
	}

	var renderedTabs []string
	for _, t := range tabData {
		label := t.icon + t.label
		if m.currentTab == t.id {
			renderedTabs = append(renderedTabs, ActiveSidebarItemStyle.Render(label))
		} else {
			renderedTabs = append(renderedTabs, SidebarItemStyle.Render(label))
		}
	}
	
	// Enforce sidebar width and height strictly to prevent it from disappearing
	sidebarContent := lipgloss.JoinVertical(lipgloss.Left, renderedTabs...)
	sidebar := SidebarStyle.
		Width(sidebarWidth).
		MaxWidth(sidebarWidth).
		Height(mainHeight).
		MaxHeight(mainHeight).
		Render(sidebarContent)

	var content string
	innerContentWidth := totalContentWidth
	if innerContentWidth < 0 {
		innerContentWidth = 0
	}

	if m.showConfirm {
		dialogBox := DialogStyle.Width(50).Render(
			lipgloss.JoinVertical(lipgloss.Center,
				LabelStyle.Render("Confirm Action"),
				"",
				fmt.Sprintf("Are you sure you want to %s?", m.pendingActionLabel),
				DimTextStyle.Render(m.pendingActionTarget),
				"",
				"(y) Yes  /  (N) No",
			),
		)
		content = lipgloss.Place(innerContentWidth, mainHeight, lipgloss.Center, lipgloss.Center, dialogBox)
	} else {
		switch m.currentTab {
		case "history":
			title := HeaderStyle.Render("History (" + IconHistory + ")")
			instructions := DimTextStyle.Render("Enter/C: Copy | R: Refresh | Del: Delete")
			content = lipgloss.JoinVertical(lipgloss.Left, title, instructions, "", m.historyList.View())
		case "files":
			title := HeaderStyle.Render("Remote Files (" + IconFiles + ")")
			instructions := DimTextStyle.Render("Space: Mark | Ctrl+A: All | Nav: Move | Enter: Enter/Dwnld")
			if m.showFilePicker {
				content = "Select a file to upload (ESC to cancel):\n\n" + m.filePicker.View()
			} else {
				content = lipgloss.JoinVertical(lipgloss.Left, title, instructions, "", m.filesList.View())
			}
		case "clipboard":
			channelLabel := "PRIVATE"
			if m.clipboardChannel == "guest" {
				channelLabel = "GUEST"
			}
			tabHeader := HeaderStyle.Render("Shared Clipboard - " + channelLabel)
			instrChannel := ""
			if !m.isGuestMode {
				instrChannel = "Ctrl+G: Toggle | "
			}
			instructions := DimTextStyle.Render(instrChannel + "Ctrl+S: Push | Ctrl+R: Fetch")
			
			panelWidth := innerContentWidth - 6 // -6 accounts for PanelStyle padding (4) + border (2)
			if panelWidth < 0 {
				panelWidth = 0
			}

			inputBox := PanelStyle.Width(panelWidth).Render(
				LabelStyle.Render("Input Area:") + "\n" +
				m.clipTextArea.View(),
			)

			logBox := PanelStyle.Width(panelWidth).Render(
				LabelStyle.Render("Sync Log:") + "\n" +
				m.clipViewport.View(),
			)

			content = lipgloss.JoinVertical(lipgloss.Left, tabHeader, instructions, inputBox, logBox)
		case "settings":
			content = m.renderSettings()
		}
	}

	// Wrap content to ensure it fills the space correctly and consistently
	// and doesn't overflow or push the sidebar.
	styledContent := lipgloss.NewStyle().
		Width(totalContentWidth).
		MaxWidth(totalContentWidth).
		Height(mainHeight).
		MaxHeight(mainHeight).
		Render(content)

	// Ensure sidebar is joined with content accurately using JoinHorizontal
	mainSection := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, styledContent)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		mainSection,
		footer,
	)
}
