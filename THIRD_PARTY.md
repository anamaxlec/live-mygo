# Third-party notices

完整依赖版本以 `go.mod`、`go.sum` 为准；运行库清单在应用包 `Contents/Resources/linked-libraries.txt`。

| 组件 | 用途 | 许可证文件或来源 |
| --- | --- | --- |
| [MyGo](https://github.com/egoist/mygo) | 原生窗口、GPU UI、WebView 登录 | MIT，模块内 `LICENSE` |
| [libmpv](https://github.com/mpv-player/mpv) 及 FFmpeg 等运行库 | 视频、音频、字幕渲染 | 应用包 `Contents/Resources/licenses/` 中的对应原文；构建收集 Homebrew keg 中的 LICENSE/COPYING/COPYRIGHT |
| [purego](https://github.com/ebitengine/purego) | Go 到原生 API 的调用 | Apache-2.0，模块内许可证 |
| [goja](https://github.com/dop251/goja) | 斗鱼平台返回的签名脚本 | MIT，模块内许可证 |
| [coder/websocket](https://github.com/coder/websocket) | 平台实时弹幕连接 | ISC，模块内许可证 |
| [Lucide](https://github.com/lucide-icons/lucide) | SVG 界面图标 | `assets/icons/LICENSE`，包含 ISC 和 Feather 的 MIT 声明 |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) 等 Go 依赖 | 图像、字体与编码 | 对应模块内许可证 |

应用图标的可编辑源是 `assets/app-icon.svg`。深色 SVG 和 PNG、ICNS 由 `cmd/icon` 生成。

功能和界面参考：[Pure Live](https://github.com/liuchuancong/pure_live)、[Simple Live](https://github.com/xuanxuan123xuan/dart_simple_live)、[MyGo CloudMusic](https://github.com/egoist/mygo-cloudmusic)、[IINA](https://iina.io/)。当前源码许可证原文位于仓库 `LICENSE`。
