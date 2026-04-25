package tui

import (
	"context"
	"desktop-app/api"
	"desktop-app/clipboardops"
	"desktop-app/config"
	"desktop-app/discoveryops"
	"desktop-app/fileops"
	"desktop-app/historyops"
	"desktop-app/platform"
	"desktop-app/session"
	"errors"
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
	discoveryOps *discoveryops.Service

	// UI State
	currentTab string // "history", "files", "clipboard", "settings"
	width      int
	height     int
	ready      bool // true after receiving valid WindowSizeMsg
	isConnected bool
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
	clipboardInputFocused bool

	// Settings Form
	inputs      []textinput.Model
	focusIndex  int
	isGuestMode bool

	// Clipboard channel ("" = private/main, "guest" = guest)
	clipboardChannel string

	// Clipboard sync tracking to prevent duplicate writes
	lastRemoteClipboard string

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

	showTrustPrompt      bool
	pendingTrustServerIP string
	pendingTrustRole     string
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
	Discover key.Binding
	DeleteWord key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Tab, k.Enter, k.Fetch, k.Push, k.CopyLog},
		{k.SelectAll, k.Clear, k.Upload, k.Open, k.Delete, k.Help, k.Quit},
		{k.DeleteWord, k.ToggleChannel, k.Discover},
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
		key.WithKeys("ctrl+g", "g"),
		key.WithHelp("g", "toggle clipboard"),
	),
	Discover: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "discover server"),
	),
	DeleteWord: key.NewBinding(
		key.WithKeys("ctrl+backspace", "ctrl+h", "alt+backspace"),
		key.WithHelp("ctrl+backspace", "delete word"),
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
	cfg := config.Get()
	sess := session.New(cfg.ServerIP, cfg.PairingCode)
	apiClient := sess.APIClient()
	wsClient := sess.WSClient()

	fileOps := fileops.New(apiClient, cfg.DownloadFolder)
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
	
	// Add Ctrl+Left/Right and Ctrl+Backspace/Delete to textarea
	ta.KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"))
	ta.KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"))
	ta.KeyMap.DeleteWordBackward = key.NewBinding(key.WithKeys("alt+backspace", "ctrl+backspace", "ctrl+w", "ctrl+h"))
	ta.KeyMap.DeleteWordForward = key.NewBinding(key.WithKeys("alt+delete", "ctrl+delete", "alt+d"))

	// Setup Spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(SecondaryColor)

	// Setup Help
	h := help.New()

	// Setup Settings Inputs
	inputs := make([]textinput.Model, 3)
	
	// Add Ctrl shortcuts to inputs
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"))
		inputs[i].KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"))
		inputs[i].KeyMap.DeleteWordBackward = key.NewBinding(key.WithKeys("alt+backspace", "ctrl+backspace", "ctrl+w", "ctrl+h"))
		inputs[i].KeyMap.DeleteWordForward = key.NewBinding(key.WithKeys("alt+delete", "ctrl+delete", "alt+d"))
	}

	inputs[0].Placeholder = "Server IP (e.g., localhost:26260)"
	inputs[0].SetValue(cfg.ServerIP)
	inputs[0].Focus()

	inputs[1].Placeholder = "Pairing Code"
	inputs[1].SetValue(cfg.PairingCode)
	inputs[1].EchoMode = textinput.EchoPassword
	inputs[1].EchoCharacter = '•'

	inputs[2].Placeholder = "Download Folder"
	inputs[2].SetValue(cfg.DownloadFolder)

	// Setup Discovery Service
	discOps := discoveryops.New(cfg.PairingCode)

	app := &AppModel{
		session:        sess,
		apiClient:      apiClient,
		wsClient:       wsClient,
		fileOps:        fileOps,
		clipOps:       clipOps,
		historyOps:     historyOps,
		discoveryOps:  discOps,
		currentTab:     "history",
		statusMsg:      "🔴 Disconnected",
		isConnected:   false,
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
	// Enable alternate screen and mouse support for proper terminal handshake
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p

	// Register WebSocket Callbacks with nil checks to prevent crashes
	m.wsClient.SetOnHistoryUpdate(func() {
		if m.program != nil {
			m.program.Send(wsHistoryUpdateMsg{})
		}
	})
	m.wsClient.SetOnFilesUpdate(func() {
		if m.program != nil {
			m.program.Send(wsFilesUpdateMsg{})
		}
	})
	m.wsClient.SetOnClipUpdate(func() {
		if m.program != nil {
			m.program.Send(wsClipUpdateMsg{})
		}
	})
	m.wsClient.SetOnStatusChange(func(status string) {
		if m.program != nil {
			m.program.Send(wsStatusMsg{status: status})
		}
	})

	_, err := p.Run()
	return err
}

