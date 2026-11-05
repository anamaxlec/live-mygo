<img src="assets/app-icon.svg" alt="Live MyGo 图标" width="120">

# Live MyGo

**基于 MyGo 的原生播放器练手项目，用于学习原生 UI、视频播放和弹幕绘制。**

[macOS 0.4.2-beta.1](https://github.com/anamaxlec/live-mygo/releases/download/v0.4.2-beta.1/LiveMyGo-v0.4.2-beta.1-macos-arm64.dmg) · [Windows 安装包 0.4.1-beta.3](https://github.com/anamaxlec/live-mygo/releases/download/v0.4.1-beta.3/LiveMyGo-v0.4.1-beta.3-windows-amd64-setup.exe) · [Windows 便携版](https://github.com/anamaxlec/live-mygo/releases/download/v0.4.1-beta.3/LiveMyGo-v0.4.1-beta.3-windows-amd64.zip) · [版本页面](https://github.com/anamaxlec/live-mygo/releases/tag/v0.4.2-beta.1)

## 界面与功能

界面使用系统磨砂材质，支持浅色、深色和跟随系统外观。直播画面内置悬浮控制条，右侧默认显示气泡弹幕。

- 浏览推荐和分区，搜索主播，通过房间号或官方链接进入直播间。
- 切换清晰度、播放线路和画面比例，观看时继续浏览其他直播间。
- 调整画面弹幕和弹幕列表样式，筛选消息、屏蔽用户或关键词。
- 管理关注、分组、置顶和别名，刷新开播状态，订阅开播提醒。
- 使用官方登录窗口或导入 Cookie，账号信息加密保存在本机。
- 跨来源快捷搜索，优先查找本机关注和历史。
- 小窗置顶、只听声音、定时停止和多窗口观看。
- 导入导出关注与偏好，可选静音恢复上次直播。
- 保存当前画面或约 30 秒片段，导出播放诊断，检查应用更新。

macOS 提供 mpv 与原生硬解两个内核。当前版本进展及验证范围见 [产品进展](docs/PRODUCT-NEXT.md)。Windows 下载包仍为 0.4.1-beta.3，新功能的实机复验清单见 [Windows 新功能交接](docs/windows/2026-11-05-HANDOFF.md)。

## 安装

测试版适用于 **macOS 27.0 及以上的 Apple Silicon Mac**。下载并打开 DMG，将 `Live MyGo` 拖入 `Applications` 文件夹。

应用使用 ad hoc 签名，尚未经过 Apple 公证。首次打开如被 macOS 阻止，请在系统设置中允许打开。

Windows x64 测试版提供当前用户安装包或便携 ZIP，安装包无需管理员权限；便携包解压后运行 `LiveMyGo.exe`。已在 Windows 11 x64 实机验证界面、播放和弹幕：视频通过 libmpv Render API 的软件输出合成到 MyGo 位图，本地测试视频和各平台真实直播均有画面；对齐与打包细节见 [Windows 交接文件夹](docs/windows/README.md)，macOS 更新记录见 [macOS 更新与验证](docs/macos/README.md)。

## 快捷键

- `⌘L` / `⌘K`：快捷搜索直播；`⌘F`：搜索当前页面或观看时快捷搜索
- `Space`：暂停或继续；`M`：静音；`D`：切换画面弹幕
- `K` / `Tab`：唤出播放器控制条；`⌘R`：刷新或重新连接
- `⌃⌘F`：全屏；`Esc`：返回或退出全屏
- `⌘1`–`⌘4`：切换页面；`⌘,`：打开设置

## 项目结构

- `cmd/`：图标生成、应用打包与平台验证工具。
- `internal/app/`：桌面应用的界面与生命周期代码。
- `internal/accounts/`、`internal/library/`、`internal/platforms/`、`internal/player/`：账号、本地数据、直播平台和播放器模块。
- `assets/`：应用图标、界面图标和平台标识。
- `docs/`：设计、功能、版本记录及 MyGo 技术参考。

## 构建

需要 Go 1.27.3、Homebrew 的 `mpv`、`ffmpeg` 和 Xcode Command Line Tools。

```sh
go test ./...
sh scripts/build.sh
open build/LiveMyGo.app
```

打包 DMG 需要 `dmgbuild==1.6.7`：

```sh
python3 -m venv .local/packaging
.local/packaging/bin/pip install dmgbuild==1.6.7
DMGBUILD=.local/packaging/bin/dmgbuild sh scripts/package.sh
```

## 待做事项

- [ ] 完成 Apple 签名与公证，适配更低版本的 macOS。
- [ ] 使用真实账号验证登录、Cookie 保存及不同账号的画质权限。
- [ ] 验证长时间播放和高峰弹幕下的稳定性，测量实际能耗。
- [x] 完成 Windows 实机播放、高 DPI 与视频叠层验证；账号登录仍待验证。
- [ ] 在 Windows 实机复验本轮新增的搜索、数据管理、多窗口和保存功能。
- [ ] 完善短片段起始关键帧、系统画中画和多画面布局。
- [ ] 探索应用内发送弹幕等平台互动。

## 许可证与致谢

源码遵循 [GNU Affero General Public License v3](LICENSE)，第三方组件及许可证见 [THIRD_PARTY.md](THIRD_PARTY.md)。

项目使用 [MyGo](https://github.com/egoist/mygo) 和 [libmpv](https://github.com/mpv-player/mpv)，功能与界面参考 [Pure Live](https://github.com/liuchuancong/pure_live)、[Simple Live](https://github.com/xuanxuan123xuan/dart_simple_live)、[MyGo CloudMusic](https://github.com/egoist/mygo-cloudmusic) 和 [IINA](https://iina.io/)。

框架资料见 [MyGo 技术参考](docs/MYGO.md)，界面约定见 [设计准则](docs/DESIGN.md)。
