# Third-party notices

完整依赖版本以 `go.mod`、`go.sum` 为准；运行库清单在应用包 `Contents/Resources/linked-libraries.txt`。

| 组件 | 用途 | 许可证文件或来源 |
| --- | --- | --- |
| [MyGo](https://github.com/egoist/mygo) | 原生窗口、GPU UI、WebView 登录 | MIT，模块内 `LICENSE` |
| [libmpv](https://github.com/mpv-player/mpv) 及 FFmpeg 等运行库 | 视频、音频、字幕渲染 | 应用包 `Contents/Resources/licenses/` 中的对应原文；构建收集 Homebrew keg 中的 LICENSE/COPYING/COPYRIGHT |
| [purego](https://github.com/ebitengine/purego) | Go 到原生 API 的调用 | Apache-2.0，模块内许可证 |
| [goja](https://github.com/dop251/goja) | 直播来源签名脚本 | MIT，模块内许可证 |
| [coder/websocket](https://github.com/coder/websocket) | 平台实时弹幕连接 | ISC，模块内许可证 |
| [Lucide](https://github.com/lucide-icons/lucide) | SVG 界面图标 | `assets/icons/LICENSE`，包含 ISC 和 Feather 的 MIT 声明 |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) 等 Go 依赖 | 图像、字体与编码 | 对应模块内许可证 |

应用图标的可编辑源是 `assets/app-icon.svg`。深色 SVG 和 PNG、ICNS 由 `cmd/icon` 生成。

来源标识位于 `assets/platforms/`，按 [哔哩哔哩](https://www.bilibili.com/)、[斗鱼](https://open.douyu.com/) 、[虎牙](https://hd.huya.com/pc/contactUs/)、[抖音](https://live.douyin.com/) 和 [企鹅体育](https://www.qie.tv/) 官网图形重绘为单色 SVG，用于界面中的来源识别。

功能和界面参考：[Pure Live](https://github.com/liuchuancong/pure_live)、[Simple Live](https://github.com/xuanxuan123xuan/dart_simple_live)、[MyGo CloudMusic](https://github.com/egoist/mygo-cloudmusic)、[IINA](https://iina.io/)。当前源码许可证原文位于仓库 `LICENSE`。

抖音签名实现参考 [Simple Live 的 douyin_sign.dart](https://github.com/xuanxuan123xuan/dart_simple_live/blob/stable/simple_live_core/lib/src/scripts/douyin_sign.dart)，对应脚本保存在 `internal/platforms/scripts/`，保留来源和原始声明。Simple Live 使用 AGPL-3.0；其中 a_bogus 原文另含仅供学习交流的声明。

Windows 播放运行库来自 [mpv 安装页](https://mpv.io/installation/) 列出的 [shinchiro 构建](https://github.com/shinchiro/mpv-winbuild-cmake/releases/tag/20261021)，版本与下载摘要记录在打包工具和测试包的 `PLAYER-SOURCE.txt`。Windows 图标与 manifest 由构建工具 [rsrc v0.10.2](https://github.com/akavel/rsrc) 编译，工具使用 MIT 许可证。
