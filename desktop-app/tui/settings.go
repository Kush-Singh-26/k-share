package tui

import (
	"desktop-app/clipboardops"
	"desktop-app/config"
	"desktop-app/fileops"
	"desktop-app/historyops"
	"desktop-app/session"
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *AppModel) updateSettingsView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Handle manual word deletion for PowerShell compatibility
		// (PowerShell often sends unexpected codes that aren't captured by Bubble's KeyMap)
		if key == "ctrl+backspace" || key == "ctrl+h" || key == "alt+backspace" {
			// Delete the word before the cursor (i.e. the last word if cursor is at end)
			input := &m.inputs[m.focusIndex]
			text := input.Value()
			if len(text) > 0 {
				runes := []rune(text)
				// Find start of last word
				i := len(runes) - 1
				// Skip trailing spaces
				for i >= 0 && unicode.IsSpace(runes[i]) {
					i--
				}
				if i >= 0 {
					// Find end of previous word
					wordEnd := i
					for i >= 0 && !unicode.IsSpace(runes[i]) {
						i--
					}
					wordStart := i + 1
					// Delete the word
					newText := string(runes[:wordStart]) + string(runes[wordEnd+1:])
					input.SetValue(newText)
				}
			}
			return m, tea.Batch(cmds...)
		}
		if key == "ctrl+delete" || key == "alt+delete" {
			// Delete the word after the cursor
			input := &m.inputs[m.focusIndex]
			text := input.Value()
			if len(text) > 0 {
				runes := []rune(text)
				// Skip current word
				i := 0
				for i < len(runes) && !unicode.IsSpace(runes[i]) {
					i++
				}
				// Skip spaces
				for i < len(runes) && unicode.IsSpace(runes[i]) {
					i++
				}
				if i < len(runes) {
					// Delete from start to next word
					newText := string(runes[i:])
					input.SetValue(newText)
				}
			}
			return m, tea.Batch(cmds...)
		}

		// Handle save shortcut first
		if key == "enter" && m.focusIndex == len(m.inputs)-1 {
			cmd := m.saveSettings()
			return m, cmd
		}

		// Handle navigation shortcuts
		if key == "up" {
			m.focusIndex--
		} else if key == "down" {
			m.focusIndex++
		}

		if m.focusIndex > len(m.inputs)-1 {
			m.focusIndex = 0
		} else if m.focusIndex < 0 {
			m.focusIndex = len(m.inputs) - 1
		}

		for i := 0; i <= len(m.inputs)-1; i++ {
			if i == m.focusIndex {
				cmds = append(cmds, m.inputs[i].Focus())
				continue
			}
			m.inputs[i].Blur()
		}

		// Pass all keys to inputs for native handling (including ctrl+backspace, ctrl+delete, etc)
		for i := range m.inputs {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}

	default:
		// Non-key updates - pass to all inputs
		for i := range m.inputs {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) saveSettings() tea.Cmd {
	_ = config.SetServerIP(m.inputs[0].Value())
	_ = config.SetPairingCode(m.inputs[1].Value())
	_ = config.SetDownloadFolder(m.inputs[2].Value())

	err := config.Save()
	if err != nil {
		m.statusMsg = fmt.Sprintf("%s Save failed: %v", IconError, err)
		return nil
	}

	// Re-initialize session with new values
	if m.session != nil {
		m.session.Close()
	}
	cfg := config.Get()
	m.session = session.New(cfg.ServerIP, cfg.PairingCode)
	m.apiClient = m.session.APIClient()
	m.wsClient = m.session.WSClient()
	m.fileOps = fileops.New(m.apiClient, cfg.DownloadFolder)

	// Re-initialize clipOps and historyOps with new apiClient
	m.clipOps = clipboardops.New(m.apiClient)
	m.historyOps = historyops.New(m.apiClient)

	// Re-register WebSocket Callbacks with nil checks
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
	m.wsClient.SetOnClipGuestUpdate(func() {
		if m.program != nil {
			m.program.Send(wsClipGuestUpdateMsg{})
		}
	})
	m.wsClient.SetOnStatusChange(func(status string) {
		if m.program != nil {
			m.program.Send(wsStatusMsg{status: status})
		}
	})

	m.statusMsg = Branding.GetIcon(IconLoading, "") + "Reconnecting..."

	// Return sessionReconnectedMsg to signal that the session was changed
	// The Update loop will handle refreshing the data with new session
	return func() tea.Msg {
		return sessionReconnectedMsg{}
	}
}

func (m *AppModel) renderSettings() string {
	var b strings.Builder

	labels := []string{"Server IP", "Pairing Code", "Download Folder"}

	for i := range m.inputs {
		labelStyle := LabelStyle
		inputStyle := InputStyle
		if i == m.focusIndex {
			labelStyle = FocusLabelStyle
			inputStyle = FocusedInputStyle
		}

		b.WriteString(labelStyle.Render(labels[i]))
		b.WriteString("\n")
		b.WriteString(inputStyle.Render(m.inputs[i].View()))
		if i < len(m.inputs)-1 {
			b.WriteString("\n\n")
		}
	}

	hint := DimTextStyle.Render("\nPress Enter on last field or use Up/Down.")
	formContent := b.String()
	
	// Create Dashboard Panel
	roleStr := "Admin"
	if m.isGuestMode {
		roleStr = "Guest"
	}
	
	dashboardContent := lipgloss.JoinVertical(lipgloss.Left,
		HeaderStyle.Render("Session Dashboard"),
		"Role: " + roleStr,
		"Uptime: Active",
		"Status: " + m.statusMsg,
		"",
		DimTextStyle.Render("Changes will reconnect"),
		DimTextStyle.Render("session automatically."),
	)
	
	// Determine panel widths based on overall content width
	_, totalContentWidth, _ := m.calcLayout()
	panelWidth := (totalContentWidth / 2) - 4
	if panelWidth < 20 { panelWidth = 20 }

	settingsPanel := PanelStyle.Width(panelWidth).Render(formContent)
	dashboardPanel := PanelStyle.Width(panelWidth).Render(dashboardContent)
	
	splitView := lipgloss.JoinHorizontal(lipgloss.Top, settingsPanel, " ", dashboardPanel)
	
	return lipgloss.JoinVertical(
		lipgloss.Center,
		splitView,
		hint,
	)
}
