# MyGo 技术参考

项目锁定 `github.com/egoist/mygo v0.9.0`。本地 [mygo/](mygo/README.md) 保留提交 `b02f13981d96` 的历史文档快照与 [MIT 许可证](mygo/MYGO-LICENSE)。核对当前 API 和行为时使用 [v0.9.0 源码与文档](https://github.com/egoist/mygo/tree/v0.9.0)。

[v0.9.0 发布说明](https://github.com/egoist/mygo/releases/tag/v0.9.0) · [在线文档](https://mygo.egoist.dev/)

处理 Windows 适配时先看这些内容：

- [窗口](mygo/windows.md)和[原生 UI 窗口](mygo/ui/windows.md)：标题栏安全区、透明背景、窗口控制、全屏与系统材质。
- [布局](mygo/ui/layout.md)和[输入](mygo/ui/input.md)：控件尺寸、焦点、键盘快捷键；`ui.Cmd` 在 Windows 上对应 Ctrl。
- [原生界面](mygo/native.md)与 [UI 文档目录](mygo/ui/README.md)：绘制、动画、列表与弹出面板。
- [系统凭据](mygo/native.md#secrets)：账号加密密钥通过 MyGo 保存在系统存储，Windows 使用 Credential Manager。

获取与项目一致的框架源码：

```sh
go mod download
go list -m -f '{{.Dir}}' github.com/egoist/mygo
```

原生视频和 MyGo UI 的合成关系需要查看该模块的 `internal/windows/surface.go`、`compositor.go`、`webview_embed.go` 和 `titlebar.go`。Live MyGo 的对应实现是 `internal/player/surface_windows.go`、`library_windows.go` 与 `internal/app/ui_player.go`。

## 原生视频验证

macOS 和 Windows 可通过 `-native-video` 启动独立验证窗口，输入本地文件或可直接访问的 HTTP(S) 视频地址，包括 HLS。空格切换播放与暂停，窗口内可切换静音。验证沿用 `-mute`、`-duration` 和 `-report`，报告记录首帧就绪时间、画面尺寸和播放位置。

```sh
build/LiveMyGo.app/Contents/MacOS/live-mygo -native-video /path/to/sample.mp4 -mute -duration 10s -report /tmp/live-mygo-native-video.json
```

平台房间继续走现有播放器。它需要保留 HTTP-FLV、平台请求头和直播功能，而 `ui/video` 的公开入口接受文件、字节或 URL。各系统支持的格式见 [v0.9.0 视频文档](https://github.com/egoist/mygo/blob/v0.9.0/docs/ui/video.md)。
