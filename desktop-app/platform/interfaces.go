package platform

// FolderOpener opens a directory in the platform's file manager.
type FolderOpener interface {
	OpenFolder(path string) error
}

// URLOpener opens a URL in the platform's default browser.
type URLOpener interface {
	OpenURL(url string) error
}

// ClipboardAdapter abstracts platform clipboard behavior.
type ClipboardAdapter interface {
	ReadText() (string, error)
	WriteText(text string) error
	ReadImage() ([]byte, error)
	WriteImage(data []byte) error
}

// Notifier abstracts platform notification behavior.
type Notifier interface {
	Notify(title, body string) error
}

// TrayMenuAdapter abstracts tray or menu integration.
type TrayMenuAdapter interface {
	SetTitle(title string)
	SetTooltip(tooltip string)
	AddAction(label, tooltip string, onClick func())
}
