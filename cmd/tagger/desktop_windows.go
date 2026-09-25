//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procGetConsoleWindow = user32.NewProc("GetConsoleWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
)

const swHide = 0

// hideConsoleWindow hides the attached console so a double-clicked
// tagger.exe behaves like a background app. Debug with -hide-console=false.
func hideConsoleWindow() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swHide)
	}
}

// openWebUI opens the default browser; errors are intentionally ignored
// since the URL is always reachable manually via the Start Menu entry.
func openWebUI(url string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