type clearToastMsg struct{}

func (m *AppModel) clearToastCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return clearToastMsg{}
	})
}

func (m *AppModel) connectCmd() tea.Cmd {
	return func() tea.Msg {
		role, err := m.session.Connect()
		return connectionResultMsg{role: role, err: err}
	}
}

func (m *AppModel) refreshAllCmd() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return m.spinner.Tick() },
		m.connectCmd(),
	)
}

func (m *AppModel) loadDataCmd() tea.Cmd {
	return tea.Batch(
		m.fetchHistoryCmd(),
		m.fetchFilesCmd(),
		m.fetchClipboardCmd(),
	)
}

func (m *AppModel) completeConnectionCmd(role string) tea.Cmd {
	return func() tea.Msg {
		m.session.CompleteConnection(role)
		return connectionCompletedMsg{role: role}
	}
}

type connectionCompletedMsg struct {
	role string
}

func (m *AppModel) copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		clipboard.Write(clipboard.FmtText, []byte(text))
		return clipboardWrittenMsg{text: text}
	}
}

type clipboardWrittenMsg struct {
	text string
}

func (m *AppModel) getHeaderView() string {
	// Connection badge - use dedicated flag instead of parsing text
	statusDot := "●"
	dotColor := ErrorColor
	if m.isConnected {
		dotColor = SuccessColor
	}
	connectedText := "Disconnected"
	if m.isConnected {
		connectedText = "Connected"
	}
	connectionBadge := lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Foreground(dotColor).Render(statusDot),
		" ",
		lipgloss.NewStyle().Foreground(dotColor).Bold(true).Render(connectedText),
	)

	// Mode badge
	modeText := "GUEST"
	modeColor := DimTextColor
	if !m.isGuestMode {
		modeText = "ADMIN"
		modeColor = SecondaryColor
	}
	modeBadge := lipgloss.NewStyle().
		Foreground(modeColor).
		Background(SurfaceColor).
		Padding(0, 1).
		Render("["+modeText+"]")

	// Toast/Notification area - now part of header so it doesn't shift layout
	toast := ""
	if m.toastMsg != "" {
		toast = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(SuccessColor).
			Padding(0, 1).
			Bold(true).
			MarginLeft(2).
			Render(m.toastMsg)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Center,
			TitleStyle.Render("K-Share"),
			" ",
			connectionBadge,
			"  ",
			modeBadge,
			toast,
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
	
	// Add role indicator (Admin/Guest)
	if m.isConnected {
		roleIndicator := "[ADMIN] "
		if m.isGuestMode {
			roleIndicator = "[GUEST] "
		}
		statusText += " " + lipgloss.NewStyle().Foreground(SecondaryColor).Render(roleIndicator)
	}
	
	if m.currentTab == "files" && len(m.selectedFiles) > 0 {
		statusText += fmt.Sprintf(" • %d selected", len(m.selectedFiles))
	}

	helpView := m.help.View(keys)

	// Safety: ensure positive width even before terminal size is detected
	if m.width <= 0 {
		m.width = 80
	}

	footerWidth := m.width - sidebarWidth - 1
	if footerWidth < 0 {
		footerWidth = 0
	}

	footerStyle := lipgloss.NewStyle().
		Width(footerWidth).
		Foreground(DimTextColor).
		Padding(0, 1)

	// Add progress bars for uploads/downloads if active
	var progressViews []string
	if m.isUploading {
		progressViews = append(progressViews, "↑ "+m.uploadProgress.View())
	}
	if m.isDownloading {
		progressViews = append(progressViews, "↓ "+m.downloadProgress.View())
	}

	if m.help.ShowAll {
		// Full help is multi-line, render it separately from status text
		return lipgloss.JoinVertical(lipgloss.Left,
			footerStyle.Render(statusText),
			"",
			helpView,
		)
	}

	if len(progressViews) > 0 {
		progressText := lipgloss.NewStyle().
			Foreground(SecondaryColor).
			Render(strings.Join(progressViews, "  "))
		return footerStyle.Render(statusText + "  " + progressText + "  " + helpView)
	}

	return footerStyle.Render(statusText + "  " + helpView)
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

	// Fixed header/footer heights - no more lipgloss.Height calls
	// This removes expensive double-rendering from every frame
	headerHeight := 8
	footerHeight := 4

	mainHeight := m.height - headerHeight - footerHeight
	if mainHeight < 10 {
		mainHeight = 10 // Minimum usable height
	}

	sidebarActualWidth := sidebarWidth

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
	// Set some reasonable defaults so View() doesn't render junk at size 0
	m.width = 80
	m.height = 24

	m.wsClient.SetOnClipGuestUpdate(func() {
		if m.program != nil {
			m.program.Send(wsClipGuestUpdateMsg{})
		}
	})

	return tea.Batch(
		tea.SetWindowTitle("K-Share TUI"),
		tea.WindowSize(),
		m.refreshAllCmd(),
	)
}

