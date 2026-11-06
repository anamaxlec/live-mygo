//go:build darwin || windows

package app

import (
	"image"
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
	"testing"
	"time"
)

func TestWatchedRoomRemainsInRecommendations(t *testing.T) {
	a := polishApp(t)
	r := platforms.Room{Platform: platforms.Bilibili, ID: "1", Title: "已观看但仍在推荐", Anchor: "测试主播", Live: true}
	a.library.History = []platforms.Room{r}
	a.catalog = []platforms.Room{r}
	tester := ui.NewTester(a.view, 1180, 920)
	if _, ok := tester.Find("播放 测试主播 已观看但仍在推荐"); !ok {
		t.Fatal("观看历史仍排除了推荐卡片")
	}
}
func TestHomepageMixesSourcesAndKeepsPersonalGroupsFirst(t *testing.T) {
	a := polishApp(t)
	a.page = "home"
	a.catalog = mergeRecommendations([][]platforms.Room{{{Platform: platforms.Bilibili, ID: "1", Title: "推荐甲", Anchor: "甲", Live: true}}, {{Platform: platforms.Douyu, ID: "1", Title: "推荐乙", Anchor: "乙", Live: true}}})
	a.library.History = []platforms.Room{{Platform: platforms.Huya, ID: "2", Title: "历史", Anchor: "历史主播"}}
	tester := ui.NewTester(a.view, 1180, 920)
	recent, _ := tester.Find("查看全部最近观看")
	recommended, ok := tester.Find("为你推荐")
	if !ok || recommended.Y <= recent.Y || len(a.catalog) != 2 {
		t.Fatalf("首页分组：recent=%+v recommended=%+v exists=%v count=%d", recent, recommended, ok, len(a.catalog))
	}
	for _, page := range []string{"bilibili", "douyu", "huya", "douyin", "qie"} {
		platformPage := polishApp(t)
		platformPage.page = page
		platformPage.library.History = a.library.History
		platformPage.catalog = a.catalog
		tester = ui.NewTester(platformPage.view, 1180, 920)
		if _, ok := tester.Find("查看全部最近观看"); ok {
			t.Fatal("平台页仍重复显示首页最近观看模块", page)
		}
	}
}

func TestReturningHomeRestoresRecommendationsWithoutLoading(t *testing.T) {
	a := polishApp(t)
	a.page = "home"
	a.catalog = []platforms.Room{{Platform: platforms.Bilibili, ID: "1", Title: "首页缓存", Anchor: "主播", Live: true}}
	a.grid.KeepOffset = true
	a.openPage("favorites")
	a.catalog = []platforms.Room{{Platform: platforms.Douyu, ID: "2", Title: "其他页面", Live: true}}
	a.openPage("home")
	if len(a.catalog) != 1 || a.catalog[0].Title != "首页缓存" || a.catalogLoading || !a.grid.KeepOffset {
		t.Fatalf("返回首页丢失已有卡片或滚动状态：%+v loading=%v", a.catalog, a.catalogLoading)
	}
	a.homeCatalog.updated = time.Now().Add(-2 * time.Minute)
	if a.restoreHomeCatalog() || len(a.catalog) != 1 {
		t.Fatal("过期缓存应保留展示，同时允许后台刷新")
	}
}

func TestWindowStateStoreKeepsMainBoundsWhenQuittingCompanion(t *testing.T) {
	a := polishApp(t)
	store := appWindowStateStore{app: a}
	normal := mygo.WindowState{X: 80, Y: 60, Width: 1320, Height: 840}
	if err := store.SaveWindowStates(map[string]mygo.WindowState{"main": normal}); err != nil {
		t.Fatal(err)
	}
	saved, err := store.LoadWindowStates()
	if err != nil || saved["main"] != normal {
		t.Fatal("窗口边界未保存", saved, err)
	}
	a.companion = true
	a.companionBounds = mygo.Rectangle{X: normal.X, Y: normal.Y, Width: normal.Width, Height: normal.Height}
	if err := store.SaveWindowStates(map[string]mygo.WindowState{"main": {Width: 480, Height: 270}}); err != nil {
		t.Fatal(err)
	}
	saved, err = store.LoadWindowStates()
	if err != nil || saved["main"] != normal {
		t.Fatal("小窗覆盖了普通窗口边界", saved, err)
	}
}

