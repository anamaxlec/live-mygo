//go:build darwin || windows

package app

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestImmersiveKeepsCommentsAndPutsTheSwitchBesideSidebar(t *testing.T) {
	a := polishApp(t)
	a.page, a.shown = "player", true
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "123", Title: "测试直播"}
	view := ui.NewTester(a.view, 1180, 780)
	view.Move(480, 540)
	view.Advance(200 * time.Millisecond)
	videoBefore, ok := view.Find("直播画面，双击切换全屏")
	chatBefore, chatOK := view.Find("弹幕列表")
	assertBesideSidebar := func() {
		t.Helper()
		button, buttonOK := view.Find("沉浸式观看 · T")
		sidebar, sidebarOK := view.Find("显示 / 隐藏导航侧栏")
		if !buttonOK || !sidebarOK || button.Y != sidebar.Y || button.X != sidebar.X+sidebar.W+5 {
			t.Fatalf("沉浸开关应紧贴侧栏按钮右侧：侧栏 %+v，沉浸 %+v", sidebar, button)
		}
	}
	if !ok || !chatOK {
		t.Fatal("缺少播放画面或弹幕列表")
	}
	assertBesideSidebar()
	if err := view.Click("沉浸式观看 · T"); err != nil {
		t.Fatal(err)
	}
	view.Move(480, 540)
	view.Advance(80 * time.Millisecond)
	during, _ := view.Find("直播画面，双击切换全屏")
	if during.Y >= videoBefore.Y || during.Y <= videoBefore.Y-watchHeaderHeight {
		t.Fatalf("切换时画面没有连续跟随顶栏：%+v", during)
	}
	view.Advance(200 * time.Millisecond)
	video, ok := view.Find("直播画面，双击切换全屏")
	chat, chatOK := view.Find("弹幕列表")
	if !ok || !chatOK || video.Y != videoBefore.Y-watchHeaderHeight || video.H != videoBefore.H+watchHeaderHeight ||
		video.X != videoBefore.X || video.W != videoBefore.W || chat.X != chatBefore.X || chat.W != chatBefore.W {
		t.Fatalf("切换后画面或弹幕未保持对齐：画面 %+v → %+v，弹幕 %+v → %+v", videoBefore, video, chatBefore, chat)
	}
	if _, shown := view.Find("沉浸式浮动顶栏"); shown {
		t.Fatal("指针在画面中时不应弹出顶栏")
	}
	if _, shown := view.Find("沉浸式观看 · T"); shown {
		t.Fatal("底栏不应显示已移动的沉浸开关")
	}
	view.Move(video.X+video.W/2, video.Y+5)
	view.Advance(180 * time.Millisecond)
	panel, shown := view.Find("沉浸式浮动顶栏")
	if !shown || math.Abs(float64(panel.X+panel.W/2-video.X-video.W/2)) > .5 ||
		panel.W != watchControlsWidth(video.W) || math.Abs(float64(panel.Y-video.Y-transportBottom)) > .5 {
		t.Fatalf("浮动顶栏没有按底栏宽度和边距对齐：%+v，画面 %+v", panel, video)
	}
	view.Move(panel.X+panel.W/2, panel.Y+panel.H/2)
	view.Advance(180 * time.Millisecond)
	if _, shown := view.Find("直播间选项"); !shown {
		t.Fatal("移入浮动顶栏后应保留直播间操作")
	}
	assertBesideSidebar()
	view.Move(chat.X+chat.W/2, 140)
	view.Advance(180 * time.Millisecond)
	if _, shown := view.Find("沉浸式浮动顶栏"); shown {
		t.Fatal("指针移到侧边评论区后应收起浮动顶栏")
	}
	view.Move(video.X+video.W/2, video.Y+5)
	view.Advance(180 * time.Millisecond)
	if err := view.Click("沉浸式观看 · T"); err != nil {
		t.Fatal(err)
	}
	view.Advance(200 * time.Millisecond)
	video, _ = view.Find("直播画面，双击切换全屏")
	if a.page != "player" || a.immersive || video != videoBefore {
		t.Fatalf("浮动顶栏开关应退出沉浸模式并还原播放页：%+v", video)
	}
}

