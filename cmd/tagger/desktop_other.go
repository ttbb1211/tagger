//go:build !windows

package main

// 非 Windows 构建（Docker、Linux）没有桌面窗口：保持纯服务形态。
func hideConsoleWindow()          {}
func openWebUI(url string)        {}
func ensureDataDirExists(string)  {}

func runWebViewWindow(url, dataDir string) bool { return false }