func TestHomeRecommendationsExcludeOfflineAndRemoveKnownStoppedRooms(t *testing.T) {
	bili := platforms.Room{Platform: platforms.Bilibili, ID: "1", Title: "正在直播", Live: true}
	douyu := platforms.Room{Platform: platforms.Douyu, ID: "1", Title: "另一个平台", Live: true}
	offline := platforms.Room{Platform: platforms.Qie, ID: "2", Title: "已下播", Live: false}
	rooms := mergeRecommendations([][]platforms.Room{{bili, offline, bili}, {offline, douyu}})
	if len(rooms) != 2 || rooms[0] != bili || rooms[1] != douyu {
		t.Fatal("推荐应排除未开播房间，保留跨平台身份并去重", rooms)
	}
	a := polishApp(t)
	a.catalog, a.homeCatalog.rooms = rooms, rooms
	a.removeOfflineRecommendation(platforms.Bilibili, "1")
	if len(a.catalog) != 1 || len(a.homeCatalog.rooms) != 1 || a.catalog[0] != douyu {
		t.Fatal("确认下播的房间仍在当前或缓存推荐里")
	}
}

func TestReturningToBrowseKeepsLayoutStillWhileContentAppears(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := polishApp(t)
		a.page, a.returnPage, a.focusMode, a.shown = "player", "home", true, true
		a.catalog = []platforms.Room{{Platform: platforms.Bilibili, ID: "1", Title: "缓存直播", Live: true}}
		a.homeCatalog = homeCatalogState{rooms: a.catalog, updated: time.Now()}
		a.targetRoom = a.catalog[0]
		a.stream = &platforms.Stream{Room: a.targetRoom}
		a.videoBitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 9)))
		tester := ui.NewTester(a.view, 1180, 780)
		tester.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
		if err := tester.Click("返回列表 · Esc"); err != nil {
			t.Fatal(err)
		}
		tester.Frame()
		if px := tester.Image().RGBAAt(300, 200); px.R > 32 || px.G > 32 || px.B > 32 {
			t.Fatalf("返回首帧提前露出列表底色：%v", px)
		}
		tester.Advance(20 * time.Millisecond)
		before, found := tester.Find("为你推荐")
		if !found || a.navWidth != sidebarWidth {
			t.Fatalf("返回时仍在挤压内容：sidebar=%v found=%v", a.navWidth, found)
		}
		tester.Advance(80 * time.Millisecond)
		after, _ := tester.Find("为你推荐")
		if before.X != after.X || before.W != after.W || a.navWidth != sidebarWidth {
			t.Fatal("侧栏出现时浏览内容仍在横向移动", before, after)
		}
		card := a.nowCardBounds
		if !a.returnShrink || card.W < 100 || card.H < 50 {
			t.Fatal("未进入收拢过渡或未取得卡片边界", card)
		}
		tester.SetSize(1180, 840)
		if updated := a.nowCardBounds; math.Abs(float64(updated.Y-card.Y-60)) > 1 {
			t.Fatal("窗口高度变化后未跟随实际卡片位置", card, updated)
		}
		button, found := tester.Find("返回正在播放的直播间")
		if !found {
			t.Fatal("浏览页未创建正在播放卡片")
		}
		tester.ClickAt(button.X+button.W/2, button.Y+button.H/2)
		if a.page != "home" {
			t.Fatal("过渡层允许点击穿透卡片")
		}
		if !reduced {
			tester.Advance(returnCardCatchDip - tester.Now().Sub(a.returnShrinkAt))
			tester.Frame()
			dip, _ := tester.Find("返回正在播放的直播间")
			tester.Advance(returnCardCatchPeak - returnCardCatchDip)
			tester.Frame()
			peak, _ := tester.Find("返回正在播放的直播间")
			if dip.W >= button.W-2 || peak.W <= button.W+1 {
				t.Fatalf("卡片交接没有轻压再回弹，原宽 %.1f，压缩 %.1f，回弹 %.1f", button.W, dip.W, peak.W)
			}
		}
		tester.Advance(returnCardCatchEnd)
		if a.returnShrink {
			t.Fatal("动画结束后仍保留过渡层")
		}
		if err := tester.Click("返回正在播放的直播间"); err != nil {
			t.Fatal("动画结束后无法通过卡片恢复直播间", err)
		}
		if !reduced {
			tester.Advance(cardOpenDuration / 2)
			tester.Frame()
			if a.cardOpenAt.IsZero() || a.page != "home" {
				t.Fatal("重新打开未从卡片展开，或提前切换了页面")
			}
			tester.Advance(cardOpenDuration)
		}
		if a.page != "player" {
			t.Fatal("展开结束后未切到播放页")
		}
	}
}