func TestMonitorLayoutAndRoomControls(t *testing.T) {
	a := polishApp(t)
	a.page = "favorites"
	a.addMonitorRoom(platforms.Room{Platform: platforms.Bilibili, ID: "123", Anchor: "甲"})
	a.addMonitorRoom(platforms.Room{Platform: platforms.Douyu, ID: "456", Anchor: "乙"})
	view := ui.NewTester(a.view, 1180, 780)
	if err := view.Click("监控室"); err != nil {
		t.Fatal(err)
	}
	left, ok := view.Find("监控画面 甲")
	right, rightOK := view.Find("监控画面 乙")
	if !ok || !rightOK || left.X != 0 || left.Y != monitorToolbarHeight || left.Y+left.H != 780 ||
		right.X != left.W || right.Y != left.Y || right.H != left.H || right.X+right.W != 1180 {
		t.Fatalf("自动排列应填满工具栏下方的画布：%+v %+v", left, right)
	}
	if _, ok := view.Find("监控室工具栏"); !ok {
		t.Fatal("监控工具栏应常驻")
	}
	for _, label := range []string{"开启此路声音 · 甲", "开启此路声音 · 乙", "开启此路弹幕 · 甲"} {
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if !a.monitorAudioEnabled("bilibili:123") || !a.monitorAudioEnabled("douyu:456") ||
		!a.monitorState().Settings["bilibili:123"].Danmaku || a.monitorState().Settings["douyu:456"].Danmaku {
		t.Fatal("声音或弹幕串到了另一间直播间")
	}
	if err := view.Click("关闭此路声音 · 甲"); err != nil {
		t.Fatal(err)
	}
	if a.monitorAudioEnabled("bilibili:123") || !a.monitorAudioEnabled("douyu:456") {
		t.Fatal("静音第一路不应影响第二路")
	}
	slider, ok := view.Find("调整 甲 的音量")
	if !ok {
		t.Fatal("顶栏没有显示所选房间的音量")
	}
	view.ClickAt(slider.X+slider.W*.3, slider.Y+slider.H/2)
	if a.monitorVolume("bilibili:123") >= 45 || a.monitorVolume("douyu:456") != 60 {
		t.Fatal("音量调整没有单独应用到所选房间")
	}

	drag := func(label string, dx, dy float32) {
		t.Helper()
		grip, ok := view.Find(label)
		if !ok {
			t.Fatalf("缺少 %s", label)
		}
		x, y := grip.X+grip.W/2, grip.Y+grip.H/2
		view.Press(x, y)
		view.Move(x+dx/2, y+dy/2)
		view.Move(x+dx, y+dy)
		view.Release(x+dx, y+dy)
		view.Advance(25 * time.Millisecond)
	}
	drag("缩放 甲", -140, -150)
	resized, _ := view.Find("监控画面 甲")
	if math.Abs(float64(resized.W-(left.W-140))) > 1 || math.Abs(float64(resized.H-(left.H-150))) > 1 {
		t.Fatalf("缩放没有跟随拖动距离：原 %+v，调整后 %+v", left, resized)
	}
	drag("移动 甲", 90, 65)
	moved, _ := view.Find("监控画面 甲")
	other, _ := view.Find("监控画面 乙")
	if math.Abs(float64(moved.X-90)) > 1 || math.Abs(float64(moved.Y-left.Y-65)) > 1 || other != right {
		t.Fatalf("移动第一路时应保留第二路的位置：移动 %+v，另一间 %+v", moved, other)
	}
	if len(a.monitorState().Layout) != 2 {
		t.Fatal("自定义布局没有保存")
	}
	if err := view.Click("选择监控室直播间"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("乙 · 斗鱼"); err != nil {
		t.Fatal(err)
	}
	if _, ok := view.Find("调整 乙 的音量"); !ok || a.monitorSelected != "douyu:456" {
		t.Fatal("应能从顶栏切换所选房间和音量控制")
	}
	if err := view.Click("自动排列"); err != nil {
		t.Fatal(err)
	}
	if len(a.monitorState().Layout) != 0 {
		t.Fatal("自动排列没有清除自定义坐标")
	}
	view.Key(0, ui.KeyEscape)
	if a.page != "favorites" {
		t.Fatal("退出监控室后应回到原页面")
	}
}

func TestMonitorBackMovesTheOutgoingPage(t *testing.T) {
	a := polishApp(t)
	a.page = "favorites"
	a.addMonitorRoom(platforms.Room{Platform: platforms.Bilibili, ID: "123", Anchor: "测试直播"})
	view := ui.NewTester(a.view, 1180, 780)
	if err := view.Click("监控室"); err != nil {
		t.Fatal(err)
	}
	before := view.Image()
	if err := view.Click("退出监控室 · Esc"); err != nil {
		t.Fatal(err)
	}
	view.Advance(110 * time.Millisecond)
	during := view.Image()
	view.Advance(250 * time.Millisecond)
	after := view.Image()
	left, right := during.RGBAAt(24, 500), during.RGBAAt(1150, 500)
	if a.page != "favorites" || left != after.RGBAAt(24, 500) || left == before.RGBAAt(24, 500) ||
		right != before.RGBAAt(1150, 500) || right == after.RGBAAt(1150, 500) {
		t.Fatalf("返回中途应露出左边的关注页，并在右边保留滑出的监控室：%s，左 %v，右 %v", a.page, left, right)
	}
}

func TestMonitorAddShowsFavoritesBeforeHistory(t *testing.T) {
	a := polishApp(t)
	a.page = "monitor"
	favorite := platforms.Room{Platform: platforms.Bilibili, ID: "123", Anchor: "常看主播", Title: "关注直播"}
	recent := platforms.Room{Platform: platforms.Douyu, ID: "456", Anchor: "最近主播", Title: "历史直播"}
	a.library.Favorites = []platforms.Room{favorite}
	a.library.History = []platforms.Room{recent, favorite}
	view := ui.NewTester(a.view, 1180, 780)
	if err := view.Click("添加直播间"); err != nil {
		t.Fatal(err)
	}
	view.Advance(250 * time.Millisecond)
	favorites, ok := view.Find("我的关注")
	history, recentOK := view.Find("最近观看")
	if !ok || !recentOK || favorites.Y >= history.Y {
		t.Fatalf("关注没有显示在观看历史前：%+v %+v", favorites, history)
	}
	if err := view.Click("加入监控室 常看主播 关注直播"); err != nil {
		t.Fatal(err)
	}
	if rooms := a.monitorRooms(); len(rooms) != 1 || rooms[0].ID != favorite.ID {
		t.Fatal("点击关注房间后没有加入监控室")
	}
	if _, ok := view.Find("已添加 常看主播 关注直播"); !ok {
		t.Fatal("添加后应在原列表标出该房间，便于继续添加")
	}
	view.Type("https://live.bilibili.com/789")
	if err := view.Click("加入监控室"); err != nil {
		t.Fatal(err)
	}
	if rooms := a.monitorRooms(); len(rooms) != 2 || rooms[1].ID != "789" {
		t.Fatal("粘贴直播链接后没有加入监控室")
	}
}