func (m *AppModel) isAnyInputFocused() bool {
	if m.showConfirm || m.showTrustPrompt {
		return false
	}
	switch m.currentTab {
	case "history":
		return m.historyList.FilterState() == list.Filtering
	case "files":
		return m.filesList.FilterState() == list.Filtering || m.showFilePicker
	case "clipboard":
		return m.clipboardInputFocused
	case "settings":
		return true // Settings always has an input focused
	}
	return false
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
		if strings.Contains(msg.status, "Connected") {
			m.isConnected = true
		} else if strings.Contains(msg.status, "Disconnected") || strings.Contains(msg.status, "Failed") {
			m.isConnected = false
			m.isLoading = false
		}
	case connectionResultMsg:
		m.isLoading = false
		if msg.err != nil {
			if errors.Is(msg.err, session.ErrTrustRequired) {
				certInfo, serverIP, pendingRole, ok := m.session.PendingTrust()
				if ok {
					m.showTrustPrompt = true
					m.pendingTrustServerIP = serverIP
					m.pendingTrustRole = pendingRole
					m.statusMsg = Branding.GetIcon(IconLoading, "") + "Certificate not trusted. Accept? (y/n)"
					_ = certInfo
				}
			} else {
				m.statusMsg = fmt.Sprintf("%s Connection Failed: %v", IconError, msg.err)
				m.isConnected = false
				// Auto-discovery fallback on connection failure
				m.isLoading = true
				m.statusMsg = Branding.GetIcon(IconLoading, "") + "Connection failed. Searching for server..."
				cmds = append(cmds, func() tea.Msg {
					ip := m.discoveryOps.Discover(26260, func(s string) {})
					return discoveryResultMsg{ip: ip}
				})
			}
		} else {
			cmds = append(cmds, m.completeConnectionCmd(msg.role))
		}
	case connectionCompletedMsg:
		m.isGuestMode = m.session.IsGuest()
		m.clipboardChannel = m.session.ClipboardChannel()
		m.isConnected = true
		m.statusMsg = fmt.Sprintf("%s Connected as %s", Branding.GetIcon(IconSuccess, ""), msg.role)
		cmds = append(cmds, m.loadDataCmd())
	case sessionReconnectedMsg:
		m.isLoading = true
		cmds = append(cmds, m.refreshAllCmd())
	case discoveryResultMsg:
		m.isLoading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s Discovery failed: %v", IconError, msg.err)
		} else if msg.ip != "" {
			m.statusMsg = fmt.Sprintf("%s Found server at %s", Branding.GetIcon(IconSuccess, ""), msg.ip)
			m.inputs[0].SetValue(msg.ip)
			// Update config and reconnect
			_ = config.SetServerIP(msg.ip)
			_ = config.Save()
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Reconnecting..."
			cmds = append(cmds, func() tea.Msg { return sessionReconnectedMsg{} })
		} else {
			m.statusMsg = fmt.Sprintf("%s No server found", IconError)
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		if m.isLoading {
			m.spinner, cmd = m.spinner.Update(msg)
		}
		return m, cmd
	case clearToastMsg:
		m.toastMsg = ""
	case progress.FrameMsg:
		var cmd tea.Cmd
		uploadModel, cmd := m.uploadProgress.Update(msg)
		downloadModel, _ := m.downloadProgress.Update(msg)
		m.uploadProgress = uploadModel.(progress.Model)
		m.downloadProgress = downloadModel.(progress.Model)
		return m, cmd
	case progressMsg:
		if msg.isDownload {
			m.downloadProgress.SetPercent(msg.percent)
			m.isDownloading = msg.percent < 1.0
		} else {
			m.uploadProgress.SetPercent(msg.percent)
			m.isUploading = msg.percent < 1.0
		}
	case clipboardReadMsg:
		if len(msg.data) > 0 {
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Uploading screenshot..."
			cmds = append(cmds, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				_, err := m.clipOps.UploadScreenshotWithPrefix(ctx, msg.data, m.currentPath)
				if err != nil {
					return filesOpResultMsg{err: err}
				}
				return filesOpResultMsg{msg: "Screenshot uploaded"}
			})
		} else if len(msg.text) > 0 {
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Pushing text..."
			cmds = append(cmds, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				err := m.clipOps.PushText(ctx, msg.text, m.clipboardChannel)
				if err != nil {
					return filesOpResultMsg{err: err}
				}
				return filesOpResultMsg{msg: "Text pushed"}
			})
		} else {
			m.isLoading = false
			m.isUploading = false
			m.statusMsg = "Nothing to paste"
		}
	case tea.KeyMsg:
		key := msg.String()

		if m.showConfirm {
			switch key {
			case "y", "Y":
				cmd := m.pendingAction()
				m.showConfirm = false
				m.pendingAction = nil
				cmds = append(cmds, cmd)
			case "n", "N", "esc":
				m.showConfirm = false
				m.pendingAction = nil
			}
		} else if m.showTrustPrompt {
			switch key {
			case "y", "Y":
				m.showTrustPrompt = false
				m.session.TrustPending()
				m.statusMsg = Branding.GetIcon(IconLoading, "") + "Trust accepted, reconnecting..."
				m.isLoading = true
				cmds = append(cmds, m.refreshAllCmd())
			case "n", "N", "esc":
				m.showTrustPrompt = false
				m.session.CancelPendingTrust()
				m.statusMsg = fmt.Sprintf("%s Connection Cancelled", IconError)
			}
		} else {
			switch key {
			case "ctrl+c":
				return m, tea.Quit
			case "q":
				if !m.isAnyInputFocused() {
					return m, tea.Quit
				}
			case "ctrl+o":
				m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Opening download folder..."
				cmds = append(cmds, func() tea.Msg {
					_ = platform.OpenFolder(config.Get().DownloadFolder)
					return nil
				})
			case "ctrl+l":
				text := m.statusMsg
				icons := []string{IconSuccess, IconError, IconLoading, IconHistory, IconFiles, IconClipboard, IconSettings, IconServer, IconDelete}
				for _, icon := range icons {
					text = strings.TrimPrefix(text, icon)
				}
				text = strings.TrimSpace(text)
				m.toastMsg = "Log Copied!"
				cmds = append(cmds, m.copyToClipboardCmd(text), m.clearToastCmd())
			case "?":
				if !m.isAnyInputFocused() {
					m.help.ShowAll = !m.help.ShowAll
					return m, nil
				}
			case "tab":
				if m.currentTab == "history" {
					m.currentTab = "files"
					m.clipboardInputFocused = false
				} else if m.currentTab == "files" {
					m.currentTab = "clipboard"
					m.clipTextArea.Focus()
					m.clipboardInputFocused = true
				} else if m.currentTab == "clipboard" {
					m.clipTextArea.Blur()
					m.clipboardInputFocused = false
					m.currentTab = "settings"
				} else {
					m.currentTab = "history"
				}
			case "shift+tab":
				if m.currentTab == "settings" {
					m.currentTab = "clipboard"
					m.clipTextArea.Focus()
					m.clipboardInputFocused = true
				} else if m.currentTab == "clipboard" {
					m.clipTextArea.Blur()
					m.clipboardInputFocused = false
					m.currentTab = "files"
				} else if m.currentTab == "files" {
					m.currentTab = "history"
				} else {
					m.clipboardInputFocused = false
					m.currentTab = "settings"
				}
			case "ctrl+d":
				// Discover server - only in settings tab
				if m.currentTab == "settings" {
					m.isLoading = true
					m.statusMsg = Branding.GetIcon(IconLoading, "") + "Searching for server..."
					cmds = append(cmds, func() tea.Msg {
						ip := m.discoveryOps.Discover(26260, func(s string) {
							_ = s // suppress unused warning
						})
						return discoveryResultMsg{ip: ip}
					})
				}
			case "ctrl+v":
				// Paste & Send - read clipboard and push/upload
				m.isLoading = true
				m.isUploading = true
				m.statusMsg = Branding.GetIcon(IconLoading, "") + "Reading clipboard..."
				cmds = append(cmds, func() tea.Msg {
					// First try image clipboard
					imgData := m.clipOps.ReadSystemImageClipboard()
					if len(imgData) > 0 {
						return clipboardReadMsg{data: imgData}
					}
					// Then try text clipboard using golang.design/x/clipboard
					text := clipboard.Read(clipboard.FmtText)
					if len(text) > 0 {
						return clipboardReadMsg{text: string(text)}
					}
					return clipboardReadMsg{}
				})
			case "esc":
				if m.currentTab == "clipboard" && m.clipboardInputFocused {
					m.clipTextArea.Blur()
					m.clipboardInputFocused = false
					return m, nil
				}
			case "enter", "i":
				if m.currentTab == "clipboard" && !m.clipboardInputFocused {
					m.clipTextArea.Focus()
					m.clipboardInputFocused = true
					return m, nil
				}
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 0 && m.height > 0 {
			_, w, h := m.calcLayout()
			m.syncComponentSizes(w, h)
			if !m.ready {
				m.ready = true
				// Clear screen immediately when transitioning from unready to ready
				return m, tea.ClearScreen
			}
		}
	}

	// Route KeyMsg ONLY to the active tab
	switch msg.(type) {
	case tea.KeyMsg:
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
	default:
		// Non-Key messages go to ALL components
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

	// Add logo below tabs if there's enough space
	if mainHeight >= 12 {
		sidebarLogo := LogoStyle.Render(RenderLogo(sidebarWidth - 2))
		kShareText := lipgloss.NewStyle().
			Foreground(SecondaryColor).
			Align(lipgloss.Center).
			Width(sidebarWidth - 2).
			Render("K-Share")
		sidebarContent = lipgloss.JoinVertical(
			lipgloss.Left,
			sidebarContent,
			sidebarLogo,
			kShareText,
		)
	}

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
	} else if m.showTrustPrompt {
		WarningColor := lipgloss.NewStyle().Foreground(ErrorColor)
		trustBox := DialogStyle.Width(60).Render(
			lipgloss.JoinVertical(lipgloss.Center,
				WarningColor.Render("⚠  Untrusted Certificate"),
				"",
				LabelStyle.Render("Server:"),
				DimTextStyle.Render(m.pendingTrustServerIP),
				"",
				LabelStyle.Render("Detected Role:"),
				DimTextStyle.Render(m.pendingTrustRole),
				"",
				DimTextStyle.Render("This server's certificate has not been trusted yet."),
				DimTextStyle.Render("Trusting it will store its certificate locally."),
				"",
				"(Y) Trust & Connect  /  (N) Cancel",
			),
		)
		content = lipgloss.Place(innerContentWidth, mainHeight, lipgloss.Center, lipgloss.Center, trustBox)
	} else {
		switch m.currentTab {
		case "history":
			title := HeaderStyle.Render("History (" + IconHistory + ")")
			instructions := DimTextStyle.Render("Enter/C: Copy | R: Refresh | Del: Delete")
			listView := m.historyList.View()
			if len(m.historyList.Items()) == 0 {
				emptyState := lipgloss.NewStyle().
					Foreground(DimTextColor).
					Align(lipgloss.Center).
					Render("No history items yet.\n\nUse R to refresh.\nOr copy something to get started!")
				listView = lipgloss.Place(innerContentWidth-2, mainHeight-6, lipgloss.Center, lipgloss.Center, emptyState)
			}
			content = lipgloss.JoinVertical(lipgloss.Left, title, instructions, "", listView)
		case "files":
			title := HeaderStyle.Render("Remote Files (" + IconFiles + ")")
			
			displayPath := "/ (Root)"
			if m.currentPath != "" {
				displayPath = "/" + m.currentPath
			}
			pathView := lipgloss.NewStyle().
				Foreground(SecondaryColor).
				Bold(true).
				Padding(0, 1).
				Render("📂 " + displayPath)

			instructions := DimTextStyle.Render("Enter: Open | Shift+Enter: Download | Alt+Enter: Download Current | Space: Mark")
			if m.showFilePicker {
				content = "Select a file to upload (ESC to cancel):\n\n" + m.filePicker.View()
			} else {
				listView := m.filesList.View()
				if len(m.filesList.Items()) == 0 && m.currentPath == "" {
					emptyState := lipgloss.NewStyle().
						Foreground(DimTextColor).
						Align(lipgloss.Center).
						Render("No files found.\n\nUse U to upload files.\nOr wait for others to share.")
					listView = lipgloss.Place(innerContentWidth-2, mainHeight-6, lipgloss.Center, lipgloss.Center, emptyState)
				}
				content = lipgloss.JoinVertical(lipgloss.Left, title, pathView, instructions, "", listView)
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
			instructions := DimTextStyle.Render(instrChannel + "Ctrl+S: Push | Ctrl+R: Fetch | Enter/i: Focus | Esc: Blur")
			
			panelWidth := innerContentWidth - 6 // -6 accounts for PanelStyle padding (4) + border (2)
			if panelWidth < 0 {
				panelWidth = 0
			}

			// Apply focus styles to panels based on current focus state
			inputLabel := "Input Area:"
			logLabel := "Sync Log:"
			
			var inputBox, logBox string
			if m.clipboardInputFocused {
				inputLabelStyle := FocusLabelStyle.Width(panelWidth)
				inputBox = inputLabelStyle.Render(inputLabel) + "\n" + m.clipTextArea.View()
				logBox = PanelStyle.Width(panelWidth).Render(logLabel + "\n" + m.clipViewport.View())
			} else {
				logLabelStyle := FocusLabelStyle.Width(panelWidth)
				logBox = logLabelStyle.Render(logLabel) + "\n" + m.clipViewport.View()
				inputBox = PanelStyle.Width(panelWidth).Render(inputLabel + "\n" + m.clipTextArea.View())
			}

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

	mainUI := lipgloss.JoinVertical(lipgloss.Left,
		header,
		mainSection,
		footer,
	)

	return mainUI
}
