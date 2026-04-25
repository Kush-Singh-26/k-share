package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

type clipboardLoadedMsg struct {
	text string
	err  error
}

func (m *AppModel) fetchClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		text, err := m.clipOps.FetchText(ctx, m.clipboardChannel)
		return clipboardLoadedMsg{text: text, err: err}
	}
}

type imageClipboardLoadedMsg struct {
	data []byte
	err  error
}

func (m *AppModel) fetchClipboardImageCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		data, err := m.clipOps.FetchImage(ctx)
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

			m.clipTextArea.SetValue(msg.text)

			timestamp := time.Now().Format("15:04:05")
			newContent := fmt.Sprintf("[%s] Remote: %s", timestamp, msg.text)

			m.clipLog = append(m.clipLog, newContent)
			if len(m.clipLog) > 500 {
				m.clipLog = m.clipLog[1:]
			}
			m.clipViewport.SetContent(strings.Join(m.clipLog, "\n"))
			m.clipViewport.GotoBottom()
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
			m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Image received (preview disabled)"
			timestamp := time.Now().Format("15:04:05")
			newContent := fmt.Sprintf("[%s] Remote Image received (%d bytes)", timestamp, len(msg.data))
			m.clipLog = append(m.clipLog, newContent)
			if len(m.clipLog) > 500 {
				m.clipLog = m.clipLog[1:]
			}
			m.clipViewport.SetContent(strings.Join(m.clipLog, "\n"))
			m.clipViewport.GotoBottom()
		}

	case tea.KeyMsg:
		key := msg.String()

		// Manual word deletion for PowerShell compatibility
		// (PowerShell often sends unexpected codes that aren't captured by Bubble's KeyMap)
		// This is a fallback that assumes cursor is at the end of the text (most common case)
		switch key {
		case "ctrl+backspace", "ctrl+h", "alt+backspace":
			// Delete the word before the cursor (i.e. the last word if cursor is at end)
			text := m.clipTextArea.Value()
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
					m.clipTextArea.SetValue(newText)
				}
			}
			return m, nil
		case "ctrl+delete", "alt+delete":
			// Delete the word after the cursor
			text := m.clipTextArea.Value()
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
					m.clipTextArea.SetValue(newText)
				}
			}
			return m, nil
		}

		// Handle clipboard-specific shortcuts first
		switch key {
		case "ctrl+s":
			text := m.clipTextArea.Value()
			if text != "" {
				m.isLoading = true
				m.statusMsg = Branding.GetIcon(IconLoading, "") + "Pushing text..."
				return m, tea.Batch(func() tea.Msg { return m.spinner.Tick() }, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					err := m.clipOps.PushText(ctx, text, m.clipboardChannel)
					if err != nil {
						return clipOpResultMsg{err: err}
					}
					return localClipboardPushedMsg{text: text}
				})
			}
			return m, nil
		case "ctrl+r":
			m.isLoading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Refreshing all..."
			return m, tea.Batch(func() tea.Msg { return m.spinner.Tick() }, m.refreshAllCmd())
		case "ctrl+i":
			m.isLoading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Fetching image preview..."
			return m, tea.Batch(func() tea.Msg { return m.spinner.Tick() }, m.fetchClipboardImageCmd())
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
			return m, nil
		}

		// Pass all other keys (including ctrl+left/right, ctrl+backspace, ctrl+delete)
		// directly to textarea for native handling
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
