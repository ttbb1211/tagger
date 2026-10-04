# Tagger v1.7.2 — 播放器歌名改用主题松绿，系统信息指向本仓库

本仓库是 [Ericwyn/tagger](https://github.com/Ericwyn/tagger) 的 fork（`ttbb1211/tagger`），面向中文音乐库整理：整轨 CUE、中文数据源、歌词 `.lrc` 等。

本版是 v1.7.1 的小补丁，**只改前端样式与一个链接常量**，音频解码、扫描与写入逻辑与 v1.7.1 完全一致（后端除版本号外无改动）。

## 本版变化

**① 顶栏播放器的歌名换成主题松绿，正在播什么一眼就能看到。**

播放器的歌名（如 `Plastic Love`）此前**没有指定颜色**，继承主题墨色 `--ink`。它虽然对比度最高，但在顶栏里跟右侧时间、下方副标题（艺术家 · 专辑）一样「沉」，扫一眼分不出主次。

- 给歌名显式指定 `color: var(--success)`（`frontend/src/index.css`），**字号与字重刻意保持不变**（12px / bold）。
- 用**主题变量**而不是硬编码色值：四套主题自动取各自的松绿，无需分别维护——
  象牙白 `#27645b`、灰绿色 `#2f6d5b`、陶土纸 `#46715e`、午夜墨 `#61a99d`。
- 对比度实测：象牙白底 `#f7f6f1` 上为 `6.34 : 1`，午夜墨底 `#181916` 上为 `6.45 : 1`，均通过 WCAG AA 正文门槛（4.5:1）。

**② 设置 →「系统与安全」→ 系统信息里的 `GitHub` 链接指向本仓库。**

原先指向上游 `github.com/Ericwyn/tagger`，现改为 **`github.com/ttbb1211/tagger`**（`frontend/src/pages/SettingsPage.tsx`）。

## 验证

- 前端类型检查、`217/217` 单元测试（含设置页仓库链接断言）与 `vite build` 全部通过；构建产物确认含 `.global-player-copy strong{color:var(--success);font-size:12px}`。
- 本版无后端逻辑改动，Go 代码与 v1.7.1 完全一致。

## 下载

| 文件 | 用途 |
| --- | --- |
| `tagger_setup_1.7.2_windows_amd64.exe` | Windows x86_64 安装包 |
| `tagger_1.7.2_windows_amd64.zip` / 同名 `.sha256` | Windows 便携版及校验 |
| `tagger_1.7.2_linux_amd64.tar.gz` / 同名 `.sha256` | Linux x86_64 |
| `tagger_1.7.2_linux_arm64.tar.gz` / 同名 `.sha256` | Linux arm64 |

Windows 版依赖 WebView2 Runtime；旧版覆盖安装时注意选择之前的安装目录。服务端部署后应核对二进制版本与前端资产哈希。源码构建可运行 `make build VERSION=1.7.2`。

> 从 v1.6.13 及更早升级的用户：**v1.7.0 起 ALAC 编码的 M4A 已可在网页与 Windows 桌面端直接播放**（Go 后端实时软解为 PCM、流式下发 WAV，支持拖动进度）。ALAC 播放本身不需要重新扫描。
