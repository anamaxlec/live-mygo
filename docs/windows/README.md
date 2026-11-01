# Windows 测试交接

当前为 `0.4.1-beta.2`，目标是 Windows x64，为未签名测试构建。可使用同版本的安装包 `LiveMyGo-v0.4.1-beta.2-windows-amd64-setup.exe`（当前用户安装，无需管理员权限），或解压应用 ZIP 后直接运行 `LiveMyGo.exe`。便携包里的 `libmpv-2.dll` 和 `cacert.pem` 需要保留。

此版本包含 Windows 播放 CPU 开销优化：渲染线程改由 mpv 新帧回调唤醒，暂停或空闲时不再每 4 ms 轮询；RGB 帧的 alpha 改为按 32 位批量补齐。

2026-10-30 已在 Windows 11 x64、200% 缩放（192 DPI，3072×1920）实机运行。`go test ./...`、`go vet ./...`、`go run ./cmd/package-windows -test` 均通过。视频已可用：libmpv 的 D3D11 子窗口没有进入 MyGo 的合成层，因此改为 Render API 软件输出（`vo=libmpv`、`MPV_RENDER_API_TYPE_SW`），把 RGB 帧上传为 MyGo 位图，再由 UI 线程合成。本地 H.264 测试视频和 B 站、虎牙、斗鱼、抖音、企鹅体育真实直播均有画面与音频解码；QA 报告 `rendering.backend` 为 `libmpv-render-sw`，不同平台首帧约 0.6–2.1 秒。另一次 97 秒直播测试（2560×1440、60 fps 源，渲染上限 1920×1080）渲染 585 帧、0 丢帧、`d3d11va-copy`，CPU 约 4.7 秒用户态 + 5.7 秒内核态，峰值 RSS 约 493 MiB。macOS 已验证原生播放、实时消息、小窗往返和界面布局，并运行 race 测试。

## 第一轮先检查

1. 用本地 H.264 视频确认有画面、声音和移动的时间位置，再测试直播，方便区分播放器和网络问题。
2. 确认视频处于 MyGo 控件下方，播放器控制条、弹幕、菜单都能显示和点击；拖动窗口和调整评论栏宽度时不闪烁、不遮挡其他内容。
3. 测试 100%、125%、150%、200% 缩放及跨屏移动，检查右上角系统按钮、标题、工具按钮、四角和细边线；小窗进出后再检查一次。
4. 检查音量拖动、取消静音、清晰度、换线、全屏、小窗置顶、定时停止，以及悬浮控件自动隐藏。

## 2026-10-30 实机记录

