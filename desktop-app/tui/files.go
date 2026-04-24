package tui

import (
	"context"
	"desktop-app/api"
	"desktop-app/config"
	"desktop-app/platform"
	"desktop-app/presentation"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type filesListItem struct {
	file api.FileInfo
	app  *AppModel
}

func (i filesListItem) Title() string {
	if i.file.Name == ".." {
		return "↖️ .."
	}
	
	prefix := ""
	if i.app != nil && i.app.selectedFiles[i.file.Name] {
		prefix = "[x] "
	} else {
		prefix = "[ ] "
	}

	icon := presentation.FileIcon(i.file.Name, i.file.IsDirectory) + " "
	
	return prefix + icon + filepath.Base(i.file.Name)
}

func (i filesListItem) Description() string { 
	if i.file.Name == ".." || i.file.IsDirectory {
		return "Directory"
	}
	return presentation.FormatSize(i.file.Size) 
}

func (i filesListItem) FilterValue() string { return filepath.Base(i.file.Name) }

func (m *AppModel) fetchFilesCmd() tea.Cmd {
	return func() tea.Msg {
		files, err := m.fileOps.ListFiles(context.Background(), m.currentPath)
		if err != nil {
			return filesOpResultMsg{err: err}
		}
		return filesLoadedMsg{files: files}
	}
}

func (m *AppModel) updateFilesList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// If filepicker is active
	if m.showFilePicker {
		m.filePicker, cmd = m.filePicker.Update(msg)
		
		if didSelect, path := m.filePicker.DidSelectFile(msg); didSelect {
			m.isLoading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + fmt.Sprintf("Uploading %s...", filepath.Base(path))
			m.showFilePicker = false // close picker
			
			// If we are deep inside a folder, append that to the base name
			uploadName := filepath.Base(path)
			if m.currentPath != "" {
				uploadName = m.currentPath + "/" + uploadName
			}

			return m, tea.Batch(cmd, func() tea.Msg {
				f, err := os.Open(path)
				if err != nil {
					return filesOpResultMsg{err: err}
				}
				defer f.Close()

				stat, err := f.Stat()
				if err != nil {
					return filesOpResultMsg{err: err}
				}

				progressReader := NewProgressReader(f, stat.Size(), m.program)

				err = m.fileOps.UploadFile(context.Background(), uploadName, progressReader)
				if err != nil {
					return filesOpResultMsg{err: err}
				}
				return filesOpResultMsg{msg: fmt.Sprintf("Uploaded: %s", filepath.Base(path))}
			})
		}
		
		// cancel picker with ESC
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
			m.showFilePicker = false
			return m, cmd
		}
		
		return m, cmd
	}

	// Normal remote files list
	switch msg := msg.(type) {
	case filesLoadedMsg:
		m.isLoading = false
		var listItems []list.Item
		
		if m.currentPath != "" {
			listItems = append(listItems, filesListItem{file: api.FileInfo{Name: "..", IsDirectory: true}, app: m})
		}
		
		for _, f := range msg.files {
			if m.isGuestMode && !strings.HasPrefix(f.Name, "Public/") {
				continue
			}
			listItems = append(listItems, filesListItem{file: f, app: m})
		}
		cmd = m.filesList.SetItems(listItems)
		m.statusMsg = Branding.GetIcon(IconSuccess, "") + fmt.Sprintf("Loaded %d files", len(msg.files))
		return m, cmd

	case filesOpResultMsg:
		m.isLoading = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s Error: %v", IconError, msg.err)
			return m, nil
		}
		m.statusMsg = Branding.GetIcon(IconSuccess, "") + msg.msg
		m.toastMsg = msg.msg
		m.selectedFiles = make(map[string]bool) // Clear selection on success
		return m, tea.Batch(m.fetchFilesCmd(), m.clearToastCmd())

	case tea.KeyMsg:
		if m.filesList.FilterState() == list.Filtering {
			m.filesList, cmd = m.filesList.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case " ":
			if i, ok := m.filesList.SelectedItem().(filesListItem); ok {
				if i.file.Name != ".." {
					m.selectedFiles[i.file.Name] = !m.selectedFiles[i.file.Name]
					return m, nil
				}
			}
		case "ctrl+a":
			// Select all files
			items := m.filesList.Items()
			for _, item := range items {
				if f, ok := item.(filesListItem); ok && f.file.Name != ".." {
					m.selectedFiles[f.file.Name] = true
				}
			}
			return m, nil
		case "esc":
			// Clear selection
			m.selectedFiles = make(map[string]bool)
			return m, nil
		case "backspace":
			if m.currentPath != "" {
				parts := strings.Split(m.currentPath, "/")
				if len(parts) > 1 {
					m.currentPath = strings.Join(parts[:len(parts)-1], "/")
				} else {
					m.currentPath = ""
				}
				m.selectedFiles = make(map[string]bool)
				return m, m.fetchFilesCmd()
			}
		case "enter":
			if i, ok := m.filesList.SelectedItem().(filesListItem); ok {
				// Navigate Up
				if i.file.Name == ".." {
					parts := strings.Split(m.currentPath, "/")
					if len(parts) > 1 {
						m.currentPath = strings.Join(parts[:len(parts)-1], "/")
					} else {
						m.currentPath = ""
					}
					m.selectedFiles = make(map[string]bool)
					return m, m.fetchFilesCmd()
				}
				
				// Navigate Down
				if i.file.IsDirectory {
					m.currentPath = i.file.Name
					m.selectedFiles = make(map[string]bool)
					return m, m.fetchFilesCmd()
				}
				
				// Download logic (Multi-select or single)
				filesToDownload := []api.FileInfo{}
				if len(m.selectedFiles) > 0 {
					for _, item := range m.filesList.Items() {
						if fItem, ok := item.(filesListItem); ok && m.selectedFiles[fItem.file.Name] {
							filesToDownload = append(filesToDownload, fItem.file)
						}
					}
				} else {
					filesToDownload = append(filesToDownload, i.file)
				}

			m.isLoading = true
			m.isDownloading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + fmt.Sprintf("Downloading %d files...", len(filesToDownload))
			
			return m, func() tea.Msg {
				for _, f := range filesToDownload {
					err := m.fileOps.DownloadWithProgress(context.Background(), f, func(percent float64) {
						if m.program != nil {
							m.program.Send(progressMsg{percent: percent, isDownload: true})
						}
					})
					if err != nil {
						return filesOpResultMsg{err: err}
					}
				}
				return filesOpResultMsg{msg: fmt.Sprintf("Downloaded %d files", len(filesToDownload))}
			}
			}
		case "d", "delete":
			if i, ok := m.filesList.SelectedItem().(filesListItem); ok {
				if i.file.Name == ".." {
					return m, nil
				}
				
				filesToDelete := []string{}
				if len(m.selectedFiles) > 0 {
					for name, selected := range m.selectedFiles {
						if selected {
							filesToDelete = append(filesToDelete, name)
						}
					}
				} else {
					filesToDelete = append(filesToDelete, i.file.Name)
				}

				m.showConfirm = true
				m.pendingActionLabel = fmt.Sprintf("delete %d files", len(filesToDelete))
				m.pendingActionTarget = strings.Join(filesToDelete, ", ")
				if len(m.pendingActionTarget) > 40 {
					m.pendingActionTarget = m.pendingActionTarget[:37] + "..."
				}

				m.pendingAction = func() tea.Cmd {
					return func() tea.Msg {
						for _, fname := range filesToDelete {
							err := m.fileOps.DeleteFile(context.Background(), fname)
							if err != nil {
								return filesOpResultMsg{err: err}
							}
						}
						return filesOpResultMsg{msg: fmt.Sprintf("Deleted %d files", len(filesToDelete))}
					}
				}
				return m, nil
			}
		case "o":
			return m, func() tea.Msg {
				_ = platform.OpenFolder(config.Current.DownloadFolder)
				return nil
			}
		case "u":
			m.showFilePicker = true
			return m, m.filePicker.Init()
		case "r":
			m.isLoading = true
			m.statusMsg = Branding.GetIcon(IconLoading, "") + "Fetching files..."
			return m, m.fetchFilesCmd()
		}
		m.filesList, cmd = m.filesList.Update(msg)
	}
	return m, cmd
}
