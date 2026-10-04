# Tagger v1.7.0 — ALAC 软解播放

本仓库是 [Ericwyn/tagger](https://github.com/Ericwyn/tagger) 的 fork（`ttbb1211/tagger`），面向中文音乐库整理：整轨 CUE、中文数据源、歌词 `.lrc` 等。

## 本版变化

**ALAC 编码的 M4A 现在可在网页与 Windows 桌面端内播放。** Chromium/WebView2 本身没有 ALAC 解码器；本版由 Go 后端读取 MP4、按需将 ALAC 解码为 PCM，合成浏览器支持的 WAV 流。前端、Windows WebView2 及服务器版走同一音频接口；AAC 编码的 M4A 仍按原路径直发，不转码。

- **边解边播**：初始化时解析 MP4 索引、解码末包以取得精确 PCM 长度；随后按请求读取音频包，不需要先把整首转完，也不会创建或堆积临时 WAV 文件。
- **拖动进度**：以 WAV 为虚拟文件提供字节 `Range`、`206`、`ETag`、`304`。每次拖动从最近 ALAC 包开始解码，不缓存整首 PCM；`If-Range` 不匹配时忽略 Range 并完整重传。
- **缓存更新**：服务端音频方案号与前端请求 URL 同步由 2 提升至 3，避免新 WAV 与旧的 M4A/整轨 WAV 缓存片段混用。
- **边界**：当前仅允许单声道/双声道，经典 PCM WAV 的约 4 GiB 单文件上限；其他不支持的容器或异常音频会返回明确错误，不再静默失败。解码使用纯 Go `saprobe-alac v1.0.0-rc2`，无需 ffmpeg 常驻或系统级解码器。

## 验证

- 服务器端合成的 **16-bit/44.1 kHz** 与 **24-bit/48 kHz** ALAC 样本：完整响应的 PCM 字节和纯解码结果一致；头部、非样本对齐的多个中段与末端 Range 均与完整响应逐字节一致；416、304、If-Range 过期→200 均覆盖。
- Go 全量单元测试、Linux 与 Windows 目标平台 vet、前端类型检查以及 **217/217** 前端测试通过；交叉编译的 Windows 测试二进制对老板真实曲库里的 **16-bit/44.1 kHz** 与 **24-bit/48 kHz** ALAC 曲目只读验证，解码/中途跳播通过；最终构建产物随发布工作流再确认。

## 下载

| 文件 | 用途 |
| --- | --- |
| `tagger_setup_1.7.0_windows_amd64.exe` | Windows x86_64 安装包 |
| `tagger_1.7.0_windows_amd64.zip` / 同名 `.sha256` | Windows 便携版及校验 |
| `tagger_1.7.0_linux_amd64.tar.gz` / 同名 `.sha256` | Linux x86_64 |
| `tagger_1.7.0_linux_arm64.tar.gz` / 同名 `.sha256` | Linux arm64 |

Windows 版依赖 WebView2 Runtime；旧版覆盖安装时注意选择之前的安装目录。服务端部署后应核对二进制版本与前端资产哈希。源码构建可运行 `make build VERSION=1.7.0`。

> 若从 v1.6.11 或更早升级，建议先「设置 → 曲库 → 重新扫描」，再视需要「清除缺失记录」；v1.6.12 起已修复增量扫描的幽灵记录处理。ALAC 播放本身**不需要重新扫描**（前提是已有曲目属性包含 `codec=ALAC`）。
