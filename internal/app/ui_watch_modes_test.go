//go:build darwin || windows

package app

import (
	"context"
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

func TestImmersiveFadesBothBarsWithoutShowingWindowButtons(t *testing.T) {
	a := polishApp(t)
	a.page, a.shown = "player", true
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "123", Title: "测试直播"}
	view := ui.NewTester(a.view, 1180, 780)
	view.Move(480, 540)
	view.Advance(300 * time.Millisecond)
	videoBefore, ok := view.Find("直播画面，双击切换全屏")
	chatBefore, chatOK := view.Find("弹幕列表")
	if !ok || !chatOK || a.focusMode {
		t.Fatal("缺少播放画面或弹幕列表，或侧栏没有展开")
	}
	button, ok := view.Find("沉浸式观看 · T")
	fullscreen, fullOK := view.Find(shortcutLabel("全屏 · ⌃⌘F"))
	if !ok || !fullOK || button.Y != fullscreen.Y || button.X >= fullscreen.X {
		t.Fatalf("沉浸开关应在底栏全屏按钮左侧：沉浸 %+v，全屏 %+v", button, fullscreen)
	}
	if err := view.Click("沉浸式观看 · T"); err != nil {
		t.Fatal(err)
	}
	view.Move(700, 540)
	view.Advance(300 * time.Millisecond)
	video, ok := view.Find("直播画面，双击切换全屏")
	chat, chatOK := view.Find("弹幕列表")
	if !ok || !chatOK || !a.focusMode || video.Y != videoBefore.Y-watchHeaderHeight || video.X >= videoBefore.X ||
		chat.W != chatBefore.W || chat.X+chat.W != chatBefore.X+chatBefore.W {
		t.Fatalf("沉浸时应收起顶栏和侧栏、保留弹幕列表：画面 %+v → %+v，弹幕 %+v → %+v", videoBefore, video, chatBefore, chat)
	}
	if !a.watchButtonsHidden {
		t.Fatal("沉浸时应隐藏系统窗口按钮")
	}
	panel, shown := view.Find("沉浸式浮动顶栏")
	if !shown || panel.W != video.W-24 || math.Abs(float64(panel.Y-video.Y-transportBottom)) > .5 {
		t.Fatalf("移动画面内的指针应显示沉浸顶栏：%+v，画面 %+v", panel, video)
	}
	view.Move(panel.X+panel.W/2, panel.Y+panel.H/2)
	view.Advance(180 * time.Millisecond)
	if _, shown := view.Find("直播间选项"); !shown || !a.watchButtonsHidden {
		t.Fatal("顶栏可用时仍应隐藏系统窗口按钮")
	}
	view.Move(chat.X+chat.W/2, 300)
	view.Advance(3200 * time.Millisecond)
	view.Advance(200 * time.Millisecond)
	if _, shown := view.Find("沉浸式浮动顶栏"); shown {
		t.Fatal("无操作时应收起沉浸顶栏")
	}
	if _, shown := view.Find("沉浸式观看 · T"); shown || !a.watchButtonsHidden {
		t.Fatal("无操作时应同时收起底栏，系统窗口按钮仍隐藏")
	}
	// 测试时钟已经推进，设定与真实鼠标移动等效的控制条超时。
	view.Move(video.X+video.W/2, video.Y+video.H/2)
	a.controlsUntil = view.Now().Add(2800 * time.Millisecond)
	view.Frame()
	view.Advance(200 * time.Millisecond)
	if _, shown := view.Find("沉浸式浮动顶栏"); !shown || !a.watchButtonsHidden {
		t.Fatal("再次显示控制条时不应显示系统窗口按钮")
	}
	if _, shown := view.Find("沉浸式观看 · T"); !shown {
		t.Fatal("再次显示顶栏时也应显示底栏")
	}
	view.Key(0, ui.KeyEscape)
	view.Advance(300 * time.Millisecond)
	video, _ = view.Find("直播画面，双击切换全屏")
	if a.page != "player" || a.immersive || a.focusMode || a.watchButtonsHidden || video != videoBefore {
		t.Fatalf("Esc 应退出沉浸模式并恢复侧栏和原布局：%+v", video)
	}
}

