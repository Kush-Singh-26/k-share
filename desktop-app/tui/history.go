package tui

import (
	"context"
	"desktop-app/api"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		items, err := m.historyOps.Load(ctx)
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
		if len(listItems) == 0 {
			m.historyList.SetItems([]list.Item{})
		} else {
			cmd = m.historyList.SetItems(listItems)
		}
		m.statusMsg = Branding.GetIcon(IconSuccess, "") + fmt.Sprintf("Loaded %d history items", len(msg.items))
		return m, cmd

	case historyOpResultMsg:
		m.isLoading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s Error: %v", IconError, msg.err)
			// Still refresh to stay in sync
			return m, tea.Batch(m.fetchHistoryCmd(), m.clearToastCmd())
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
				m.statusMsg = Branding.GetIcon(IconSuccess, "") + "Copied: " + i.item.Text
				if len(m.statusMsg) > 50 {
					m.statusMsg = m.statusMsg[:47] + "..."
				}
				m.toastMsg = "Copied!"
				return m, tea.Batch(m.copyToClipboardCmd(i.item.Text), m.clearToastCmd())
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
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Refreshing all..."
			return m, tea.Batch(func() tea.Msg { return m.spinner.Tick() }, m.refreshAllCmd())
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
						ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
						defer cancel()
						err := m.historyOps.Delete(ctx, i.item.ID)
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