func TestSidebarReturnSettingExpandsWithoutPageFade(t *testing.T) {
	a := polishApp(t)
	a.settings.tab = settingsPlayback
	settings := ui.NewTester(a.settingsView, 820, 1000)
	if err := settings.Click("收拢到卡片"); err != nil {
		t.Fatal(err)
	}
	settings.Frame()
	if err := settings.Click("展开侧栏"); err != nil {
		t.Fatal(err)
	}
	if !a.library.Preferences.ClassicReturn {
		t.Fatal("未保存展开侧栏选项")
	}
	a.page, a.returnPage, a.focusMode, a.shown = "player", "history", true, true
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "1", Title: "测试直播"}
	a.videoBitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 9)))
	view := ui.NewTester(a.view, 1180, 780)
	if err := view.Click("返回列表 · Esc"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.page != "history" || a.returnShrink || a.returnFrame != nil {
		t.Fatal("展开侧栏未直接进入列表，或启动了收拢")
	}
	heading, found := view.Find("最近观看的直播会留在这里")
	if !found {
		t.Fatal("返回首帧未显示浏览内容")
	}
	img := view.Image()
	ink := false
	for y := int(heading.Y); y < int(heading.Y+heading.H) && !ink; y++ {
		for x := int(heading.X); x < int(heading.X+heading.W); x++ {
			if img.RGBAAt(x, y).R < 150 {
				ink = true
				break
			}
		}
	}
	if !ink {
		t.Fatal("返回首帧仍在淡入浏览内容")
	}
	view.Advance(100 * time.Millisecond)
	if a.navWidth <= 0 || a.navWidth >= sidebarWidth {
		t.Fatalf("侧栏没有从收起状态展开，中间帧宽度 %.1f", a.navWidth)
	}
	view.Advance(130 * time.Millisecond)
	if a.navWidth != sidebarWidth {
		t.Fatalf("侧栏未完整展开，最终宽度 %.1f", a.navWidth)
	}
}

func TestOfflineRoomReturnsWithSelectedMotion(t *testing.T) {
	for _, mode := range []string{"收拢到卡片", "展开侧栏"} {
		t.Run(mode, func(t *testing.T) {
			a := polishApp(t)
			a.page, a.returnPage, a.focusMode = "player", "history", true
			a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "24422", Title: "未开播的房间"}
			a.title, a.status = a.targetRoom.Title, "该斗鱼房间当前未直播"
			a.library.Preferences.ClassicReturn = mode == "展开侧栏"
			a.library.Preferences.ChatConnection = false
			view := ui.NewTester(a.view, 1180, 780)
			if err := view.Click("返回直播列表"); err != nil {
				t.Fatal(err)
			}
			view.Frame()
			view.Advance(100 * time.Millisecond)
			if a.page != "history" {
				t.Fatalf("返回到了 %q", a.page)
			}
			if a.library.Preferences.ClassicReturn {
				if a.returnShrink || a.navWidth <= 0 || a.navWidth >= sidebarWidth {
					t.Fatalf("未使用侧栏展开过渡，当前宽度 %.1f", a.navWidth)
				}
			} else if !a.returnShrink || a.returnFrame != nil || a.nowCardBounds.W < 100 {
				t.Fatal("未使用无画面的收拢过渡", a.nowCardBounds)
			}
			view.Advance(returnCardCatchEnd)
			if _, found := view.Find("未开播"); !found || a.returnShrink || a.navWidth != sidebarWidth {
				t.Fatal("返回后未显示未开播状态或动效未结束")
			}
		})
	}
}

