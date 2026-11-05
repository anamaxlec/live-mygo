# macOS 双内核播放

设置 → 播放 → 播放内核，可以选择 `mpv` 或 `原生硬解（测试）`。重新打开应用后生效，设置页同时显示当前实际运行的内核。旧配置继续使用 mpv。两种内核都支持硬解，区别在解封装、显示和音频输出路径。

## 实现

- mpv：libmpv Render API、VideoToolbox、独立 OpenGL 视图。AppKit 的自动 update / reshape 合并至 UI 帧，在与后台绘制相同的锁内应用；默认焦点操作也保护 CGL context。渲染前重新绑定 context，结束时先停后台渲染，再由 UI 线程释放视图。
- mpv 的硬件帧截图由 Render API advanced control 在 GPU 线程处理，避免 VideoToolbox 帧直接交给 swscale。每次回调都执行 update；隐藏或调整尺寸时用 skip rendering 确认帧，不进行窗口呈现。该线程不调用等待 mpv core 的普通命令。
- 原生硬解：FFmpeg 解封装、VideoToolbox 硬解、CoreImage / Metal 显示、AVSampleBufferAudioRenderer 播放 AAC。直接传递 CVPixelBuffer，视频不经过 Go RGBA 位图。解码队列上限 12 帧；暂停和遮挡状态减少定时唤醒；Objective-C 后台调用使用 autorelease pool。首个音频包未到达时先消费视频队列，避免初始化相互等待；长暂停后续播重新打开 HTTP 流，隐藏窗口时丢弃视频输出以保持音频读取。
- 原生内核当前支持带解码配置的 H.264 / HEVC、AAC 音轨。其他编码或无法初始化的音轨会显示错误，可切回 mpv；未支持的命令返回明确错误。保持比例、填满、拉伸、音量、静音、暂停、清晰度和线路沿用共享控制。
- 两条路径都校验 TLS 证书。原生内核使用应用包内 CA；请求头仅随本次取流传入，不复用上一房间。
- `LIVE_MYGO_PLAYER=mpv` / `vt` 可在验证时覆盖设置，普通用户无需配置。Windows 保留原有 mpv 软件输出，不显示 macOS 内核选择。

## 构建

需要 Go 1.27.3、Xcode Command Line Tools、Homebrew `mpv` 与 `ffmpeg`。普通 macOS cgo 构建即包含两套内核，无需 `vtplayer` 标签。`scripts/build.sh` 会把直接链接的 FFmpeg 路径改为应用包内的动态库，然后签名。

```sh
go test -race -count=1 ./...
sh scripts/build.sh
```

`CGO_ENABLED=0` 仍可构建 mpv 路径，原生硬解选项不可用。硬件解码生命周期测试需要系统 VideoToolbox 服务；受限沙盒可能返回 -12911，需在正常本机运行环境中执行。

## 验证入口

```sh
LIVE_MYGO_PLAYER=mpv LIVE_MYGO_DATA_DIR=/tmp/live-mygo-mpv-qa build/LiveMyGo.app/Contents/MacOS/live-mygo -platform douyu -room 6979222 -mute -duration 5m -report /tmp/mpv-report.json
LIVE_MYGO_PLAYER=vt LIVE_MYGO_DATA_DIR=/tmp/live-mygo-vt-qa build/LiveMyGo.app/Contents/MacOS/live-mygo -platform douyu -room 6979222 -mute -duration 5m -report /tmp/vt-report.json
```

房间号应在测试当天重新核对主播身份，短号接口可能返回 HTML。报告仅保存房间身份、清晰度和运行指标，不保存播放 URL 或 Cookie。`-mute` 将音量设为 0，音频仍解码并驱动同步。

本地截图和报告放在 `build/reports/playback-backends/`。测试混合了 UI 操作、前后台切换和不同清晰度，不用于宣称功耗下降比例；比较性能需固定来源、分辨率、窗口、可见状态和弹幕负载。

## 2026-11-04 回归结果

最终 mpv 窗口回归约 5 分半；原生内核约 6 分半，包含约 160 秒暂停和前后台切换。原生报告全程保留 AAC 音轨，续播后恢复；一次网络 I/O 错误由自动重连恢复。本轮没有新增 SIGSEGV 报告。完整 race 测试、macOS vet、Windows 交叉 vet 通过。

这不是数小时连续播放或功耗验收。HEVC、HDR、更多音轨与跨屏仍待实测；原生内核保留测试标记，默认继续使用 mpv。本地逐轮报告和截图位于上述 build/reports 目录。

## 2026-11-05 功能回归

原生内核混合验收约 36 分钟，先播放本地 H.264/AAC 样本，随后经用户同意换到正在直播的斗鱼 5720533（邦Sa），并测试只听声音、恢复画面、独立窗口和整体退出。报告约 4.3 万次实际画面提交，全程保留 AAC。6657 下播后的等待实例没有首帧，不计作长时播放验证。

原生只听声音的硬件测试确认音频时钟前进且视频解码计数为零，恢复后重新出帧。压缩录制测试保留 H.264/AAC，保存失败不写入播放错误。两种内核的截图均经原生保存对话框走通，分别为 640×360 样本与 960×540 真实直播；mpv 重启恢复的约 10 分钟报告中所有采样均为 `muted=true`。

短片段为 MKV：原生样本约 31.6 秒，mpv 真实直播约 30.4 秒。mpv 样本开始处出现参考帧警告，片段起始 GOP 仍需完善。录制不另开网络连接、不重新编码；暂停、换房、换线即结束，不是回看或完整直播录像功能。

本地报告与样本在 `/private/tmp/live-mygo-product-qa/`，界面截图与真实组件导出的动效预览在 `build/reports/interaction-motion/`。新增清空菜单回归覆盖原生菜单在同一帧修改消息造成的下标失效，实际菜单复验已通过。
