# Tagger v1.7.3 — 新品牌图标（黑胶唱片）+ 播放器歌名改用宝蓝

本仓库是 [Ericwyn/tagger](https://github.com/Ericwyn/tagger) 的 fork（`ttbb1211/tagger`），面向中文音乐库整理：整轨 CUE、中文数据源、歌词 `.lrc` 等。

本版是 v1.7.2 的小补丁，**只改品牌图标与前端样式**，音频解码、扫描与写入逻辑与 v1.7.2 完全一致（后端除版本号外无改动）。

## 本版变化

**① 品牌图标换成黑胶唱片。**

原先的标记是「实心墨圆 + 一条橙色标签条」，抽象、看不出是什么。现换成**黑胶唱片**：墨色盘面 + 两道沟槽 + **红底中心标签** + 米白中孔。

中心红标不是随手挑的红，而是取自**邓丽君《GREATEST HITS》复黑版 Polydor 红标的 600DPI 扫描件**——对标签环做环形采样，主导色为 `#db3a23`（略偏橙的 Polydor 红）。

**五处一并替换**：

| 位置 | 文件 |
| --- | --- |
| 浏览器标签页图标 | `frontend/public/favicon.svg` |
| 浏览器回退图标（16/24/32/48/64 五档） | `frontend/public/favicon.ico` |
| Windows 可执行文件 / 窗口 / 任务栏图标 | `cmd/tagger/icon.ico` → `cmd/tagger/rsrc_windows_amd64.syso` |
| 安装包与快捷方式图标 | `packaging/windows/tagger.iss` 的 `SetupIconFile`（指向上面的 `favicon.ico`） |
| 应用内顶栏小圆标 | `frontend/src/components/TaggerMark.tsx` |
| 仓库品牌资产 | `frontend/public/brand/tagger-mark{,-mono}.svg` |

两处细节值得说明：

- **深色适配**：`favicon.svg` 与品牌资产带 `prefers-color-scheme: dark` 分支（盘面转米白 `#f0eee5`、沟槽转浅灰 `#c9c6bc`、中孔转深 `#181916`）；顶栏小圆标走主题变量（`--brand-red` / `--brand-groove`），午夜墨主题下同样自动翻转。
- **沟槽描边用 `vector-effect: non-scaling-stroke` 恒为 1.2 设备像素**。这不是洁癖：这个标在顶栏只显示 **31px**，若按 viewBox 单位描边，沟槽线宽只有 **0.34 设备像素**，实际渲染出来整条看不见；而按比例缩放又会让大尺寸下的沟槽粗到 4px 像描边。已用 headless Chrome 在 256px 下实测：不加该属性时沟槽 4px，加了恒为 1px，符合预期。

**② 顶栏播放器的歌名改用宝蓝（取代 v1.7.2 的松绿）。**

播放器的歌名（如 `Plastic Love`）此前**没有指定颜色**，继承主题墨色 `--ink`。它虽然对比度最高，但在顶栏里跟右侧时间、下方副标题（艺术家 · 专辑）一样「沉」，扫一眼分不出主次。v1.7.2 曾改为主题松绿 `--success`，本版再改为**宝蓝**。

- 新开专用变量 `--player-title`，**刻意不复用 `--blue`**——`--blue` 同时被任务页 `.job-kind.kind-scan` 扫描徽章占用，直接改会连带改到那里。
- 四套主题：象牙白 / 灰绿 / 陶土纸 `#0f52ba`，午夜墨提亮为 `#6f9df0`。
- **字号 12px 与字重 bold 刻意保持不变**，只换颜色。
- 对比度实测：象牙白 `6.63 : 1` / 灰绿 `6.51 : 1` / 陶土纸 `6.43 : 1` / 午夜墨 `6.52 : 1`，均通过 WCAG AA 正文门槛（4.5:1）。

## 验证

- 前端类型检查（`tsc --noEmit`）、`217/217` 单元测试与 `vite build` 全部通过。
- 构建产物确认含新规则 `.global-player-copy strong{color:var(--player-title);font-size:12px}` 与品牌变量 `--brand-red:#db3a23`。
- 图标几何用 headless Chrome 逐尺寸实测（16 / 31 / 64 / 128 / 256px，亮色与深色两套），ICO 五档尺寸解析校验通过，Windows 可执行文件内嵌图标经二进制比对确认已换成新版。
- 本版无后端逻辑改动，Go 代码与 v1.7.2 一致（仅版本号）。

## 下载

| 文件 | 用途 |
| --- | --- |
| `tagger_setup_1.7.3_windows_amd64.exe` | Windows x86_64 安装包 |
| `tagger_1.7.3_windows_amd64.zip` / 同名 `.sha256` | Windows 便携版及校验 |
| `tagger_1.7.3_linux_amd64.tar.gz` / 同名 `.sha256` | Linux x86_64 |
| `tagger_1.7.3_linux_arm64.tar.gz` / 同名 `.sha256` | Linux arm64 |

Windows 版依赖 WebView2 Runtime；旧版覆盖安装时注意选择之前的安装目录。服务端部署后应核对二进制版本与前端资产哈希。源码构建可运行 `make build VERSION=1.7.3`。

> 从 v1.6.13 及更早升级的用户：**v1.7.0 起 ALAC 编码的 M4A 已可在网页与 Windows 桌面端直接播放**（Go 后端实时软解为 PCM、流式下发 WAV，支持拖动进度）。ALAC 播放本身不需要重新扫描。