func TestHomepageRecommendationsShowSourceOnEveryCard(t *testing.T) {
	for _, withHistory := range []bool{false, true} {
		a := polishApp(t)
		a.page = "home"
		a.catalog = []platforms.Room{
			{Platform: platforms.Bilibili, ID: "1", Title: "推荐甲", Anchor: "甲"},
			{Platform: platforms.Douyu, ID: "2", Title: "推荐乙", Anchor: "乙"},
			{Platform: platforms.Huya, ID: "3", Title: "推荐丙", Anchor: "丙"},
		}
		if withHistory {
			a.library.History = []platforms.Room{{Platform: platforms.Bilibili, ID: "4", Title: "历史", Anchor: "历史主播"}}
		}
		tester := ui.NewTester(a.view, 1180, 920)
		for _, room := range a.catalog {
			if _, ok := tester.Find("来源：" + room.Platform.Name()); !ok {
				t.Fatalf("history=%v：推荐卡片缺少来源 %s", withHistory, room.Platform.Name())
			}
		}
	}
}
func TestClosingPlaybackCancelsSleepAndRemovesCard(t *testing.T) {
	a := polishApp(t)
	a.page = "player"
	a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1"}
	a.shown = true
	a.sleepUntil = time.Now().Add(time.Hour)
	a.sleepTimer = time.NewTimer(time.Hour)
	a.closePlayback()
	if a.shown || a.targetRoom.ID != "" || !a.sleepUntil.IsZero() || a.page != "home" {
		t.Fatal("关闭播放没有清理状态")
	}
}

func TestCloseCardStopsImmediatelyAndLeavesOnlyFragments(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := polishApp(t)
		a.page, a.shown = "history", true
		a.targetRoom = platforms.Room{Platform: platforms.Douyu, ID: "1", Title: "测试直播", Anchor: "测试主播"}
		view := ui.NewTester(a.view, 1180, 780)
		view.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
		card, found := view.Find("返回正在播放的直播间")
		if !found {
			t.Fatal("缺少直播卡片")
		}
		if err := view.Click("关闭当前直播"); err != nil {
			t.Fatal(err)
		}
		if a.shown || a.targetRoom.ID != "" {
			t.Fatal("播放等待碎裂完成才停止")
		}
		view.Advance(95 * time.Millisecond)
		view.Frame()
		if _, found := view.Find("关闭当前直播"); found {
			t.Fatal("退出中的卡片仍可操作")
		}
		fragments := view.Image()
		fragments.Pix = append([]byte(nil), fragments.Pix...)
		view.Advance(cardShatterDuration)
		final := view.Image()
		var different int
		for y := int(card.Y); y < int(card.Y+card.H+34); y++ {
			for x := int(card.X); x < int(card.X+card.W); x++ {
				if fragments.RGBAAt(x, y) != final.RGBAAt(x, y) {
					different++
				}
			}
		}
		if reduced && different != 0 || !reduced && different < 40 {
			t.Fatalf("关闭时碎片与减少动态效果设置不符：reduced=%t pixels=%d", reduced, different)
		}
	}
}

