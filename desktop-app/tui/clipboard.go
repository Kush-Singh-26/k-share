package tui

import (
	"context"
	"desktop-app/presentation"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.design/x/clipboard"
)

type clipboardLoadedMsg struct {
	text string
	err  error
}

func (m *AppModel) fetchClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		text, err := m.clipOps.FetchText(context.Background(), m.clipboardChannel)
		return clipboardLoadedMsg{text: text, err: err}
	}
}

type imageClipboardLoadedMsg struct {
	data []byte
	err  error
}

func (m *AppModel) fetchClipboardImageCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := m.clipOps.FetchImage(context.Background())
		return imageClipboardLoadedMsg{data: data, err: err}
	}
}

type localClipboardPushedMsg struct {
	text string
}

func (m *AppModel) updateClipboardView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case clipboardLoadedMsg:
		m.isLoading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s Clipboard Error: %v", IconError, msg.err)
		} else {
			m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Clipboard synced"
			m.toastMsg = "Clipboard Synced!"
			
			m.clipTextArea.SetValue(msg.text)
			
			timestamp := time.Now().Format("15:04:05")
			newContent := fmt.Sprintf("[%s] Remote: %s", timestamp, msg.text)
			
			m.clipLog = append(m.clipLog, newContent)
			if len(m.clipLog) > 500 {
				m.clipLog = m.clipLog[1:]
			}
			m.clipViewport.SetContent(strings.Join(m.clipLog, "\n"))
			m.clipViewport.GotoBottom()
			
			if msg.text != "" {
				clipboard.Write(clipboard.FmtText, []byte(msg.text))
				m.toastMsg = "Remote Clipboard Auto-copied!"
			}
			
			cmd = tea.Batch(cmd, m.clearToastCmd())
		}
		
	case localClipboardPushedMsg:
		m.isLoading = false
		m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Local clipboard pushed"
		m.toastMsg = "Clipboard Pushed!"
		m.clipTextArea.SetValue(msg.text)
		
		timestamp := time.Now().Format("15:04:05")
		newContent := fmt.Sprintf("[%s] Local: %s", timestamp, msg.text)
		
		m.clipLog = append(m.clipLog, newContent)
		if len(m.clipLog) > 500 {
			m.clipLog = m.clipLog[1:]
		}
		m.clipViewport.SetContent(strings.Join(m.clipLog, "\n"))
		m.clipViewport.GotoBottom()
		
		if msg.text != "" {
			clipboard.Write(clipboard.FmtText, []byte(msg.text))
			m.toastMsg = "Local Clipboard Pushed & Copied!"
		}
		
		cmd = tea.Batch(cmd, m.clearToastCmd())

	case clipOpResultMsg:
		m.isLoading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s Error: %v", IconError, msg.err)
		} else {
			m.statusMsg = Branding.GetIcon(IconSuccess, "") + msg.msg
			m.toastMsg = msg.msg
			cmd = m.clearToastCmd()
		}

	case imageClipboardLoadedMsg:
		m.isLoading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s Image Error: %v", IconError, msg.err)
		} else if len(msg.data) > 0 {
			m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Image preview loaded"
			timestamp := time.Now().Format("15:04:05")
			// Scale image down to ~60 chars wide to fit the TUI viewport
			ansi := presentation.ANSIImage(msg.data, 60)
			newContent := fmt.Sprintf("[%s] Remote Image:\n%s", timestamp, ansi)
			m.clipLog = append(m.clipLog, newContent)
			if len(m.clipLog) > 500 {
				m.clipLog = m.clipLog[1:]
			}
			m.clipViewport.SetContent(strings.Join(m.clipLog, "\n"))
			m.clipViewport.GotoBottom()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s":
			text := m.clipTextArea.Value()
			if text != "" {
				m.isLoading = true
				m.statusMsg = Branding.GetIcon(IconLoading, "") + "Pushing text..."
				return m, func() tea.Msg {
					err := m.clipOps.PushText(context.Background(), text, m.clipboardChannel)
					if err != nil {
						return clipOpResultMsg{err: err}
					}
					return localClipboardPushedMsg{text: text}
				}
			}
		case "ctrl+r":
			m.isLoading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Fetching clipboard..."
			return m, m.fetchClipboardCmd()
		case "ctrl+i":
			m.isLoading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Fetching image preview..."
			return m, m.fetchClipboardImageCmd()
		case "ctrl+g":
			// Toggle clipboard channel (admin only)
			if !m.isGuestMode {
				if m.clipboardChannel == "" {
					m.clipboardChannel = "guest"
					m.statusMsg = "Switched to Guest Clipboard"
				} else {
					m.clipboardChannel = ""
					m.statusMsg = "Switched to Private Clipboard"
				}
				return m, m.fetchClipboardCmd()
			}
		}

		var taCmd tea.Cmd
		m.clipTextArea, taCmd = m.clipTextArea.Update(msg)

		var vpCmd tea.Cmd
		m.clipViewport, vpCmd = m.clipViewport.Update(msg)

		return m, tea.Batch(taCmd, vpCmd)

	default:
		var taCmd tea.Cmd
		m.clipTextArea, taCmd = m.clipTextArea.Update(msg)

		var vpCmd tea.Cmd
		m.clipViewport, vpCmd = m.clipViewport.Update(msg)

		return m, tea.Batch(taCmd, vpCmd)
	}
	return m, cmd
}
