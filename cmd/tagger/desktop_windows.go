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
	// ★ 启动前先清一次缓存：此刻 WebView2 还没创建，缓存文件一定没被占用，
	// 删除必定成功。关窗时清则不可靠——msedgewebview2.exe 子进程常常还没退干净，
	// 文件被占着删不掉。两处都做：这里是「保证」，关窗那次是「尽量」。
	clearWebViewCache(dataDir)

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
	w.Run()     // 阻塞至窗口关闭（点标题栏 ×）
	w.Destroy() // 发 WM_CLOSE，尽量让 WebView2 先释放
	clearWebViewCache(dataDir)
	return true
}

// clearWebViewCache 删除 WebView2 的磁盘缓存目录。
//
// ★ 为什么必须清：音频响应的 ETag 只由「文件本身」决定（相对路径+大小+mtime+标签）。
// 程序升级改了切片算法、文件却没变时，ETag 与前端 URL 都不变，
// 浏览器（尤其 WebView2）会继续用旧缓存里的字节 —— 2026-10-03 实测症状是
// 「白噪音—正常几秒—白噪音」反复，因为旧错位块与新对齐块混在同一份缓存里交替命中。
// audioSliceScheme / AUDIO_CACHE_SCHEME 版本号能覆盖「以后」的升级，
// 这里再兜一层，保证每次启动都是干净缓存。
//
// 只删 Default/Cache（媒体、图片缓存），保留同级的 Cookies、Local Storage、
// IndexedDB —— 那些丢了会退出登录、丢界面偏好。
// 失败静默：缓存删不掉不该影响启动或关窗退出。
func clearWebViewCache(dataDir string) {
	if dataDir == "" {
		return
	}
	_ = os.RemoveAll(filepath.Join(dataDir, "webview", "EBWebView", "Default", "Cache"))
}

// ensureDataDirExists 保证 WebView2 的用户数据目录存在。
func ensureDataDirExists(dataDir string) {
	_ = os.MkdirAll(filepath.Join(dataDir, "webview"), 0o755)
}
