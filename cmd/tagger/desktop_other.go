//go:build !windows

package main

// 非 Windows 构建（Docker、Linux）没有桌面窗口与托盘：保持纯服务形态。
func hideConsoleWindow()          {}
func openWebUI(url string)        {}
func ensureDataDirExists(string)  {}
func startTray()                  {}

func runWebViewWindow(url, dataDir string) bool { return false }

var (
	trayReopen = make(chan struct{}, 1)
	trayQuit   = make(chan struct{}, 1)
)