func TestSleepDeadlineStopsOnlyCurrentTimer(t *testing.T) {
	a := polishApp(t)
	a.page, a.shown = "player", true
	a.sleepUntil = time.Now().Add(time.Hour)
	a.finishSleep(a.sleepUntil.Add(-time.Hour))
	if !a.shown {
		t.Fatal("已取消的定时仍停止播放")
	}
	a.finishSleep(a.sleepUntil)
	if a.shown || a.page != "home" || !a.sleepUntil.IsZero() {
		t.Fatal("当前睡眠定时未停止播放或清理状态")
	}
}

func TestSleepControlStartsAndCancelsTimer(t *testing.T) {
	a := polishApp(t)
	a.page, a.shown = "player", true
	a.controlsUntil = time.Now().Add(time.Hour)
	defer a.cancelSleep()
	tester := ui.NewTester(a.view, 1180, 780)
	if err := tester.Click("睡眠定时"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("15 分钟后停止"); err != nil {
		t.Fatal(err)
	}
	if a.sleepUntil.IsZero() {
		t.Fatal("选择时长未启用定时")
	}
	if err := tester.Click("睡眠定时"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("取消定时"); err != nil {
		t.Fatal(err)
	}
	if !a.sleepUntil.IsZero() || a.sleepTimer != nil {
		t.Fatal("取消定时未生效")
	}
}

func TestCompanionTitleOverlaysVideoAndHidesWithControls(t *testing.T) {
	a := polishApp(t)
	a.page, a.companion, a.shown = "player", true, true
	a.controlsUntil = time.Time{}
	tester := ui.NewTester(a.view, 480, 270)
	video, _ := tester.Find("直播画面，双击切换全屏")
	if video.X != 0 || video.Y != 0 || video.W != 480 || video.H != 270 {
		t.Fatal("小窗标题仍占据画面高度", video)
	}
	if _, ok := tester.Find("小窗标题栏"); ok {
		t.Fatal("小窗初始显示标题栏")
	}
	tester.Move(240, 100)
	tester.Advance(200 * time.Millisecond)
	bar, ok := tester.Find("小窗标题栏")
	if !ok || bar.Y < video.Y || bar.Y+bar.H > video.Y+video.H {
		t.Fatal("移动后标题未内嵌画面", bar, ok)
	}
	tester.Advance(4 * time.Second)
	if _, ok := tester.Find("小窗标题栏"); ok {
		t.Fatal("无操作后小窗标题未隐藏")
	}
}
func TestVolumeSliderKeepsPositiveVolumeAndUnmutes(t *testing.T) {
	a := polishApp(t)
	a.page = "player"
	a.focusMode = true
	a.shown = true
	a.volume = 60
	a.stats.Muted = true
	a.controlsUntil = time.Now().Add(time.Hour)
	tester := ui.NewTester(a.view, 1180, 780)
	r, ok := tester.Find("播放音量")
	if !ok {
		t.Fatal("无音量滑块")
	}
	tester.Press(r.X+r.W*.65, r.Y+r.H/2)
	tester.Move(r.X+r.W*.8, r.Y+r.H/2)
	tester.Release(r.X+r.W*.8, r.Y+r.H/2)
	tester.Frame()
	if a.volume < 60 || a.stats.Muted {
		t.Fatalf("拖动后静音或音量归零：%v %v", a.volume, a.stats.Muted)
	}
}
func TestChatDividerDragPersistsWidthAndKeepsVideoAligned(t *testing.T) {
	a := polishApp(t)
	a.page = "player"
	a.focusMode = true
	tester := ui.NewTester(a.view, 1180, 780)
	r, ok := tester.Find("调整弹幕栏宽度")
	if !ok {
		t.Fatal("缺少分隔条")
	}
	tester.Press(r.X+4, r.Y+100)
	tester.Move(r.X-80, r.Y+100)
	tester.Release(r.X-80, r.Y+100)
	tester.Advance(time.Second)
	if a.preferredChatWidth() < 350 {
		t.Fatal("分隔条拖动未更新宽度", a.preferredChatWidth())
	}
	frame, _ := tester.Find("直播间内容容器")
	chat, _ := tester.Find("弹幕列表")
	video, _ := tester.Find("直播画面，双击切换全屏")
	if chat.Y != video.Y || chat.Y+chat.H != video.Y+video.H || chat.X+chat.W > frame.X+frame.W {
		t.Fatal("调整后直播和评论区没有对齐")
	}
}
func TestChatDividerFollowsPointerAcrossFrames(t *testing.T) {
	a := polishApp(t)
	a.page = "player"
	a.focusMode = true
	tester := ui.NewTester(a.view, 1180, 780)
	r, ok := tester.Find("调整弹幕栏宽度")
	if !ok {
		t.Fatal("缺少分隔条")
	}
	y := r.Y + 100
	tester.Press(r.X+4, y)
	tester.Move(r.X-40, y)
	tester.Move(r.X-80, y)
	held := a.preferredChatWidth()
	for range 8 {
		tester.Advance(16 * time.Millisecond)
		tester.Frame()
		if a.preferredChatWidth() != held {
			t.Fatal("指针停住时弹幕栏宽度仍在变化")
		}
	}
	tester.Release(r.X-80, y)
	tester.Advance(time.Second)
	if a.preferredChatWidth() < 350 {
		t.Fatal("连续拖动时宽度没有跟随指针", a.preferredChatWidth())
	}
	r, _ = tester.Find("调整弹幕栏宽度")
	tester.Press(r.X+4, y)
	tester.Move(r.X+200, y)
	tester.Release(r.X+200, y)
	tester.Advance(250 * time.Millisecond)
	chat, _ := tester.Find("弹幕列表")
	if a.preferredChatWidth() != 192 || chat.W != 192 {
		t.Fatalf("未达到新的最窄宽度：偏好 %.1f，弹幕栏 %.1f", a.preferredChatWidth(), chat.W)
	}
}

func TestStartupPageAndChatWidthDefaults(t *testing.T) {
	if startupPage("") != "home" || startupPage("unknown") != "home" || startupPage("favorites") != "favorites" {
		t.Fatal("启动页回退错误")
	}
	a := polishApp(t)
	a.library.Preferences.ChatWidth = 900
	if a.preferredChatWidth() != 560 {
		t.Fatal("弹幕宽度没有限制")
	}
}

func TestSmallWindowSidebarKeepsPlaybackAndSettingsReachable(t *testing.T) {
	a := polishApp(t)
	a.targetRoom = platforms.Room{Platform: platforms.Qie, ID: "1", Title: "正在播放"}
	a.shown = true
	tester := ui.NewTester(a.view, 1000, 580)
	for _, label := range []string{"关闭当前直播", "设置"} {
		r, ok := tester.Find(label)
		if !ok || r.Y < 0 || r.Y+r.H > 580 {
			t.Fatal("小尺寸下操作溢出", label, r, ok)
		}
	}
}

func TestChatFooterAndItsSettingsAreRemoved(t *testing.T) {
	a := polishApp(t)
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "1"}
	tester := ui.NewTester(func(c *ui.Context) { a.chatView(c, chatPanelWidth) }, 286, 700)
	for _, label := range []string{"自动跟随", "暂停跟随", "前往官网发送弹幕", "显示跟随状态", "显示官网互动入口"} {
		if _, ok := tester.Find(label); ok {
			t.Fatal("仍保留已移除的底栏", label)
		}
	}
}

func TestConnectionCardActionsShareGeometry(t *testing.T) {
	for _, width := range []int{240, 286, 360, 560} {
		a := polishApp(t)
		a.targetRoom = platforms.Room{Platform: platforms.Douyin, ID: "1"}
		a.status = "直播间未开播"
		tester := ui.NewTester(a.connectionStatus, width, 400)
		retry, retryOK := tester.Find("重新连接")
		back, backOK := tester.Find("返回直播列表")
		if !retryOK || !backOK || math.Abs(float64(retry.W-back.W)) > .1 || retry.H != back.H || retry.Y != back.Y {
			t.Fatalf("width=%d：连接卡片按钮不等大或未对齐，retry=%+v back=%+v", width, retry, back)
		}
	}
}
