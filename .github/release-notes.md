# Tagger v1.7.1 — 修复封面曲目的播放键被遮挡

本仓库是 [Ericwyn/tagger](https://github.com/Ericwyn/tagger) 的 fork（`ttbb1211/tagger`），面向中文音乐库整理：整轨 CUE、中文数据源、歌词 `.lrc` 等。

本版是 v1.7.0 的小补丁，**只改前端样式**，音频解码、扫描与写入逻辑与 v1.7.0 完全一致。

## 本版变化

**修复：有内嵌封面的曲目，封面上的「播放/暂停」按钮一直看不见。**

曲目列表每一行的封面在悬停或正在播放时会浮出 ▶/⏸ 按钮。该按钮与封面是同一容器里的两个绝对定位元素，但封面 `<img>` 带 `z-index:4`、按钮**没有 `z-index`**，于是只要曲目有内嵌封面，按钮就被封面压在下层、完全看不到——hover 与不 hover 外观一模一样。整轨 CUE 的虚拟轨道**没有内嵌封面**（走占位图，不生成 `<img>`），所以只有它一直正常。这正是「整轨专辑有按钮、单曲却没有」的原因。

- 给按钮补 `z-index:5`，使其稳定浮在封面之上（`frontend/src/index.css`）。
- 顺带把按钮那层蒙版由 78% 调淡到 **60%**：既看得清居中的 ▶/⏸，又不会把封面糊成一片浅色。

## 验证

- 用真实 `index.css` 与真实封面在 headless Chrome 下做对照渲染：修复前有封面的行无按钮、无封面的行有按钮；修复后两种行都正常显示 ▶/⏸，且默认态（未悬停）封面 100% 不受影响。
- 前端类型检查、`217/217` 单元测试与 `vite build` 全部通过；Go 全量测试与 Linux/Windows 目标平台 vet 通过。本版无后端改动。

## 下载

| 文件 | 用途 |
| --- | --- |
| `tagger_setup_1.7.1_windows_amd64.exe` | Windows x86_64 安装包 |
| `tagger_1.7.1_windows_amd64.zip` / 同名 `.sha256` | Windows 便携版及校验 |
| `tagger_1.7.1_linux_amd64.tar.gz` / 同名 `.sha256` | Linux x86_64 |
| `tagger_1.7.1_linux_arm64.tar.gz` / 同名 `.sha256` | Linux arm64 |

Windows 版依赖 WebView2 Runtime；旧版覆盖安装时注意选择之前的安装目录。服务端部署后应核对二进制版本与前端资产哈希。源码构建可运行 `make build VERSION=1.7.1`。

> 从 v1.6.13 及更早升级的用户：**v1.7.0 起 ALAC 编码的 M4A 已可在网页与 Windows 桌面端直接播放**（Go 后端实时软解为 PCM、流式下发 WAV，支持拖动进度）。ALAC 播放本身不需要重新扫描。
