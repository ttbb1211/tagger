//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	webview2 "github.com/jchv/go-webview2"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	user32               = syscall.NewLazyDLL("user32.dll")
	procShowWindow       = user32.NewProc("ShowWindow")
)

const swHide = 0

// hideConsoleWindow 隐藏随启动附带的控制台窗口（双击启动场景）。
// 注意 GetConsoleWindow 在 kernel32 而非 user32。
func hideConsoleWindow() {
	if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swHide)
	}
}

// openWebUI 用系统默认浏览器打开界面（WebView2 不可用时的回退）。
func openWebUI(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// runWebViewWindow 在原生 WebView2 窗口中承载界面，并阻塞至窗口关闭。
// 返回 false 表示 WebView2 运行时不可用，调用方应回退浏览器方案。
// 窗口关闭（点标题栏 ×）后本函数返回，调用方随即结束进程——不常驻托盘。
func runWebViewWindow(url, dataDir, appVersion string) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(dataDir, "webview"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Tagger " + appVersion,
			Width:  1280,
			Height: 820,
			Center: true,
			IconId: 1, // rsrc_windows_amd64.syso 内嵌的品牌图标
		},
	})
	if w == nil {
		return false
	}
	w.SetTitle("Tagger " + appVersion)
	w.Navigate(url)
	defer w.Destroy()
	w.Run()
	return true
}

// ensureDataDirExists 保证 WebView2 的用户数据目录存在。
func ensureDataDirExists(dataDir string) {
	_ = os.MkdirAll(filepath.Join(dataDir, "webview"), 0o755)
}