func TestImmersiveReturnsWithMotionAndRestoresOnReentry(t *testing.T) {
	a := polishApp(t)
	a.page, a.shown, a.returnPage = "player", true, "favorites"
	a.targetRoom = platforms.Room{Platform: platforms.Bilibili, ID: "123", Title: "测试直播"}
	a.stream = &platforms.Stream{Room: a.targetRoom}
	view := ui.NewTester(a.view, 1180, 780)
	view.Key(0, ui.KeyT)
	view.Move(480, 540)
	view.Advance(300 * time.Millisecond)
	if !a.library.Preferences.Immersive {
		t.Fatal("应记住所选的沉浸模式")
	}
	if err := view.Click("返回列表 · Esc"); err != nil {
		t.Fatal(err)
	}
	view.Advance(20 * time.Millisecond)
	if a.page != "favorites" || !a.returnShrink || !a.returnImmersive {
		t.Fatal("从沉浸顶栏返回应播放卡片收拢动画")
	}
	if _, shown := view.Find("沉浸式浮动顶栏"); !shown {
		t.Fatal("返回动效应沿用沉浸布局")
	}
	view.Advance(returnCardCatchEnd + 100*time.Millisecond)
	if a.returnShrink || a.returnImmersive || a.watchButtonsHidden {
		t.Fatal("返回完成后应恢复浏览页窗口按钮")
	}
	a.page = "player"
	view.Frame()
	view.Advance(300 * time.Millisecond)
	if !a.immersive || !a.focusMode || !a.watchButtonsHidden {
		t.Fatal("再次进入播放页应恢复记住的沉浸模式")
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
	if !ok || !rightOK || left.X != 0 || left.Y != monitorToolbarHeight || left.W != 1180 ||
		right.Y != left.Y+left.H || right.H != left.H || right.Y+right.H != 780 {
		t.Fatalf("宽窗口里两路上下排列时 16:9 画面更大：%+v %+v", left, right)
	}
	if _, ok := view.Find("监控室工具栏"); !ok {
		t.Fatal("监控工具栏应常驻")
	}
	view.Move(left.X+left.W/2, left.Y+left.H/2)
	view.Advance(200 * time.Millisecond)
	for _, label := range []string{"开启此路声音 · 甲", "开启此路弹幕 · 甲"} {
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	view.Move(right.X+right.W/2, right.Y+right.H/2)
	view.Advance(200 * time.Millisecond)
	if _, shown := view.Find("开启此路弹幕 · 甲"); shown {
		t.Fatal("指针离开后，画面上的控制条应收起")
	}
	if _, shown := view.Find("甲 正在出声"); !shown {
		t.Fatal("出声的画面应常驻声音标记")
	}
	if err := view.Click("开启此路声音 · 乙"); err != nil {
		t.Fatal(err)
	}
	if !a.monitorAudioEnabled("bilibili:123") || !a.monitorAudioEnabled("douyu:456") ||
		!a.monitorState().Settings["bilibili:123"].Danmaku || a.monitorState().Settings["douyu:456"].Danmaku {
		t.Fatal("声音或弹幕串到了另一间直播间")
	}
	view.Move(left.X+left.W/2, left.Y+left.H/2)
	view.Advance(200 * time.Millisecond)
	slider, ok := view.Find("调整 甲 的音量")
	if !ok {
		t.Fatal("出声的画面上应能调整这一路音量")
	}
	view.ClickAt(slider.X+slider.W*.3, slider.Y+slider.H/2)
	if a.monitorVolume("bilibili:123") >= 45 || a.monitorVolume("douyu:456") != 60 {
		t.Fatal("音量调整没有单独应用到这一路")
	}
	if err := view.Click("关闭此路声音 · 甲"); err != nil {
		t.Fatal(err)
	}
	if a.monitorAudioEnabled("bilibili:123") || !a.monitorAudioEnabled("douyu:456") {
		t.Fatal("静音第一路不应影响第二路")
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
	view.Move(resized.X+resized.W/2, resized.Y+resized.H/2)
	view.Advance(200 * time.Millisecond)
	drag("移动 甲", 90, 65)
	moved, _ := view.Find("监控画面 甲")
	other, _ := view.Find("监控画面 乙")
	if math.Abs(float64(moved.X-90)) > 1 || math.Abs(float64(moved.Y-left.Y-65)) > 1 || other != right {
		t.Fatalf("移动第一路时应保留第二路的位置：移动 %+v，另一间 %+v", moved, other)
	}
	if len(a.monitorState().Layout) != 2 {
		t.Fatal("自定义布局没有保存")
	}
	if err := view.Click("监控室布局"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("两列"); err != nil {
		t.Fatal(err)
	}
	first, _ := view.Find("监控画面 甲")
	second, _ := view.Find("监控画面 乙")
	if len(a.monitorState().Layout) != 0 || a.monitorState().Arrange != "2" || first.W != 590 || second.W != 590 ||
		first.Y != second.Y || math.Abs(float64(first.X-second.X)) != 590 {
		t.Fatalf("选择固定排列后应清除自定义坐标并左右排列：%+v %+v", first, second)
	}
	view.Key(0, ui.KeyEscape)
	if a.page != "favorites" {
		t.Fatal("退出监控室后应回到原页面")
	}
}

func TestMonitorAutoArrangeKeepsVideoProportions(t *testing.T) {
	canvas := ui.Rect{W: 1180, H: 722}
	for count, want := range map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 9: 3} {
		if got := monitorGridColumns(canvas, count); got != want {
			t.Fatalf("%d 路应排成 %d 列，实际 %d 列", count, want, got)
		}
	}
	frames := monitorGrid(canvas, 3, 2)
	if frames[2].W != frames[0].W || math.Abs(float64(frames[2].X+frames[2].W/2-canvas.W/2)) > .5 {
		t.Fatalf("末行应保持同宽并居中：%+v", frames)
	}
}

func TestMonitorNarrowTileCanAdjustVolumeFromRoomMenu(t *testing.T) {
	a := polishApp(t)
	a.page = "monitor"
	first := platforms.Room{Platform: platforms.Bilibili, ID: "123", Anchor: "甲"}
	second := platforms.Room{Platform: platforms.Douyu, ID: "456", Anchor: "乙"}
	a.addMonitorRoom(first)
	a.addMonitorRoom(second)
	a.toggleMonitorAudio(roomKey(first))
	a.setMonitorArrangement("focus")
	view := ui.NewTester(a.view, 1000, 650)
	// 乙占右侧 30% 的窄格，即使静音也能预先设定音量。
	if _, visible := view.Find("调整 乙 的音量"); visible {
		t.Fatal("窄格应从菜单打开音量面板")
	}
	if err := view.Click("直播间操作 · 乙"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("调整音量"); err != nil {
		t.Fatal(err)
	}
	view.Advance(220 * time.Millisecond)
	slider, ok := view.Find("调整 乙 的音量")
	if !ok {
		t.Fatal("房间菜单没有打开音量滑块")
	}
	view.ClickAt(slider.X+slider.W*.25, slider.Y+slider.H/2)
	if volume := a.monitorVolume(roomKey(second)); volume > 40 || a.monitorVolume(roomKey(first)) != 60 ||
		a.monitorAudioEnabled(roomKey(second)) {
		t.Fatalf("音量没有单独应用到静音的房间乙：%v", volume)
	}
	view.Key(0, ui.KeyEscape)
	if a.page != "monitor" || a.monitorVolumeRoom != "" {
		t.Fatal("Esc 应先关闭音量面板")
	}
}

func TestMonitorDragFollowsPointerAfterHittingTheEdge(t *testing.T) {
	a := polishApp(t)
	a.page = "monitor"
	a.addMonitorRoom(platforms.Room{Platform: platforms.Bilibili, ID: "123", Anchor: "甲"})
	a.addMonitorRoom(platforms.Room{Platform: platforms.Douyu, ID: "456", Anchor: "乙"})
	a.setMonitorArrangement("2")
	view := ui.NewTester(a.view, 1180, 780)
	tile, _ := view.Find("监控画面 甲")
	view.Move(tile.X+tile.W/2, tile.Y+tile.H/2)
	view.Advance(200 * time.Millisecond)
	grip, ok := view.Find("移动 甲")
	if !ok {
		t.Fatal("缺少移动手柄")
	}
	x, y := grip.X+grip.W/2, grip.Y+grip.H/2
	view.Press(x, y)
	view.Move(x-200, y)
	view.Move(x+60, y)
	view.Release(x+60, y)
	view.Advance(25 * time.Millisecond)
	moved, _ := view.Find("监控画面 甲")
	if math.Abs(float64(moved.X-tile.X-60)) > 1 {
		t.Fatalf("撞到左边界后往回拖，画面应跟随指针总位移：原 %+v，现 %+v", tile, moved)
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

func TestMonitorRoomReturnsThroughMonitorToOriginalPage(t *testing.T) {
	a := polishApp(t)
	a.ctx = context.Background()
	a.page = "favorites"
	room := platforms.Room{Platform: platforms.Bilibili, ID: "123", Anchor: "甲"}
	a.addMonitorRoom(room)
	a.monitorViewers = []*app{{
		parent: a, monitorKey: roomKey(room), shown: true, targetRoom: room,
		stream:      &platforms.Stream{Room: room},
		videoBitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 16, 9))),
	}}
	view := ui.NewTester(a.view, 1180, 780)
	if err := view.Click("监控室"); err != nil {
		t.Fatal(err)
	}
	view.Move(550, 400)
	view.Advance(220 * time.Millisecond)
	if err := view.Click("直播间操作 · 甲"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("打开直播间"); err != nil {
		t.Fatal(err)
	}
	if a.cardOpenAt.IsZero() {
		t.Fatal("有画面的监控格应展开到播放页")
	}
	view.Advance(1700 * time.Millisecond)
	if a.page != "player" {
		t.Fatalf("展开结束后没有进入播放页：%s", a.page)
	}
	view.Key(0, ui.KeyEscape)
	if a.page != "monitor" || a.returnShrink {
		t.Fatalf("播放页应返回监控室且不启动没有目标卡片的收拢：%s", a.page)
	}
	view.Key(0, ui.KeyEscape)
	if a.page != "favorites" {
		t.Fatalf("从监控室退出时应回到之前的关注页：%s", a.page)
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
