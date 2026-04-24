//go:build windows

package platform

import (
	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/sys/windows/registry"
)

func InstallContextMenu() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("❌ Failed to get executable path: %v", err)
	}

	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend`, registry.ALL_ACCESS)
	if err != nil {
		log.Printf("❌ Failed to create registry key in HKCU: %v\n", err)
		return
	}
	defer k.Close()

	if err := k.SetStringValue("", "Send to Phone (K-Share)"); err != nil {
		log.Fatal(err)
	}
	if err := k.SetStringValue("Icon", exe); err != nil {
		log.Fatal(err)
	}

	ck, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend\command`, registry.ALL_ACCESS)
	if err != nil {
		log.Fatal(err)
	}
	defer ck.Close()

	cmd := fmt.Sprintf(`"%s" -send "%%1"`, exe)
	if err := ck.SetStringValue("", cmd); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✅ Context menu installed successfully!")
	time.Sleep(2 * time.Second)
}

func UninstallContextMenu() {
	err := registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend\command`)
	if err != nil {
		log.Printf("⚠️ Failed to delete command key: %v", err)
	}
	err = registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\*\shell\KShareSend`)
	if err != nil {
		log.Printf("⚠️ Failed to delete main key: %v", err)
	} else {
		fmt.Println("✅ Context menu uninstalled successfully!")
	}
	time.Sleep(2 * time.Second)
}
