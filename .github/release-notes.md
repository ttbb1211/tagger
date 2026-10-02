# Tagger v1.6.5

本仓库是 [Ericwyn/tagger](https://github.com/Ericwyn/tagger) 的 fork（`ttbb1211/tagger`），面向中文音乐库整理：整轨 CUE 专辑、中文数据源、歌词 `.lrc`、简体转换等。

本版是 **v1.6.4 之后的一个安装体验修复**：Windows 安装向导现在**始终显示「选择安装位置」页**——以前升级时这一步会被静默跳过，装到哪里全凭上次的选择，用户完全不知情。

## 下载

| 文件 | 用途 |
| --- | --- |
| `tagger_setup_1.6.5_windows_amd64.exe` | Windows x86_64 **安装包**（向导可选**安装位置**与音乐库目录，装完自动启动） |
| `tagger_1.6.5_windows_amd64.zip` | Windows x86_64 **便携版**（解压即用，双击 `tagger.exe`） |
| `tagger_1.6.5_windows_amd64.zip.sha256` | 便携包的 SHA-256 校验值 |
| `tagger_1.6.5_linux_amd64.tar.gz` / `tagger_1.6.5_linux_arm64.tar.gz` | Linux 二进制（含 `.sha256`） |

Windows 版双击后弹出 WebView2 原生窗口（**依赖 WebView2 Runtime，与 Chrome 无关**；缺失时自动回退「后台服务 + 默认浏览器」，或用 `-ui=server` 纯后台运行），关窗口即退出。

源码构建：

```bash
make build VERSION=1.6.5     # 产物 dist/tagger
docker build --build-arg VERSION=1.6.5 -t my-tagger .
```

> 版本号由构建注入（`-ldflags -X .../internal/version.Version=`）。界面顶栏与曲库页底部会显示 `v1.6.5`，设置页「系统信息」同样可查。

## v1.6.5 主要变化

### 安装向导：不再跳过「选择安装位置」

**现象**：安装（尤其是升级）时，向导从许可页直接跳到「选择音乐库目录」页，**中间没有让用户选安装位置的步骤**；装完之后翻遍 `C:\Program Files` 也找不到程序，不知道装到哪去了。

**原因**：安装脚本从未设置 Inno Setup 的 `DisableDirPage`，于是走了它的默认值 `auto`。官方对 `auto` 的定义是：

> 启动时查注册表，发现**同一 AppId 已安装**，就**不显示「选择安装位置」页**；页面不显示时**永远使用默认目录**。

而 `UsePreviousAppDir` 的默认值 `yes` 又会把「默认目录」替换为**上一次安装的目录**。两条叠加的结果就是：**首次安装时选过一次目录，此后每次升级都被静默沿用，向导再也不问**。（`auto` 并不是「智能显示」，而是「已装就藏」。）

**现在**：显式设置 `DisableDirPage=no`，安装位置页**每次都会出现**；同时开启 `AlwaysShowDirOnReadyPage=yes`，「准备安装」页会再显示一次目标目录，安装前可以核对。

**向导页面顺序**（改后）：`许可协议 → 选择安装位置 → 选择音乐库目录 → 准备安装 → 安装 → 完成`。

## 升级注意

- **无数据格式变化，也不需要重新扫描**——本版只改安装脚本，程序本体与 v1.6.4 功能一致。
- **「选择安装位置」页会预填你上次安装的目录**（本机实测为 `D:\Program Files\Tagger`）。**保持默认直接下一步即可原地升级**；若改成别的路径，会与旧的那份**并存两份**，需要自行卸载旧目录。
- 想确认当前装在哪里：`设置 → 系统信息`，或看开始菜单快捷方式的「属性 → 目标」，或注册表 `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{7E4A2C19-8B3D-4F6A-9C21-D5E8F0A63B47}_is1` 的 `InstallLocation`。
- 从 v1.6.0 ~ v1.6.4 直接覆盖安装即可。
- 尚未覆盖：审核页的「在本批次中搜索」和历史页的修订搜索是浏览器内的本地过滤，未做简繁互认（曲库搜索框已完整支持，见 v1.6.4）。
- 沿用提醒：整轨 CUE 的歌词若未导出 `.lrc`，只留在曲库索引里，**完整重扫会丢失**，界面保存时会提示。
- 从 v1.5.1 升级：**不要用 v1.5.1 自带的卸载程序**（它会崩），直接跑安装包走「检测到旧版→卸载旧版→装新版」。
