//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	user32                    = syscall.NewLazyDLL("user32.dll")
	procShowWindow            = user32.NewProc("ShowWindow")
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
)

const (
	swHide         = 0
	spiGetWorkArea = 0x0030 // SystemParametersInfoW 的动作码：取主屏可用工作区
)

// windowSizeForScreen 按屏幕可用工作区（已扣掉任务栏）算初始窗口尺寸。
//
// 写死 1280x820 时，顶栏「导航 + 播放器」只剩几十像素间距，看着挤成一团；
// 最大化就宽松。这里改成按屏幕比例给：取工作区 88%，夹在 1280x820 ~ 1760x1120
// 之间；小屏时再退让到不超过工作区本身，避免窗口跑到屏幕外面。
// 取不到工作区时回退到原来的 1280x820。
// 返回值是 uint —— webview2.WindowOptions 的 Width/Height 就是这个类型。
func windowSizeForScreen() (uint, uint) {
	const (
		minWidth, minHeight = 1280, 820
		maxWidth, maxHeight = 1760, 1120
		ratio               = 88
	)

	var area struct {
		Left, Top, Right, Bottom int32
	}
	if ret, _, _ := procSystemParametersInfoW.Call(
		spiGetWorkArea, 0, uintptr(unsafe.Pointer(&area)), 0,
	); ret == 0 {
		return minWidth, minHeight
	}
	screenWidth := int(area.Right - area.Left)
	screenHeight := int(area.Bottom - area.Top)
	if screenWidth <= 0 || screenHeight <= 0 {
		return minWidth, minHeight
	}

	width := clampInt(screenWidth*ratio/100, minWidth, maxWidth)
	height := clampInt(screenHeight*ratio/100, minHeight, maxHeight)
	// 小屏兜底：宁可铺满，也不要超出屏幕
	if width > screenWidth {
		width = screenWidth
	}
	if height > screenHeight {
		height = screenHeight
	}
	return uint(width), uint(height)
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

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
	windowWidth, windowHeight := windowSizeForScreen()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(dataDir, "webview"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Tagger " + appVersion,
			Width:  windowWidth,
			Height: windowHeight,
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
