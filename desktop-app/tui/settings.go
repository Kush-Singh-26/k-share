package tui

import (
	"desktop-app/clipboardops"
	"desktop-app/config"
	"desktop-app/fileops"
	"desktop-app/historyops"
	"desktop-app/session"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *AppModel) updateSettingsView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "up", "down":
			s := msg.String()
			if s == "enter" && m.focusIndex == len(m.inputs)-1 {
				// Save Settings
				m.saveSettings()
				return m, nil
			}

			// Cycle focus
			if s == "up" {
				m.focusIndex--
			} else {
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
		}

		// Update all inputs with key events
		for i := range m.inputs {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}

	default:
		// Non-key updates
		for i := range m.inputs {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) saveSettings() {
	config.Current.ServerIP = m.inputs[0].Value()
	config.Current.PairingCode = m.inputs[1].Value()
	config.Current.DownloadFolder = m.inputs[2].Value()

	err := config.Save()
	if err != nil {
		m.statusMsg = fmt.Sprintf("%s Save failed: %v", IconError, err)
	} else {
		m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Settings saved"

		// Re-initialize session with new values
		if m.session != nil {
			m.session.Close()
		}
		m.session = session.New(config.Current.ServerIP, config.Current.PairingCode)
		m.apiClient = m.session.APIClient()
		m.wsClient = m.session.WSClient()
		m.fileOps = fileops.New(m.apiClient, config.Current.DownloadFolder)

		// Re-initialize clipOps and historyOps with new apiClient
		m.clipOps = clipboardops.New(m.apiClient)
		m.historyOps = historyops.New(m.apiClient)

		// Re-register WebSocket Callbacks (Issue #5)
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

		// Reconnect WS
		go m.wsClient.Connect()
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
		fmt.Sprintf("Role: %s", roleStr),
		fmt.Sprintf("Uptime: Active"),
		fmt.Sprintf("Status: %s", m.statusMsg),
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
		RenderLogo(totalContentWidth),
		"\n",
		splitView,
		hint,
	)
}
