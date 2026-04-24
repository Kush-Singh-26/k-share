//go:build windows

package platform

import "os/exec"

func OpenFolder(path string) error {
	return exec.Command("explorer", path).Start()
}

func OpenURL(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
