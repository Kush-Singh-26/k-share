package tui

import "desktop-app/api"

// Typed result messages to preventCase contamination between Update handlers
// (Issue #13)

type filesLoadedMsg struct {
	files []api.FileInfo
}

type filesOpResultMsg struct {
	msg    string
	status string
	err    error
}

type historyLoadedMsg struct {
	items []api.HistoryItem
}

type historyOpResultMsg struct {
	msg    string
	status string
	err    error
}

type clipOpResultMsg struct {
	msg    string
	status string
	err    error
}

type wsClipUpdateMsg struct{}
type wsClipGuestUpdateMsg struct{}
type wsHistoryUpdateMsg struct{}
type wsFilesUpdateMsg struct{}

type wsStatusMsg struct {
	status string
}

type connectionResultMsg struct {
	role           string
	err            error
}

type sessionReconnectedMsg struct{}

type trustResultMsg struct {
	trusted bool
}

type discoveryResultMsg struct {
	ip   string
	err  error
}

type clipboardReadMsg struct {
	text string
	data []byte
}
