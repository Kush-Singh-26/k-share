package tui

import (
	"context"
	"desktop-app/api"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"golang.design/x/clipboard"
)

// historyListItem implements the list.Item interface
type historyListItem struct {
	item api.HistoryItem
}

func (i historyListItem) Title() string       { return i.item.Text }
func (i historyListItem) Description() string { return i.item.Timestamp.Format(time.RFC822) }
func (i historyListItem) FilterValue() string { return i.item.Text }

// Command to fetch history
func (m *AppModel) fetchHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.historyOps.Load(context.Background())
		if err != nil {
			return historyOpResultMsg{err: err}
		}
		return historyLoadedMsg{items: items}
	}
}

func (m *AppModel) updateHistoryList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case historyLoadedMsg:
		m.isLoading = false
		var listItems []list.Item
		for _, item := range msg.items {
			listItems = append(listItems, historyListItem{item: item})
		}
		cmd = m.historyList.SetItems(listItems)
		m.statusMsg = Branding.GetIcon(IconSuccess, "") + fmt.Sprintf("Loaded %d history items", len(msg.items))
		return m, cmd

	case historyOpResultMsg:
		m.isLoading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s Error: %v", IconError, msg.err)
			return m, nil
		}
		m.statusMsg = Branding.GetIcon(IconSuccess, "") + msg.msg
		m.toastMsg = msg.msg
		return m, tea.Batch(m.fetchHistoryCmd(), m.clearToastCmd())

	case tea.KeyMsg:
		if m.historyList.FilterState() == list.Filtering {
			m.historyList, cmd = m.historyList.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "enter", "c", "y":
			// Copy to clipboard
			if i, ok := m.historyList.SelectedItem().(historyListItem); ok {
				clipboard.Write(clipboard.FmtText, []byte(i.item.Text))
				m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Copied: " + i.item.Text
				if len(m.statusMsg) > 50 {
					m.statusMsg = m.statusMsg[:47] + "..."
				}
				m.toastMsg = "Copied!"
				return m, m.clearToastCmd()
			}
		case "up", "k":
			m.historyList.CursorUp()
			return m, nil
		case "down", "j":
			m.historyList.CursorDown()
			return m, nil
		case "pgup":
			m.historyList.Paginator.PrevPage()
			return m, nil
		case "pgdown":
			m.historyList.Paginator.NextPage()
			return m, nil
		case "home":
			m.historyList.Select(0)
			return m, nil
		case "end":
			m.historyList.Select(len(m.historyList.Items()) - 1)
			return m, nil
		case "r":
			m.isLoading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Fetching history..."
			return m, m.fetchHistoryCmd()
		case "delete", "d":
			if i, ok := m.historyList.SelectedItem().(historyListItem); ok {
				m.showConfirm = true
				m.pendingActionLabel = "delete this history item"
				m.pendingActionTarget = i.item.Text
				
				// Shorten target if too long
				if len(m.pendingActionTarget) > 40 {
					m.pendingActionTarget = m.pendingActionTarget[:37] + "..."
				}

				m.pendingAction = func() tea.Cmd {
					return func() tea.Msg {
						err := m.historyOps.Delete(context.Background(), i.item.ID)
						if err != nil {
							return historyOpResultMsg{err: err}
						}
						return historyOpResultMsg{msg: "Deleted item"}
					}
				}
				return m, nil
			}
		}
		m.historyList, cmd = m.historyList.Update(msg)
	}
	return m, cmd
}
