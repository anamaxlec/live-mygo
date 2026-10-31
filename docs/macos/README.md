# macOS 更新与验证

2026-10-31 已对照 Windows 的 `80e4672`（`0.4.1-beta`）同步 macOS，并补齐浏览页侧栏收起、系统按钮安全区与卡片过渡。共享的默认画质、斗鱼清晰度、关注开播刷新、搜索返回、评论区拖宽和小窗关闭处理均随这次同步生效。macOS 继续使用原生 OpenGL 视频视图与 VideoToolbox，不改为 Windows 的软件帧上传路径。

## 本次 macOS 验证

- `go test -race -count=1 ./...`：账号、应用、本地数据、平台和播放器测试通过。新增的布局测试覆盖红绿灯占位、快速反向切换、动画中间状态和网格列数；`go vet ./...` 通过。
- 原生窗口已检查浅色、深色、浏览侧栏展开与收起、搜索与返回、关注开播优先展示、清晰度菜单及小窗恢复。系统关闭小窗后回到完整窗口，直播继续播放。
- 在设置中把默认画质改为“高清”，斗鱼实际解码为 960×540；手动切到蓝光4M 后变为 1920×1080，两档均使用 VideoToolbox。另做 35 秒独立前台测试，默认最高画质为 1920×1080@30，提交 1026 帧、0 丢帧，首帧观察约 643 ms；这是实际播放检查，不作为功耗基准。
- 截图与报告位于本地 `build/reports/macos-041/`。该目录不进入 Git，也不包含在安装包中。测试使用独立的 `LIVE_MYGO_DATA_DIR`，不修改日常账号、关注和观看记录。
- 本次未启用斗鱼弹幕兼容开关，弹幕加密连接未通过；未验证真实账号、高权限画质、跨屏缩放和长期功耗。

以下保留 Windows 对照值及 macOS 的实现约定。

## 已经共享、macOS 直接生效的改动

- 侧栏搜索框改用 `ui.TextInputBase`，不再带内部左右内边距；搜索行与导航项统一为 20×20 图标盒、`PaddingX(8)`、`Gap(10)`。结果是图标中心线在 32 DIP，搜索占位文字与导航标签在 52 DIP。
- 导航项、设置项统一使用同样的图标盒和间距，图标与文字的垂直中心一致。
- 分区栏和收藏页筛选行整体左移 `filterTabPad`（11 DIP），使首个标签的文字与页面标题、分组标题、卡片左边线对齐；胶囊背景向左侧挂出 11 DIP。`filterButton` 的 `PaddingX` 也使用同一个常量。
- 顶部刷新改成图标加“刷新”文字的紧凑按钮（高 28、图标 14、文字 11.5、加载时 12×12 Spinner），tooltip 由 `shortcutLabel` 输出；macOS 显示 `⌘R`。
- 浏览页、分区页、收藏页和历史页的可用宽度统一由 `contentNavWidth()` 与 `browseLeftInset(c)` 计算；网格列数按侧栏展开时的宽度固定，卡片宽度按实际可用宽度计算，因此侧栏宽度变化时不会跳列。

## 两平台的标题栏与侧栏

| 项目 | Windows 现状 | macOS 现状 |
| --- | --- | --- |
| 浏览页标题栏 | 58 DIP，标题垂直中心 29 | 58 DIP，标题垂直中心 29 |
| 侧栏开关 | `Left(17).Top(14)`，30×30 | `Left(max(100, TitleBar.Left+14)).Top(14)`，30×30 |
| 浏览页侧栏收起 | 200 ms `EaseInOut` | 200 ms `EaseInOut`，支持中途反向 |
| 内容左边距 | `max(26, 54 - navWidth)` | `max(26, toggleLeft+37-navWidth)` |
| 标题栏左侧安全区 | `max(12, TitleBar.Left+14)` | `max(100, TitleBar.Left+14)` |

`browseLeftInset(c)` 里的 `54` 等于“开关右边缘 47 + 7 DIP 间距”。macOS 从自己的开关位置推导安全线；默认系统占位时为 137，系统占位变宽时随之增加。

## macOS 实现约定

### 1. 标题、分区和卡片共用一条左边线

浏览页与播放页都使用 58 DIP 标题栏，让红绿灯、侧栏开关与页面标题共享垂直中线。标题行、分区栏、分组标题和列表 padding 使用同一个 `leftInset`：

```go
left := a.browseLeftInset(c)
// 标题
ui.Row(c).Height(watchHeaderHeight).Padding(0, titlebarRight(c, contentInset), 0, left)
// 分区栏与收藏筛选行
ui.Row(c).Height(30).Margin(0, contentInset, 20, left-filterTabPad)
// 推荐列表和网格
ui.List(...).Padding(0, contentInset, 20, left)
```

侧栏开关为 30×30，先通过 `titlebarLeft(c)` 让出红绿灯安全区；`leftInset` 为 `max(contentInset, toggleLeft+30+7-navWidth)`。展开时使用 26 DIP，收起和动画期间按按钮右缘留出位置。页面宽度通过 `contentNavWidth()` 计算，不单独移动标题。

### 2. 分区标签文字对齐

