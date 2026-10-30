# MyGo 技术参考

项目锁定 `github.com/egoist/mygo v0.7.2-0.20261028022740-b02f13981d96`，commit `b02f13981d96`。本目录的 [mygo/](mygo/README.md) 是该版本的完整官方文档快照，保留原文与 [MIT 许可证](mygo/MYGO-LICENSE)。

[官方仓库](https://github.com/egoist/mygo/tree/b02f13981d96) · [在线文档](https://mygo.egoist.dev/)

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

原生视频和 MyGo UI 的合成关系需要查看该模块的 `internal/windows/surface.go`、`compositor.go`、`webview_embed.go` 和 `titlebar.go`。Live MyGo 的对应实现是 `internal/player/surface_windows.go`、`library_windows.go` 与 `internal/app/ui_player.go`。测试阶段保留当前 MyGo 版本，方便比较运行结果。