- 标题栏：Windows 没有红绿灯，播放页导航按钮改从窗口左边缘开始，标题保持在播放器左边线；窗口按钮仍由系统绘制在右上角。
- 圆角与黑线：视频不再使用原生子窗口，画面在 MyGo 位图内绘制，播放器 14 px 圆角、聊天面板右上角和窗口右缘不再露出 1–2 px 黑线。
- 播放链路：`internal/player/render_windows.go` 用 `MPV_RENDER_API_TYPE_SW` 输出 `rgb0` 帧，`internal/app/ui_video_windows.go` 在 UI 线程调用 `ui.Bitmap.Update` 上传；窗口隐藏或最小化时停止呈现，恢复后由 dirty 标记补一帧。注意 `MPV_RENDER_PARAM_SW_POINTER` 直接传像素指针，不是指针的指针；曾因传错导致 `mpv_render_context_render` 访问冲突。窗口启动适配视频时会连续改变尺寸，缓冲池必须丢弃旧尺寸缓冲；否则池子被占满后只渲染 4 帧就停住，斗鱼直播曾复现该问题（表现为画面冻结、声音和弹幕继续）。
- 本地播放：`-file` 加载 1920×1080 H.264/AAC 测试视频，画面可见；测试视频只有 8 秒，长时检查可用 `LIVE_MYGO_QA_LOOP=1`。
- 真实直播：虎牙、斗鱼、B 站、抖音、企鹅体育均实测有画面；斗鱼 9999 房间 45 秒测试渲染 1110 帧、0 丢帧。斗鱼启动时窗口会连续缩放，旧尺寸缓冲曾把池子占满，导致只出 4 帧后画面冻结；现已按新尺寸回收缓冲，详见“播放链路”。
- 跨平台压测（2026-10-31）：直接进房各 25 秒，B 站 729 帧、虎牙 1439 帧、斗鱼 610 帧、抖音 1379 帧、企鹅体育 725 帧，全部 0 丢帧；每轮启动都经历 24–25 次尺寸变化，修复后均持续出帧。测试源覆盖 960×540@25、1280×720@30、2560×1440@60 和 1080×1920@60（竖屏）。
- 首页：关注且正在直播的直播间排在“为你推荐”之前，并使用与推荐卡同规格的 16:9 大卡片；优先使用本次刷新或上次保存的开播状态，进入首页、从播放页返回和窗口重新聚焦时后台刷新，并把标题、封面和开播状态写回收藏。
- 搜索：搜索结果顶栏增加图标加文字的“返回”按钮，返回时清空搜索并展开侧栏；从播放页返回浏览也会恢复侧栏，不再只能靠左上角开关操作。
- 安装器：目标目录的 Live MyGo 正在运行时提示“关闭并继续安装”，默认按钮为“否”；自动关闭超时后再询问是否强制结束。静默安装直接尝试关闭。
- 斗鱼清晰度：取流请求不再携带 `iar`/`ive`/`hevc`/`fa` 等自动画质参数，服务端现在按 `rate` 返回对应码流。实测“高清/超清/蓝光4M”分别为 540p/720p/1080p；选择清晰度后播放地址和画面分辨率都会变化。未登录时“原画”可能被服务端降到蓝光4M。
- 默认画质：播放设置新增“默认画质”，可选最高画质、原画、蓝光、超清、高清、流畅，默认最高画质。进入直播间时按平台返回的档位自动选择，缺少目标档位时取最接近的可用档位；实测斗鱼默认“高清”为 960×540、最高画质为 1080p 蓝光4M，B 站默认原画，虎牙默认蓝光50M、画面宽 2560。用户在播放器手动选择但平台降档时，会提示实际切换到哪个清晰度，不再静默停留。
- 清晰度菜单：列出平台返回的全部档位并支持滚动；斗鱼未登录时在菜单内说明高码率档位由平台账号权限决定。
- 悬停提示：播放器标题、首页精选和推荐卡片不再在悬停时重复显示标题；按钮和图标的功能提示保留。
- 设置排版：“推荐样式”从画面弹幕页的标题栏移到“样式 / 布局 / 屏蔽”同一行右侧，改为轻量的图标文字操作，并与内容列右缘对齐，不再挤在窗口按钮下方。
- 小窗：进入小窗后隐藏 MyGo 在 Windows 上绘制的系统标题栏按钮，拖动和缩放小窗后会再次隐藏；标题栏关闭键只退出小窗并恢复完整窗口，不再结束整个应用。
- 评论区分隔条：拖动改为按按下时的指针绝对位置计算宽度，不再使用逐帧 `dx`；分隔条跟随指针移动后仍能继续拖动，实测宽度可在 240–560 DIP 内连续调整。
- DPI：本次只在 200% 缩放验证。PowerShell 未声明 DPI 时 `GetDpiForSystem` 会返回 96，窗口实际按 192 DPI 创建；后续仍需用 100%、125%、150% 及跨屏移动复测。
- 界面对齐：浏览页标题栏与播放页统一为 58 DIP；侧栏搜索占位和导航标签同在 52 DIP，图标中心线 32 DIP；左侧开关位于 (17,14)，内容收起时统一让出 54 DIP；分区胶囊整体左移 11 DIP，让标签文字与标题、分组标题、卡片同线；刷新按钮改为图标加“刷新”文字；浏览页侧栏动画 200 ms，标题跟随动画宽度，网格列数在动画中保持稳定。
- 未覆盖：全屏、定时停止、音量与取消静音、换线、账号登录、WebView2 登录窗口。
- 调试开关：`LIVE_MYGO_MPV_LOG=<文件>` 输出 mpv 日志；`LIVE_MYGO_QA_LOOP=1` 循环本地视频。日志可能包含播放地址，分享前需检查；验证报告仍不包含 Cookie 与播放 URL。

## 构建与测试

在源码根目录，使用 Go 1.27.3：

```powershell
$env:LIVE_MYGO_LIBMPV = Resolve-Path .\build\windows-amd64\libmpv-2.dll
go test ./...
go vet ./...
go run ./cmd/package-windows -test
```

`internal/player` 的测试会真实加载 libmpv，未设置 `LIVE_MYGO_LIBMPV` 时需要把 `libmpv-2.dll` 放到测试程序旁边。构建命令生成 `build/windows-amd64/` 和 `build/releases/`。资源工具固定为 `github.com/akavel/rsrc@v0.10.2`，用于嵌入图标与 DPI manifest；不需要 CMake 或 Visual Studio 来编译本项目。

