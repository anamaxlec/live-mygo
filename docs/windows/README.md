# Windows 测试交接

当前为 `0.4.0-beta.2`，目标是 Windows x64，为未签名测试构建。使用同版本的应用 ZIP 和源码 ZIP；解压应用包后直接运行 `LiveMyGo.exe`。文件夹里的 `libmpv-2.dll` 和 `cacert.pem` 需要保留。

已在 macOS 完成交叉编译；Windows 实机运行尚未验证。macOS 已验证原生播放、实时消息、小窗往返和界面布局，并运行 race 测试。Windows 的视频合成、硬件解码、账号登录和高 DPI 需要下列检查。

## 第一轮先检查

1. 用本地 H.264 视频确认有画面、声音和移动的时间位置，再测试直播，方便区分播放器和网络问题。
2. 确认视频处于 MyGo 控件下方，播放器控制条、弹幕、菜单都能显示和点击；拖动窗口和调整评论栏宽度时不闪烁、不遮挡其他内容。
3. 测试 100%、125%、150%、200% 缩放及跨屏移动，检查右上角系统按钮、标题、工具按钮、四角和细边线；小窗进出后再检查一次。
4. 检查音量拖动、取消静音、清晰度、换线、全屏、小窗置顶、定时停止，以及悬浮控件自动隐藏。

## 构建与测试

在源码根目录，使用 Go 1.27.3：

```powershell
go test ./...
go vet ./...
go run ./cmd/package-windows -test
```

构建命令生成 `build/windows-amd64/` 和 `build/releases/`。资源工具固定为 `github.com/akavel/rsrc@v0.10.2`，用于嵌入图标与 DPI manifest；不需要 CMake 或 Visual Studio 来编译本项目。

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

界面关闭时保存报告。报告不包含 Cookie 或播放 URL。`samples` 中检查尺寸、`hardware_decoder`、编解码器和丢帧；确认画面实际可见。Windows 使用 mpv 的 D3D11 子窗口输出，`rendering.backend` 为 `mpv-d3d11-window`；该路径没有 Render API 的呈现回调，`rendered_frames` 和 `first_frame_observed_ms` 为 0 时不能据此判断黑屏。

## 需要重点验证的实现

- `internal/player/surface_windows.go`：视频子窗口位置、DPI、圆角裁切，以及与 MyGo UI 的前后关系。这是第一项实机检查。
- `internal/player/library_windows.go`：从程序旁加载 DLL，使用 D3D11；TLS 证书校验保持开启。
- `internal/app/ui_platform.go` 和 `keys_windows.go`：标题栏右侧安全区、Ctrl 快捷键与 F11 全屏。
- MyGo `Secrets`：使用 Windows Credential Manager；官网登录还需要 WebView2 Runtime。真实账号测试使用自己的账号。
- [设计准则](../DESIGN.md)：并列按钮的宽高、圆角、文字与图标位置，正常、悬停、按下、焦点与禁用状态都要检查。侧栏展开时标题与播放器左边线对齐，导航按钮位于其左侧。

在“关于”中点击版本号 5 次可开启赛事彩蛋。它读取频道列表，并解析媒体地址交给原生播放器；右侧展示播放线路。部分外部线路会失效，本机抽查遇到 HTTP 404 和连接失败，尚未验证这一来源的实际出画。先测试本地视频和其他已开播房间，避免混淆问题来源。

线路切换应即时更新选中态并取消上一条连接；连接卡片空白区域不能拦截线路点击。失败和连接中的两种状态都需要实测。

MyGo 的完整版本文档和源码定位见 [技术参考](../MYGO.md)。若画面或叠层异常，先结合这里列出的合成实现定位，再修改框架或渲染方式。