当前规则：胶囊 `PaddingX(11)`，整行向左移动 11。视觉结果是胶囊背景比标题线多出 11 DIP，但“全部直播”的文字与标题、分组标题、卡片严格同线。这是有意的光学对齐，不要为了对齐背景再把整行右移，否则文字又会缩进。

macOS 上需要确认两处都用了 `a.browseLeftInset(c)-filterTabPad`：

- `internal/app/ui_browse.go` 的 `categoryBar`；
- 收藏/历史页的平台筛选行。

### 3. 侧栏展开/收起动画

浏览页使用 200 ms `EaseInOut`，播放页保留 220 ms；播放页启用 `FollowVideo` 时直接调整尺寸。macOS 已移除非播放页强制展开的赋值。

- 侧栏容器、标题和列表位置读取动画中的 `navWidth`。
- 推荐、分区网格以及首页个人分组的列数按展开宽度确定；卡片宽度按当前可用空间计算。计算列数时不能带入收起过程变化的 `leftInset`，否则临界宽度下仍会跳列。
- 侧栏保留 `Clip()` 和 `Hidden(navWidth < .1)`，列表状态不在动画中重置。

### 4. 刷新按钮

沿用共享的 `refreshButton`：

- 高度 28、`PaddingX(9)`、圆角 7、图标 14、文字 11.5、图标与文字间距 6。
- 加载中把图标换成 12×12 Spinner，按钮同时禁用，避免重复请求。
- 文案固定为“刷新”，tooltip 为 `shortcutLabel("刷新 · ⌘R")`；右侧安全区用 `titlebarRight(c, contentInset)`。
- 不要再使用只有图标的刷新按钮；这是上一轮明确指出的问题。

### 5. 播放页标题

`watchHeader` 的 macOS 分支要保持：

```go
titleEdge := navWidth + watchFrameInset + watchFrameBorder
inset := max(titlebarLeft(c), titleEdge-navigationWidth-titleGap)
```

导航按钮组宽 65、与标题间距 12。侧栏收起/展开时标题必须跟随动画中的 `navWidth`，而不是固定 `sidebarWidth`；否则标题会在动画期间跳动。Windows 分支因为左上角没有红绿灯，导航按钮从窗口左缘开始，标题仍对齐播放器左边线。

## 验证

### 自动化

`internal/app/ui_browse_test.go` 中的 `TestBrowsePagesShareOneLeftEdgeWhileSidebarMoves` 会在 `navWidth` 为 0、20、232 时断言：

- 页面标题、分组标题、第一个卡片按钮的 `X` 相同；
- 分区胶囊背景的 `X` 比文字线小 11。

测试按平台计算期望值：Windows 收起时左边线为 `max(26, 54-navWidth)`，macOS 为 `max(26,137-navWidth)`（默认系统占位）。运行：

```sh
go test ./internal/app -run 'TestBrowsePagesShareOneLeftEdgeWhileSidebarMoves|TestBrowseSidebar|TestBrowseCardsKeepColumns|TestLiveCardsKeepTitleBelowWidescreenCover'
```

开关测试同时覆盖不同的系统按钮占位及快速反向操作；列数测试覆盖 1000、1110、1360 DIP 窗宽。播放页标题对齐见 `internal/app/ui_player_test.go`。

### 手动

1. 100%、125%、150%、200% 缩放都检查一遍，再跨屏拖动窗口，确认红绿灯不遮挡标题或开关。
2. 侧栏展开、收起各录一段慢放视频，逐帧检查标题、分区标签文字、分组标题、卡片左边线是否始终重合；收起时开关与分区行不能重叠。
3. 检查刷新按钮与窗口右侧安全区，窗口变窄时标题不被挤压，刷新按钮不压到标题。
4. 浅色、深色和跟随系统外观各看一次；磨砂背景下检查侧栏分割线和胶囊背景是否仍然清晰。
5. 若启用了收起动画，检查动画期间列表不重建、滚动位置不丢失、卡片不跳列。

## Windows 参考值

- 侧栏宽度 232 DIP，内容基础内边距 26 DIP。
- 浏览页标题栏 58 DIP；标题垂直中心 29。
- 侧栏开关 30×30，位于 (17,14)，中心 (32,29)，右边缘 47。
- 标题/分区文字/分组标题/卡片左边线：展开时 232+26=258，收起时 54；动画中间为 `navWidth + max(26,54-navWidth)`。
- 分区胶囊内边距 11，胶囊背景比文字线左 11。
- 浏览页侧栏动画 200 ms；播放页 220 ms。
- 刷新按钮高 28；列表行间距 20，卡片行间距 25。

## 涉及文件

- `internal/app/ui_browse.go`：标题栏、分区栏、刷新按钮、网格宽度。
- `internal/app/ui_desktop.go`：侧栏动画、搜索框和导航项、浏览页开关。
- `internal/app/ui_discovery.go`：推荐区宽度和列数。
- `internal/app/ui_player.go`：播放页标题与导航按钮。
- `internal/app/ui_platform.go`：标题栏左右安全区、快捷键文案。
- `internal/app/ui_easter.go`：赛事列表的左边线。
- `internal/app/ui_browse_test.go`：对齐回归测试。