安装包使用 NSIS 3.10 编译，脚本为 `cmd/package-windows/installer.nsi`：

```powershell
makensis /INPUTCHARSET UTF8 cmd\package-windows\installer.nsi
```

安装到 `%LOCALAPPDATA%\Programs\LiveMyGo`，写入当前用户的卸载项并创建开始菜单和桌面快捷方式；不需要管理员权限。安装包未签名，SmartScreen 会提示未知发布者。

运行时固定使用 mpv 安装页列出的 shinchiro 构建 `20261021`，下载归档的 SHA-256 在打包工具中校验。`PLAYER-SOURCE.txt` 记录来源。开发时可以通过 `LIVE_MYGO_LIBMPV` 指定另一份兼容的 DLL。

使用隔离数据目录测试：

```powershell
$env:LIVE_MYGO_DATA_DIR = Join-Path $env:TEMP 'live-mygo-windows-qa'
.\build\windows-amd64\LiveMyGo.exe -mute -report .\build\reports\windows.json
```

测试包若附带 `test-pattern.mp4`，可用它先做本地播放：

```powershell
.\build\windows-amd64\LiveMyGo.exe -file .\build\windows-amd64\test-pattern.mp4 -mute -report .\build\reports\windows-local.json
```

界面关闭时保存报告。报告不包含 Cookie 或播放 URL。`samples` 中检查尺寸、`hardware_decoder`、编解码器和丢帧；`rendering.backend` 应为 `libmpv-render-sw`，`video_surface_visible` 为 true。`first_frame_observed_ms` 记录第一帧进入报告的时间（本机约 0.6–2.1 秒），`rendered_frames` 是实际提交给 MyGo 的帧数；这两个字段现在能直接反映视频是否出画。播放时 surface 会按 DPI 缩放，但渲染目标上限为 1920×1080，4K 屏幕会先缩到该尺寸再交给 UI。

## 需要重点验证的实现

- `internal/player/surface_windows.go`：无原生窗口，只维护 HWND、可见性、目标尺寸和帧缓冲；`TakeFrame`/`ReleaseFrame` 由 UI 线程取帧和归还缓冲，`publish` 通过 `Win.Invalidate` 触发重绘。
- `internal/player/render_windows.go`：加载 `mpv_render_context_*`，由 mpv 更新回调唤醒渲染线程，在可见且有更新时渲染到 RGB0 缓冲；窗口消失时停止呈现，暂停或空闲时阻塞等待，避免后台空转。
- `internal/app/ui_video_windows.go`：把最新帧包装成 `image.RGBA`，创建或更新 `ui.Bitmap`；更新后再归还上一帧缓冲，保证合成时数据有效。
- `internal/player/library_windows.go`：从程序旁加载 DLL，`vo=libmpv`；TLS 证书校验保持开启。
- `internal/app/ui_browse.go`、`ui_desktop.go`、`ui_discovery.go`、`ui_player.go`：标题栏高度、侧栏动画、统一左边线、分区标签、刷新按钮和网格列数。对齐细节与 macOS 移植说明见 [macOS 交接文档](../macos/README.md)。
- `internal/app/ui_platform.go` 和 `keys_windows.go`：标题栏右侧安全区、Ctrl 快捷键与 F11 全屏。
- MyGo `Secrets`：使用 Windows Credential Manager；官网登录还需要 WebView2 Runtime。真实账号测试使用自己的账号。
- [设计准则](../DESIGN.md)：并列按钮的宽高、圆角、文字与图标位置，正常、悬停、按下、焦点与禁用状态都要检查。侧栏展开时标题与播放器左边线对齐，导航按钮位于其左侧。

在“关于”中点击版本号 5 次可开启赛事彩蛋。它读取频道列表，并解析媒体地址交给原生播放器；右侧展示播放线路。部分外部线路会失效，本机抽查遇到 HTTP 404 和连接失败，尚未验证这一来源的实际出画。先测试本地视频和其他已开播房间，避免混淆问题来源。

线路切换应即时更新选中态并取消上一条连接；连接卡片空白区域不能拦截线路点击。失败和连接中的两种状态都需要实测。

MyGo 的完整版本文档和源码定位见 [技术参考](../MYGO.md)。若画面或叠层异常，先结合这里列出的合成实现定位，再修改框架或渲染方式。
