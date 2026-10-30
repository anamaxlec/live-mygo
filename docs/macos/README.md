# macOS 界面对齐交接

2026-10-30 的 Windows 适配中，集中修了顶栏、侧栏、分区标签、刷新按钮和侧栏展开动画的对齐问题。Windows 与 macOS 共用 `internal/app` 下的布局代码，一部分修正已经直接作用于 macOS；另一部分只在 `runtime.GOOS == "windows"` 下启用。本文记录 Windows 的基准值、macOS 需要补的改动和验证方法，方便后续在 Mac 上逐项对照。

## 已经共享、macOS 直接生效的改动

- 侧栏搜索框改用 `ui.TextInputBase`，不再带内部左右内边距；搜索行与导航项统一为 20×20 图标盒、`PaddingX(8)`、`Gap(10)`。结果是图标中心线在 32 DIP，搜索占位文字与导航标签在 52 DIP。
- 导航项、设置项统一使用同样的图标盒和间距，图标与文字的垂直中心一致。
- 分区栏和收藏页筛选行整体左移 `filterTabPad`（11 DIP），使首个标签的文字与页面标题、分组标题、卡片左边线对齐；胶囊背景向左侧挂出 11 DIP。`filterButton` 的 `PaddingX` 也使用同一个常量。
- 顶部刷新改成图标加“刷新”文字的紧凑按钮（高 28、图标 14、文字 11.5、加载时 12×12 Spinner），tooltip 由 `shortcutLabel` 输出；macOS 显示 `⌘R`。
- 浏览页、分区页、收藏页和历史页的可用宽度统一由 `contentNavWidth()` 与 `browseLeftInset()` 计算；网格列数按侧栏展开时的宽度固定，卡片宽度按实际可用宽度计算，因此侧栏宽度变化时不会跳列。

## 只在 Windows 生效、需要 macOS 决策的部分

| 项目 | Windows 现状 | macOS 现状 |
| --- | --- | --- |
| 浏览页标题栏 | 58 DIP，上下 padding 0，标题垂直中心 29 | 84 DIP，上 18 / 下 10 |
| 侧栏开关 | 浏览页左上角绝对定位 `Left(17).Top(14)`，30×30，中心 (32,29) | 浏览页没有开关 |
| 浏览页侧栏收起 | 200 ms `EaseInOut`，`navWidth` 参与标题和内容定位 | 非播放页强制 `navWidth = sidebarWidth`，实际不收起 |
| 收起后的内容左边线 | `max(26, 54 - navWidth)` | 恒为 26 |
| 标题栏左侧安全区 | `max(12, TitleBar.Left+14)` | `max(100, TitleBar.Left+14)` |

`browseLeftInset()` 里的 `54` 等于“开关右边缘 47 + 7 DIP 间距”。macOS 如果新增开关，必须用同样的方法从开关的位置推导安全线，不能直接照抄 54。

## macOS 建议改法

### 1. 标题、分区和卡片共用一条左边线

保留 macOS 84 DIP 的标题栏高度是合理的，红绿灯需要空间。关键是标题行、分区栏、分组标题和列表 padding 使用同一个 `leftInset`：

```go
left := a.browseLeftInset()
// 标题
ui.Row(c).Height(headerHeight).Padding(top, titlebarRight(c, contentInset), bottom, left)
// 分区栏与收藏筛选行
ui.Row(c).Height(30).Margin(0, contentInset, 20, left-filterTabPad)
// 推荐列表和网格
ui.List(...).Padding(0, contentInset, 20, left)
```

如果暂时不做侧栏收起，`browseLeftInset()` 在 macOS 返回 26 即可；标题、分区标签文字、分组标题、卡片都会落在同一条线上。

如果要在 macOS 也支持收起：

- 开关可放在标题栏左侧的导航按钮组中（与播放页的 `watch-back` / `watch-sidebar` 一致），避免与红绿灯抢位置；也可以放在内容区左上角，但必须用 `c.TitleBar().Left` 让出红绿灯安全区。
- 开关建议仍是 30×30，垂直中心与标题栏中心一致。`leftInset` 取 `max(contentInset, toggleRight+7-navWidth)`。
- 标题行 padding-left、`contentNavWidth()`、网格列表 padding-left 必须读取同一个 `leftInset`，否则收起过程中标题会独自移动。

### 2. 分区标签文字对齐

当前规则：胶囊 `PaddingX(11)`，整行向左移动 11。视觉结果是胶囊背景比标题线多出 11 DIP，但“全部直播”的文字与标题、分组标题、卡片严格同线。这是有意的光学对齐，不要为了对齐背景再把整行右移，否则文字又会缩进。

macOS 上需要确认两处都用了 `a.browseLeftInset()-filterTabPad`：

- `internal/app/ui_browse.go` 的 `categoryBar`；
- 收藏/历史页的平台筛选行。

### 3. 侧栏展开/收起动画

`internal/app/ui_desktop.go` 里目前有：

```go
if a.page != "player" && runtime.GOOS != "windows" {
    a.navWidth = sidebarWidth
}
```

这会让 macOS 浏览页始终展开。若要移植 Windows 的行为：

- 删除这个强制赋值，让 `AnimateTo("navigation-width", target, ...)` 的结果生效。
- 浏览页时长与 Windows 一致使用 200 ms `EaseInOut`；播放页保留 220 ms，或按 `FollowVideo` 的逻辑走。
- 所有依赖侧栏宽度的布局都读取 `a.navWidth`，不要读取常量 `sidebarWidth`：侧栏容器宽度、标题 padding-left、`contentNavWidth()`、播放页标题的 `titleEdge`。
- 网格列数继续用 `sidebarWidth`（展开值）计算，卡片宽度用 `available`，这样收起时卡片平滑变宽但不跳列。
- 侧栏容器保留 `Clip()` 与 `Hidden(navWidth < .1)`；不要在动画中重建列表，避免丢失滚动位置。

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

测试按平台计算期望值：Windows 收起时左边线为 `max(26, 54-navWidth)`，macOS 当前为 26。macOS 修改后运行：

```sh
go test ./internal/app -run 'TestBrowsePagesShareOneLeftEdgeWhileSidebarMoves|TestLiveCardsKeepTitleBelowWidescreenCover'
```

如果 macOS 新增开关，把测试里的 Windows 分支换成你的开关安全线，并让 `browseLeftInset()` 使用同一个公式。播放页标题对齐可参考 `internal/app/ui_player_test.go`。

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
