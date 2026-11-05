//go:build darwin || windows

package app

import (
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
		tester := ui.NewTester(a.view, 1180, 780)
		tester.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
		a.backToBrowse()
		tester.Frame()
		before, found := tester.Find("为你推荐")
		if !found || a.navWidth != sidebarWidth {
			t.Fatalf("返回时仍在挤压内容：sidebar=%v found=%v", a.navWidth, found)
		}
		tester.Advance(80 * time.Millisecond)
		after, _ := tester.Find("为你推荐")
		if before.X != after.X || before.W != after.W || a.navWidth != sidebarWidth {
			t.Fatal("侧栏出现时浏览内容仍在横向移动", before, after)
		}
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
	tester.Release(r.X-80, y)
	tester.Advance(time.Second)
	if a.preferredChatWidth() < 350 {
		t.Fatal("连续拖动时宽度没有跟随指针", a.preferredChatWidth())
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
	tester := ui.NewTester(func(c *ui.Context) { a.chatView(c) }, 286, 700)
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
