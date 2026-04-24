//go:build !windows

package platform

import "fmt"

func InstallContextMenu() {
	fmt.Println("Context menu installation is only supported on Windows.")
}

func UninstallContextMenu() {
	fmt.Println("Context menu uninstallation is only supported on Windows.")
}
